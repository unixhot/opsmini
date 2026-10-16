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
	// GeneratedAt is the human-readable generation time shown in the HTML header
	// (the Markdown body carries its own timestamp line).
	GeneratedAt string
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
	generatedAt := time.Now().Format("2006-01-02 15:04:05")
	if meta.GeneratedAt == "" {
		meta.GeneratedAt = generatedAt
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "> 生成时间：%s ｜ 会话：`%s`\n", generatedAt, sess.ID)
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
		b.WriteString("*本次执行没有产生结论内容。*\n\n")
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
	Name    string `json:"name"`   // path relative to the reports root
	Dir     string `json:"dir"`    // task folder (reports/<slug>)
	Format  string `json:"format"` // "html" or "md"
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

// ReportsDir exposes the absolute directory holding generated reports, so the
// panel can show users where the files actually land.
func ReportsDir(dataDir string) string { return reportsRoot(dataDir) }

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

// ReportArtifact describes where a generated report was archived.
//
// Files always live under <dataDir>/reports/<task-slug>/, i.e. inside the panel
// data directory, so every kind of generated data has one predictable home.
type ReportArtifact struct {
	Dir      string `json:"dir"`      // absolute directory holding the files
	Markdown string `json:"markdown"` // absolute path of the .md file
	HTML     string `json:"html"`     // absolute path of the .html file
	RelHTML  string `json:"rel_html"` // .html path relative to the reports root (for URLs)
	RelMD    string `json:"rel_md"`   // .md path relative to the reports root
	Bytes    int64  `json:"bytes"`    // size of the HTML file
}

// ArchiveReport writes a report to <dataDir>/reports/<task-slug>/ in two forms:
// a self-contained .html file (the artifact users open or download) and a .md
// file (machine readable, and the input for re-rendering if the template changes).
//
// markdown is the report body as produced by BuildReport.
func ArchiveReport(dataDir, taskName, sessionID, markdown string, meta ReportMeta) (ReportArtifact, error) {
	slug := slugify(taskName)
	root := reportsRoot(dataDir)
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ReportArtifact{}, err
	}
	stamp := time.Now().Format("20060102-150405")
	mdName := fmt.Sprintf("%s-%s.md", slug, stamp)
	htmlName := fmt.Sprintf("%s-%s.html", slug, stamp)

	html := RenderReportHTML(reportTitle(sessionID, meta, markdown), sessionID, markdown, reportMetaChips(meta))

	artifact := ReportArtifact{
		Dir:      dir,
		Markdown: filepath.Join(dir, mdName),
		HTML:     filepath.Join(dir, htmlName),
		RelMD:    filepath.Join(slug, mdName),
		RelHTML:  filepath.Join(slug, htmlName),
	}
	if err := os.WriteFile(artifact.Markdown, []byte(markdown), 0o644); err != nil {
		return ReportArtifact{}, err
	}
	if err := os.WriteFile(artifact.HTML, []byte(html), 0o644); err != nil {
		return ReportArtifact{}, err
	}
	if fi, err := os.Stat(artifact.HTML); err == nil {
		artifact.Bytes = fi.Size()
	}
	return artifact, nil
}

// ListReports returns the archived reports, newest first.
func ListReports(dataDir string) ([]ReportFile, error) {
	root := reportsRoot(dataDir)
	out := []ReportFile{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		format, ok := reportFormat(d.Name())
		if !ok {
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
			Format:  format,
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

// reportFormat classifies an archived report by extension.
func reportFormat(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "html", true
	case ".md", ".markdown":
		return "md", true
	default:
		return "", false
	}
}

// ContentType returns the MIME type to serve an archived report with.
func ContentType(format string) string {
	if format == "html" {
		return "text/html; charset=utf-8"
	}
	return "text/markdown; charset=utf-8"
}

// ReadReport reads an archived report by its relative name, returning the bytes,
// the base file name and the format ("html" or "md").
//
// The path is resolved and re-validated against the reports root so a crafted
// name cannot escape it (path traversal).
func ReadReport(dataDir, name string) ([]byte, string, string, error) {
	root := reportsRoot(dataDir)
	clean := filepath.Clean(strings.TrimPrefix(name, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, "", "", fmt.Errorf("报告路径非法：%s", name)
	}
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, "", "", fmt.Errorf("报告路径非法：%s", name)
	}
	format, ok := reportFormat(rel)
	if !ok {
		return nil, "", "", fmt.Errorf("不支持的报告格式：%s", name)
	}
	data, rerr := os.ReadFile(full)
	if rerr != nil {
		return nil, "", "", rerr
	}
	return data, filepath.Base(rel), format, nil
}
