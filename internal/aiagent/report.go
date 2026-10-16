package aiagent

import (
	"fmt"
	"github.com/opsmini/opsmini/internal/model"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ReportMeta describes the task context of an AI report.
type ReportMeta struct {
	// TaskName is the scheduled task name (empty for an ad-hoc session export).
	TaskName string
	// Prompt is the instruction the agent was given.
	Prompt string
	// Status is "success" / "failed".
	Status string
	// Steps / Tools count reasoning steps and executed tool calls.
	Steps int
	Tools int
	// Skipped lists tool calls refused because they need human approval.
	Skipped []string
	// Duration is the wall-clock run time.
	Duration time.Duration
	// Unattended marks a scheduled run (documented in the header).
	Unattended bool
}

// BuildReport renders a session transcript as a Markdown report: a header with
// the run summary, the conclusion, and an appendix listing every tool call so the
// reader can audit how the conclusion was reached.
func (s *Store) BuildReport(sessionID string, meta ReportMeta) (string, error) {
	sess, err := s.repo.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	msgs, err := s.repo.ListMessages(sessionID, 0)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	title := meta.TaskName
	if title == "" {
		title = sess.Title
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "> 生成时间：%s ｜ 会话：`%s`\n", time.Now().Format("2006-01-02 15:04:05"), sess.ID)
	if meta.Unattended {
		b.WriteString("> 执行方式：**无人值守的定时任务**（需人工确认的操作会被自动拒绝）\n")
	}
	b.WriteString("\n")

	// --- summary ---
	b.WriteString("## 执行摘要\n\n")
	b.WriteString("| 项目 | 值 |\n|---|---|\n")
	status := meta.Status
	if status == "" {
		status = "success"
	}
	statusText := "成功"
	if status != "success" {
		statusText = "失败"
	}
	fmt.Fprintf(&b, "| 状态 | %s |\n", statusText)
	fmt.Fprintf(&b, "| 推理步数 | %d |\n", meta.Steps)
	fmt.Fprintf(&b, "| 工具调用 | %d |\n", meta.Tools)
	if meta.Duration > 0 {
		fmt.Fprintf(&b, "| 耗时 | %s |\n", meta.Duration.Round(time.Second))
	}
	if len(meta.Skipped) > 0 {
		fmt.Fprintf(&b, "| 因无人值守被拒绝 | %s |\n", strings.Join(meta.Skipped, "、"))
	}
	b.WriteString("\n")

	if strings.TrimSpace(meta.Prompt) != "" {
		b.WriteString("## 任务指令\n\n```\n")
		b.WriteString(strings.TrimSpace(meta.Prompt))
		b.WriteString("\n```\n\n")
	}

	// --- conclusion: the final assistant message ---
	var conclusion string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && strings.TrimSpace(msgs[i].Content) != "" {
			conclusion = msgs[i].Content
			break
		}
	}
	b.WriteString("## 结论\n\n")
	if strings.TrimSpace(conclusion) == "" {
		b.WriteString("_本次执行没有产生结论内容。_\n\n")
	} else {
		b.WriteString(strings.TrimSpace(conclusion))
		b.WriteString("\n\n")
	}

	// --- appendix: tool calls in order ---
	// One row per tool call: arguments and observation side by side, so the
	// reader can audit exactly how the conclusion was reached.
	type callRow struct {
		name   string
		args   string
		result string
		status string
	}
	var order []string
	rows := map[string]*callRow{}
	classify := func(outcome string) string {
		switch {
		case outcome == "":
			return "未返回"
		case strings.HasPrefix(outcome, "无人值守") || strings.Contains(outcome, "已拒绝执行"):
			return "被拒绝"
		case strings.Contains(outcome, "硬禁止"):
			return "硬禁止"
		case strings.Contains(outcome, "失败"):
			return "失败"
		default:
			return "完成"
		}
	}
	for _, m := range msgs {
		switch m.Role {
		case model.AIRoleAssistant:
			// ToolCalls is stored as JSON, so go through the conversion helper.
			for _, c := range fromModel(m).ToolCalls {
				if _, ok := rows[c.ID]; ok {
					continue
				}
				rows[c.ID] = &callRow{name: c.Function.Name, args: c.Function.Arguments, status: "未返回"}
				order = append(order, c.ID)
			}
		case model.AIRoleTool:
			r, ok := rows[m.ToolCallID]
			if !ok {
				r = &callRow{name: m.Name, status: "未返回"}
				rows[m.ToolCallID] = r
				order = append(order, m.ToolCallID)
			}
			r.result = m.Content
			r.status = classify(strings.TrimSpace(m.Content))
		}
	}

	if len(order) > 0 {
		b.WriteString("## 附：执行过程\n\n")
		fmt.Fprintf(&b, "共 %d 次工具调用。\n\n", len(order))
		b.WriteString("| # | 工具 | 参数 | 观察结果 | 状态 |\n|---|---|---|---|---|\n")
		var lines []string
		for i, id := range order {
			r := rows[id]
			lines = append(lines, fmt.Sprintf("| %d | `%s` | %s | %s | %s |",
				i+1, r.name, shortArgs(r.args), escapeCell(truncateText(strings.TrimSpace(r.result), 200)), r.status))
		}
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
	}

	return b.String(), nil
}

// shortArgs renders tool arguments compactly for the appendix table.
func shortArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return "-"
	}
	return escapeCell(truncateText(raw, 120))
}

// escapeCell makes a value safe inside a Markdown table cell.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// --- archive on disk ---

// ReportFile is one archived report on disk.
type ReportFile struct {
	Name    string `json:"name"` // path relative to the reports root
	Dir     string `json:"dir"`  // task folder (reports/<slug>)
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"` // unix seconds
}

// reportsRoot returns the reports directory under the panel data directory.
func reportsRoot(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "/var/lib/opsmini/data"
	}
	return filepath.Join(dataDir, "reports")
}

// slugRe matches characters that are unsafe in a directory or file name.
var slugRe = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// slugify turns a task name into a path-safe folder/file fragment.
func slugify(s string) string {
	out := slugRe.ReplaceAllString(strings.TrimSpace(s), "-")
	out = strings.Trim(out, "-.")
	if out == "" {
		out = "task"
	}
	return truncateText(out, 60)
}

// SaveReport archives a report as Markdown under
// <dataDir>/reports/<task-slug>/<task-slug>-<timestamp>.md and returns the path.
func SaveReport(dataDir, taskName, content string) (string, error) {
	slug := slugify(taskName)
	dir := filepath.Join(reportsRoot(dataDir), slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.md", slug, time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ListReports returns the archived reports, newest first.
func ListReports(dataDir string) ([]ReportFile, error) {
	root := reportsRoot(dataDir)
	out := []ReportFile{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		out = append(out, ReportFile{
			Name:    rel,
			Dir:     slugify(filepath.Dir(rel)),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return out, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// ReadReport reads an archived report by its relative name. The path is validated
// so a crafted name cannot escape the reports directory.
func ReadReport(dataDir, name string) ([]byte, string, error) {
	root := reportsRoot(dataDir)
	clean := filepath.Clean(strings.TrimPrefix(name, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, "", fmt.Errorf("报告路径非法：%s", name)
	}
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, "", fmt.Errorf("报告路径非法：%s", name)
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return nil, "", fmt.Errorf("只允许下载 Markdown 报告")
	}
	data, rerr := os.ReadFile(full)
	if rerr != nil {
		return nil, "", rerr
	}
	return data, filepath.Base(rel), nil
}
