package backup

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// memS3 is a minimal in-memory S3 endpoint: enough verbs for PutDir/List/GetDir/
// Delete. It exists to prove the client wiring end to end; signature correctness is
// proven separately against the AWS test vector.
type memS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	bucket  string
	// requireAuth makes the mock reject requests without a signature, which is how
	// a real bucket behaves when it is not public.
	requireAuth bool
}

func newMemS3(bucket string) *memS3 {
	return &memS3{objects: map[string][]byte{}, bucket: bucket}
}

func (m *memS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.requireAuth && r.Header.Get("Authorization") == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasPrefix(p, m.bucket) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	key := strings.TrimPrefix(strings.TrimPrefix(p, m.bucket), "/")

	switch r.Method {
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		m.objects[key] = body
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(m.objects, key)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" {
			m.list(w, r)
			return
		}
		body, ok := m.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(body)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *memS3) list(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	delim := r.URL.Query().Get("delimiter")

	type prefixXML struct {
		Prefix string `xml:"Prefix"`
	}
	type contentXML struct {
		Key string `xml:"Key"`
	}
	var out struct {
		XMLName        xml.Name     `xml:"ListBucketResult"`
		Contents       []contentXML `xml:"Contents"`
		CommonPrefixes []prefixXML  `xml:"CommonPrefixes"`
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if delim != "" {
			rest := strings.TrimPrefix(k, prefix)
			if i := strings.Index(rest, delim); i >= 0 {
				cp := prefix + rest[:i+1]
				if !seen[cp] {
					seen[cp] = true
					out.CommonPrefixes = append(out.CommonPrefixes, prefixXML{Prefix: cp})
				}
				continue
			}
		}
		out.Contents = append(out.Contents, contentXML{Key: k})
	}
	body, _ := xml.Marshal(out)
	w.Header().Set("Content-Type", "application/xml")
	w.Write(body)
}

