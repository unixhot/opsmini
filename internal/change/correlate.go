package change

import (
	"sort"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Correlator links incidents to the changes that happened shortly before them.
//
// It produces CORRELATION, not causation. The score says "this change is worth
// looking at given this incident", and every consumer (panel, agent prompt) must
// phrase it that way. Claiming causation from temporal adjacency is how AIOps
// tools lose their users' trust.
type Correlator struct {
	store *Store
}

// NewCorrelator wraps a store.
func NewCorrelator(store *Store) *Correlator { return &Correlator{store: store} }

// ScoredChange is one correlated change with its evidence.
type ScoredChange struct {
	Event   model.ChangeEvent `json:"event"`
	Score   float64           `json:"score"`
	Reasons []string          `json:"reasons"`
}

// DefaultWindow is how far back we look for candidate changes.
const DefaultWindow = 30 * time.Minute

// minScore filters out the long tail so the agent is not handed a wall of
// unrelated events.
const minScore = 0.3

// Related returns changes in the window before the incident, best first.
func (c *Correlator) Related(inc model.Incident, window time.Duration) []ScoredChange {
	if window <= 0 {
		window = DefaultWindow
	}
	anchor := inc.FirstSeen
	if anchor.IsZero() {
		anchor = time.Now()
	}
	candidates := c.store.ListBetween(anchor.Add(-window), anchor, 300)
	out := make([]ScoredChange, 0, len(candidates))
	for _, ev := range candidates {
		score, reasons := score(inc, ev)
		if score < minScore {
			continue
		}
		out = append(out, ScoredChange{Event: ev, Score: score, Reasons: reasons})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Event.At.After(out[j].Event.At)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// score compares an incident against one change. The weights are deliberately
// simple and documented so users can reason about them — a learned black box
// would be unexplainable exactly when an explanation matters most.
func score(inc model.Incident, ev model.ChangeEvent) (float64, []string) {
	var reasons []string
	best := 0.0
	labels := splitLabels(ev.Labels)
	subj := strings.ToLower(inc.Subject)
	title := strings.ToLower(inc.Title)
	obj := strings.ToLower(ev.ObjectKey)

	// 1.0 — the incident is literally about this object
	if subj != "" && (subj == obj || strings.Contains(subj, obj) || strings.Contains(obj, subj)) {
		best = 1.0
		reasons = append(reasons, "事件对象与被变更对象相同")
	}
	// 0.9 — same path / same service root
	if best < 0.9 {
		if a, b := pathRoot(obj), pathRoot(subj); a != "" && a == b {
			best = 0.9
			reasons = append(reasons, "同一路径根："+a)
		}
	}
	// 0.8 — service / unit name appears on both sides
	if best < 0.8 {
		for _, l := range labels {
			if l == "" || strings.HasPrefix(l, "port:") || strings.HasPrefix(l, "domain:") {
				continue
			}
			if strings.Contains(subj, l) || strings.Contains(title, l) {
				best = 0.8
				reasons = append(reasons, "服务名匹配："+l)
				break
			}
		}
	}
	// 0.7 — package name matches
	if best < 0.7 && ev.Kind == model.ChangeKindPackage {
		for _, l := range labels {
			if strings.Contains(subj, l) || strings.Contains(title, l) {
				best = 0.7
				reasons = append(reasons, "软件包匹配："+l)
				break
			}
		}
	}
	// 0.6 — port mentioned in the change matches the incident
	if best < 0.6 {
		for _, l := range labels {
			if !strings.HasPrefix(l, "port:") {
				continue
			}
			p := strings.TrimPrefix(l, "port:")
			if strings.Contains(subj, ":"+p) || strings.Contains(title, p+" ") || strings.HasSuffix(title, p) {
				best = 0.6
				reasons = append(reasons, "端口匹配："+p)
				break
			}
		}
	}
	// 0.4 — category proximity (auth incidents vs account changes, etc.)
	if best < 0.4 && categoryRelated(inc.Family, ev.Kind) {
		best = 0.4
		reasons = append(reasons, "类别相关："+inc.Family+" ↔ "+ev.Kind)
	}
	return best, reasons
}

// categoryRelated encodes the handful of cross-category relationships that are
// genuinely useful (and avoids inventing a matrix).
func categoryRelated(family, kind string) bool {
	switch kind {
	case model.ChangeKindAccount:
		return family == "auth" || family == "auth.anomaly" || strings.HasPrefix(family, "auth")
	case model.ChangeKindFirewall:
		return family == "net" || strings.HasPrefix(family, "net")
	case model.ChangeKindCron:
		return family == "proc" || strings.HasPrefix(family, "proc")
	case model.ChangeKindConfig:
		return family == "log" || family == "metric" || strings.HasPrefix(family, "log")
	case model.ChangeKindService:
		return family == "metric" || family == "net" || strings.HasPrefix(family, "metric")
	}
	return false
}

func splitLabels(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// pathRoot returns the first meaningful segment of a path or subject, used to
// decide "these two things belong to the same service".
func pathRoot(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ":"); i > 0 && !strings.Contains(s[:i], "/") {
		s = s[i+1:] // strip prefixes like "log_growth:"
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	for _, p := range parts {
		if p == "" || p == "etc" || p == "var" || p == "log" || p == "usr" || p == "lib" || p == "opt" || p == "conf" || p == "conf.d" {
			continue
		}
		return strings.TrimSuffix(p, "."+ext(p))
	}
	return ""
}

func ext(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return ""
}

// Brief renders one correlated change for the agent's evidence block.
//
// It carries metadata only. The diff body deliberately stays behind a tool call:
// configuration content is untrusted input, and keeping it out of the prompt by
// default is the most effective prompt-injection mitigation available.
func (s ScoredChange) Brief() string {
	ev := s.Event
	var b strings.Builder
	b.WriteString(ev.Kind)
	b.WriteString("｜")
	b.WriteString(ev.At.Format("2006-01-02 15:04:05"))
	b.WriteString("｜")
	b.WriteString(ev.ObjectKey)
	if ev.Added > 0 || ev.Removed > 0 {
		b.WriteString("｜+")
		b.WriteString(itoa(ev.Added))
		b.WriteString(" −")
		b.WriteString(itoa(ev.Removed))
	}
	if ev.ActorUser != "" {
		b.WriteString("｜操作者：")
		b.WriteString(ev.ActorUser)
		if ev.ActorIP != "" {
			b.WriteString("@")
			b.WriteString(ev.ActorIP)
		}
		if ev.Confidence == model.ConfidenceInferred {
			b.WriteString("（推测）")
		}
	}
	if ev.Fields != "" {
		b.WriteString("｜涉及字段：")
		b.WriteString(ev.Fields)
	}
	b.WriteString("｜相关性 ")
	b.WriteString(formatScore(s.Score))
	b.WriteString("（")
	b.WriteString(strings.Join(s.Reasons, "；"))
	b.WriteString("）")
	b.WriteString("｜详情用 change_diff 查看 id=")
	b.WriteString(ev.ID)
	return b.String()
}

func formatScore(f float64) string {
	// one decimal, no fmt dependency in hot path
	i := int(f*10 + 0.5)
	return string([]byte{byte('0' + i/10), '.', byte('0' + i%10)})
}
