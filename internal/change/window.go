package change

import (
	"fmt"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// WindowMetric is one metric compared across the two windows around a change.
type WindowMetric struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Unit     string `json:"unit"`
	Before   Stat   `json:"before"`
	After    Stat   `json:"after"`
	DeltaPct float64 `json:"delta_pct"`
	Worsened bool   `json:"worsened"`
	// Note explains a judgement that the numbers alone do not make obvious.
	Note string `json:"note,omitempty"`
}

// Stat is the summary of one window.
type Stat struct {
	N   int     `json:"n"`
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
	Min float64 `json:"min"`
}

// SignalDelta counts perception signals before and after the change.
type SignalDelta struct {
	Before int      `json:"before"`
	After  int      `json:"after"`
	Items  []string `json:"items,omitempty"`
}

// WindowResult is the whole comparison. Verdict is deliberately coarse: the
// panel shows it as a badge and the agent prompt quotes it, so "degraded" must
// mean the numbers moved in a bad direction, not "something happened".
type WindowResult struct {
	ChangeID  string         `json:"change_id"`
	Path      string         `json:"path"`
	T0        int64          `json:"t0"`
	BeforeSec int            `json:"before_sec"`
	AfterSec  int            `json:"after_sec"`
	Partial   bool           `json:"partial"` // after-window still filling in
	Metrics   []WindowMetric `json:"metrics"`
	Signals   SignalDelta    `json:"signals"`
	Verdict   string         `json:"verdict"` // degraded / stable / insufficient
	Reasons   []string       `json:"reasons"`
}

// MinWindowSamples is the point count a window needs before it can be compared.
// Samples land every 10s, so 3 points is roughly half a minute; below that a
// single spike would decide the verdict.
const MinWindowSamples = 3

// CompareWindows builds the before/after comparison around t0.
//
// It is a pure function on purpose: the metric history is the only input, which
// makes the judgement testable without a database or a fake clock.
func CompareWindows(points []model.MetricPoint, t0 int64, before, after time.Duration, now time.Time) WindowResult {
	res := WindowResult{
		T0:        t0,
		BeforeSec: int(before / time.Second),
		AfterSec:  int(after / time.Second),
	}
	bStart := t0 - int64(before/time.Second)
	aEnd := t0 + int64(after/time.Second)
	// The change just happened: the observation window is still filling in.
	// Comparing against a half-filled window would understate the damage, so say
	// so instead of pretending the numbers are final.
	if now.Unix() < aEnd {
		res.Partial = true
		aEnd = now.Unix()
	}
	if aEnd <= t0 {
		aEnd = t0 + 1
	}

	var beforePts, afterPts []model.MetricPoint
	for _, p := range points {
		switch {
		case p.Time >= bStart && p.Time < t0:
			beforePts = append(beforePts, p)
		case p.Time >= t0 && p.Time <= aEnd:
			afterPts = append(afterPts, p)
		}
	}

	type spec struct {
		key, label, unit string
		get              func(model.MetricPoint) float64
		// absTolerance is the smallest change that is worth reporting at all;
		// relTolerance is the proportional change that counts as degradation.
		absTolerance float64
		relTolerance float64
		higherWorse  bool
	}
	specs := []spec{
		{"cpu", "CPU 使用率", "%", func(p model.MetricPoint) float64 { return p.CPU }, 5, 0.20, true},
		{"mem", "内存使用率", "%", func(p model.MetricPoint) float64 { return p.Mem }, 3, 0.15, true},
		{"load1", "系统负载(1m)", "", func(p model.MetricPoint) float64 { return p.Load1 }, 0.5, 0.50, true},
		{"net_recv", "网络接收", "B/s", func(p model.MetricPoint) float64 { return p.NetRecv }, 0, 1.00, true},
		{"net_sent", "网络发送", "B/s", func(p model.MetricPoint) float64 { return p.NetSent }, 0, 1.00, true},
	}

	sufficient := true
	for _, sp := range specs {
		b := summarize(beforePts, sp.get)
		a := summarize(afterPts, sp.get)
		m := WindowMetric{Key: sp.key, Label: sp.label, Unit: sp.unit, Before: b, After: a}
		if b.N < MinWindowSamples || a.N < MinWindowSamples {
			sufficient = false
			m.Note = fmt.Sprintf("样本不足（前 %d 点 / 后 %d 点，至少需要 %d 点）", b.N, a.N, MinWindowSamples)
			res.Metrics = append(res.Metrics, m)
			continue
		}
		if b.Avg > 0 {
			m.DeltaPct = (a.Avg - b.Avg) / b.Avg * 100
		}
		rise := a.Avg - b.Avg
		m.Worsened = rise > sp.absTolerance && (b.Avg == 0 || rise/b.Avg > sp.relTolerance)
		if m.Worsened {
			res.Reasons = append(res.Reasons, fmt.Sprintf("%s 从 %.1f%s 升到 %.1f%s（+%.0f%%）", sp.label, b.Avg, sp.unit, a.Avg, sp.unit, m.DeltaPct))
		}
		res.Metrics = append(res.Metrics, m)
	}

	switch {
	case !sufficient:
		res.Verdict = "insufficient"
	case len(res.Reasons) > 0:
		res.Verdict = "degraded"
	default:
		res.Verdict = "stable"
	}
	if res.Partial {
		res.Reasons = append(res.Reasons, fmt.Sprintf("变更刚发生，观测窗口只积累了 %d 秒（目标 %d 秒），结论仍可能变化", aEnd-t0, int(after/time.Second)))
	}
	return res
}

// summarize collapses one window into avg/max/min.
func summarize(pts []model.MetricPoint, get func(model.MetricPoint) float64) Stat {
	if len(pts) == 0 {
		return Stat{}
	}
	s := Stat{N: len(pts), Min: get(pts[0]), Max: get(pts[0])}
	var sum float64
	for _, p := range pts {
		v := get(p)
		sum += v
		if v > s.Max {
			s.Max = v
		}
		if v < s.Min {
			s.Min = v
		}
	}
	s.Avg = sum / float64(len(pts))
	return s
}
