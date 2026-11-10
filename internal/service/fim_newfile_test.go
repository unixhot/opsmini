package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// fimTestService builds a FimService backed by an in-memory DB and a fixed
// monitored-path list, so the tests do not depend on the host's /etc.
func fimTestService(t *testing.T, paths, critical []string) *FimService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.FimBaseline{}, &model.FimChange{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewFimService(repository.NewFimBaselineRepo(db), repository.NewFimChangeRepo(db))
	svc.SetSettings(func(key string) string {
		switch key {
		case "fim_paths":
			return joinComma(paths)
		case "fim_critical_paths":
			return joinComma(critical)
		}
		return ""
	})
	return svc
}

func joinComma(list []string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

// 新建的文件必须被报出来。
//
// 基线只覆盖「建基线时已存在的文件」，旧实现里新落地的木马完全隐形——
// 那是文件完整性监控最该抓的一件事。这个用例守的就是这个洞。
func TestFimDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	watched := filepath.Join(dir, "upload.php")
	svc := fimTestService(t, []string{watched}, []string{watched})

	// 建基线时文件还不存在：基线里没有它。
	if err := svc.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got := svc.Check(); len(got) != 0 {
		t.Fatalf("空目录不该有变更，got %d", len(got))
	}

	// 文件出现了。
	if err := os.WriteFile(watched, []byte("<?php /* sample */ ?>"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := svc.Check()
	if len(changes) != 1 {
		t.Fatalf("新增文件未被检出，changes=%d", len(changes))
	}
	if changes[0].Message != "新增文件" {
		t.Fatalf("unexpected message: %q", changes[0].Message)
	}
	if changes[0].Level != "critical" {
		t.Fatalf("critical 路径上的新增文件应判 critical，got %q", changes[0].Level)
	}

	// 同一个文件不能每轮都报一次：60 秒一条会把事件列表冲掉。
	if again := svc.Check(); len(again) != 0 {
		t.Fatalf("同一文件被重复上报: %d", len(again))
	}
}

// 被监控路径之外的文件不该产生任何事件。
func TestFimIgnoresUnwatched(t *testing.T) {
	dir := t.TempDir()
	watched := filepath.Join(dir, "watched.conf")
	other := filepath.Join(dir, "other.conf")
	svc := fimTestService(t, []string{watched}, nil)
	if err := svc.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := svc.Check(); len(got) != 0 {
		t.Fatalf("未监控的文件被上报了 %d 次", len(got))
	}
}
