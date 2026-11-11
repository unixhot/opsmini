package aiagent

import "testing"

// "就绪"必须和"真的能发请求"是同一份判断。
//
// 之前 Ready() 只看地址和模型，密钥没填也算就绪：请求发出去才失败，而失败
// 会被引擎记成"处置失败"，用户看到红色 failed 却找不到原因。
func TestLLMConfigReadyRequiresEverything(t *testing.T) {
	cases := []struct {
		name string
		cfg  LLMConfig
		want bool
	}{
		{"全空", LLMConfig{}, false},
		{"只有地址", LLMConfig{BaseURL: "https://api.deepseek.com/v1"}, false},
		{"地址+模型，缺密钥", LLMConfig{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"}, false},
		{"齐全", LLMConfig{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", APIKey: "sk-x"}, true},
		// 本地推理服务本来就没有密钥。
		{"本地 ollama", LLMConfig{Provider: "ollama", BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen"}, true},
	}
	for _, c := range cases {
		if got := c.cfg.Ready(); got != c.want {
			t.Errorf("%s: Ready()=%v，期望 %v", c.name, got, c.want)
		}
	}
}
