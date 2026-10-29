package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Each test asserts on the *request the provider would receive*, not just on the
// parsed result. A wrong field name is exactly the bug that only shows up against
// a real endpoint, so it is cheaper to pin it here.

func serve(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		handler(w, r, body)
	}))
	return nil, srv.Close
}

func clientFor(base string, cfg Config) *Client {
	return New(func() Config {
		c := cfg
		c.BaseURL = base
		return c
	})
}

func TestOpenAIRequestAndToolCalls(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok","tool_calls":[{"id":"c1","type":"function","function":{"name":"restart","arguments":"{\"svc\":\"nginx\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderOpenAI, APIKey: "sk-x", Model: "gpt-4o"})
	msg, usage, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}},
		[]Tool{{Name: "restart", Description: "restart a service", Schema: map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path=%s", gotPath)
	}
	if gotAuth != "Bearer sk-x" {
		t.Fatalf("auth=%s", gotAuth)
	}
	if gotBody["model"] != "gpt-4o" {
		t.Fatalf("model=%v", gotBody["model"])
	}
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools=%v", gotBody["tools"])
	}
	if msg.Content != "ok" || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "restart" {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"svc":"nginx"}` {
		t.Fatalf("args=%s", msg.ToolCalls[0].Function.Arguments)
	}
	if usage == nil || usage.TotalTokens != 15 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestOpenAIStreamStitchesToolArgs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"choices":[{"delta":{"content":"he"}}]}`,
			`{"choices":[{"delta":{"content":"llo"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c9","type":"function","function":{"name":"run","arguments":"{\"cmd\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`,
		}
		for _, f := range frames {
			io.WriteString(w, "data: "+f+"\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{APIKey: "k", Model: "m"})
	var deltas []string
	msg, _, err := c.ChatStream(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "hello" {
		t.Fatalf("content=%q", msg.Content)
	}
	if strings.Join(deltas, "") != "hello" {
		t.Fatalf("deltas=%v", deltas)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Arguments != `{"cmd":"ls"}` {
		t.Fatalf("calls=%+v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].ID != "c9" {
		t.Fatalf("id=%s", msg.ToolCalls[0].ID)
	}
}

// Anthropic is worth the most scrutiny: the system prompt, the tool blocks and the
// tool result placement are all different from OpenAI, and getting any of them
// wrong produces a 400 from the real API rather than a wrong answer.
func TestAnthropicRequestShape(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		io.WriteString(w, `{"content":[{"type":"text","text":"done"},{"type":"tool_use","id":"toolu_1","name":"restart","input":{"svc":"nginx"}}],"stop_reason":"tool_use","usage":{"input_tokens":11,"output_tokens":7}}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderAnthropic, APIKey: "sk-ant", Model: "claude-sonnet-4-5"})
	msgs := []Message{
		{Role: "system", Content: "you are ops"},
		{Role: "user", Content: "restart nginx"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "toolu_0", Type: "function", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "status", Arguments: `{"svc":"nginx"}`}}}},
		{Role: "tool", ToolCallID: "toolu_0", Content: "running"},
	}
	msg, usage, err := c.Chat(context.Background(), msgs, []Tool{{Name: "restart", Schema: map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path=%s", gotPath)
	}
	if gotKey != "sk-ant" || gotVersion != anthropicVersion {
		t.Fatalf("headers key=%s version=%s", gotKey, gotVersion)
	}
	if gotBody["system"] != "you are ops" {
		t.Fatalf("system not hoisted: %v", gotBody["system"])
	}
	if gotBody["max_tokens"] == nil {
		t.Fatal("max_tokens is required by Anthropic")
	}
	raw, _ := json.Marshal(gotBody["messages"])
	var wire []struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	// system must be gone from the message list, and the tool result must be a
	// user turn carrying a tool_result block.
	if len(wire) != 3 {
		t.Fatalf("messages=%s", raw)
	}
	if wire[0].Role != "user" || wire[0].Content[0].Type != "text" {
		t.Fatalf("first=%+v", wire[0])
	}
	if wire[1].Role != "assistant" || wire[1].Content[0].Type != "tool_use" {
		t.Fatalf("second=%+v", wire[1])
	}
	if wire[2].Role != "user" || wire[2].Content[0].Type != "tool_result" || wire[2].Content[0].ToolUseID != "toolu_0" {
		t.Fatalf("third=%+v", wire[2])
	}
	if msg.Content != "done" || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "restart" {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"svc":"nginx"}` {
		t.Fatalf("args=%s", msg.ToolCalls[0].Function.Arguments)
	}
	if usage.PromptTokens != 11 || usage.CompletionTokens != 7 || usage.TotalTokens != 18 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAnthropicStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":20}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_9","name":"run"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"ls\"}"}}`,
			`{"type":"message_delta","usage":{"output_tokens":9}}`,
		}
		for _, e := range events {
			io.WriteString(w, "event: x\ndata: "+e+"\n\n")
		}
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderAnthropic, APIKey: "k", Model: "m"})
	msg, usage, err := c.ChatStream(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "Hello" {
		t.Fatalf("content=%q", msg.Content)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Arguments != `{"cmd":"ls"}` || msg.ToolCalls[0].ID != "toolu_9" {
		t.Fatalf("calls=%+v", msg.ToolCalls)
	}
	if usage.PromptTokens != 20 || usage.CompletionTokens != 9 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAzureURLAndHeader(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("api-key")
		if r.Header.Get("Authorization") != "" {
			t.Error("Azure must not use a Bearer token")
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderAzure, APIKey: "az-key", Model: "gpt-4o-deploy"})
	if _, _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/openai/deployments/gpt-4o-deploy/chat/completions" {
		t.Fatalf("path=%s", gotPath)
	}
	if !strings.Contains(gotQuery, "api-version=2024-10-21") {
		t.Fatalf("query=%s", gotQuery)
	}
	if gotKey != "az-key" {
		t.Fatalf("api-key header=%q", gotKey)
	}
}

func TestGeminiRequestShape(t *testing.T) {
	var gotPath, gotQuery string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"},{"functionCall":{"name":"restart","args":{"svc":"nginx"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4,"totalTokenCount":7}}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderGemini, APIKey: "g-key", Model: "gemini-2.5-pro"})
	msg, usage, err := c.Chat(context.Background(), []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}, []Tool{{Name: "restart", Schema: map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-pro:generateContent" {
		t.Fatalf("path=%s", gotPath)
	}
	if !strings.Contains(gotQuery, "key=g-key") {
		t.Fatalf("query=%s", gotQuery)
	}
	if gotBody["systemInstruction"] == nil {
		t.Fatalf("systemInstruction missing: %v", gotBody)
	}
	if _, ok := gotBody["contents"]; !ok {
		t.Fatalf("contents missing: %v", gotBody)
	}
	if msg.Content != "hi" || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "restart" {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"svc":"nginx"}` {
		t.Fatalf("args=%s", msg.ToolCalls[0].Function.Arguments)
	}
	if usage.TotalTokens != 7 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestGeminiToolResultBecomesFunctionResponse(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderGemini, APIKey: "k", Model: "m"})
	_, _, err := c.Chat(context.Background(), []Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_restart", Type: "function", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "restart", Arguments: `{}`}}}},
		{Role: "tool", Name: "restart", Content: "done"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(gotBody["contents"])
	if !strings.Contains(string(raw), "functionResponse") || !strings.Contains(string(raw), "functionCall") {
		t.Fatalf("contents=%s", raw)
	}
}

func TestProviderDefaultsAndAuth(t *testing.T) {
	cases := []struct {
		provider   string
		wantTokens int
	}{
		{"", 4096},
		{ProviderAnthropic, 4096},
		{ProviderGemini, 8192},
	}
	for _, c := range cases {
		got := Config{Provider: c.provider}.Normalized()
		if got.MaxTokens != c.wantTokens {
			t.Fatalf("provider %q maxTokens=%d want %d", c.provider, got.MaxTokens, c.wantTokens)
		}
	}
	if (Config{}).Normalized().Provider != ProviderOpenAI {
		t.Fatal("empty provider must default to openai")
	}
	// A local Ollama endpoint needs no key; a remote one does.
	local := New(func() Config { return Config{BaseURL: "http://127.0.0.1:11434/v1", Model: "llama3"} })
	if err := local.Validate(); err != nil {
		t.Fatalf("local endpoint should not require a key: %v", err)
	}
	remote := New(func() Config { return Config{BaseURL: "https://api.example.com/v1", Model: "m"} })
	if err := remote.Validate(); err == nil {
		t.Fatal("remote endpoint without key must fail validation")
	}
}

func TestErrorBodySurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid x-api-key"}}`)
	}))
	defer srv.Close()

	c := clientFor(srv.URL, Config{Provider: ProviderAnthropic, APIKey: "bad", Model: "m"})
	_, _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if err == nil {
		t.Fatal("401 must be an error")
	}
	if !strings.Contains(err.Error(), "鉴权失败") || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("error should name the auth problem and include the body: %v", err)
	}
}
