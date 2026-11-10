package drill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeOp records what happened, and can be told to fail on purpose.
type fakeOp struct {
	kind     Kind
	injected int
	restored int
	failOn   string // "inject" | "restore" | ""
	mu       sync.Mutex
}

func (f *fakeOp) Kind() Kind { return f.kind }

func (f *fakeOp) Inject(ctx context.Context, r *Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "inject" {
		return errors.New("boom")
	}
	f.injected++
	r.Note = "fake"
	r.Files = append(r.Files, "/tmp/drill-fake-marker")
	return nil
}

func (f *fakeOp) Restore(ctx context.Context, r *Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn == "restore" {
		return errors.New("restore boom")
	}
	f.restored++
	return nil
}

func (f *fakeOp) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.injected, f.restored
}

func newFake(k Kind) *fakeOp { return &fakeOp{kind: k} }

// Guard 级演练（磁盘、docker）影响共享资源，没显式确认就不能开跑。
// 一个误点不该把生产机的 docker 停掉。
func TestGuardNeedsConfirm(t *testing.T) {
	op := newFake(KindDiskFill)
	m := New("", op)
	if _, err := m.Start(context.Background(), KindDiskFill, false); err == nil {
		t.Fatal("guard drill started without confirmation")
	}
	if inj, _ := op.counts(); inj != 0 {
		t.Fatalf("inject ran despite missing confirmation: %d", inj)
	}
	if _, err := m.Start(context.Background(), KindDiskFill, true); err != nil {
		t.Fatalf("confirmed start failed: %v", err)
	}
}

func TestUnknownKind(t *testing.T) {
	m := New("")
	if _, err := m.Start(context.Background(), Kind("nope"), true); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

// 同时跑两个演练，信号就分不清是谁引起的，观察价值归零。
func TestOnlyOneDrillAtATime(t *testing.T) {
	a, b := newFake(KindCPUHog), newFake(KindLogFlood)
	m := New("", a, b)
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), KindLogFlood, true); err == nil {
		t.Fatal("second concurrent drill accepted")
	}
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), KindLogFlood, true); err != nil {
		t.Fatalf("start after restore failed: %v", err)
	}
}

func TestDoubleStartSameKind(t *testing.T) {
	op := newFake(KindCPUHog)
	m := New("", op)
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), KindCPUHog, true); err == nil {
		t.Fatal("double start accepted")
	}
}

// 停止一个没在跑的演练必须是无害的：前端可能在自动恢复之后才点到按钮。
func TestStopIsIdempotent(t *testing.T) {
	op := newFake(KindCPUHog)
	m := New("", op)
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatalf("stop on idle drill errored: %v", err)
	}
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatalf("second stop errored: %v", err)
	}
}

// 到时间必须自己恢复：演练留在机器上就是故障。
func TestAutoRestoreOnTTL(t *testing.T) {
	op := newFake(KindLogFlood)
	m := New("", op)
	m.ttl = 60 * time.Millisecond
	if _, err := m.Start(context.Background(), KindLogFlood, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, restored := op.counts(); restored > 0 {
			if st := m.List(); activeCount(st) != 0 {
				t.Fatalf("still marked active after auto-restore: %+v", st)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("drill was not auto-restored before TTL+3s")
}

// Inject 失败必须回滚：半注入的故障留在机器上比不演练更糟。
func TestInjectFailureRollsBack(t *testing.T) {
	op := newFake(KindCPUHog)
	op.failOn = "inject"
	m := New("", op)
	if _, err := m.Start(context.Background(), KindCPUHog, true); err == nil {
		t.Fatal("expected inject error")
	}
	if activeCount(m.List()) != 0 {
		t.Fatal("failed drill still marked active")
	}
	if _, restored := op.counts(); restored != 1 {
		t.Fatalf("rollback restore not attempted: restored=%d", restored)
	}
	st := m.List()
	for _, s := range st {
		if s.Kind == KindCPUHog && s.Error == "" {
			t.Fatal("failure reason not surfaced to the UI")
		}
	}
}

// 恢复失败要能重试：报错但保留 active，人还能再点一次。
func TestRestoreFailureKeepsActive(t *testing.T) {
	op := newFake(KindCPUHog)
	m := New("", op)
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	op.failOn = "restore"
	if _, err := m.Stop(context.Background(), KindCPUHog); err == nil {
		t.Fatal("expected restore error")
	}
	if activeCount(m.List()) != 1 {
		t.Fatal("drill dropped from active despite failed restore")
	}
	op.failOn = ""
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatalf("retry restore failed: %v", err)
	}
	if activeCount(m.List()) != 0 {
		t.Fatal("still active after successful retry")
	}
}

// 面板崩溃或重启后，上次没来得及恢复的故障必须被清掉。
func TestReconcileCleansLeftovers(t *testing.T) {
	dir := t.TempDir()
	op := newFake(KindCPUHog)
	m := New(dir, op)
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drill-state.json")); err != nil {
		t.Fatalf("state file not written: %v", err)
	}

	// 模拟重启：新的 Manager 读同一个状态目录。
	op2 := newFake(KindCPUHog)
	m2 := New(dir, op2)
	m2.Reconcile(context.Background())
	if _, restored := op2.counts(); restored != 1 {
		t.Fatalf("leftover drill not restored on startup: %d", restored)
	}
	if _, err := os.Stat(filepath.Join(dir, "drill-state.json")); !os.IsNotExist(err) {
		t.Fatalf("state file should be removed after cleanup, got err=%v", err)
	}
}

// 恢复成功后状态文件要清空，否则下次启动会去清理一个早就不存在的东西。
func TestStateFileClearedAfterStop(t *testing.T) {
	dir := t.TempDir()
	m := New(dir, newFake(KindCPUHog))
	if _, err := m.Start(context.Background(), KindCPUHog, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(context.Background(), KindCPUHog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drill-state.json")); !os.IsNotExist(err) {
		t.Fatalf("state file left behind: %v", err)
	}
}

func activeCount(list []Status) int {
	n := 0
	for _, s := range list {
		if s.Active {
			n++
		}
	}
	return n
}
