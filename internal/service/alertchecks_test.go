package service

import (
	"runtime"
	"testing"
	"time"
)

func TestParseConditionDistinguishesZeroFromUnparseable(t *testing.T) {
	cases := []struct {
		cond      string
		op        string
		threshold float64
		ok        bool
	}{
		{"> 0", ">", 0, true}, // event rules legitimately use zero
		{">= 90", ">=", 90, true},
		{"> 85%", ">", 85, true},
		{"< 300", "<", 300, true},
		{"> CPU 核数", "", 0, false}, // relative, must be resolved from the host
		{"", ">=", 0, false},
	}
	for _, c := range cases {
		op, th, ok := parseCondition(c.cond)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v want %v", c.cond, ok, c.ok)
		}
		if !ok {
			continue
		}
		if op != c.op || th != c.threshold {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", c.cond, op, th, c.op, c.threshold)
		}
	}
}

func TestRelativeThresholdUsesCPUCount(t *testing.T) {
	v, ok := relativeThreshold("系统负载(5分钟)")
	if !ok {
		t.Fatal("load metric should resolve a relative threshold")
	}
	if int(v) != runtime.NumCPU() {
		t.Fatalf("threshold=%v want %d cores", v, runtime.NumCPU())
	}
	if _, ok := relativeThreshold("CPU 使用率"); ok {
		t.Fatal("percentage metrics must not get a relative threshold")
	}
}

func TestParseOffsetUnits(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1.234ms", 0.001234},
		{"-3.2s", 3.2},
		{"123us", 0.000123},
		{"0", 0},
		{"", 0},
	}
	for _, c := range cases {
		got, _ := parseOffset(c.in)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("%q: got %v want %v", c.in, got, c.want)
		}
	}
}

func TestEventMetricUnit(t *testing.T) {
	cases := map[string]string{
		"OOM 事件数":      " 次",
		"容器异常退出数":      " 次",
		"NTP 时间偏移":     " 秒",
		"Docker 服务不可用": "",
	}
	for metric, want := range cases {
		got, ok := eventMetricUnit(metric)
		if !ok {
			t.Fatalf("%q should be an event metric", metric)
		}
		if got != want {
			t.Fatalf("%q: unit %q want %q", metric, got, want)
		}
	}
	if _, ok := eventMetricUnit("CPU 使用率"); ok {
		t.Fatal("sampled metrics must not be treated as event metrics")
	}
}

// Whatever this host can answer, a check must never claim a false positive: on
// macOS (no /proc, no dmesg) every probe has to report "unknown" instead of zero.
func TestChecksReportUnknownInsteadOfZero(t *testing.T) {
	m := &AlertMonitor{}
	for _, c := range m.eventChecks() {
		n, _, ok := c.fn(c.window)
		if !ok {
			continue
		}
		if n < 0 {
			t.Fatalf("%s returned a negative count %d", c.metric, n)
		}
	}
	if runtime.GOOS != "linux" {
		if _, _, ok := readOnlyFS(); ok {
			t.Fatal("readOnlyFS must report unknown without /proc/mounts")
		}
		up := dmesgCount("x")
		if _, _, ok := up(time.Hour); ok {
			t.Fatal("dmesg checks must report unknown when dmesg is unavailable")
		}
	}
}

func TestCheckValueOnlyMatchesKnownMetrics(t *testing.T) {
	m := &AlertMonitor{sources: AlertSources{
		FailedCronJobs: func() (int, string, bool) { return 3, "nightly: exit 1", true },
	}}
	if v, ok := m.checkValue("计划任务失败数"); !ok || v != 3 {
		t.Fatalf("cron failures: got (%v,%v) want (3,true)", v, ok)
	}
	// 日志管道积压在 v1.4.2 随模块一起移除，这条指标必须变成"未知"而非 0。
	if _, ok := m.checkValue("日志管道积压"); ok {
		t.Fatal("removed log pipeline metric must be unknown")
	}
	// A nil source must read as unknown, not as zero.
	if _, ok := m.checkValue("备份失败数"); ok {
		t.Fatal("missing source must be unknown")
	}
	if _, ok := m.checkValue("CPU 使用率"); ok {
		t.Fatal("sampled metric must not be resolved by the event checks")
	}
}
