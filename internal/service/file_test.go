package service

import (
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
