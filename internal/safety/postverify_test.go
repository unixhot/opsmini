package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// 工具说"已隔离"不算数：文件还在原地时必须判失败，否则事件会被错误地标成
// resolved，而主机上什么都没发生。
func TestVerifyQuarantineStillPresent(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	if err := m.DB.AutoMigrate(&model.ActionLog{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "shell.php")
	if err := os.WriteFile(victim, []byte("<?php eval($_POST[1]);"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.DB.Create(&model.ActionLog{
		IncidentID: "inc1", Tool: "quarantine_file", Risk: "low_write", Verdict: "allow",
		Args: `{"path":"` + victim + `"}`, CreatedAt: time.Now(),
	})
	ok, checks := m.VerifyActions(t.Context(), "inc1")
	if ok {
		t.Fatalf("file still on disk must fail verification: %+v", checks)
	}
	if len(checks) != 1 || checks[0].OK || checks[0].Skipped {
		t.Fatalf("unexpected check: %+v", checks)
	}
}

// 文件确实被移走并且隔离区有记录 → 通过。
func TestVerifyQuarantineMoved(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	if err := m.DB.AutoMigrate(&model.ActionLog{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "gone.php")
	m.DB.Create(&model.ActionLog{
		IncidentID: "inc2", Tool: "quarantine_file", Risk: "low_write", Verdict: "allow",
		Args: `{"path":"` + victim + `"}`, CreatedAt: time.Now(),
	})
	m.DB.Create(&model.QuarantineItem{OriginalPath: victim, StoredPath: dir + "/store", CreatedAt: time.Now()})
	ok, checks := m.VerifyActions(t.Context(), "inc2")
	if !ok {
		t.Fatalf("expected pass: %+v", checks)
	}
}

// 只是"文件不见了"但隔离区没记录 —— 可能是别的进程删掉的，不能算处置成功。
func TestVerifyQuarantineNoRecord(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	if err := m.DB.AutoMigrate(&model.ActionLog{}); err != nil {
		t.Fatal(err)
	}
	m.DB.Create(&model.ActionLog{
		IncidentID: "inc3", Tool: "quarantine_file", Risk: "low_write", Verdict: "allow",
		Args: `{"path":"/tmp/definitely-not-here-xyz.php"}`, CreatedAt: time.Now(),
	})
	ok, checks := m.VerifyActions(t.Context(), "inc3")
	if ok || len(checks) != 1 || checks[0].OK {
		t.Fatalf("missing quarantine record must fail: %+v", checks)
	}
}

// 没执行过写动作（只是分析）时不产生校验项，也不该被拖成 unverified。
func TestVerifyNoWrites(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	if err := m.DB.AutoMigrate(&model.ActionLog{}); err != nil {
		t.Fatal(err)
	}
	m.DB.Create(&model.ActionLog{IncidentID: "inc4", Tool: "read_file", Risk: "readonly", Verdict: "allow"})
	ok, checks := m.VerifyActions(t.Context(), "inc4")
	if !ok || len(checks) != 0 {
		t.Fatalf("read-only run must have nothing to verify: ok=%v checks=%+v", ok, checks)
	}
}

// 看不到的（进程已退出、工具没有默认规则）算"跳过"，不能当成失败，
// 否则会把"我们不知道"说成"处置失败"。
func TestVerifyUnknownIsSkipNotFail(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	if err := m.DB.AutoMigrate(&model.ActionLog{}); err != nil {
		t.Fatal(err)
	}
	m.DB.Create(&model.ActionLog{
		IncidentID: "inc5", Tool: "snapshot_path", Risk: "low_write", Verdict: "allow",
		Args: `{"path":"/etc"}`, CreatedAt: time.Now(),
	})
	ok, checks := m.VerifyActions(t.Context(), "inc5")
	if !ok || len(checks) != 1 || !checks[0].Skipped {
		t.Fatalf("unsupported tool should skip, not fail: ok=%v %+v", ok, checks)
	}
}
