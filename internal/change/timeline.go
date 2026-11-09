package change

import (
	"sort"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// TimelineBucket is one time slot of the change timeline.
type TimelineBucket struct {
	Start string `json:"start"`
	Count int    `json:"count"`
	// ByKind splits the count by change kind (config / package / service / ...),
	// which is what makes a spike diagnosable at a glance: 200 events in one hour
	// means something very different when they are 200 config writes versus 200
	// package installs.
	ByKind map[string]int `json:"by_kind"`
	// Suspicious counts events the rule reviewer flagged inside this bucket.
	Suspicious int `json:"suspicious"`
	// Samples are a few representative events so a bar in the chart can be
	// clicked without a second query.
	Samples []TimelineSample `json:"samples"`
}

// TimelineSample is one representative change inside a bucket.
type TimelineSample struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ObjectKey string `json:"object_key"`
	ActorUser string `json:"actor_user"`
	At        string `json:"at"`
}

// Timeline is the aggregated change history over a window.
type Timeline struct {
	From      string           `json:"from"`
	To        string           `json:"to"`
	Bucket    string           `json:"bucket"`
	Total     int              `json:"total"`
	Truncated bool             `json:"truncated"`
	Buckets   []TimelineBucket `json:"buckets"`
}

// samplesPerBucket caps the samples returned per bucket. Three is enough to see
// "what kind of thing happened here" without turning the response into a list.
const samplesPerBucket = 3

// Timeline aggregates changes into fixed slots.
//
// Aggregation happens in memory rather than in SQL: SQLite date functions are
// awkward with the stored timestamp format, and the window is bounded (maxEvents)
// so the working set is small. The bound is what keeps a runaway log-parsing bug
// from turning a chart request into a table scan.
func (s *Store) Timeline(from, to time.Time, bucket time.Duration, maxEvents int) Timeline {
	if bucket <= 0 {
		bucket = time.Hour
	}
	if maxEvents <= 0 {
		maxEvents = 5000
	}
	if !to.After(from) {
		to = time.Now()
	}
	// Fetch one extra row to detect truncation without a second count query.
	events := s.ListBetween(from, to, maxEvents+1)
	truncated := false
	if len(events) > maxEvents {
		events = events[:maxEvents]
		truncated = true
	}

	reviews := s.ReviewsFor(idsOf(events))

	// Slot starts are aligned to the bucket boundary so two requests over
	// overlapping windows produce identical edges.
	start := from.Truncate(bucket)
	index := map[int64]*TimelineBucket{}
	for _, ev := range events {
		slot := ev.At.Truncate(bucket)
		if slot.Before(start) {
			slot = start
		}
		key := slot.Unix()
		b, ok := index[key]
		if !ok {
			b = &TimelineBucket{Start: slot.Format(time.RFC3339), ByKind: map[string]int{}}
			index[key] = b
		}
		b.Count++
		b.ByKind[ev.Kind]++
		if r, ok := reviews[ev.ID]; ok && r.Verdict == model.ChangeVerdictSuspicious {
			b.Suspicious++
		}
		if len(b.Samples) < samplesPerBucket {
			b.Samples = append(b.Samples, TimelineSample{
				ID: ev.ID, Kind: ev.Kind, ObjectKey: ev.ObjectKey,
				ActorUser: ev.ActorUser, At: ev.At.Format(time.RFC3339),
			})
		}
	}

	out := make([]TimelineBucket, 0, len(index))
	for _, b := range index {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })

	return Timeline{
		From:      from.Format(time.RFC3339),
		To:        to.Format(time.RFC3339),
		Bucket:    bucket.String(),
		Total:     len(events),
		Truncated: truncated,
		Buckets:   out,
	}
}

// idsOf extracts the event ids for the review lookup.
func idsOf(events []model.ChangeEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}
