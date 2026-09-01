package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/config"
)

// AIService is the LLM integration service, calling the OpenAI-compatible chat completions API.
// Compatible with OpenAI / DeepSeek / Qwen (Tongyi Qianwen) / local Ollama (all provide compatible APIs).
// Built-in OpsMini MCP tools inject real-time system info, MCP list, and Skill list into the context;
// supports function calling to search/install SkillHub skills.
type AIService struct {
	cfg        *config.AI
	client     *http.Client
	getSetting func(key string) string // runtime config (settings take precedence over config)
	sys        *SystemService          // built-in tool: system info
	firewall   *FirewallService        // built-in tool: firewall
	mcp        *McpService             // built-in tool: MCP list
	skill      *SkillService           // built-in tool: Skill list / SkillHub search & install
}

// ChatMessage is a conversation message.
type ChatMessage struct {
	Role       string     `json:"role"`                   // system / user / assistant / tool
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // tool calls in an assistant message
	ToolCallID string     `json:"tool_call_id,omitempty"` // call ID of a tool message
}

// NewAIService creates an AIService. getSetting is optional, used to override config at runtime (panel settings).
func NewAIService(cfg *config.AI, getSetting func(string) string, sys *SystemService, firewall *FirewallService, mcp *McpService, skill *SkillService) *AIService {
	return &AIService{
		cfg:        cfg,
		client:     &http.Client{Timeout: 60 * time.Second},
		getSetting: getSetting,
		sys:        sys,
		firewall:   firewall,
		mcp:        mcp,
		skill:      skill,
	}
}

// System prompt that constrains the model to the role of an ops assistant.
const systemPrompt = "你是 OpsMini 运维面板内置的 AI 助手，负责协助用户管理系统与服务器。" +
	"回答应简洁、专业、可执行；涉及具体命令时给出完整命令；不确定时明确说明。"

// deepThinkPrompt is the deep-think prompt (appended to the system prompt when enabled, universally compatible with all models).
const deepThinkPrompt = "请先进行深入思考，分步骤、条理清晰地展示你的推理分析过程，再给出最终的结论与可执行建议。"

// chatCompletionReq is the OpenAI-compatible request body.
type chatCompletionReq struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Tools    []Tool        `json:"tools,omitempty"`
}

// Tool is an OpenAI function calling tool definition.
type Tool struct {
	Type     string   `json:"type"` // "function"
	Function Function `json:"function"`
}

// Function is a tool function definition.
type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall is a tool call (the tool_calls in a response).
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// chatCompletionResp is the OpenAI-compatible response body.
type chatCompletionResp struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// setting returns a runtime config item (settings first, config as fallback).
func (s *AIService) setting(key string) string {
	if s.getSetting != nil {
		if v := s.getSetting(key); v != "" {
			return v
		}
	}
	return ""
}

// enabled reports whether the AI assistant is enabled (panel setting overrides config).
func (s *AIService) enabled() bool {
	v := s.setting("ai_enabled")
	if v == "true" || v == "1" {
		return true
	}
	if v == "false" || v == "0" {
		return false
	}
	return s.cfg != nil && s.cfg.Enabled
}

// apiKey returns the API key (env var OPSMINI_AI_KEY has the highest priority, then settings, then config).
func (s *AIService) apiKey() string {
	if k := os.Getenv("OPSMINI_AI_KEY"); k != "" {
		return k
	}
	if k := s.setting("ai_api_key"); k != "" {
		return k
	}
	return s.cfg.APIKey
}

// model returns the model name (settings first).
func (s *AIService) model() string {
	if m := s.setting("ai_model"); m != "" {
		return m
	}
	return s.cfg.Model
}

// baseURL returns the API endpoint (settings first).
func (s *AIService) baseURL() string {
	if b := s.setting("ai_base_url"); b != "" {
		return b
	}
	return s.cfg.BaseURL
}

// builtinTools are the built-in MCP tool definitions (SkillHub search/install).
var builtinTools = []Tool{
	{Type: "function", Function: Function{
		Name:        "search_skills",
		Description: "在 SkillHub 技能市场搜索技能。当用户想查找、发现或推荐技能时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keyword": map[string]any{"type": "string", "description": "搜索关键词，如 pdf、周报、excel"},
			},
			"required": []string{"keyword"},
		},
	}},
	{Type: "function", Function: Function{
		Name:        "install_skill",
		Description: "安装一个技能（从 SkillHub 下载并解压）。当用户明确要求安装某个技能时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug": map[string]any{"type": "string", "description": "技能的 slug 标识"},
			},
			"required": []string{"slug"},
		},
	}},
}

