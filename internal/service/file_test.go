package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestFileServiceResolveEscape(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	// accessing an absolute path outside root should be rejected
	if _, err := svc.List("/etc"); err != ErrPathEscape {
		t.Fatalf("List(/etc): expected ErrPathEscape, got %v", err)
	}
	// relative path traversal should be rejected
	if _, err := svc.List("../../etc"); err != ErrPathEscape {
		t.Fatalf("List(../../etc): expected ErrPathEscape, got %v", err)
	}
	// the root directory itself should be accessible
	if _, err := svc.List(""); err != nil {
		t.Fatalf("List(root): unexpected error %v", err)
	}
}

func TestFileServiceMkdirListRenameDelete(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	if err := svc.MakeDir("", "subdir"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	entries, err := svc.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "subdir" || !entries[0].IsDir {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	if err := svc.Rename("subdir", "renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	entries, _ = svc.List("")
	if len(entries) != 1 || entries[0].Name != "renamed" {
		t.Fatalf("rename failed: %+v", entries)
	}

	if err := svc.Delete("renamed"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = svc.List("")
	if len(entries) != 0 {
		t.Fatalf("delete failed: %+v", entries)
	}
}

func TestFileServiceSaveUploaded(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	if err := svc.SaveUploaded("", "hello.txt", []byte("hello opsmini")); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := svc.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "hello.txt" || entries[0].Size != 13 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestFileServiceDirUsage(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "subdir", "b.txt"), []byte("world!"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	u, err := svc.DirUsage("subdir")
	if err != nil {
		t.Fatalf("dirusage: %v", err)
	}
	if u.Size != 6 {
		t.Fatalf("size = %d, want 6", u.Size)
	}
	if u.DiskTotal <= 0 {
		t.Fatalf("disk_total should be > 0, got %d", u.DiskTotal)
	}
}

func TestFileServiceCompressDir(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	if err := os.MkdirAll(filepath.Join(root, "sub", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(root, "sub", "a.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "sub", "nested", "b.txt"), []byte("world"), 0o644)

	tmp, name, err := svc.CompressDir("sub")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	defer os.Remove(tmp)
	if name != "sub.zip" {
		t.Fatalf("name = %s, want sub.zip", name)
	}

	zr, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found["sub/a.txt"] || !found["sub/nested/b.txt"] {
		t.Fatalf("zip contents wrong: %v", found)
	}
}

func TestFileServiceDeleteDirRecursive(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root)

	if err := os.MkdirAll(filepath.Join(root, "sub", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(root, "sub", "nested", "b.txt"), []byte("x"), 0o644)

	// 非空目录应能递归删除
	if err := svc.Delete("sub"); err != nil {
		t.Fatalf("delete non-empty dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sub")); !os.IsNotExist(err) {
		t.Fatalf("sub should be gone")
	}

	// 删除 root 本身应报错
	if err := svc.Delete(""); err == nil {
		t.Fatalf("deleting root should fail")
	}
}
