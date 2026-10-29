package change

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newAnchorStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.ChangeEvent{}, &model.ChangeDiff{},
		&model.ChangeChainMeta{}, &model.ChangeOffset{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Store{db: db}
}

// A database that was pruned by an older build must heal itself instead of reporting
// a break forever.
func TestEnsureChainAnchorBackfillsPrunedStore(t *testing.T) {
	s := newAnchorStore(t)
	now := time.Now()
	s.db.Create(&model.ChangeEvent{ID: "e1", ObjectKey: "/etc/a", PrevHash: "deadbeef", ChainHash: "h1", At: now})

	s.EnsureChainAnchor()
	if got := s.chainAnchor(); got != "deadbeef" {
		t.Fatalf("anchor = %q, want the oldest row's PrevHash", got)
	}
}

// A stale anchor from a cleared table must not survive: the first new record starts
// from "", and keeping the old anchor would flag it as tampered.
func TestEnsureChainAnchorClearsStaleAnchor(t *testing.T) {
	s := newAnchorStore(t)
	s.setChainAnchor("old-generation-hash")
	s.db.Create(&model.ChangeEvent{ID: "e2", ObjectKey: "/etc/b", PrevHash: "", ChainHash: "h2", At: time.Now()})

	s.EnsureChainAnchor()
	if got := s.chainAnchor(); got != "" {
		t.Fatalf("stale anchor should be cleared, got %q", got)
	}
}

// Empty table: the anchor has nothing to point at, so it must go too.
func TestEnsureChainAnchorClearsOnEmptyTable(t *testing.T) {
	s := newAnchorStore(t)
	s.setChainAnchor("hash-of-deleted-rows")

	s.EnsureChainAnchor()
	if got := s.chainAnchor(); got != "" {
		t.Fatalf("anchor should be cleared with no rows left, got %q", got)
	}
}
