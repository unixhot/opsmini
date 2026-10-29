package sentinel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// Campaign correlation: "these separate incidents look like one intrusion".
//
// The rules are deliberately few and readable. A black box that says "these three
// events are related" is unfalsifiable, and correlation output is exactly where an
// operator needs to be able to argue with the machine.
//
// What this is NOT: causation, or permission. A campaign only groups incidents for
// reading and adds context to the agent's prompt. It does not widen any budget, does
// not bypass the write limits, and does not change which tools are callable.

// chainOrder defines the kill-chain sequence used for adjacency and for rendering.
// Stages not listed here are not part of a chain.
var chainOrder = []string{"initial_access", "execution", "persistence", "c2", "impact"}

// stageOfFamily maps an incident family onto its chain stage. Families missing from
// this map are never part of a campaign:
//
//   - metric / log_growth are operational noise and are reported as "impact" only
//     when they follow a real intrusion stage, so they cannot start a chain.
//   - change is a fact record (severity=info) and must never be promoted to an
//     attack stage — otherwise "someone edited nginx.conf" would look like a breach.
var stageOfFamily = map[string]string{
	"auth.anomaly":   "initial_access",
	"webshell":       "execution",
	"proc.anomaly":   "execution",
	"file.integrity": "persistence",
	"net.anomaly":    "c2",
}

// chainSeeds are the stages that may open a campaign. A brute-force burst alone is
// routine background noise on any internet-facing host; it only becomes the start of
// a story once something else follows it, and at that point the *following* incident
// is what attaches. Without this restriction every scanned host would accumulate a
// permanent "attack chain" made of failed logins.
var chainSeeds = map[string]bool{
	"webshell":     true,
	"proc.anomaly": true,
	"net.anomaly":  true,
}

// Campaigner folds incidents into campaigns.
type Campaigner struct {
	DB *gorm.DB
	// Window is how far back a campaign still accepts new stages.
	Window time.Duration
	// Notify is called once per campaign when it first reaches escalation.
	Notify func(c model.Campaign, incidents []model.Incident)
}

// DefaultCampaignWindow is the look-back for attaching a new incident.
const DefaultCampaignWindow = 2 * time.Hour

// NewCampaigner builds a correlator.
func NewCampaigner(db *gorm.DB) *Campaigner {
	return &Campaigner{DB: db, Window: DefaultCampaignWindow}
}

// stageLabel renders a stage for humans (and for the agent prompt).
func stageLabel(stage string) string {
	switch stage {
	case "initial_access":
		return "初始访问"
	case "execution":
		return "执行落地"
	case "persistence":
		return "持久化"
	case "c2":
		return "外联控制"
	case "impact":
		return "影响"
	}
	return stage
}

func stageIndex(stage string) int {
	for i, s := range chainOrder {
		if s == stage {
			return i
		}
	}
	return -1
}

// Attach links one incident into a campaign, creating one if needed.
//
// Idempotent: an incident already in a campaign is left alone, and it is called on
// every signal fold, so the first thing it does is the cheapest possible check.
func (c *Campaigner) Attach(inc model.Incident) bool {
	if c == nil || c.DB == nil || inc.ID == "" {
		return false
	}
	if inc.CampaignID != "" {
		return true
	}
	stage, ok := stageOfFamily[inc.Family]
	if !ok {
		return false
	}
	// Sub-threshold observations are not evidence of anything.
	if model.SeverityRank(inc.Severity) < model.SeverityRank(model.SeverityMedium) {
		return false
	}
	window := c.Window
	if window <= 0 {
		window = DefaultCampaignWindow
	}
	anchor := inc.FirstSeen
	if anchor.IsZero() {
		anchor = inc.LastSeen
	}
	if anchor.IsZero() {
		anchor = time.Now()
	}

	var open []model.Campaign
	c.DB.Where("status = ? AND last_seen > ?", model.CampaignOpen, anchor.Add(-window)).
		Order("last_seen DESC").Limit(20).Find(&open)

	best, bestScore := -1, 0.0
	var bestReasons []string
	for i, camp := range open {
		score, reasons := c.affinity(camp, inc, stage)
		if score > bestScore {
			best, bestScore, bestReasons = i, score, reasons
		}
	}

	// 0.45 is intentionally above "same time window" alone: proximity in time is
	// the weakest thing two events can share on a single-host panel.
	const attachThreshold = 0.45

	// Tolerance, not >=: 0.35+0.1 evaluates to 0.44999999999999996 and would miss a
	// threshold written as 0.45. Comparing float sums with exact equality is a bug
	// waiting for the first person who tunes a weight.
	if best >= 0 && bestScore >= attachThreshold-1e-9 {
		camp := open[best]
		c.link(&camp, inc, stage, bestScore, bestReasons)
		return true
	}

	// No campaign fits. Only a strong seed opens a new one; otherwise the incident
	// stays standalone, which is the honest answer for most of them.
	if !chainSeeds[inc.Family] {
		return false
	}
	c.create(inc, stage, time.Now())
	return true
}

