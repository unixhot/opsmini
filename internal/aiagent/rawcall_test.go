package aiagent

import "testing"

// A DeepSeek-style reply that carries the call as markup must be recognised, so
// the loop asks for a proper call instead of returning the markup as the answer.
func TestLooksLikeRawToolCall(t *testing.T) {
	raw := "<｜｜DSML｜｜ calls>\n<｜｜DSML｜｜ invoke name=\"read_file\">\n</｜｜DSML｜｜ calls>"
	if !looksLikeRawToolCall(raw) {
		t.Fatalf("DSML block not detected")
	}
	if !looksLikeRawToolCall("<antml:invoke name=\"read_file\">") {
		t.Fatalf("antml block not detected")
	}
	if !looksLikeRawToolCall("<function_calls><invoke name=\"x\"></function_calls>") {
		t.Fatalf("function_calls block not detected")
	}

	// A normal answer that merely mentions a tool name must not trip it.
	plain := "结论：日志由 app-worker 写入，建议配置 logrotate。已经在 21:35 截断过一次。"
	if looksLikeRawToolCall(plain) {
		t.Fatalf("plain answer misdetected as raw tool call")
	}
	if looksLikeRawToolCall("") {
		t.Fatalf("empty string misdetected")
	}
}

// Stripping keeps whatever prose the model wrote and drops the markup lines,
// including the bare tag-only lines that carry no information.
func TestStripRawToolCalls(t *testing.T) {
	in := "结论：文件已被截断，进程仍在写。\n<｜｜DSML｜｜ calls>\n<｜｜DSML｜｜ invoke name=\"read_file\">\n</｜｜DSML｜｜ calls>"
	got := stripRawToolCalls(in)
	if got != "结论：文件已被截断，进程仍在写。" {
		t.Fatalf("unexpected strip result: %q", got)
	}

	// Nothing but markup: the caller turns the empty string into an explicit
	// "no conclusion" message rather than showing a wall of tags.
	if got := stripRawToolCalls("<｜｜DSML｜｜ calls>\n</｜｜DSML｜｜ calls>"); got != "" {
		t.Fatalf("markup-only should strip to empty, got %q", got)
	}
}
