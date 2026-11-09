package change

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
)

func timelineTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.ChangeEvent{}, &model.ChangeDiff{}, &model.ChangeChainMeta{}, &model.ChangeReview{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s, err := NewStore(db, 0)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

// Only the requested window is aggregated, and slots line up with the bucket
// boundary so two overlapping requests produce identical edges.
func TestTimelineBucketsWithinWindow(t *testing.T) {
	s := timelineTestStore(t)
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)

	// Three events in hour 10, one in hour 12, one outside the window.
	for i := 0; i < 3; i++ {
		s.Append(NewEvent("config", "file", Object{Type: "file", Key: "/etc/a.conf"}, base.Add(time.Duration(i)*time.Minute)))
	}
	s.Append(NewEvent("package", "dpkg", Object{Type: "package", Key: "nginx"}, base.Add(2*time.Hour)))
	s.Append(NewEvent("service", "systemd", Object{Type: "service", Key: "nginx"}, base.Add(5*time.Hour)))

	tl := s.Timeline(base, base.Add(3*time.Hour), time.Hour, 100)
	if tl.Total != 4 {
		t.Fatalf("total: got %d want 4", tl.Total)
	}
	if len(tl.Buckets) != 2 {
		t.Fatalf("buckets: got %d want 2 (%+v)", len(tl.Buckets), tl.Buckets)
	}
	if tl.Buckets[0].Count != 3 || tl.Buckets[0].ByKind["config"] != 3 {
		t.Fatalf("first bucket: %+v", tl.Buckets[0])
	}
	if tl.Buckets[1].Count != 1 || tl.Buckets[1].ByKind["package"] != 1 {
		t.Fatalf("second bucket: %+v", tl.Buckets[1])
	}
}

// A bucket keeps at most three samples: enough to see what happened, not enough
// to make the chart response a second list endpoint.
func TestTimelineSampleCap(t *testing.T) {
	s := timelineTestStore(t)
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		s.Append(NewEvent("config", "file", Object{Type: "file", Key: "/etc/a.conf"}, base.Add(time.Duration(i)*time.Second)))
	}
	tl := s.Timeline(base, base.Add(time.Hour), time.Hour, 100)
	if len(tl.Buckets) != 1 {
		t.Fatalf("buckets: %d", len(tl.Buckets))
	}
	if got := len(tl.Buckets[0].Samples); got != samplesPerBucket {
		t.Fatalf("samples: got %d want %d", got, samplesPerBucket)
	}
	if tl.Buckets[0].Count != 10 {
		t.Fatalf("count must stay complete even when samples are capped: %d", tl.Buckets[0].Count)
	}
}

// Hitting the event cap must be reported, otherwise a truncated chart looks like
// a quiet one.
func TestTimelineTruncationFlag(t *testing.T) {
	s := timelineTestStore(t)
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)
	for i := 0; i < 6; i++ {
		s.Append(NewEvent("config", "file", Object{Type: "file", Key: "/etc/a.conf"}, base.Add(time.Duration(i)*time.Second)))
	}
	tl := s.Timeline(base, base.Add(time.Hour), time.Hour, 3)
	if !tl.Truncated {
		t.Fatal("expected truncated=true")
	}
	if tl.Total != 3 {
		t.Fatalf("total: got %d want 3", tl.Total)
	}
}

// An empty window must return an empty slice, not nil: the frontend iterates it
// directly and nil marshals to null, which breaks the chart binding.
func TestTimelineEmptyWindow(t *testing.T) {
	s := timelineTestStore(t)
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)
	tl := s.Timeline(base, base.Add(time.Hour), time.Hour, 100)
	if tl.Buckets == nil {
		t.Fatal("buckets must be an empty slice, not nil")
	}
	if tl.Total != 0 {
		t.Fatalf("total: %d", tl.Total)
	}
}
