package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
)

func memoryTestRepo(t *testing.T) *MemoryRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentMemory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewMemoryRepo(db)
}

// Re-learning the same fact must not grow the table: the agent will state
// "nginx config is in /etc/nginx/conf.d" in many sessions.
func TestMemoryUpsertDeduplicates(t *testing.T) {
	r := memoryTestRepo(t)
	m := &model.AgentMemory{Subject: "nginx", Fact: "配置在 /etc/nginx/conf.d", Confidence: 0.6}
	if err := r.Upsert(m); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	again := &model.AgentMemory{Subject: "nginx", Fact: "配置在 /etc/nginx/conf.d", Confidence: 0.9, SourceSession: "s2"}
	if err := r.Upsert(again); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	n, err := r.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows: got %d want 1", n)
	}
	list, _ := r.List(10)
	if list[0].Confidence != 0.9 || list[0].SourceSession != "s2" {
		t.Fatalf("re-stated fact should refresh confidence/session, got %+v", list[0])
	}
}

// A subject mentioned in the question outranks newer unrelated memories.
func TestMemoryRelevantPrefersSubject(t *testing.T) {
	r := memoryTestRepo(t)
	_ = r.Upsert(&model.AgentMemory{Subject: "redis", Fact: "RDB 在 /var/lib/redis", Confidence: 0.9})
	time.Sleep(2 * time.Millisecond)
	_ = r.Upsert(&model.AgentMemory{Subject: "nginx", Fact: "配置在 /etc/nginx", Confidence: 0.6})

	got, err := r.Relevant("nginx 为什么启动不了", 3)
	if err != nil {
		t.Fatalf("relevant: %v", err)
	}
	if len(got) == 0 || got[0].Subject != "nginx" {
		t.Fatalf("expected nginx first, got %+v", got)
	}
}

// A general question still gets the most trustworthy facts as filler.
func TestMemoryRelevantFillsWithConfident(t *testing.T) {
	r := memoryTestRepo(t)
	_ = r.Upsert(&model.AgentMemory{Subject: "redis", Fact: "RDB 在 /var/lib/redis", Confidence: 0.9})
	_ = r.Upsert(&model.AgentMemory{Subject: "guess", Fact: "可能是 8080", Confidence: 0.3})

	got, _ := r.Relevant("这台机器怎么样", 5)
	if len(got) != 1 {
		t.Fatalf("low-confidence facts must not be injected, got %d", len(got))
	}
	if got[0].Subject != "redis" {
		t.Fatalf("got %s", got[0].Subject)
	}
}

// Expired facts are invisible: a stale port number is worse than nothing.
func TestMemoryExpiredHidden(t *testing.T) {
	r := memoryTestRepo(t)
	past := time.Now().Add(-24 * time.Hour)
	_ = r.Upsert(&model.AgentMemory{Subject: "app", Fact: "端口 8080", Confidence: 0.9, ExpiresAt: &past})
	n, _ := r.Count()
	if n != 0 {
		t.Fatalf("expired memory still counted: %d", n)
	}
}

// Eviction drops the least confident first, not merely the oldest.
func TestMemoryEvictDropsLowConfidence(t *testing.T) {
	r := memoryTestRepo(t)
	// The least confident one is inserted last on purpose: age alone would keep it.
	_ = r.Upsert(&model.AgentMemory{Subject: "keep", Fact: "用户明确说过", Confidence: 1.0})
	_ = r.Upsert(&model.AgentMemory{Subject: "drop", Fact: "我猜的", Confidence: 0.1})

	if err := r.Evict(1); err != nil {
		t.Fatalf("evict: %v", err)
	}
	n, _ := r.Count()
	if n != 1 {
		t.Fatalf("rows after evict: got %d want 1", n)
	}
	list, _ := r.List(10)
	if list[0].Subject != "keep" {
		t.Fatalf("wrong survivor: %s", list[0].Subject)
	}
}

// Clear is what the settings page's "forget everything" button calls.
func TestMemoryClear(t *testing.T) {
	r := memoryTestRepo(t)
	for i := 0; i < 5; i++ {
		_ = r.Upsert(&model.AgentMemory{Subject: fmt.Sprintf("s%d", i), Fact: "f", Confidence: 0.8})
	}
	n, err := r.Clear()
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n != 5 {
		t.Fatalf("deleted: got %d want 5", n)
	}
	if left, _ := r.Count(); left != 0 {
		t.Fatalf("rows left: %d", left)
	}
}
