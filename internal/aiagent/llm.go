package aiagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMConfig is a resolved model endpoint configuration.
type LLMConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	MaxTokens int
}

// Ready reports whether the configuration is usable.
func (c LLMConfig) Ready() bool {
	return c.BaseURL != "" && c.Model != ""
}

// LLMSettings returns the live model configuration, so panel settings take
// effect without restarting the process.
type LLMSettings func() LLMConfig

// LLMClient talks to any OpenAI-compatible /chat/completions endpoint
// (OpenAI, DeepSeek, Qwen/DashScope compatible mode, Ollama, ...).
type LLMClient struct {
	client *http.Client
	cfg    LLMSettings
}

// NewLLMClient creates a client. cfg is evaluated on every call.
func NewLLMClient(cfg LLMSettings) *LLMClient {
	if cfg == nil {
		cfg = func() LLMConfig { return LLMConfig{} }
	}
	return &LLMClient{
		client: &http.Client{Timeout: 10 * time.Minute},
		cfg:    cfg,
	}
}

// chatRequest is the OpenAI-compatible request body.
type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	Tools     []toolDef `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// chatResponse is the OpenAI-compatible non-streaming response body.
type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// streamToolCall is one tool-call fragment inside a streaming delta.
type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamChunk is one SSE frame of a streaming completion.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []streamToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// endpoint resolves the chat completions URL.
func (c *LLMClient) endpoint() string {
	cfg := c.cfg()
	base := strings.TrimRight(cfg.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// newRequest builds an authenticated JSON POST request.
func (c *LLMClient) newRequest(ctx context.Context, body any, stream bool) (*http.Request, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	cfg := c.cfg()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

// Validate reports whether the model endpoint is configured.
func (c *LLMClient) Validate() error {
	cfg := c.cfg()
	if cfg.BaseURL == "" {
		return errors.New("AI 接口地址未配置")
	}
	if cfg.Model == "" {
		return errors.New("AI 模型未配置")
	}
	if cfg.APIKey == "" {
		return errors.New("AI API Key 未配置")
	}
	return nil
}

// Chat performs a non-streaming completion.
func (c *LLMClient) Chat(ctx context.Context, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	cfg := c.cfg()
	req, err := c.newRequest(ctx, chatRequest{
		Model:     cfg.Model,
		Messages:  msgs,
		Stream:    false,
		Tools:     toolsToDefs(tools),
		MaxTokens: cfg.MaxTokens,
	}, false)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("模型接口返回 %d：%s", resp.StatusCode, truncateText(string(data), 400))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("解析模型响应失败：%w", err)
	}
	if parsed.Error != nil {
		return nil, nil, fmt.Errorf("模型返回错误：%s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, parsed.Usage, errors.New("模型返回了空响应")
	}
	msg := parsed.Choices[0].Message
	return &msg, parsed.Usage, nil
}

// ChatStream performs one streaming completion. onDelta receives incremental
// assistant text (may be nil). The returned Message accumulates the full content
// plus every tool call requested by the model.
//
// Streaming tool calls arrive as fragments: the first frame carries the call id
// and the function name, and subsequent frames append pieces of the JSON
// arguments. Fragments are stitched back together by their `index`.
func (c *LLMClient) ChatStream(ctx context.Context, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	cfg := c.cfg()
	req, err := c.newRequest(ctx, chatRequest{
		Model:     cfg.Model,
		Messages:  msgs,
		Stream:    true,
		Tools:     toolsToDefs(tools),
		MaxTokens: cfg.MaxTokens,
	}, true)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, nil, fmt.Errorf("模型接口返回 %d：%s", resp.StatusCode, truncateText(string(data), 400))
	}

	var (
		content   strings.Builder
		calls     []ToolCall
		argBuf    = map[int]*strings.Builder{}
		usage     *Usage
		sawAny    bool
		gotStream = false
	)

	reader := bufio.NewReaderSize(resp.Body, 64<<10)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmed, []byte("data:")) {
				payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
				if len(payload) > 0 && string(payload) != "[DONE]" {
					gotStream = true
					var chunk streamChunk
					if err := json.Unmarshal(payload, &chunk); err == nil {
						if chunk.Error != nil {
							return nil, usage, fmt.Errorf("模型返回错误：%s", chunk.Error.Message)
						}
						if chunk.Usage != nil {
							usage = chunk.Usage
						}
						for _, ch := range chunk.Choices {
							if ch.Delta.Content != "" {
								sawAny = true
								content.WriteString(ch.Delta.Content)
								if onDelta != nil {
									onDelta(ch.Delta.Content)
								}
							}
							for _, frag := range ch.Delta.ToolCalls {
								sawAny = true
								call := ensureCall(&calls, &argBuf, frag.Index)
								if frag.ID != "" {
									call.ID = frag.ID
								}
								if frag.Type != "" {
									call.Type = frag.Type
								}
								if frag.Function.Name != "" {
									call.Function.Name += frag.Function.Name
								}
								if frag.Function.Arguments != "" {
									argBuf[frag.Index].WriteString(frag.Function.Arguments)
								}
							}
						}
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return nil, usage, ctx.Err()
			}
			// A stream cut mid-flight is only fatal when nothing was received.
			if sawAny {
				break
			}
			return nil, usage, fmt.Errorf("读取模型流失败：%w", readErr)
		}
	}

	// Flush accumulated arguments.
	for i := range calls {
		if b, ok := argBuf[i]; ok {
			calls[i].Function.Arguments = b.String()
		}
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
	}

	msg := &Message{Role: "assistant", Content: content.String(), ToolCalls: calls}

	// Some providers do not support streaming tool calls: the stream returns no
	// content and no tool calls at all. Fall back to a non-streaming call so the
	// agent still works against them.
	if !sawAny && !gotStream {
		fallback, fbUsage, fbErr := c.Chat(ctx, msgs, tools)
		if fbErr != nil {
			return nil, usage, fbErr
		}
		if fbUsage != nil {
			usage = fbUsage
		}
		if onDelta != nil && fallback.Content != "" {
			onDelta(fallback.Content)
		}
		return fallback, usage, nil
	}

	return msg, usage, nil
}

// ensureCall returns the tool call at index i, growing the slice as needed, and
// makes sure an argument buffer exists for it.
func ensureCall(calls *[]ToolCall, argBuf *map[int]*strings.Builder, i int) *ToolCall {
	for len(*calls) <= i {
		*calls = append(*calls, ToolCall{})
	}
	if (*argBuf)[i] == nil {
		(*argBuf)[i] = &strings.Builder{}
	}
	return &(*calls)[i]
}

// toolsToDefs converts tools into OpenAI function definitions.
func toolsToDefs(tools []Tool) []toolDef {
	if len(tools) == 0 {
		return nil
	}
	out := make([]toolDef, 0, len(tools))
	for _, t := range tools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, toolDef{Type: "function", Function: funcDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		}})
	}
	return out
}

// truncateText shortens s to at most n runes, appending an ellipsis.
func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
