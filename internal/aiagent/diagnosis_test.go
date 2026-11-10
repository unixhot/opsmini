package aiagent

import (
	"strings"
	"testing"
)

// 四段式只对"无人值守"生效：交互式对话里用户边问边看，硬套格式反而啰嗦。
func TestDiagnosisPromptOnlyForUnattended(t *testing.T) {
	a := &Agent{}
	cfg := a.cfg

	unattended := a.systemPrompt(RunOptions{Unattended: true}, "")
	for _, want := range []string{"问题环节", "证据（事实）", "推断（置信度）", "下一步与证据缺口"} {
		if !strings.Contains(unattended, want) {
			t.Errorf("无人值守的提示词缺少段落 %q", want)
		}
	}

	interactive := a.systemPrompt(RunOptions{Unattended: false}, "")
	if strings.Contains(interactive, "证据（事实）") {
		t.Error("交互式对话不该被强加四段式格式")
	}
	_ = cfg
}