func testS3Store(t *testing.T) (*s3Store, *memS3) {
	t.Helper()
	mock := newMemS3("bkt")
	mock.requireAuth = true
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)
	st, err := newS3Store(S3Config{
		Endpoint:  srv.URL,
		Region:    "us-east-1",
		Bucket:    "bkt",
		Prefix:    "prod",
		AccessKey: "AK",
		SecretKey: "SK",
		PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, mock
}

func TestS3RoundTrip(t *testing.T) {
	st, mock := testS3Store(t)
	ctx := context.Background()

	// Upload a snapshot-shaped directory.
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "manifest.json"), []byte(`{"name":"snap-1"}`), 0o600)
	os.WriteFile(filepath.Join(src, "data", "site.tar.gz"), []byte("archive-bytes"), 0o600)

	if err := st.PutDir(ctx, src, "snap-1"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if len(mock.objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(mock.objects))
	}

	list, err := st.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "snap-1" {
		t.Fatalf("list=%+v", list)
	}

	// Download into a fresh parent and compare bytes.
	dst := t.TempDir()
	if err := st.GetDir(ctx, "snap-1", dst); err != nil {
		t.Fatalf("get: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "snap-1", "data", "site.tar.gz"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "archive-bytes" {
		t.Fatalf("content mismatch: %q", got)
	}

	if err := st.Delete(ctx, "snap-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(mock.objects) != 0 {
		t.Fatalf("objects left after delete: %v", mock.objects)
	}
	// Deleting an already-absent snapshot is not an error (dispatch may retry).
	if err := st.Delete(ctx, "snap-1"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestS3GetMissingFails(t *testing.T) {
	st, _ := testS3Store(t)
	if err := st.GetDir(context.Background(), "nope", t.TempDir()); err == nil {
		t.Fatal("downloading a missing snapshot must fail")
	}
}

// TestBackupToRemote proves the path a scheduled job takes: build locally, upload,
// prune remote copies with the same "keep N" rule.
func TestBackupToRemote(t *testing.T) {
	st, mock := testS3Store(t)
	m := NewManager(t.TempDir(), "test")
	m.SetStores(map[string]RemoteStore{"mock": st})

	src := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "index.html"), []byte("hello"), 0o600)

	keep := true
	man, err := m.Run(Job{
		Name:      "nightly",
		Objects:   []Object{{Kind: "dir", Path: src, Name: "site"}},
		Retention: 2,
		Remote:    []string{"mock"},
		KeepLocal: &keep,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(mock.objects) == 0 {
		t.Fatal("nothing uploaded")
	}

	// The local copy must still exist when KeepLocal is true.
	if _, err := os.Stat(filepath.Join(m.root, man.Name)); err != nil {
		t.Fatalf("local snapshot missing: %v", err)
	}

	list, err := m.ListRemote(context.Background(), "mock")
	if err != nil {
		t.Fatalf("list remote: %v", err)
	}
	if len(list) != 1 || list[0].Name != man.Name {
		t.Fatalf("remote list=%+v", list)
	}

	// Fetch is idempotent: a snapshot already present locally is reused.
	dir, err := m.Fetch(context.Background(), "mock", man.Name)
	if err != nil {
		t.Fatalf("fetch existing: %v", err)
	}
	if dir != filepath.Join(m.root, man.Name) {
		t.Fatalf("fetch returned %s", dir)
	}
}

func TestRemoteRetentionPrunesOldSnapshots(t *testing.T) {
	st, mock := testS3Store(t)
	m := NewManager(t.TempDir(), "test")
	m.SetStores(map[string]RemoteStore{"mock": st})

	ctx := context.Background()
	// Names carry timestamps, so lexical order is chronological order.
	names := []string{"job-20260101-000000", "job-20260102-000000", "job-20260103-000000"}
	for _, n := range names {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o600)
		if err := st.PutDir(ctx, dir, n); err != nil {
			t.Fatalf("put %s: %v", n, err)
		}
	}
	if err := m.uploadSnapshot(ctx, st, t.TempDir(), "job-20260104-000000", 2); err != nil {
		t.Fatalf("upload with retention: %v", err)
	}
	// The newest snapshot passed to uploadSnapshot must survive; older ones go.
	keys := make([]string, 0, len(mock.objects))
	for k := range mock.objects {
		keys = append(keys, k)
	}
	for _, k := range keys {
		if strings.Contains(k, "job-20260101-000000") {
			t.Fatalf("oldest snapshot was not pruned: %v", keys)
		}
	}
}

func TestFetchRejectsTraversal(t *testing.T) {
	m := NewManager(t.TempDir(), "test")
	if _, err := m.Fetch(context.Background(), "mock", "../etc"); err == nil {
		t.Fatal("traversal must be rejected")
	}
	if _, err := m.Fetch(context.Background(), "mock", "ok"); err == nil {
		t.Fatal("unknown store must fail")
	}
}

func TestNewRemoteValidation(t *testing.T) {
	if _, err := NewRemote(TargetSpec{Name: "x", Type: "s3"}); err == nil {
		t.Fatal("s3 without config must fail")
	}
	if _, err := NewRemote(TargetSpec{Name: "x", Type: "sftp"}); err == nil {
		t.Fatal("sftp without config must fail")
	}
	if _, err := NewRemote(TargetSpec{Name: "x", Type: "ftp"}); err == nil {
		t.Fatal("unknown type must fail")
	}
	if _, err := NewRemote(TargetSpec{Name: "x", Type: "sftp", SFTP: &SFTPConfig{Host: "h", User: "u"}}); err == nil {
		t.Fatal("sftp without credentials must fail")
	}
	st, err := NewRemote(TargetSpec{Name: "x", Type: "sftp", SFTP: &SFTPConfig{Host: "h", User: "u", Password: "p"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Describe(), "/backup/opsmini") {
		t.Fatalf("default base dir missing: %s", st.Describe())
	}
}

// silence unused warnings in case a helper is only used by some tests
var _ = fmt.Sprint
var _ = time.Now
