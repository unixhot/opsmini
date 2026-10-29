package sentinel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// Shadow scoring: was the proposal worth making?
//
// A shadow run never touches the host, so there is no verification assertion to run
// and no outcome to grade. The only honest way to score it is to look at what
// happened next: did the thing it wanted to block keep knocking, is the file it
// wanted to quarantine still sitting there, is the process it wanted to freeze still
// running?
//
// The verdict feeds one decision only — "should this policy be widened?" — so it is
// framed as would_help / unnecessary / unknown rather than right/wrong. A model that
// proposed an unnecessary action was not necessarily wrong; it may have been reacting
// to something that stopped on its own.

// ShadowReviewer scores shadow runs retrospectively.
type ShadowReviewer struct {
	DB *gorm.DB
	// MinAge keeps a review from judging a run before anything could have happened
	// afterwards. Anything younger than this is skipped until the next pass.
	MinAge time.Duration
}

// NewShadowReviewer builds a reviewer.
func NewShadowReviewer(db *gorm.DB) *ShadowReviewer {
	return &ShadowReviewer{DB: db, MinAge: 10 * time.Minute}
}

// shadowOp is one proposed action parsed out of an action log row.
type shadowOp struct {
	Tool string
	Args map[string]any
}

// Review scores one incident's shadow run and stores the result.
func (r *ShadowReviewer) Review(incidentID string) (*model.ShadowReview, error) {
	if r == nil || r.DB == nil {
		return nil, fmt.Errorf("影子评审不可用")
	}
	var inc model.Incident
	if err := r.DB.First(&inc, "id = ?", incidentID).Error; err != nil {
		return nil, err
	}

	var logs []model.ActionLog
	r.DB.Where("incident_id = ? AND verdict = ?", incidentID, "shadow").Order("id").Find(&logs)

	var run model.AgentRun
	r.DB.Where("incident_id = ?", incidentID).Order("id DESC").Limit(1).Find(&run)
	after := inc.LastSeen
	if run.FinishedAt != nil && run.FinishedAt.After(after) {
		after = *run.FinishedAt
	}

	review := &model.ShadowReview{
		IncidentID: incidentID,
		RunID:      run.ID,
		ReviewedAt: time.Now(),
	}

	ops := parseShadowOps(logs)
	if len(ops) == 0 {
		review.Verdict = model.ShadowUnknown
		review.Reasons = encodeStrings([]string{"本次影子运行没有产生可评估的动作（多为纯调查）"})
		r.save(review)
		return review, nil
	}

	var (
		help, useless int
		reasons       []string
	)
	for _, op := range ops {
		verdict, reason := r.judge(op, after)
		switch verdict {
		case model.ShadowWouldHelp:
			help++
			reasons = append(reasons, "✓ "+reason)
		case model.ShadowUnnecessary:
			useless++
			reasons = append(reasons, "· "+reason)
		default:
			reasons = append(reasons, "? "+reason)
		}
	}

	switch {
	case help > useless:
		review.Verdict = model.ShadowWouldHelp
	case useless > help:
		review.Verdict = model.ShadowUnnecessary
	default:
		review.Verdict = model.ShadowUnknown
	}
	total := help + useless
	if total > 0 {
		review.Score = float64(help) / float64(total)
	}
	review.Reasons = encodeStrings(reasons)
	r.save(review)
	return review, nil
}

// save writes the review and mirrors the verdict onto the incident so the list can
// show it without a join.
func (r *ShadowReviewer) save(rev *model.ShadowReview) {
	if err := r.DB.Create(rev).Error; err != nil {
		log.Printf("sentinel: shadow review save: %v", err)
		return
	}
	r.DB.Model(&model.Incident{}).Where("id = ?", rev.IncidentID).
		Update("shadow_verdict", rev.Verdict)
}

