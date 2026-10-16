package aiagent

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Minimal Markdown → HTML renderer for reports.
//
// Report text is partly derived from the managed host (log excerpts, process
// command lines, configuration values) and from an LLM that reads them. Raw HTML
// is therefore NEVER passed through: every line is HTML-escaped first, and only
// then are the inline rules applied. A crafted log line such as
// "<script>alert(1)</script>" ends up as visible text, not markup.
//
// The supported subset is exactly what the report builder emits plus what models
// typically produce: h1-h6, paragraphs, GFM tables, fenced code, ordered and
// unordered lists, blockquotes, horizontal rules, and the inline forms
// `code`, **bold**, *italic*, ~~strike~~ and [text](url).
// ---------------------------------------------------------------------------

const (
	mdBoldOpen   = "**"
	mdStrikeOpen = "~~"
)

// escHTML escapes the five characters that matter for HTML text and attributes.
func escHTML(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// isWordByte reports whether c can be part of a word (used to decide whether an
// emphasis delimiter is really opening or closing).
func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') || c >= 0x80 // any multi-byte UTF-8 byte
}

// safeHref allows only http/https/mailto links; anything else (javascript:, data:)
// is rendered as plain text.
func safeHref(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	if l == "" || strings.HasPrefix(l, "//") {
		return false
	}
	for _, p := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// renderInline applies the inline rules to an already-escaped string.
func renderInline(s string) string { return renderInlineDepth(s, 0) }

func renderInlineDepth(s string, depth int) string {
	if s == "" {
		return ""
	}
	// Guard against pathological nesting; emphasis inside emphasis is rare in
	// reports and the cap keeps a hostile input from blowing the stack.
	if depth > 4 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 24)
	for i := 0; i < len(s); {
		switch {
		case s[i] == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j > 0 {
				b.WriteString("<code>")
				b.WriteString(s[i+1 : i+1+j])
				b.WriteString("</code>")
				i += j + 2
				continue
			}

		case strings.HasPrefix(s[i:], mdBoldOpen):
			if end := strings.Index(s[i+2:], mdBoldOpen); end > 0 {
				b.WriteString("<strong>")
				b.WriteString(renderInlineDepth(s[i+2:i+2+end], depth+1))
				b.WriteString("</strong>")
				i += end + 4
				continue
			}

		case strings.HasPrefix(s[i:], mdStrikeOpen):
			if end := strings.Index(s[i+2:], mdStrikeOpen); end > 0 {
				b.WriteString("<del>")
				b.WriteString(s[i+2 : i+2+end])
				b.WriteString("</del>")
				i += end + 4
				continue
			}

		case s[i] == '*':
			if end, ok := italicClose(s, i); ok {
				b.WriteString("<em>")
				b.WriteString(renderInlineDepth(s[i+1:end], depth+1))
				b.WriteString("</em>")
				i = end + 1
				continue
			}

		case s[i] == '[':
			if text, href, n, ok := parseLink(s, i); ok {
				if safeHref(href) {
					b.WriteString(`<a href="`)
					b.WriteString(href)
					b.WriteString(`" target="_blank" rel="noopener noreferrer">`)
					b.WriteString(text)
					b.WriteString("</a>")
				} else {
					// Unsafe or relative target: keep the label, drop the link.
					b.WriteString(text)
				}
				i += n
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// italicClose returns the index of the `*` closing an italic run that starts at
// open. It requires the opener to follow a boundary and the closer to be followed
// by one, so "2 * 3 * 4" and "a*b*c" stay literal text.
func italicClose(s string, open int) (int, bool) {
	if open > 0 && isWordByte(s[open-1]) {
		return 0, false
	}
	if open+1 >= len(s) || s[open+1] == ' ' || s[open+1] == '*' {
		return 0, false
	}
	for j := open + 1; j < len(s); j++ {
		if s[j] != '*' {
			continue
		}
		if s[j-1] == ' ' {
			continue
		}
		if j+1 < len(s) && isWordByte(s[j+1]) {
			continue
		}
		return j, true
	}
	return 0, false
}

// parseLink matches "[label](href)" starting at open and returns the label, the
// href and how many bytes were consumed.
func parseLink(s string, open int) (label, href string, n int, ok bool) {
	closeBracket := strings.IndexByte(s[open:], ']')
	if closeBracket < 1 {
		return "", "", 0, false
	}
	closeBracket += open
	if closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return "", "", 0, false
	}
	// Scan the destination with parentheses balanced, so a URL such as
	// https://host/wiki/Foo_(bar) is not truncated at the first ")".
	depth := 0
	for j := closeBracket + 2; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return s[open+1 : closeBracket], s[closeBracket+2 : j], j + 1 - open, true
			}
			depth--
		}
	}
	return "", "", 0, false
}

// splitTableRow splits a GFM table row on unescaped pipes.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '|' {
			cur.WriteByte('|')
			i++
			continue
		}
		if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(line[i])
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells
}

// isTableSeparator reports whether the line is a GFM header separator row.
func isTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "-") {
		return false
	}
	if !strings.Contains(line, "|") {
		return false
	}
	for _, c := range splitTableRow(line) {
		c = strings.TrimSpace(c)
		c = strings.TrimLeft(c, ":")
		c = strings.TrimRight(c, ":")
		if c == "" || strings.Trim(c, "-") != "" || len(c) < 2 {
			return false
		}
	}
	return true
}

// startsTable reports whether line i begins a GFM table.
func startsTable(lines []string, i int) bool {
	if !strings.Contains(lines[i], "|") {
		return false
	}
	return i+1 < len(lines) && isTableSeparator(lines[i+1])
}

