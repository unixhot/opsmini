package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memStore struct {
	m       map[string]string
	failSet bool
}

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) Get(key string) (string, error) { return s.m[key], nil }

func (s *memStore) Set(kv map[string]string) error {
	if s.failSet {
		return errors.New("set failed")
	}
	for k, v := range kv {
		s.m[k] = v
	}
	return nil
}

func TestLoadOrCreateInstanceIDGeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	store := newMemStore()

	id, persisted := LoadOrCreateInstanceID(store, dir)
	if !persisted {
		t.Fatal("expected persisted=true")
	}
	if !IsInstanceID(id) {
		t.Fatalf("unexpected id format: %q", id)
	}
	if !strings.HasPrefix(id, "opsmini-") {
		t.Fatalf("expected opsmini- prefix, got %q", id)
	}
	if got := store.m[InstanceIDKey]; got != id {
		t.Fatalf("settings not written: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, instanceIDFileName))
	if err != nil {
		t.Fatalf("id file not written: %v", err)
	}
	if strings.TrimSpace(string(data)) != id {
		t.Fatalf("id file content mismatch: %q", string(data))
	}

	// 二次调用（模拟进程重启）必须返回同一个 ID。
	again, persisted := LoadOrCreateInstanceID(store, dir)
	if !persisted || again != id {
		t.Fatalf("id changed across restart: %q -> %q", id, again)
	}
}

func TestLoadOrCreateInstanceIDFileOnlyBackfillsDB(t *testing.T) {
	dir := t.TempDir()
	const existing = "opsmini-111111111111"
	if err := os.WriteFile(filepath.Join(dir, instanceIDFileName), []byte(existing+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newMemStore()

	id, persisted := LoadOrCreateInstanceID(store, dir)
	if id != existing {
		t.Fatalf("expected file value %q, got %q", existing, id)
	}
	if !persisted {
		t.Fatal("expected persisted=true")
	}
	if store.m[InstanceIDKey] != existing {
		t.Fatalf("settings not backfilled: %q", store.m[InstanceIDKey])
	}
}

func TestLoadOrCreateInstanceIDDBWinsAndFixesFile(t *testing.T) {
	dir := t.TempDir()
	const dbValue = "opsmini-dbdbdbdbdbdb"
	const fileValue = "opsmini-filefilefile"
	store := newMemStore()
	store.m[InstanceIDKey] = dbValue
	if err := os.WriteFile(filepath.Join(dir, instanceIDFileName), []byte(fileValue), 0o644); err != nil {
		t.Fatal(err)
	}

	id, _ := LoadOrCreateInstanceID(store, dir)
	if id != dbValue {
		t.Fatalf("expected DB value %q to win, got %q", dbValue, id)
	}
	data, _ := os.ReadFile(filepath.Join(dir, instanceIDFileName))
	if strings.TrimSpace(string(data)) != dbValue {
		t.Fatalf("file not repaired from DB: %q", string(data))
	}
}

func TestLoadOrCreateInstanceIDIgnoresInvalidStoredValue(t *testing.T) {
	dir := t.TempDir()
	store := newMemStore()
	store.m[InstanceIDKey] = "garbage"

	id, _ := LoadOrCreateInstanceID(store, dir)
	if !IsInstanceID(id) {
		t.Fatalf("expected regenerated id, got %q", id)
	}
	if id == "garbage" {
		t.Fatal("invalid value must not be reused")
	}
}

func TestLoadOrCreateInstanceIDNotPersistedWhenBothFail(t *testing.T) {
	store := newMemStore()
	store.failSet = true
	// 指向一个不可写的位置：把“目录”设成已存在的普通文件，写文件必然失败。
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	id, persisted := LoadOrCreateInstanceID(store, blocker)
	if persisted {
		t.Fatal("expected persisted=false")
	}
	if !IsInstanceID(id) {
		t.Fatalf("id must still be valid in memory: %q", id)
	}
}

func TestInstanceIDUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		id := newInstanceID()
		if seen[id] {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = true
	}
}
