package service

import (
	"testing"
	"time"
)

// 面板上 duration 是自由文本，用户写什么都得能解释；解释不了就当"立即告警"，
// 这样至少不会因为写了个奇怪的字符串就永远不报警。
func TestParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5 分钟", 5 * time.Minute},
		{"5分钟", 5 * time.Minute},
		{"1 分钟", time.Minute},
		{"30 秒", 30 * time.Second},
		{"1 小时", time.Hour},
		{"5m", 5 * time.Minute},
		{"90s", 90 * time.Second},
		{"2h", 2 * time.Hour},
		{"0 分钟", 0},
		{"", 0},
		{"abc", 0},
		{"5", 0},
	}
	for _, c := range cases {
		if got := parseWindow(c.in); got != c.want {
			t.Errorf("parseWindow(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// 核心回归：规则写"持续 5 分钟"，刚开始越线时不能报，满了才报。
// 修改前 duration 完全没被读取，一次采样越线就立刻 fire。
func TestSustainedSince(t *testing.T) {
	now := time.Now()
	if sustainedSince(now, 5*time.Minute, now) {
		t.Fatal("刚越线就判定达标，duration 未生效")
	}
	if !sustainedSince(now.Add(-5*time.Minute-time.Second), 5*time.Minute, now) {
		t.Fatal("越线超过 5 分钟仍未达标")
	}
	if !sustainedSince(now, 0, now) {
		t.Fatal("duration 为 0 时应立即触发")
	}
	if !sustainedSince(now, -1, now) {
		t.Fatal("负数 duration 应按 0 处理")
	}
}
