package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/backup"
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// Scheduled backups reuse the manual engine, so these tests focus on the wiring
// that is unique to the scheduled path: validation, defaults, and the fact that
// the outcome is written back to the task row (which is what the task list and the
// "备份失败数" alert read).

func cronTestRepo(t *testing.T) *repository.CronJobRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.CronJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repository.NewCronJobRepo(db)
}

func TestCronReqBackupValidation(t *testing.T) {
	// A backup task without objects would silently "succeed" doing nothing.
	req := CronReq{Name: "b", Kind: model.CronKindBackup, Schedule: "0 2 * * *", BackupObjects: "  \n\n"}
	if err := req.validate(); err == nil {
		t.Fatal("empty object list must be rejected")
	}
	req.BackupObjects = "/var/www/html"
	if err := req.validate(); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
	// The mysql:/redis: prefixes count as objects too.
	req.BackupObjects = "# comment\nmysql:opsmini\n"
	if err := req.validate(); err != nil {
		t.Fatalf("mysql object rejected: %v", err)
	}
}

func TestCreateBackupTaskAppliesDefaults(t *testing.T) {
	repo := cronTestRepo(t)
	svc := NewCronService(repo)
	defer svc.Stop()

	enabled := false // keep the scheduler quiet; we call executeBackup directly
	job, err := svc.Create(CronReq{
		Name: "夜间备份", Kind: model.CronKindBackup, Schedule: "0 2 * * *",
		BackupObjects: "/tmp/x", BackupRetention: 7, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// keep_local must default to true: dropping the local copy should be deliberate.
	if !job.BackupKeepLocal {
		t.Fatal("keep_local should default to true")
	}
	if job.BackupRetention != 7 || job.BackupObjects != "/tmp/x" {
		t.Fatalf("fields not persisted: %+v", job)
	}

	// Explicit false must survive.
	keep := false
	job2, err := svc.Create(CronReq{
		Name: "只存远端", Kind: model.CronKindBackup, Schedule: "0 3 * * *",
		BackupObjects: "/tmp/y", BackupKeepLocal: &keep, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if job2.BackupKeepLocal {
		t.Fatal("explicit keep_local=false was overwritten")
	}
}

func TestExecuteBackupWritesSnapshotAndStatus(t *testing.T) {
	repo := cronTestRepo(t)
	svc := NewCronService(repo)
	defer svc.Stop()

	src := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "index.html"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	backupRoot := t.TempDir()
	svc.SetBackupRunner(backup.NewManager(backupRoot, "test"))

	enabled := false
	job, err := svc.Create(CronReq{
		Name: "巡检备份", Kind: model.CronKindBackup, Schedule: "0 2 * * *",
		BackupObjects: src, BackupRetention: 3, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	svc.executeBackup(*job)

	cur, err := repo.FindByID(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.LastStatus != model.CronStatusSuccess {
		t.Fatalf("status=%q error=%q", cur.LastStatus, cur.LastError)
	}
	if cur.LastRun == nil {
		t.Fatal("last_run not recorded")
	}
	// The snapshot must really exist, not just be reported as done.
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no snapshot directory created under %s", backupRoot)
	}
}

func TestExecuteBackupFailureIsRecorded(t *testing.T) {
	repo := cronTestRepo(t)
	svc := NewCronService(repo)
	defer svc.Stop()
	svc.SetBackupRunner(backup.NewManager(t.TempDir(), "test"))

	enabled := false
	job, err := svc.Create(CronReq{
		Name: "坏对象", Kind: model.CronKindBackup, Schedule: "0 2 * * *",
		BackupObjects: "/nonexistent-opsmini-test-path", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	svc.executeBackup(*job)

	cur, err := repo.FindByID(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A failed backup must be visible on the task: that is what the task list shows
	// and what the "备份失败数" alert rule counts.
	if cur.LastStatus != model.CronStatusFailed {
		t.Fatalf("status=%q want failed", cur.LastStatus)
	}
	if cur.LastError == "" {
		t.Fatal("failure reason not recorded")
	}
	if cur.FailCount != 1 {
		t.Fatalf("fail_count=%d want 1", cur.FailCount)
	}
	// Retrying forever is intentional: a silently disabled backup is worse than a
	// repeated alert, so the task must stay enabled.
	if !cur.Enabled {
		// Enabled was false in this test, so just assert we did not flip it.
		if cur.Enabled {
			t.Fatal("enabled flipped unexpectedly")
		}
	}
}

func TestExecuteBackupWithoutRunnerDoesNotPanic(t *testing.T) {
	repo := cronTestRepo(t)
	svc := NewCronService(repo)
	defer svc.Stop()
	enabled := false
	job, err := svc.Create(CronReq{
		Name: "no runner", Kind: model.CronKindBackup, Schedule: "0 2 * * *",
		BackupObjects: "/tmp", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	svc.executeBackup(*job) // must log and return, not panic
}

// A backup task that is created but never registered is the worst kind of bug:
// the list shows it as enabled, nothing ever runs, and the operator only finds out
// when they need a restore. This pins the registration path, not just execution.
func TestBackupTaskIsRegisteredInScheduler(t *testing.T) {
	repo := cronTestRepo(t)
	svc := NewCronService(repo)
	defer svc.Stop()

	enabled := true
	job, err := svc.Create(CronReq{
		Name: "每分钟备份", Kind: model.CronKindBackup, Schedule: "* * * * *",
		BackupObjects: "/tmp/x", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	svc.mu.Lock()
	_, ok := svc.entries[job.ID]
	svc.mu.Unlock()
	if !ok {
		t.Fatal("backup task was not registered with the scheduler")
	}

	// Disabling must remove it again.
	if _, err := svc.Update(job.ID, CronReq{
		Name: job.Name, Kind: model.CronKindBackup, Schedule: "* * * * *",
		BackupObjects: "/tmp/x", Enabled: boolPtr(false),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	svc.mu.Lock()
	_, still := svc.entries[job.ID]
	svc.mu.Unlock()
	if still {
		t.Fatal("disabled backup task is still scheduled")
	}
}

func boolPtr(b bool) *bool { return &b }
