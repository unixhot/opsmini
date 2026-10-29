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

// Anthropic Messages API.
//
// Three differences from OpenAI are structural, not cosmetic, and they are why
// this needs its own adapter rather than a header tweak:
//   - the system prompt is a top-level field, not a message;
//   - content is a list of blocks (text / tool_use / tool_result), so a tool call
//     is part of the assistant turn instead of a separate array;
//   - tool results come back as *user* messages carrying a tool_result block.
//
// Anthropic-compatible relays (one-api, new-api, many enterprise gateways) speak
// exactly this format, so supporting it also covers "Claude behind a proxy".

const anthropicVersion = "2023-06-01"

type antRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []antMessage `json:"messages"`
	Tools     []antTool    `json:"tools,omitempty"`
	Stream    bool         `json:"stream,omitempty"`
}

type antMessage struct {
	Role    string     `json:"role"` // user | assistant
	Content []antBlock `json:"content"`
}

type antBlock struct {
	Type string `json:"type"` // text | tool_use | tool_result
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type antTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type antResponse struct {
	Content    []antBlock `json:"content"`
	StopReason string     `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func antURL(cfg Config) string {
	base := cfg.BaseURL
	if strings.HasSuffix(base, "/messages") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/messages"
	}
	return base + "/v1/messages"
}

func antNewRequest(ctx context.Context, cfg Config, body any, stream bool) (*http.Request, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, antURL(cfg), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

// buildAnthropic converts neutral messages into Anthropic's shape.
//
// Two rules of this API shape the conversion:
//   - a system prompt must be hoisted out of the message list;
//   - consecutive messages of the same role are merged, because tool results are
//     user-role messages and would otherwise sit next to the real user turn.
func buildAnthropic(msgs []Message) (system string, out []antMessage) {
	var sys []string
	for _, m := range msgs {
		role := m.Role
		switch role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sys = append(sys, m.Content)
			}
		case "user":
			appendAnt(&out, "user", antBlock{Type: "text", Text: m.Content})
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				appendAnt(&out, "assistant", antBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := map[string]any{}
				if tc.Function.Arguments != "" {
					// A malformed argument string is the model's problem; send an empty
					// object so the turn stays valid instead of failing the whole call.
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				}
				id := tc.ID
				if id == "" {
					// Anthropic requires an id, some relays omit it.
					id = "toolu_" + tc.Function.Name
				}
				appendAnt(&out, "assistant", antBlock{Type: "tool_use", ID: id, Name: tc.Function.Name, Input: input})
			}
		case "tool":
			appendAnt(&out, "user", antBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content})
		}
	}
	return strings.Join(sys, "\n\n"), out
}

// appendAnt appends a block, merging into the previous message when the role
// matches (Anthropic wants alternating roles but tolerates merged ones).
func appendAnt(out *[]antMessage, role string, block antBlock) {
	if n := len(*out); n > 0 && (*out)[n-1].Role == role {
		(*out)[n-1].Content = append((*out)[n-1].Content, block)
		return
	}
	*out = append(*out, antMessage{Role: role, Content: []antBlock{block}})
}

func toAntTools(tools []Tool) []antTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]antTool, 0, len(tools))
	for _, t := range tools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, antTool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	return out
}

// fromAnthropic converts a response back to the neutral message.
func fromAnthropic(blocks []antBlock) *Message {
	msg := &Message{Role: "assistant"}
	var text strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			var tc ToolCall
			tc.ID = b.ID
			tc.Type = "function"
			tc.Function.Name = b.Name
			tc.Function.Arguments = string(args)
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
	msg.Content = text.String()
	return msg
}

func (c *Client) chatAnthropic(ctx context.Context, cfg Config, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	system, converted := buildAnthropic(msgs)
	req, err := antNewRequest(ctx, cfg, antRequest{
		Model: cfg.Model, MaxTokens: cfg.MaxTokens, System: system,
		Messages: converted, Tools: toAntTools(tools),
	}, false)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, httpError("Anthropic", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	var parsed antResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("解析 Anthropic 响应失败：%w", err)
	}
	if parsed.Error != nil {
		return nil, nil, fmt.Errorf("Anthropic 返回错误：%s", parsed.Error.Message)
	}
	usage := &Usage{
		PromptTokens:     parsed.Usage.InputTokens,
		CompletionTokens: parsed.Usage.OutputTokens,
		TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
	}
	msg := fromAnthropic(parsed.Content)
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil, usage, errors.New("模型返回了空响应")
	}
	return msg, usage, nil
}

func (c *Client) streamAnthropic(ctx context.Context, cfg Config, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	system, converted := buildAnthropic(msgs)
	req, err := antNewRequest(ctx, cfg, antRequest{
		Model: cfg.Model, MaxTokens: cfg.MaxTokens, System: system,
		Messages: converted, Tools: toAntTools(tools), Stream: true,
	}, true)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, httpError("Anthropic", resp)
	}

	type toolAcc struct {
		id   string
		name string
		json strings.Builder
	}
	var (
		content   strings.Builder
		toolOrder []int
		toolsAcc  = map[int]*toolAcc{}
		usage     = &Usage{}
		sawAny    bool
	)

	reader := bufio.NewReaderSize(resp.Body, 64<<10)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmed, []byte("data:")) {
				payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
				var ev struct {
					Type  string `json:"type"`
					Index int    `json:"index"`
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
					ContentBlock struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
					Message struct {
						Usage struct {
							InputTokens int `json:"input_tokens"`
						} `json:"usage"`
					} `json:"message"`
					Usage struct {
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(payload, &ev); err == nil {
					switch ev.Type {
					case "content_block_start":
						if ev.ContentBlock.Type == "tool_use" {
							toolsAcc[ev.Index] = &toolAcc{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
							toolOrder = append(toolOrder, ev.Index)
						}
					case "content_block_delta":
						if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
							sawAny = true
							content.WriteString(ev.Delta.Text)
							if onDelta != nil {
								onDelta(ev.Delta.Text)
							}
						}
						if ev.Delta.Type == "input_json_delta" {
							if acc := toolsAcc[ev.Index]; acc != nil {
								acc.json.WriteString(ev.Delta.PartialJSON)
							}
						}
					case "message_start":
						usage.PromptTokens = ev.Message.Usage.InputTokens
					case "message_delta":
						if ev.Usage.OutputTokens > 0 {
							usage.CompletionTokens = ev.Usage.OutputTokens
						}
					case "error":
						return nil, usage, fmt.Errorf("Anthropic 返回错误：%s", ev.Error.Message)
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
			if sawAny || len(toolOrder) > 0 {
				break
			}
			return nil, usage, fmt.Errorf("读取 Anthropic 流失败：%w", readErr)
		}
	}

	msg := &Message{Role: "assistant", Content: content.String()}
	for _, idx := range toolOrder {
		acc := toolsAcc[idx]
		args := acc.json.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		var tc ToolCall
		tc.ID = acc.id
		tc.Type = "function"
		tc.Function.Name = acc.name
		tc.Function.Arguments = args
		msg.ToolCalls = append(msg.ToolCalls, tc)
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.TotalTokens == 0 {
		usage = estimateUsage(msgs, msg)
	}
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		// Nothing streamed back: retry without streaming instead of returning empty.
		return c.chatAnthropic(ctx, cfg, msgs, tools)
	}
	return msg, usage, nil
}
