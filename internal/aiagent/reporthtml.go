package aiagent

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Single-file HTML report.
//
// The output is fully self-contained: styles are inlined, there is no script, no
// external stylesheet, no web font and no image. A report can therefore be
// opened straight from the data directory, attached to a mail or copied to a USB
// stick and it still renders. Because the panel ships no bundled CJK font, the
// stylesheet relies on the reader's own system fonts, which every desktop and
// mobile browser has — this keeps the binary small and the report light.
// ---------------------------------------------------------------------------

// reportCSS is the stylesheet inlined into every report. It is intentionally
// small and print-aware so Ctrl+P produces a usable PDF.
const reportCSS = `
*,*::before,*::after{box-sizing:border-box}
body{margin:0;padding:32px 20px 64px;background:#f6f7f9;color:#1f2430;
 font:15px/1.75 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"PingFang SC","Hiragino Sans GB","Microsoft YaHei","Noto Sans CJK SC","Source Han Sans SC",sans-serif;
 -webkit-text-size-adjust:100%}
.wrap{max-width:880px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:12px;
 padding:40px 44px 48px}
.head{border-bottom:2px solid #2563eb;padding-bottom:20px;margin-bottom:28px}
.brand{display:flex;align-items:center;gap:8px;font-size:12px;font-weight:600;color:#2563eb;
 letter-spacing:.06em;text-transform:uppercase;margin-bottom:10px}
.brand i{width:8px;height:8px;border-radius:2px;background:#2563eb;display:inline-block}
h1{font-size:24px;line-height:1.35;margin:0 0 14px;font-weight:600;color:#111827}
/* Heading levels inside the report body. The document's own title is the h1 in
   .head; a model-written "# ..." inside the body is a section, not a page title. */
.body h1{font-size:19px;margin:32px 0 12px;padding-bottom:8px;border-bottom:1px solid #e5e7eb}
.body>*:first-child{margin-top:0}
.metas{display:flex;flex-wrap:wrap;gap:8px}
.meta{font-size:12.5px;color:#4b5563;background:#f3f4f6;border:1px solid #e5e7eb;
 border-radius:6px;padding:3px 10px}
.meta.ok{color:#166534;background:#f0fdf4;border-color:#bbf7d0}
.meta.bad{color:#991b1b;background:#fef2f2;border-color:#fecaca}
.meta.warn{color:#92400e;background:#fffbeb;border-color:#fde68a}
.meta code{font-size:12px;color:#4b5563}
h2{font-size:18px;margin:34px 0 12px;padding-left:11px;border-left:3px solid #2563eb;
 font-weight:600;color:#111827}
h3{font-size:15.5px;margin:24px 0 8px;font-weight:600;color:#1f2937}
h4,h5,h6{font-size:14px;margin:18px 0 6px;font-weight:600;color:#374151}
p{margin:0 0 12px}
ul,ol{margin:0 0 14px;padding-left:24px}
li{margin:4px 0}
li>ul,li>ol{margin:4px 0 4px}
strong{font-weight:600;color:#111827}
a{color:#2563eb;text-decoration:none;border-bottom:1px solid #bfdbfe}
a:hover{border-bottom-color:#2563eb}
code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace;
 font-size:12.5px;background:#f3f4f6;border:1px solid #e5e7eb;border-radius:4px;padding:1px 5px;
 color:#b91c1c;word-break:break-word}
pre{margin:0 0 16px;padding:14px 16px;background:#f8fafc;border:1px solid #e5e7eb;
 border-radius:8px;overflow-x:auto}
pre code{background:none;border:0;padding:0;color:#1f2937;font-size:12.5px;line-height:1.65;
 white-space:pre-wrap;overflow-wrap:anywhere}
blockquote{margin:0 0 16px;padding:10px 16px;background:#f8fafc;border-left:3px solid #cbd5e1;
 color:#4b5563;font-size:13.5px}
blockquote p{margin:0}
table{width:100%;border-collapse:collapse;margin:0 0 18px;font-size:13px;display:block;overflow-x:auto}
table thead th{background:#f3f4f6;text-align:left;font-weight:600;color:#374151;
 border:1px solid #e5e7eb;padding:8px 10px;white-space:nowrap}
table td{border:1px solid #e5e7eb;padding:8px 10px;vertical-align:top}
table tbody tr:nth-child(even){background:#fafbfc}
/* Keep the narrow leading columns (index / tool name, item / value) from being
   squeezed to one character per line by a verbose last column. */
table th:first-child,table td:first-child{min-width:38px}
table th:nth-child(2),table td:nth-child(2){min-width:152px}
hr{border:0;border-top:1px solid #e5e7eb;margin:26px 0}
del{color:#9ca3af}
.foot{margin-top:36px;padding-top:16px;border-top:1px solid #e5e7eb;font-size:12px;color:#9ca3af;
 display:flex;justify-content:space-between;flex-wrap:wrap;gap:8px}
@media (max-width:720px){
 body{padding:12px 8px 40px}
 .wrap{padding:24px 18px 32px;border-radius:8px}
 h1{font-size:20px}
}
@media print{
 body{background:#fff;padding:0;font-size:11.5pt}
 .wrap{max-width:none;border:0;border-radius:0;padding:0}
 .head{border-bottom-width:1px}
 h2{page-break-after:avoid}
 h3{page-break-after:avoid}
 pre,table,blockquote{page-break-inside:avoid}
 a{color:#111827;border-bottom:0}
 .foot{color:#6b7280}
}
`