// Backfill correlates incidents that were recorded before this feature existed (or
// while the panel was down). Without it, upgrading produces a panel that stays empty
// until the next intrusion — the operator cannot tell "no chains" from "not working".
//
// It walks oldest first so the opening stage creates the chain and later stages join
// it, which is the same order the live path sees.
func (c *Campaigner) Backfill(window time.Duration) int {
	if c == nil || c.DB == nil {
		return 0
	}
	if window <= 0 {
		window = 24 * time.Hour
	}
	var list []model.Incident
	c.DB.Where("(campaign_id = '' OR campaign_id IS NULL) AND last_seen > ?", time.Now().Add(-window)).
		Order("first_seen asc").Limit(300).Find(&list)
	n := 0
	for _, inc := range list {
		if c.Attach(inc) {
			n++
		}
	}
	return n
}

// affinity scores how well an incident fits an existing campaign.
//
// Everything is measured against the incident's own timestamp, not "now": during a
// backfill, "now" is hours later than the events and every comparison would say
// "too old".
func (c *Campaigner) affinity(camp model.Campaign, inc model.Incident, stage string) (float64, []string) {
	var reasons []string
	score := 0.0

	anchor := inc.FirstSeen
	if anchor.IsZero() {
		anchor = inc.LastSeen
	}
	gap := anchor.Sub(camp.LastSeen)
	// Out-of-order arrival: the chain already contains something newer than this
	// incident. Attaching would rewrite history, so refuse.
	if gap < -5*time.Minute {
		return 0, nil
	}

	stages := decodeStrings(camp.Stages)
	subjects := decodeStrings(camp.Subjects)
	stageKnown := contains(stages, stage)

	// Shared indicator: same subject string, or an address/path token that appears
	// on both sides. This is the strongest evidence available on one host.
	// Only the subject is tokenized: titles contain prose, and prose shares words by
	// accident. "45.1.2.3" inside "45.1.2.3:4444" is the same source, though, so the
	// comparison goes through tokens instead of string equality.
	shared := ""
	incTokens := tokensOf(inc.Subject)
	campTokens := tokensOf(strings.Join(subjects, " "))
	for _, sub := range subjects {
		if sub != "" && sub == inc.Subject {
			shared = sub
			break
		}
	}
	if shared == "" {
		for tok := range incTokens {
			if campTokens[tok] {
				shared = tok
				break
			}
		}
	}
	if shared != "" {
		score += 0.5
		reasons = append(reasons, "共享指标："+shared)
	} else if inc.Subject != "" {
		for _, sub := range subjects {
			if a, b := pathRoot(sub), pathRoot(inc.Subject); a != "" && a == b {
				shared = a
				score += 0.35
				reasons = append(reasons, "同一路径根："+a)
				break
			}
		}
	}

	// Stage regression: a later incident sitting at an earlier kill-chain stage is
	// usually an unrelated scan, not a step of this intrusion. Only strong evidence
	// (a shared indicator) justifies folding it back into the chain.
	maxIdx := -1
	for _, st := range stages {
		if i := stageIndex(st); i > maxIdx {
			maxIdx = i
		}
	}
	idx := stageIndex(stage)
	if !stageKnown && idx < maxIdx && shared == "" {
		return 0, nil
	}

	newStage := !stageKnown
	if newStage {
		score += 0.25
		reasons = append(reasons, "新增环节："+stageLabel(stage))
		// Nearby stages read like one intrusion; two apart still counts, because real
		// intrusions skip steps (execution straight to C2 with no persistence seen).
		near := false
		for _, st := range stages {
			if d := idx - stageIndex(st); d >= -2 && d <= 2 && d != 0 {
				near = true
				break
			}
		}
		if near {
			score += 0.15
			reasons = append(reasons, "与已有环节接近")
		}
	} else if chainSeeds[inc.Family] && gap <= 15*time.Minute {
		// Two execution-stage events four minutes apart on one host are one intrusion,
		// not two. Without this, every extra webshell or suspicious process opens a
		// parallel chain telling the same story — which is exactly the noise this
		// feature is supposed to remove.
		score += 0.35
		reasons = append(reasons, "同一环节的并发事件")
	}

	// Recency: the tighter the gap, the more likely it is one story.
	switch {
	case gap <= 15*time.Minute:
		score += 0.1
		reasons = append(reasons, fmt.Sprintf("%d 分钟内的后续事件", int(gap.Minutes())))
	case gap <= time.Hour:
		score += 0.05
	}

	if score > 1 {
		score = 1
	}
	return score, reasons
}

