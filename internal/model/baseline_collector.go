package model

import "time"

// CollectorBaseline persists a collector's learned baseline.
//
// Baselines used to live only in memory, which made every restart a cold start:
// the first minutes after a restart are exactly when a "49 MB in 40 seconds"
// judgement matters, and exactly when an empty baseline produces either silence
// or a false alarm. Persisting them turns "relearn from zero" into "resume".
//
// Rows expire by UpdatedAt: a baseline from three weeks ago describes a host that
// no longer exists, so collectors ignore and rewrite anything older than a week.
type CollectorBaseline struct {
	Collector string `gorm:"primaryKey;size:32" json:"collector"`
	// Subject identifies what the baseline is about: a file path for logwatch,
	// a process name for procwatch.
	Subject string `gorm:"primaryKey;size:256" json:"subject"`
	// Bucket is the collector's own sub-key (hour of day for logwatch, so a
	// nightly batch job and a daytime web log get separate baselines).
	Bucket    int       `gorm:"primaryKey" json:"bucket"`
	Mean      float64   `json:"mean"`
	Variance  float64   `json:"variance"`
	Samples   int       `json:"samples"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CollectorBaseline) TableName() string { return "collector_baselines" }
