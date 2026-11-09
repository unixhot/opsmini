package aiagent

import (
	"strings"
	"testing"

	"github.com/opsmini/opsmini/internal/model"
)

// A remembered fact must reach the system prompt, otherwise the whole feature is
// a write-only table.
func TestSystemPromptInjectsMemories(t *testing.T) {
	a := &Agent{cfg: Config{
		Memories: func(query string) []string {
			if !strings.Contains(query, "nginx") {
				return nil
			}
			return []string{"nginx：配置在 /etc/nginx/conf.d"}
		},
	}}
	out := a.systemPrompt(RunOptions{}, "nginx 启动失败")
	if !strings.Contains(out, "/etc/nginx/conf.d") {
		t.Fatalf("memory not injected:\n%s", out)
	}
	// The prompt must tell the model which source wins, otherwise a stale memory
	// is asserted with the same authority as a fresh tool result.
	if !strings.Contains(out, "以实时结果为准") {
		t.Fatalf("missing precedence hint:\n%s", out)
	}
}

func TestSystemPromptSkipsWhenNoMatch(t *testing.T) {
	a := &Agent{cfg: Config{
		Memories: func(query string) []string { return nil },
	}}
	out := a.systemPrompt(RunOptions{}, "磁盘满了")
	if strings.Contains(out, "已确认的事实") {
		t.Fatalf("empty memory set must not add a section:\n%s", out)
	}
}

// Without wiring, the prompt must be exactly the base prompt: a nil function is
// the "feature off" signal.
func TestSystemPromptNoMemoriesWired(t *testing.T) {
	a := &Agent{cfg: Config{}}
	out := a.systemPrompt(RunOptions{}, "任意问题")
	if strings.Contains(out, "已确认的事实") {
		t.Fatalf("no memories wired, but a section appeared")
	}
}

func TestLastUserText(t *testing.T) {
	h := []Message{
		{Role: model.AIRoleSystem, Content: "sys"},
		{Role: model.AIRoleUser, Content: "第一问"},
		{Role: model.AIRoleAssistant, Content: "回答"},
		{Role: model.AIRoleUser, Content: "第二问"},
		{Role: model.AIRoleTool, Content: "结果"},
	}
	if got := lastUserText(h); got != "第二问" {
		t.Fatalf("got %q", got)
	}
	if got := lastUserText(nil); got != "" {
		t.Fatalf("empty history should yield empty string, got %q", got)
	}
}