// link appends the incident to an existing campaign.
func (c *Campaigner) link(camp *model.Campaign, inc model.Incident, stage string, score float64, reasons []string) {
	stages := decodeStrings(camp.Stages)
	if !contains(stages, stage) {
		stages = append(stages, stage)
		stages = orderStages(stages)
	}
	subjects := decodeStrings(camp.Subjects)
	if inc.Subject != "" && !contains(subjects, inc.Subject) {
		subjects = append(subjects, inc.Subject)
		if len(subjects) > 12 {
			subjects = subjects[:12]
		}
	}

	sev := camp.Severity
	if model.SeverityRank(inc.Severity) > model.SeverityRank(sev) {
		sev = inc.Severity
	}
	if escalated(stages) {
		sev = model.SeverityCritical
	}

	// Rolling confidence: a fresh attach that scored low should not drag a strong
	// campaign down, so keep the best evidence seen rather than an average.
	if score > camp.Score {
		camp.Score = score
	}
	camp.Stages = encodeStrings(stages)
	camp.Subjects = encodeStrings(subjects)
	camp.StageCount = len(stages)
	camp.Severity = sev
	camp.LastSeen = latest(camp.LastSeen, inc.LastSeen)
	camp.Title = campaignTitle(stages, camp.IncidentCount+1)
	camp.UpdatedAt = time.Now()

	var count int64
	c.DB.Model(&model.Incident{}).Where("campaign_id = ?", camp.ID).Count(&count)
	camp.IncidentCount = int(count) + 1
	camp.Narrative = campaignNarrative(*camp)

	first := camp.NotifiedAt == nil
	if err := c.DB.Model(&model.Campaign{}).Where("id = ?", camp.ID).Updates(map[string]any{
		"title":          camp.Title,
		"severity":       camp.Severity,
		"score":          camp.Score,
		"stages":         camp.Stages,
		"subjects":       camp.Subjects,
		"stage_count":    camp.StageCount,
		"incident_count": camp.IncidentCount,
		"narrative":      camp.Narrative,
		"last_seen":      camp.LastSeen,
		"updated_at":     camp.UpdatedAt,
	}).Error; err != nil {
		log.Printf("sentinel: link campaign %s: %v", camp.ID, err)
		return
	}
	c.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).Update("campaign_id", camp.ID)

	if first && escalated(stages) && camp.Severity == model.SeverityCritical && c.Notify != nil {
		var list []model.Incident
		c.DB.Where("campaign_id = ?", camp.ID).Order("first_seen").Find(&list)
		c.Notify(*camp, list)
		now := time.Now()
		c.DB.Model(&model.Campaign{}).Where("id = ?", camp.ID).Update("notified_at", now)
	}
}

// create opens a new campaign from a seed incident.
func (c *Campaigner) create(inc model.Incident, stage string, now time.Time) {
	stages := []string{stage}
	camp := model.Campaign{
		ID:            newCampaignID(inc.ID),
		Title:         campaignTitle(stages, 1),
		Severity:      inc.Severity,
		Status:        model.CampaignOpen,
		Score:         0.3,
		StageCount:    1,
		IncidentCount: 1,
		Stages:        encodeStrings(stages),
		Subjects:      encodeStrings([]string{inc.Subject}),
		FirstSeen:     inc.FirstSeen,
		LastSeen:      inc.LastSeen,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	camp.Narrative = campaignNarrative(camp)
	if err := c.DB.Create(&camp).Error; err != nil {
		log.Printf("sentinel: create campaign: %v", err)
		return
	}
	c.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).Update("campaign_id", camp.ID)
}

