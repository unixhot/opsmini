package backup

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "site")
	writeFile(t, filepath.Join(src, "index.html"), "<h1>v1</h1>")
	writeFile(t, filepath.Join(src, "conf", "nginx.conf"), "server {}")

	m := NewManager(filepath.Join(base, "backups"), "1.4.0")
	man, err := m.Run(Job{Name: "web", Objects: []Object{{Kind: "dir", Path: src, Name: "site"}}})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if len(man.Objects) != 1 || man.Objects[0].SHA256 == "" {
		t.Fatalf("manifest incomplete: %+v", man)
	}

	// Change the data, then restore and expect the old content back.
	writeFile(t, filepath.Join(src, "index.html"), "<h1>v2-broken</h1>")
	writeFile(t, filepath.Join(src, "extra.txt"), "should disappear")

	dir, err := m.SnapshotDir(man.Name)
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.Restore(RestoreOpts{SnapshotDir: dir, Confirm: man.Name})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(res.Restored) != 1 {
		t.Fatalf("restored = %v", res.Restored)
	}
	body, _ := os.ReadFile(filepath.Join(src, "index.html"))
	if string(body) != "<h1>v1</h1>" {
		t.Fatalf("content not restored: %q", body)
	}
	if _, err := os.Stat(filepath.Join(src, "extra.txt")); err == nil {
		t.Fatalf("restore should replace the directory, not merge into it")
	}
	// A pre-restore snapshot must exist so a bad restore is undoable.
	if res.SnapshotBackup == "" {
		t.Fatalf("missing pre-restore snapshot")
	}
	if _, err := os.Stat(res.SnapshotBackup); err != nil {
		t.Fatalf("pre-restore snapshot not on disk: %v", err)
	}
}

func TestRestoreRequiresConfirmation(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "site")
	writeFile(t, filepath.Join(src, "a.txt"), "x")
	m := NewManager(filepath.Join(base, "backups"), "test")
	man, err := m.Run(Job{Name: "web", Objects: []Object{{Kind: "dir", Path: src}}})
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := m.SnapshotDir(man.Name)
	if _, err := m.Restore(RestoreOpts{SnapshotDir: dir, Confirm: "wrong-name"}); err == nil {
		t.Fatalf("mismatched confirmation must be rejected")
	}
}

func TestDryRunDoesNotTouchData(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "site")
	writeFile(t, filepath.Join(src, "a.txt"), "v1")
	m := NewManager(filepath.Join(base, "backups"), "test")
	man, _ := m.Run(Job{Name: "web", Objects: []Object{{Kind: "dir", Path: src}}})
	dir, _ := m.SnapshotDir(man.Name)

	writeFile(t, filepath.Join(src, "a.txt"), "v2")
	res, err := m.Restore(RestoreOpts{SnapshotDir: dir, Confirm: man.Name, DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(res.Preflight) == 0 {
		t.Fatalf("dry run should report preflight checks")
	}
	body, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(body) != "v2" {
		t.Fatalf("dry run modified data: %q", body)
	}
}

func TestRetentionKeepsNewest(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "site")
	writeFile(t, filepath.Join(src, "a.txt"), "x")
	m := NewManager(filepath.Join(base, "backups"), "test")
	for i := 0; i < 4; i++ {
		if _, err := m.Run(Job{Name: "web", Objects: []Object{{Kind: "dir", Path: src}}, Retention: 2}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	list, _ := m.List("")
	if len(list) != 2 {
		t.Fatalf("retention kept %d snapshots, want 2", len(list))
	}
}

func TestBackupFailsCleanlyOnMissingObject(t *testing.T) {
	base := t.TempDir()
	m := NewManager(filepath.Join(base, "backups"), "test")
	_, err := m.Run(Job{Name: "bad", Objects: []Object{{Kind: "dir", Path: filepath.Join(base, "nope")}}})
	if err == nil {
		t.Fatalf("missing source must fail")
	}
	// No half-written snapshot may be left behind.
	if list, _ := m.List(""); len(list) != 0 {
		t.Fatalf("partial snapshot left on disk: %v", list)
	}
}

func TestSnapshotDirRejectsTraversal(t *testing.T) {
	m := NewManager(t.TempDir(), "test")
	for _, name := range []string{"../etc", "a/b", "..", ""} {
		if _, err := m.SnapshotDir(name); err == nil {
			t.Fatalf("name %q should be rejected", name)
		}
	}
}

func TestArchiveRejectsTraversalEntries(t *testing.T) {
	base := t.TempDir()
	archive := filepath.Join(base, "evil.tar.gz")
	// A tar with ../../etc/passwd must not escape the destination.
	if err := writeEvilArchive(archive); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(base, "out")
	_ = os.MkdirAll(dst, 0o755)
	if err := extractTarGz(archive, dst); err == nil {
		t.Fatalf("traversal archive must be rejected")
	}
	if _, err := os.Stat(filepath.Join(base, "etc", "passwd")); err == nil {
		t.Fatalf("archive escaped the destination")
	}
	_ = strings.TrimSpace("")
}

// writeEvilArchive builds a tar.gz containing a "../" entry.
func writeEvilArchive(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("pwned")
	h := &tar.Header{Name: "../../etc/passwd", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	if _, err := tw.Write(body); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}
