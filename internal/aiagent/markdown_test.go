package aiagent

import (
	"strings"
	"testing"
)

// TestRenderMarkdownEscapesHostData is the security-critical test: report text is
// partly taken from the managed host (log excerpts, process command lines), so a
// crafted line must never become markup.
func TestRenderMarkdownEscapesHostData(t *testing.T) {
	md := strings.Join([]string{
		"# 巡检结论",
		"",
		"日志里出现了 <script>alert('xss')</script> 这条记录。",
		"",
		"进程名 <img src=x onerror=alert(1)> 看起来可疑。",
		"",
		"配置里写着 a=\"b\" 与 c='d'。",
		"",
		"[点我](javascript:alert(1)) 与 [正常](https://opsmini.com/docs) 的对比。",
		"",
		"```",
		"grep '<host>' /etc/hosts && echo \"</script>\"",
		"```",
	}, "\n")

	out := renderMarkdown(md)

	// The raw tag must never appear. "onerror=" legitimately appears inside the
	// escaped text (&lt;img ... onerror=... &gt;), which is exactly the point:
	// it is inert there.
	for _, forbidden := range []string{"<script", "</script", "<img", "javascript:alert"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("host-supplied text leaked into markup as %q\n%s", forbidden, out)
		}
	}
	// The dangerous text must still be *visible* to the reader, properly escaped.
	for _, want := range []string{
		"&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		"&lt;img src=x onerror=alert(1)&gt;",
		"a=&quot;b&quot;",
		"c=&#39;d&#39;",
		"&lt;/script&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("escaped text %q is missing from the rendering", want)
		}
	}
	// The safe link survives, the unsafe one degrades to plain text.
	if !strings.Contains(out, `<a href="https://opsmini.com/docs"`) {
		t.Error("a safe https link must be rendered as a link")
	}
	if !strings.Contains(out, "点我") {
		t.Error("the label of an unsafe link must still be shown")
	}
	if strings.Contains(out, "点我)") {
		t.Errorf("the unsafe link must be consumed whole, no stray bracket\n%s", out)
	}
}

// TestRenderMarkdownBlocks checks the block-level subset the report builder uses.
func TestRenderMarkdownBlocks(t *testing.T) {
	md := strings.Join([]string{
		"# 标题一",
		"## 标题二",
		"",
		"| 项目 | 值 |",
		"|---|---|",
		"| 状态 | 成功 |",
		"| 磁盘 | 95% \\| 已用 |",
		"",
		"- 第一项",
		"- 第二项",
		"",
		"1. 第一步",
		"2. 第二步",
		"",
		"> 无人值守的定时任务",
		"",
		"```bash",
		"df -h",
		"```",
		"",
		"普通段落 **加粗** 与 `代码` 混排。",
		"",
		"---",
	}, "\n")

	out := renderMarkdown(md)

	for _, want := range []string{
		"<h1>标题一</h1>",
		"<h2>标题二</h2>",
		"<th>项目</th>",
		"<td>成功</td>",
		// An escaped pipe must survive as a literal pipe inside the cell.
		"<td>95% | 已用</td>",
		"<ul>",
		"<li>第一项</li>",
		"<ol>",
		"<li>第一步</li>",
		"<blockquote>",
		"<pre><code>df -h</code></pre>",
		"<strong>加粗</strong>",
		"<code>代码</code>",
		"<hr>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendering is missing %q\n%s", want, out)
		}
	}
	// The table must have exactly the header's column count, and a trailing pipe in
	// the source must not create a phantom column.
	if n := strings.Count(out, "<td>"); n != 4 {
		t.Errorf("want 4 body cells (2 rows x 2 cols), got %d\n%s", n, out)
	}
}

// TestRenderMarkdownEdgeCases covers inputs that must not panic or mis-detect.
func TestRenderMarkdownEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"   ":             "",
		"a*b*c":           "<p>a*b*c</p>\n",           // no emphasis without boundaries
		"2 * 3 * 4":       "<p>2 * 3 * 4</p>\n",       // arithmetic stays literal
		"snake_case_name": "<p>snake_case_name</p>\n", // underscores are not emphasis
		"#nospace":        "<p>#nospace</p>\n",        // not a heading (needs a space)
		"**":              "<p>**</p>\n",              // unterminated bold stays text
		"`unclosed":       "<p>`unclosed</p>\n",       // unterminated code stays text
		"-not-a-list":     "<p>-not-a-list</p>\n",     // needs a space after the marker
		"a\r\nb":          "<p>a b</p>\n",             // CRLF is normalised
		"<":               "<p>&lt;</p>\n",            // bare angle bracket is escaped
		"~~del~~":         "<p><del>del</del></p>\n",
		"*斜体*":            "<p><em>斜体</em></p>\n",
		"**`x`**":         "<p><strong><code>x</code></strong></p>\n",
		// A link whose URL contains brackets must be consumed whole.
		"[a](https://h/wiki/Foo_(bar))": "<p><a href=\"https://h/wiki/Foo_(bar)\" target=\"_blank\" rel=\"noopener noreferrer\">a</a></p>\n",
		// An empty heading collapses to its marker.
		"# ": "<p>#</p>\n",
	}
	for in, want := range cases {
		if got := renderMarkdown(in); got != want {
			t.Errorf("renderMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
	// A very long line must not blow up.
	if out := renderMarkdown(strings.Repeat("巡", 20000)); !strings.Contains(out, "巡") {
		t.Error("long input was dropped")
	}
}

// TestRenderReportHTMLStructure checks the standalone document wrapper.
func TestRenderReportHTMLStructure(t *testing.T) {
	body := "# 每日主机巡检 · 2026-10-16\n\n生成时间占位\n\n## 结论\n\n一切正常。"
	html := RenderReportHTML("每日主机巡检 · 2026-10-16", "sess9", body, []htmlMeta{
		{Text: "执行成功", Class: "ok"},
		{Text: "无人值守定时任务", Class: "warn"},
	})

	// The builder's own H1 is dropped so the styled header is not duplicated.
	if strings.Count(html, "每日主机巡检 · 2026-10-16") != 2 { // <title> + <h1>
		t.Errorf("title should appear exactly twice (title tag + heading)\n%s", html)
	}
	for _, want := range []string{
		`<html lang="zh-CN">`,
		"<title>每日主机巡检 · 2026-10-16</title>",
		`class="meta ok">执行成功`,
		`class="meta warn">无人值守定时任务`,
		"sess9",
		"<h2>结论</h2>",
		"@media print",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML is missing %q", want)
		}
	}
	assertSelfContained(t, html)

	// An English-only report should not claim to be Chinese.
	en := RenderReportHTML("Daily check", "s1", "All clear.", nil)
	if !strings.Contains(en, `<html lang="en">`) {
		t.Error("an English report must be tagged lang=en")
	}
}
