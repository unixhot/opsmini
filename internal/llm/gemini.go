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
	"net/url"
	"strings"
)

// Google Gemini (generativeLanguage API).
//
// Gemini's shape is the most different of the four: the model name is part of the
// URL rather than the body, content is a list of parts inside contents, tool calls
// appear as functionCall parts, and tool results must be sent back as
// functionResponse parts inside a *user* turn. Supporting it matters because it is
// the third ecosystem operators actually ask for, after OpenAI-compatible and
// Anthropic.

type gemPart struct {
	Text             string           `json:"text,omitempty"`
	FunctionCall     *gemFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *gemFuncResponse `json:"functionResponse,omitempty"`
}

type gemFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type gemFuncResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type gemContent struct {
	Role  string    `json:"role,omitempty"` // user | model
	Parts []gemPart `json:"parts"`
}

type gemRequest struct {
	Contents          []gemContent `json:"contents"`
	SystemInstruction *gemContent  `json:"systemInstruction,omitempty"`
	Tools             []gemToolSet `json:"tools,omitempty"`
	GenerationConfig  *gemGenCfg   `json:"generationConfig,omitempty"`
}

type gemToolSet struct {
	FunctionDeclarations []gemFuncDecl `json:"functionDeclarations"`
}

type gemFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type gemGenCfg struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type gemResponse struct {
	Candidates []struct {
		Content      gemContent `json:"content"`
		FinishReason string     `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// gemURL builds the generate (or streamGenerate) URL. The key goes in the query
// string because that is what Generative Language expects.
func gemURL(cfg Config, stream bool) string {
	base := cfg.BaseURL
	if !strings.Contains(base, "/v1beta") && !strings.Contains(base, "/v1") && !strings.Contains(base, ":generateContent") {
		base += "/v1beta"
	}
	action := ":generateContent"
	if stream {
		action = ":streamGenerateContent"
	}
	if strings.Contains(base, ":generateContent") || strings.Contains(base, ":streamGenerateContent") {
		// A full URL was configured; only the streaming mode may need rewriting.
		if stream && strings.Contains(base, ":generateContent") {
			base = strings.Replace(base, ":generateContent", ":streamGenerateContent", 1)
		}
	} else {
		base += "/models/" + cfg.Model + action
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	u := base + sep + "key=" + url.QueryEscape(cfg.APIKey)
	if stream {
		u += "&alt=sse"
	}
	return u
}

// buildGemini converts neutral messages into Gemini's contents.
func buildGemini(msgs []Message) (system string, out []gemContent) {
	var sys []string
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sys = append(sys, m.Content)
			}
		case "user":
			appendGem(&out, "user", gemPart{Text: m.Content})
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				appendGem(&out, "model", gemPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				args := map[string]any{}
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				appendGem(&out, "model", gemPart{FunctionCall: &gemFunctionCall{Name: tc.Function.Name, Args: args}})
			}
		case "tool":
			name := m.Name
			if name == "" {
				name = m.ToolCallID
			}
			appendGem(&out, "user", gemPart{FunctionResponse: &gemFuncResponse{
				Name:     name,
				Response: map[string]any{"result": m.Content},
			}})
		}
	}
	return strings.Join(sys, "\n\n"), out
}

func appendGem(out *[]gemContent, role string, part gemPart) {
	if n := len(*out); n > 0 && (*out)[n-1].Role == role {
		(*out)[n-1].Parts = append((*out)[n-1].Parts, part)
		return
	}
	*out = append(*out, gemContent{Role: role, Parts: []gemPart{part}})
}

func toGemTools(tools []Tool) []gemToolSet {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]gemFuncDecl, 0, len(tools))
	for _, t := range tools {
		decls = append(decls, gemFuncDecl{Name: t.Name, Description: t.Description, Parameters: t.Schema})
	}
	return []gemToolSet{{FunctionDeclarations: decls}}
}

// fromGemini converts a candidate's parts back to a neutral message.
func fromGemini(parts []gemPart) *Message {
	msg := &Message{Role: "assistant"}
	var text strings.Builder
	for _, p := range parts {
		if p.Text != "" {
			text.WriteString(p.Text)
		}
		if p.FunctionCall != nil {
			args, _ := json.Marshal(p.FunctionCall.Args)
			var tc ToolCall
			// Gemini has no call ids; the tool name is the only stable handle, and
			// the loop only needs it to match the result back.
			tc.ID = "call_" + p.FunctionCall.Name
			tc.Type = "function"
			tc.Function.Name = p.FunctionCall.Name
			tc.Function.Arguments = string(args)
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
	msg.Content = text.String()
	return msg
}

func gemBuildRequest(cfg Config, msgs []Message, tools []Tool, stream bool) ([]byte, error) {
	system, contents := buildGemini(msgs)
	req := gemRequest{Contents: contents, Tools: toGemTools(tools)}
	if system != "" {
		req.SystemInstruction = &gemContent{Parts: []gemPart{{Text: system}}}
	}
	if cfg.MaxTokens > 0 {
		req.GenerationConfig = &gemGenCfg{MaxOutputTokens: cfg.MaxTokens}
	}
	return json.Marshal(req)
}

func (c *Client) chatGemini(ctx context.Context, cfg Config, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	body, err := gemBuildRequest(cfg, msgs, tools, false)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gemURL(cfg, false), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, httpError("Gemini", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, nil, err
	}
	var parsed gemResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("解析 Gemini 响应失败：%w", err)
	}
	if parsed.Error != nil {
		return nil, nil, fmt.Errorf("Gemini 返回错误：%s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return nil, nil, errors.New("Gemini 返回了空响应（可能被安全策略拦截）")
	}
	usage := &Usage{
		PromptTokens:     parsed.UsageMetadata.PromptTokenCount,
		CompletionTokens: parsed.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      parsed.UsageMetadata.TotalTokenCount,
	}
	msg := fromGemini(parsed.Candidates[0].Content.Parts)
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return nil, usage, errors.New("Gemini 返回了空内容")
	}
	return msg, usage, nil
}

func (c *Client) streamGemini(ctx context.Context, cfg Config, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	body, err := gemBuildRequest(cfg, msgs, tools, true)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gemURL(cfg, true), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, httpError("Gemini", resp)
	}

	var (
		content strings.Builder
		calls   []ToolCall
		usage   = &Usage{}
		sawAny  bool
	)
	reader := bufio.NewReaderSize(resp.Body, 64<<10)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			// With alt=sse every frame is "data: {...}"; without it the endpoint
			// returns a JSON array, which is handled by the non-streaming path.
			if bytes.HasPrefix(trimmed, []byte("data:")) {
				payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
				var chunk gemResponse
				if err := json.Unmarshal(payload, &chunk); err == nil {
					if chunk.Error != nil {
						return nil, usage, fmt.Errorf("Gemini 返回错误：%s", chunk.Error.Message)
					}
					if chunk.UsageMetadata.TotalTokenCount > 0 {
						usage.PromptTokens = chunk.UsageMetadata.PromptTokenCount
						usage.CompletionTokens = chunk.UsageMetadata.CandidatesTokenCount
						usage.TotalTokens = chunk.UsageMetadata.TotalTokenCount
					}
					for _, cand := range chunk.Candidates {
						for _, p := range cand.Content.Parts {
							if p.Text != "" {
								sawAny = true
								content.WriteString(p.Text)
								if onDelta != nil {
									onDelta(p.Text)
								}
							}
							if p.FunctionCall != nil {
								args, _ := json.Marshal(p.FunctionCall.Args)
								var tc ToolCall
								tc.ID = "call_" + p.FunctionCall.Name
								tc.Type = "function"
								tc.Function.Name = p.FunctionCall.Name
								tc.Function.Arguments = string(args)
								calls = append(calls, tc)
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
			return nil, usage, fmt.Errorf("读取 Gemini 流失败：%w", readErr)
		}
	}

	msg := &Message{Role: "assistant", Content: content.String(), ToolCalls: calls}
	if usage.TotalTokens == 0 {
		usage = estimateUsage(msgs, msg)
	}
	if !sawAny && len(calls) == 0 {
		return c.chatGemini(ctx, cfg, msgs, tools)
	}
	return msg, usage, nil
}
