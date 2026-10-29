package llm

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
)

// OpenAI-compatible /chat/completions.
//
// This one protocol covers most of the market: OpenAI itself, DeepSeek, Qwen
// (DashScope compatible mode), Moonshot, Zhipu, Ollama, vLLM, LM Studio, and the
// one-api/new-api relays. Azure differs only in URL shape and auth header, so it
// reuses the same body.

type oaRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	Tools     []oaTool  `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	// stream_options asks for a final usage frame. Providers that do not know the
	// field ignore it; OpenAI-compatible relays that reject unknown fields are why
	// this is only set when the caller supports it (see streamOpenAI).
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type oaStreamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// oaURL resolves the chat completions URL for a provider.
func oaURL(cfg Config) string {
	base := cfg.BaseURL
	if cfg.Provider == ProviderAzure {
		// Azure addresses a deployment, not a model, and requires api-version.
		if strings.Contains(base, "/openai/deployments/") {
			if strings.Contains(base, "api-version=") {
				return base + "/chat/completions"
			}
			return base + "/chat/completions?api-version=" + cfg.AzureAPIVersion
		}
		return base + "/openai/deployments/" + cfg.Model + "/chat/completions?api-version=" + cfg.AzureAPIVersion
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// oaAuthorize sets the header the provider expects.
func oaAuthorize(req *http.Request, cfg Config) {
	if cfg.Provider == ProviderAzure {
		req.Header.Set("api-key", cfg.APIKey)
		return
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
}

func toOATools(tools []Tool) []oaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]oaTool, 0, len(tools))
	for _, t := range tools {
		var d oaTool
		d.Type = "function"
		d.Function.Name = t.Name
		d.Function.Description = t.Description
		d.Function.Parameters = t.Schema
		out = append(out, d)
	}
	return out
}

func (c *Client) chatOpenAI(ctx context.Context, cfg Config, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	body, err := json.Marshal(oaRequest{
		Model: cfg.Model, Messages: msgs, Stream: false,
		Tools: toOATools(tools), MaxTokens: cfg.MaxTokens,
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oaURL(cfg), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	oaAuthorize(req, cfg)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, httpError("模型", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	var parsed oaResponse
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

func (c *Client) streamOpenAI(ctx context.Context, cfg Config, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	body, err := json.Marshal(oaRequest{
		Model: cfg.Model, Messages: msgs, Stream: true,
		Tools: toOATools(tools), MaxTokens: cfg.MaxTokens,
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oaURL(cfg), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	oaAuthorize(req, cfg)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Some providers do not stream at all; fall back rather than fail.
		return c.chatOpenAI(ctx, cfg, msgs, tools)
	}

	var (
		content   strings.Builder
		calls     []ToolCall
		argBuf    = map[int]*strings.Builder{}
		usage     *Usage
		sawAny    bool
		gotStream bool
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
					var chunk oaStreamChunk
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
								call := ensureCall(&calls, argBuf, frag.Index)
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
			if sawAny {
				break
			}
			return nil, usage, fmt.Errorf("读取模型流失败：%w", readErr)
		}
	}

	flushArgs(calls, argBuf)
	msg := &Message{Role: "assistant", Content: content.String(), ToolCalls: calls}
	if usage == nil {
		// Many providers omit usage unless asked; estimate so budgets still work.
		usage = estimateUsage(msgs, msg)
	}
	// A stream that produced neither content nor tool calls (some providers simply
	// do not support streaming tool calls) is retried non-streaming.
	if !sawAny && !gotStream {
		return c.chatOpenAI(ctx, cfg, msgs, tools)
	}
	return msg, usage, nil
}

// ensureCall returns the tool call at index i, creating it if needed.
func ensureCall(calls *[]ToolCall, argBuf map[int]*strings.Builder, i int) *ToolCall {
	for len(*calls) <= i {
		*calls = append(*calls, ToolCall{Type: "function"})
	}
	if argBuf[i] == nil {
		argBuf[i] = &strings.Builder{}
	}
	return &(*calls)[i]
}

// flushArgs moves buffered argument fragments into the calls and fills defaults.
func flushArgs(calls []ToolCall, argBuf map[int]*strings.Builder) {
	for i := range calls {
		if b, ok := argBuf[i]; ok {
			calls[i].Function.Arguments = b.String()
		}
		if calls[i].Type == "" {
			calls[i].Type = "function"
		}
	}
}

// estimateUsage approximates token counts when the provider sends none.
//
// Roughly 4 characters per token for mixed English/Chinese text. It is only used
// for budget accounting, never shown as authoritative.
func estimateUsage(in []Message, out *Message) *Usage {
	count := 0
	for _, m := range in {
		count += len(m.Content) + len(m.ToolCallID) + 8
		for _, tc := range m.ToolCalls {
			count += len(tc.Function.Arguments) + len(tc.Function.Name) + 8
		}
	}
	prompt := count / 4
	completion := 0
	if out != nil {
		completion = (len(out.Content) + 8) / 4
		for _, tc := range out.ToolCalls {
			completion += (len(tc.Function.Arguments) + 8) / 4
		}
	}
	return &Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
}