// judge scores one proposed action against the world after the run.
func (r *ShadowReviewer) judge(op shadowOp, after time.Time) (string, string) {
	switch op.Tool {
	case "block_ip":
		ip := strOf(op.Args, "ip")
		if ip == "" {
			return model.ShadowUnknown, "封禁建议缺少 IP"
		}
		var n int64
		r.DB.Model(&model.Signal{}).
			Where("observed_at > ? AND (subject LIKE ? OR message LIKE ?)", after, "%"+ip+"%", "%"+ip+"%").
			Count(&n)
		if n > 0 {
			return model.ShadowWouldHelp, fmt.Sprintf("封禁对象 %s 在运行后仍出现 %d 次，封禁本可阻断", ip, n)
		}
		return model.ShadowUnnecessary, fmt.Sprintf("封禁对象 %s 在运行后未再出现，该封禁并非必需", ip)

	case "quarantine_file":
		path := strOf(op.Args, "path")
		if path == "" {
			return model.ShadowUnknown, "隔离建议缺少路径"
		}
		exists := fileExists(path)
		var n int64
		r.DB.Model(&model.Signal{}).
			Where("observed_at > ? AND (subject LIKE ? OR message LIKE ?)", after, "%"+path+"%", "%"+path+"%").
			Count(&n)
		if exists && n > 0 {
			return model.ShadowWouldHelp, fmt.Sprintf("%s 仍在且运行后又被提及 %d 次，隔离本可消除风险", path, n)
		}
		if exists {
			return model.ShadowWouldHelp, fmt.Sprintf("%s 仍在磁盘上，未被任何人清理，隔离建议成立", path)
		}
		return model.ShadowUnnecessary, fmt.Sprintf("%s 已不存在，隔离建议当时已无必要", path)

	case "freeze_process":
		pid := intOf(op.Args, "pid")
		if pid <= 0 {
			return model.ShadowUnknown, "冻结建议缺少 PID"
		}
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err == nil {
			return model.ShadowWouldHelp, fmt.Sprintf("进程 pid=%d 仍在运行，冻结建议成立", pid)
		}
		return model.ShadowUnnecessary, fmt.Sprintf("进程 pid=%d 已不存在，冻结建议当时已无必要", pid)

	case "truncate_log":
		path := strOf(op.Args, "path")
		if path == "" {
			return model.ShadowUnknown, "截断建议缺少路径"
		}
		if fileExists(path) {
			return model.ShadowWouldHelp, fmt.Sprintf("%s 仍在，截断（或先扩容）的建议仍有效", path)
		}
		return model.ShadowUnnecessary, fmt.Sprintf("%s 已不存在，截断建议已无意义", path)
	}
	return model.ShadowUnknown, op.Tool + "：无自动判定依据，交人工判断"
}

// ReviewPending scores every unreviewed shadow incident older than MinAge.
//
// Bounded per pass: a backlog after an upgrade must not turn into one huge query
// loop that stalls the panel's perception layer.
func (r *ShadowReviewer) ReviewPending(limit int) int {
	if r == nil || r.DB == nil {
		return 0
	}
	if limit <= 0 {
		limit = 50
	}
	minAge := r.MinAge
	if minAge <= 0 {
		minAge = 10 * time.Minute
	}
	cut := time.Now().Add(-minAge)

	var ids []string
	r.DB.Model(&model.Incident{}).
		Where("mode = ? AND shadow_verdict = ? AND last_seen < ?", model.ModeShadow, "", cut).
		Order("last_seen DESC").Limit(limit).Pluck("id", &ids)

	done := 0
	for _, id := range ids {
		if _, err := r.Review(id); err == nil {
			done++
		}
	}
	return done
}

// Start runs the reviewer on a timer.
func (r *ShadowReviewer) Start(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 30 * time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		first := time.After(5 * time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			case <-first:
				r.runOnce()
			case <-t.C:
				r.runOnce()
			}
		}
	}()
}

func (r *ShadowReviewer) runOnce() {
	n := r.ReviewPending(50)
	if n > 0 {
		log.Printf("sentinel: shadow reviews written: %d", n)
	}
}

// --- helpers -----------------------------------------------------------------

// parseShadowOps parses the shadow action rows into (tool, args) pairs.
func parseShadowOps(logs []model.ActionLog) []shadowOp {
	out := make([]shadowOp, 0, len(logs))
	for _, l := range logs {
		if strings.TrimSpace(l.Tool) == "" {
			continue
		}
		args := map[string]any{}
		if strings.TrimSpace(l.Args) != "" {
			_ = json.Unmarshal([]byte(l.Args), &args)
		}
		out = append(out, shadowOp{Tool: l.Tool, Args: args})
	}
	return out
}

func strOf(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func intOf(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