// Sweep closes campaigns that have been quiet long enough that no further stage is
// plausibly coming. Closing is bookkeeping only — nothing is deleted, and a late
// incident simply starts a new chain.
func (c *Campaigner) Sweep(quiet time.Duration) int {
	if c == nil || c.DB == nil {
		return 0
	}
	if quiet <= 0 {
		quiet = 12 * time.Hour
	}
	res := c.DB.Model(&model.Campaign{}).
		Where("status = ? AND last_seen < ?", model.CampaignOpen, time.Now().Add(-quiet)).
		Update("status", model.CampaignClosed)
	return int(res.RowsAffected)
}

// escalated reports whether the covered stages justify raising the chain to
// critical. Three distinct stages, or "something ran and something talked to the
// outside", is the point where a human should be woken up.
func escalated(stages []string) bool {
	if len(stages) >= 3 {
		return true
	}
	return contains(stages, "execution") && contains(stages, "c2")
}

func campaignTitle(stages []string, count int) string {
	labels := make([]string, 0, len(stages))
	for _, s := range orderStages(stages) {
		labels = append(labels, stageLabel(s))
	}
	title := "疑似攻击链：" + strings.Join(labels, " → ")
	if count > 1 {
		title += fmt.Sprintf("（%d 个事件）", count)
	}
	if len(title) > 180 {
		title = title[:180]
	}
	return title
}

// campaignNarrative is the human/agent-readable summary. It states what was seen and
// explicitly says the link is correlational — the panel must not present a guess as
// a finding.
func campaignNarrative(camp model.Campaign) string {
	stages := orderStages(decodeStrings(camp.Stages))
	var b strings.Builder
	b.WriteString("以下事件按时间与指标关联，构成一条疑似攻击链（关联推断，非因果定论）：\n")
	for _, s := range stages {
		b.WriteString("  · " + stageLabel(s) + "\n")
	}
	if subs := decodeStrings(camp.Subjects); len(subs) > 0 {
		limit := subs
		if len(limit) > 6 {
			limit = limit[:6]
		}
		b.WriteString("关联指标：" + strings.Join(limit, "、") + "\n")
	}
	b.WriteString(fmt.Sprintf("覆盖 %d 个环节、%d 个事件，置信度 %.2f。",
		camp.StageCount, camp.IncidentCount, camp.Score))
	return b.String()
}

func orderStages(in []string) []string {
	out := append([]string{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		return stageIndex(out[i]) < stageIndex(out[j])
	})
	return out
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// pathRoot returns the first two path segments, so /var/www/a/x.php and
// /var/www/b/y.php share "/var/www".
func pathRoot(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		return ""
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 2 {
		if len(parts) == 1 {
			return "/" + parts[0]
		}
		return ""
	}
	return "/" + parts[0] + "/" + parts[1]
}

// tokensOf extracts comparable indicator tokens (addresses, paths, unit names) from
// free text so two incidents can be matched without exact subject equality.
func tokensOf(s string) map[string]bool {
	out := map[string]bool{}
	add := func(tok string) {
		tok = strings.Trim(strings.TrimSpace(tok), "/")
		if len(tok) < 4 || isDigits(tok) {
			// Pure numbers are ports and counters; matching on them would tie any two
			// incidents that happen to mention 8080.
			return
		}
		out[tok] = true
	}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';' || r == '|' || r == '(' || r == ')' ||
			r == '[' || r == ']' || r == '"' || r == '\'' || r == '\n' || r == '\t' ||
			r == '：' || r == '，' || r == '、' || r == '→'
	}) {
		add(f)
		// host:port — the host is the identifying half.
		if i := strings.LastIndex(f, ":"); i > 0 {
			add(f[:i])
		}
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func encodeStrings(v []string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeStrings(raw string) []string {
	var out []string
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func newCampaignID(seed string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	// Keep the whole id within the 32-char column: 2 + 18 + 8 = 28.
	if len(seed) >= 18 {
		return "cp" + seed[:18] + suffix
	}
	return "cp" + seed + suffix
}
