package aiagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// newTestStore opens an in-memory SQLite database and returns a Store on top of it.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AISession{}, &model.AIMessage{}, &model.AIPendingApproval{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(repository.NewAIRepo(db), nil)
}

// seedSession writes a small transcript: one executed tool, one refused tool, and
// a final answer.
func seedSession(t *testing.T, st *Store) string {
	t.Helper()
	sess, err := st.EnsureSession("", SystemUserID, "每日巡检 · 2026-10-16")
	if err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	mustAppend := func(msgs ...Message) {
		if err := st.Append(sess.ID, msgs...); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	mustAppend(Message{Role: model.AIRoleUser, Content: "对主机做一次巡检"})

	var ok ToolCall
	ok.ID = "call_1"
	ok.Type = "function"
	ok.Function.Name = "sys_overview"
	ok.Function.Arguments = `{"detail":true}`
	var refused ToolCall
	refused.ID = "call_2"
	refused.Type = "function"
	refused.Function.Name = "exec_command"
	refused.Function.Arguments = `{"command":"df -h"}`

	mustAppend(Message{Role: model.AIRoleAssistant, ToolCalls: []ToolCall{ok, refused}})
	mustAppend(toolMessage("call_1", "sys_overview", "主机名: node1\n磁盘 95% 已用"))
	mustAppend(toolMessage("call_2", "exec_command", "无人值守的定时任务：工具 exec_command 属于「需人工确认」的操作，已被安全策略拒绝执行。"))
	mustAppend(Message{Role: model.AIRoleAssistant, Content: "结论：磁盘使用率 95%，需要尽快清理。"})
	return sess.ID
}

// TestBuildReport checks the report structure: header, summary, conclusion and the
// audit appendix with one row per tool call.
func TestBuildReport(t *testing.T) {
	st := newTestStore(t)
	id := seedSession(t, st)

	md, err := st.BuildReport(id, ReportMeta{
		TaskName:   "每日巡检",
		Prompt:     "对主机做一次巡检",
		Steps:      2,
		Tools:      1,
		Skipped:    []string{"exec_command"},
		Unattended: true,
	})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}

	for _, want := range []string{
		"# 每日巡检",
		"无人值守的定时任务",
		"| 状态 | 成功 |",
		"| 推理步数 | 2 |",
		"| 因无人值守被拒绝 | exec_command |",
		"## 任务指令",
		"对主机做一次巡检",
		"## 结论",
		"结论：磁盘使用率 95%，需要尽快清理。",
		"## 附：执行过程",
		"共 2 次工具调用",
		"`sys_overview`",
		"`exec_command`",
		"被拒绝",
		"完成",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	// The refused call must be flagged, and the conclusion must come from the last
	// assistant message rather than from an intermediate one.
	if strings.Contains(md, "已发起") {
		t.Error("each tool call must appear once, not as separate发起/完成 rows")
	}
}

// TestBuildReportFailureStatus verifies the status cell for a failed run.
func TestBuildReportFailureStatus(t *testing.T) {
	st := newTestStore(t)
	id := seedSession(t, st)
	md, err := st.BuildReport(id, ReportMeta{TaskName: "巡检", Status: "failed"})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if !strings.Contains(md, "| 状态 | 失败 |") {
		t.Error("a failed run must be reported as failed")
	}
}

// TestSaveListReadReport covers the on-disk archive lifecycle.
func TestSaveListReadReport(t *testing.T) {
	dir := t.TempDir()
	path, err := SaveReport(dir, "每日巡检", "# 报告正文")
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("unexpected path: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("report file missing: %v", err)
	}
	// The folder is derived from the task name.
	if filepath.Base(filepath.Dir(path)) != "每日巡检" {
		t.Errorf("unexpected report folder: %s", filepath.Dir(path))
	}

	files, err := ListReports(dir)
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 report, got %d", len(files))
	}
	if files[0].Dir != "每日巡检" {
		t.Errorf("unexpected dir field: %s", files[0].Dir)
	}

	data, base, err := ReadReport(dir, files[0].Name)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if string(data) != "# 报告正文" {
		t.Errorf("unexpected content: %q", string(data))
	}
	if !strings.HasSuffix(base, ".md") {
		t.Errorf("unexpected base name: %s", base)
	}
}

// TestReadReportRejectsPaths verifies that a crafted name cannot escape the
// reports directory or read non-Markdown files.
func TestReadReportRejectsPaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"../../../etc/passwd",
		"..%2f..%2fetc/passwd",
		"/etc/passwd",
		"sub/../../secret.md",
	} {
		if _, _, err := ReadReport(dir, name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
	// A Markdown file outside the reports root must not be reachable either.
	outside := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadReport(dir, "notes.txt"); err == nil {
		t.Error("only Markdown reports may be downloaded")
	}
}

// TestListReportsMissingDir verifies an empty (not yet created) reports folder is
// not an error.
func TestListReportsMissingDir(t *testing.T) {
	files, err := ListReports(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("want no reports, got %d", len(files))
	}
}

// TestSystemSessionsVisible verifies that scheduler-owned sessions (user 0) are
// readable by every user, while other users' sessions stay private.
func TestSystemSessionsVisible(t *testing.T) {
	st := newTestStore(t)
	sysID := seedSession(t, st) // created with SystemUserID

	if _, err := st.GetSession(sysID, 7); err != nil {
		t.Errorf("system session must be readable by any user: %v", err)
	}
	other, err := st.EnsureSession("", 42, "别人的会话")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSession(other.ID, 7); err == nil {
		t.Error("another user's session must not be readable")
	}

	list, err := st.ListSessions(7, 50)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	found := false
	for _, s := range list {
		if s.ID == sysID {
			found = true
		}
		if s.UserID == 42 {
			t.Error("another user's session leaked into the list")
		}
	}
	if !found {
		t.Error("system session is missing from the list")
	}
}

// TestSlugify verifies that task names become safe path fragments.
func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"每日巡检":        "每日巡检",
		"daily check": "daily-check",
		"a/b:c*d?e":   "a-b-c-d-e",
		"..":          "task",
		"   ":         "task",
		"nested/path": "nested-path",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