// callLLM makes a single LLM call and returns the parsed response.
func (s *AIService) callLLM(messages []ChatMessage, tools []Tool) (*chatCompletionResp, error) {
	body, err := json.Marshal(chatCompletionReq{Model: s.model(), Messages: messages, Stream: false, Tools: tools})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(s.baseURL(), "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey())

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // limit 4MB
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var parsed chatCompletionResp
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("LLM error: %s", parsed.Error.Message)
	}
	return &parsed, nil
}

// Chat sends a conversation and returns the model's reply (supports function calling tool calls).
func (s *AIService) Chat(messages []ChatMessage, deepThink bool) (string, error) {
	if !s.enabled() {
		return "", errors.New("AI assistant is disabled")
	}
	if s.apiKey() == "" {
		return "", errors.New("AI API key not configured (set OPSMINI_AI_KEY or ai.api_key)")
	}

	// inject system prompt + host system info collected in real time by built-in MCP tools
	msgs := append([]ChatMessage{{Role: "system", Content: s.buildSystemPrompt(deepThink)}}, messages...)

	// multi-round tool call loop (at most 3 rounds)
	for round := 0; round < 3; round++ {
		resp, err := s.callLLM(msgs, builtinTools)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", errors.New("LLM returned empty response")
		}
		msg := resp.Choices[0].Message

		// no tool calls -> return directly
		if len(msg.ToolCalls) == 0 {
			return strings.TrimSpace(msg.Content), nil
		}

		// execute tool calls and append the results
		msgs = append(msgs, ChatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})
		for _, tc := range msg.ToolCalls {
			result, terr := s.callTool(tc.Function.Name, tc.Function.Arguments)
			if terr != nil {
				result = "工具调用失败：" + terr.Error()
			}
			msgs = append(msgs, ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: result})
		}
		// continue to the next round, letting the LLM generate a final answer based on tool results
	}

	// exceeded rounds: ask once more without tools
	resp, err := s.callLLM(msgs, nil)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) > 0 {
		return strings.TrimSpace(resp.Choices[0].Message.Content), nil
	}
	return "", errors.New("LLM returned empty response")
}

// buildSystemPrompt assembles the system prompt (with optional deep-think prompt + system info collected by built-in tools).
func (s *AIService) buildSystemPrompt(deepThink bool) string {
	prompt := systemPrompt
	if deepThink {
		prompt += "\n\n" + deepThinkPrompt
	}
	return prompt + "\n\n" + s.systemContext()
}

// ChatStream is the streaming conversation: it calls the LLM stream API and invokes onChunk per chunk.
// Streaming mode does not enable function calling (tools are not passed); ordinary Q&A and system info
// queries can be answered from the context injected by buildSystemPrompt; tool calls such as
// search/install skills still go through the non-streaming Chat.
func (s *AIService) ChatStream(ctx context.Context, messages []ChatMessage, deepThink bool, onChunk func(string)) error {
	if !s.enabled() {
		return errors.New("AI assistant is disabled")
	}
	if s.apiKey() == "" {
		return errors.New("AI API key not configured (set OPSMINI_AI_KEY or ai.api_key)")
	}

	msgs := append([]ChatMessage{{Role: "system", Content: s.buildSystemPrompt(deepThink)}}, messages...)

	body, err := json.Marshal(chatCompletionReq{Model: s.model(), Messages: msgs, Stream: true})
	if err != nil {
		return err
	}
	url := strings.TrimRight(s.baseURL(), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey())
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("LLM API %d: %s", resp.StatusCode, truncate(string(data), 300))
	}

	// parse SSE line by line (data: {...}), extracting delta.content.
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 {
			continue
		}
		if string(data) == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onChunk(chunk.Choices[0].Delta.Content)
		}
	}
	return nil
}