// renderMarkdown converts the report subset described at the top of this file to
// HTML. The result is a fragment: no <html>/<body> wrapper.
func renderMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	md = strings.ReplaceAll(md, "\x00", "")
	lines := strings.Split(md, "\n")

	var b strings.Builder
	// pending accumulates consecutive plain lines into one paragraph so soft
	// wraps inside a paragraph do not turn into <br>.
	var pending []string

	flushParagraph := func() {
		if len(pending) == 0 {
			return
		}
		b.WriteString("<p>")
		b.WriteString(renderInline(escHTML(strings.Join(pending, " "))))
		b.WriteString("</p>\n")
		pending = nil
	}

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		// fenced code block
		if fence := fenceMarker(trimmed); fence != "" {
			flushParagraph()
			body := []string{}
			i++
			for ; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
					break
				}
				body = append(body, lines[i])
			}
			b.WriteString("<pre><code>")
			b.WriteString(escHTML(strings.Join(body, "\n")))
			b.WriteString("</code></pre>\n")
			continue
		}

		if trimmed == "" {
			flushParagraph()
			continue
		}

		if lvl, text, ok := heading(trimmed); ok {
			flushParagraph()
			b.WriteString("<h")
			b.WriteByte(byte('0' + lvl))
			b.WriteString(">")
			b.WriteString(renderInline(escHTML(text)))
			b.WriteString("</h")
			b.WriteByte(byte('0' + lvl))
			b.WriteString(">\n")
			continue
		}

		if isHR(trimmed) {
			flushParagraph()
			b.WriteString("<hr>\n")
			continue
		}

		if startsTable(lines, i) {
			flushParagraph()
			header := splitTableRow(lines[i])
			i += 2 // skip the separator row
			var rows [][]string
			for ; i < len(lines); i++ {
				if !strings.Contains(lines[i], "|") || strings.TrimSpace(lines[i]) == "" {
					break
				}
				rows = append(rows, splitTableRow(lines[i]))
			}
			i--
			b.WriteString("<table>\n<thead><tr>")
			for _, c := range header {
				b.WriteString("<th>")
				b.WriteString(renderInline(escHTML(c)))
				b.WriteString("</th>")
			}
			b.WriteString("</tr></thead>\n<tbody>\n")
			for _, r := range rows {
				b.WriteString("<tr>")
				for ci := 0; ci < len(header); ci++ {
					cell := ""
					if ci < len(r) {
						cell = r[ci]
					}
					b.WriteString("<td>")
					b.WriteString(renderInline(escHTML(cell)))
					b.WriteString("</td>")
				}
				b.WriteString("</tr>\n")
			}
			b.WriteString("</tbody></table>\n")
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			flushParagraph()
			var quoted []string
			for ; i < len(lines); i++ {
				t := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(t, ">") {
					i--
					break
				}
				quoted = append(quoted, strings.TrimSpace(strings.TrimPrefix(t, ">")))
			}
			b.WriteString("<blockquote><p>")
			b.WriteString(renderInline(escHTML(strings.Join(quoted, " "))))
			b.WriteString("</p></blockquote>\n")
			continue
		}

		if items, ok := listItems(lines, i, false); ok {
			flushParagraph()
			b.WriteString("<ul>\n")
			for _, it := range items {
				b.WriteString("<li>")
				b.WriteString(renderInline(escHTML(it)))
				b.WriteString("</li>\n")
			}
			b.WriteString("</ul>\n")
			i += len(items) - 1
			continue
		}

		if items, ok := listItems(lines, i, true); ok {
			flushParagraph()
			b.WriteString("<ol>\n")
			for _, it := range items {
				b.WriteString("<li>")
				b.WriteString(renderInline(escHTML(it)))
				b.WriteString("</li>\n")
			}
			b.WriteString("</ol>\n")
			i += len(items) - 1
			continue
		}

		pending = append(pending, trimmed)
	}
	flushParagraph()
	return b.String()
}

// fenceMarker returns the fence token (``` or ~~~) if the line opens a code fence.
func fenceMarker(trimmed string) string {
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, f) {
			return f
		}
	}
	return ""
}

// heading parses an ATX heading, returning its level (1-6) and text.
func heading(trimmed string) (int, string, bool) {
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	lvl := 0
	for lvl < len(trimmed) && trimmed[lvl] == '#' {
		lvl++
	}
	if lvl > 6 || lvl >= len(trimmed) || trimmed[lvl] != ' ' {
		return 0, "", false
	}
	return lvl, strings.TrimSpace(trimmed[lvl:]), true
}

// isHR reports whether the line is a thematic break.
func isHR(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	marker := trimmed[0]
	if marker != '-' && marker != '*' && marker != '_' {
		return false
	}
	return strings.Trim(trimmed, string(marker)) == ""
}

// listItems collects a run of list items starting at index i.
func listItems(lines []string, i int, ordered bool) ([]string, bool) {
	var out []string
	for ; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		text, ok := matchListItem(t, ordered)
		if !ok {
			break
		}
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// matchListItem recognises "- x", "* x", "+ x" (unordered) or "1. x" / "1) x"
// (ordered) and returns the item text.
func matchListItem(t string, ordered bool) (string, bool) {
	if len(t) < 2 {
		return "", false
	}
	if ordered {
		n := 0
		for n < len(t) && t[n] >= '0' && t[n] <= '9' {
			n++
		}
		if n == 0 || n+1 >= len(t) {
			return "", false
		}
		if (t[n] != '.' && t[n] != ')') || t[n+1] != ' ' {
			return "", false
		}
		return strings.TrimSpace(t[n+2:]), true
	}
	if t[0] != '-' && t[0] != '*' && t[0] != '+' {
		return "", false
	}
	if t[1] != ' ' {
		return "", false
	}
	return strings.TrimSpace(t[2:]), true
}