// cjkRe is used to pick a sensible document language for the report.
func hasCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF) {
			return true
		}
	}
	return false
}

// htmlMeta is one headline chip rendered under the report title.
type htmlMeta struct {
	Text  string
	Class string // "", "ok", "bad", "warn"
}

// RenderReportHTML wraps report Markdown in a self-contained HTML document.
//
// title becomes both the page title and the visible heading; body is the report
// Markdown (already containing its own H1, which is dropped to avoid repeating
// the title).
func RenderReportHTML(title, sessionID string, body string, metas []htmlMeta) string {
	body = strings.TrimSpace(body)
	// The builder starts the document with "# <title>"; drop that first heading
	// so the styled header is the only place the title appears.
	if idx := strings.IndexByte(body, '\n'); idx > 0 && strings.HasPrefix(body, "# ") {
		body = strings.TrimLeft(body[idx+1:], "\n")
	}

	lang := "en"
	if hasCJK(title + body) {
		lang = "zh-CN"
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"")
	b.WriteString(lang)
	b.WriteString("\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	b.WriteString("<meta name=\"generator\" content=\"OpsMini\">\n")
	b.WriteString("<title>")
	b.WriteString(escHTML(title))
	b.WriteString("</title>\n<style>")
	b.WriteString(reportCSS)
	b.WriteString("</style>\n</head>\n<body>\n<div class=\"wrap\">\n<div class=\"head\">\n")
	b.WriteString("<div class=\"brand\"><i></i>OpsMini 报告</div>\n<h1>")
	b.WriteString(escHTML(title))
	b.WriteString("</h1>\n<div class=\"metas\">")
	for _, m := range metas {
		b.WriteString("<span class=\"meta")
		if m.Class != "" {
			b.WriteByte(' ')
			b.WriteString(m.Class)
		}
		b.WriteString("\">")
		b.WriteString(m.Text)
		b.WriteString("</span>")
	}
	if sessionID != "" {
		b.WriteString("<span class=\"meta\">会话 <code>")
		b.WriteString(escHTML(sessionID))
		b.WriteString("</code></span>")
	}
	b.WriteString("</div>\n</div>\n")
	b.WriteString("<div class=\"body\">\n")
	b.WriteString(renderMarkdown(body))
	b.WriteString("</div>\n")
	b.WriteString("<div class=\"foot\"><span>本报告由 OpsMini AI 智能体自动生成</span><span>")
	b.WriteString(escHTML(time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString("</span></div>\n</div>\n</body>\n</html>\n")
	return b.String()
}

// BuildReportHTML renders a session transcript straight to a single-file HTML
// document (used for ad-hoc conversations exported from the AI assistant).
func (s *Store) BuildReportHTML(sessionID string, meta ReportMeta) (string, error) {
	md, err := s.BuildReport(sessionID, meta)
	if err != nil {
		return "", err
	}
	return RenderReportHTML(reportTitle(sessionID, meta, md), sessionID, md, reportMetaChips(meta)), nil
}

// reportTitle prefers the task name, then the first heading of the report body.
func reportTitle(sessionID string, meta ReportMeta, md string) string {
	if strings.TrimSpace(meta.TaskName) != "" {
		return strings.TrimSpace(meta.TaskName)
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return "OpsMini 报告 " + sessionID
}

// reportMetaChips renders the run summary as header chips.
func reportMetaChips(meta ReportMeta) []htmlMeta {
	out := []htmlMeta{}
	status := meta.Status
	if status == "" {
		status = "success"
	}
	if status == "success" {
		out = append(out, htmlMeta{Text: "执行成功", Class: "ok"})
	} else {
		out = append(out, htmlMeta{Text: "执行失败", Class: "bad"})
	}
	if meta.Unattended {
		out = append(out, htmlMeta{Text: "无人值守定时任务", Class: "warn"})
	}
	if meta.Steps > 0 {
		out = append(out, htmlMeta{Text: fmt.Sprintf("推理步数 %d", meta.Steps)})
	}
	if meta.Tools > 0 {
		out = append(out, htmlMeta{Text: fmt.Sprintf("工具调用 %d", meta.Tools)})
	}
	if meta.Duration > 0 {
		out = append(out, htmlMeta{Text: "耗时 " + meta.Duration.Round(time.Second).String()})
	}
	if len(meta.Skipped) > 0 {
		out = append(out, htmlMeta{Text: "被拒绝 " + strings.Join(meta.Skipped, "、"), Class: "warn"})
	}
	if t := strings.TrimSpace(meta.GeneratedAt); t != "" {
		out = append(out, htmlMeta{Text: "生成于 " + t})
	}
	return out
}
