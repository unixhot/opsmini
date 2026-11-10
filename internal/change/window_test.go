package change

import (
	"testing"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

func pts(start, end int64, step int64, cpu func(i int) float64) []model.MetricPoint {
	var out []model.MetricPoint
	i := 0
	for t := start; t < end; t += step {
		out = append(out, model.MetricPoint{Time: t, CPU: cpu(i), Mem: 40, Load1: 1})
		i++
	}
	return out
}

// CPU 在变更后翻倍——必须判 degraded，且理由里要点名是哪一项。
func TestCompareWindowsDegraded(t *testing.T) {
	t0 := int64(10000)
	var points []model.MetricPoint
	// 前 10 分钟 10%，后 10 分钟 45%
	points = append(points, pts(t0-600, t0, 10, func(int) float64 { return 10 })...)
	points = append(points, pts(t0, t0+600, 10, func(int) float64 { return 45 })...)

	res := CompareWindows(points, t0, 10*time.Minute, 10*time.Minute, time.Unix(t0+3600, 0))
	if res.Verdict != "degraded" {
		t.Fatalf("verdict = %s, want degraded", res.Verdict)
	}
	var cpu *WindowMetric
	for i := range res.Metrics {
		if res.Metrics[i].Key == "cpu" {
			cpu = &res.Metrics[i]
		}
	}
	if cpu == nil || !cpu.Worsened {
		t.Fatalf("cpu not marked worsened: %+v", cpu)
	}
	if len(res.Reasons) == 0 {
		t.Fatal("no reason recorded")
	}
}

// 指标基本没动——不能因为"有噪声"就判 degraded，否则这个标记没人信。
func TestCompareWindowsStable(t *testing.T) {
	t0 := int64(10000)
	var points []model.MetricPoint
	points = append(points, pts(t0-600, t0, 10, func(i int) float64 { return 20 + float64(i%3) })...)
	points = append(points, pts(t0, t0+600, 10, func(i int) float64 { return 21 + float64(i%3) })...)
	res := CompareWindows(points, t0, 10*time.Minute, 10*time.Minute, time.Unix(t0+3600, 0))
	if res.Verdict != "stable" {
		t.Fatalf("verdict = %s, want stable, reasons=%v", res.Verdict, res.Reasons)
	}
}

// 变更刚发生：观测窗口没填满，必须标 partial 且不能给出稳定结论。
func TestCompareWindowsPartial(t *testing.T) {
	t0 := int64(10000)
	var points []model.MetricPoint
	points = append(points, pts(t0-600, t0, 10, func(int) float64 { return 10 })...)
	points = append(points, pts(t0, t0+60, 10, func(int) float64 { return 11 })...)
	res := CompareWindows(points, t0, 10*time.Minute, 10*time.Minute, time.Unix(t0+60, 0))
	if !res.Partial {
		t.Fatal("expected partial=true for a change that just happened")
	}
	found := false
	for _, r := range res.Reasons {
		if len(r) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a note about the partial window")
	}
}

// 数据不够就明确说不够，不要拿两个点下结论。
func TestCompareWindowsInsufficient(t *testing.T) {
	t0 := int64(10000)
	points := []model.MetricPoint{{Time: t0 - 30, CPU: 50}, {Time: t0 + 10, CPU: 90}}
	res := CompareWindows(points, t0, 10*time.Minute, 10*time.Minute, time.Unix(t0+600, 0))
	if res.Verdict != "insufficient" {
		t.Fatalf("verdict = %s, want insufficient", res.Verdict)
	}
}