// callTool executes a built-in MCP tool.
func (s *AIService) callTool(name, args string) (string, error) {
	switch name {
	case "search_skills":
		var p struct {
			Keyword string `json:"keyword"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", err
		}
		if s.skill == nil {
			return "", errors.New("技能服务未初始化")
		}
		list, err := s.skill.Search(p.Keyword, "", "score", 1, 5)
		if err != nil {
			return "", err
		}
		if len(list) == 0 {
			return "未找到相关技能。", nil
		}
		var b strings.Builder
		b.WriteString("搜索到以下技能：\n")
		for _, sk := range list {
			fmt.Fprintf(&b, "- %s（slug: %s，下载 %d，收藏 %d）\n  %s\n", sk.Name, sk.Slug, sk.Downloads, sk.Stars, truncate(sk.Description, 100))
		}
		return b.String(), nil
	case "install_skill":
		var p struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return "", err
		}
		if s.skill == nil {
			return "", errors.New("技能服务未初始化")
		}
		if err := s.skill.Install(p.Slug); err != nil {
			return "", err
		}
		return "技能 " + p.Slug + " 安装成功。", nil
	default:
		return "", fmt.Errorf("未知工具 %s", name)
	}
}

// systemContext is a built-in MCP tool: it collects host system info in real time and organizes it into text
// for context injection, so the AI assistant can directly answer system questions about host users,
// disk/CPU/memory, firewall, ports, etc.
func (s *AIService) systemContext() string {
	if s.sys == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("以下是当前主机实时系统信息（由内置工具采集，回答系统相关问题应以这些数据为准）：\n")

	sum, err := s.sys.MonitorSummary()
	if err != nil {
		return ""
	}

	fmt.Fprintf(&b, "【系统概览】主机名 %s；系统 %s %s；CPU %.1f%%（%d 核）；内存 %.1f%%（已用 %s / 共 %s）；磁盘 %.1f%%（已用 %s / 共 %s）；负载 %.2f / %.2f / %.2f；已运行 %s。\n",
		sum.Hostname, sum.OS, sum.KernelArch, sum.CPUPercent, sum.CPUCores,
		sum.MemPercent, aiFmtBytes(sum.MemUsed), aiFmtBytes(sum.MemTotal),
		sum.DiskPercent, aiFmtBytes(sum.DiskUsed), aiFmtBytes(sum.DiskTotal),
		sum.Load1, sum.Load5, sum.Load15, aiFmtUptime(sum.Uptime))

	fmt.Fprintf(&b, "【统计】进程总数 %d（僵尸 %d）；主机用户 %d（可登录 %d）；监听端口 %d（TCP %d，UDP %d）。\n",
		sum.ProcessCount, sum.ZombieCount, sum.UserCount, sum.LoginUserCount,
		sum.PortCount, sum.TcpPortCount, sum.UdpPortCount)

	// listening port list
	if ports, err := s.sys.Ports(); err == nil && len(ports) > 0 {
		b.WriteString("【监听端口列表】")
		for i, p := range ports {
			if i > 0 {
				b.WriteString("、")
			}
			fmt.Fprintf(&b, "%d/%s", p.Port, p.Protocol)
			if i >= 39 {
				fmt.Fprintf(&b, " 等（共 %d 个）", len(ports))
				break
			}
		}
		b.WriteString("\n")
	}

	// disk partitions
	if disks, err := s.sys.Disks(); err == nil && len(disks) > 0 {
		b.WriteString("【磁盘分区】")
		for i, d := range disks {
			if i > 0 {
				b.WriteString("；")
			}
			fmt.Fprintf(&b, "%s（挂载 %s，已用 %.1f%%）", d.Device, d.Mountpoint, d.UsedPercent)
			if i >= 4 {
				break
			}
		}
		b.WriteString("\n")
	}

	// firewall
	if s.firewall != nil {
		fw := s.firewall.Status()
		enabled := "停用"
		if fw.Enabled {
			enabled = "启用"
		}
		fmt.Fprintf(&b, "【防火墙】后端 %s，状态 %s，规则 %d 条。\n", fw.Backend, enabled, len(fw.Rules))
	}

	// integrated MCP services
	if s.mcp != nil {
		if list, err := s.mcp.List(); err == nil && len(list) > 0 {
			b.WriteString("【已集成 MCP 服务】")
			for i, m := range list {
				if i > 0 {
					b.WriteString("；")
				}
				state := "启用"
				if !m.Enabled {
					state = "停用"
				}
				endpoint := m.Command
				if endpoint == "" {
					endpoint = m.URL
				}
				fmt.Fprintf(&b, "%s（%s，%s）", m.Name, m.Transport, state)
				if endpoint != "" {
					fmt.Fprintf(&b, "[%s]", truncate(endpoint, 40))
				}
			}
			b.WriteString("\n")
		} else {
			b.WriteString("【已集成 MCP 服务】无\n")
		}
	}

	// installed skills
	if s.skill != nil {
		list := s.skill.List()
		if len(list) > 0 {
			b.WriteString("【已安装 Skill】")
			for i, sk := range list {
				if i > 0 {
					b.WriteString("；")
				}
				state := "启用"
				if !sk.Enabled {
					state = "停用"
				}
				fmt.Fprintf(&b, "%s（%s）", sk.Name, state)
			}
			b.WriteString("\n")
		} else {
			b.WriteString("【已安装 Skill】无\n")
		}
	}

	return b.String()
}

// aiFmtBytes formats a byte count into a readable unit.
func aiFmtBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// aiFmtUptime formats seconds into a readable uptime.
func aiFmtUptime(sec uint64) string {
	d := sec / 86400
	h := sec % 86400 / 3600
	m := sec % 3600 / 60
	if d > 0 {
		return fmt.Sprintf("%d 天 %d 小时", d, h)
	}
	if h > 0 {
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	}
	return fmt.Sprintf("%d 分钟", m)
}
