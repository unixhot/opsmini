package model

import "time"

// LogPipeRegistry records how far one input has read one file.
//
// This is the log pipeline's version of filebeat's registry: without it a
// restart re-reads from byte 0 and floods the output with duplicates, or skips
// whatever was appended while the process was down.
type LogPipeRegistry struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	TaskID   string `gorm:"size:96;index;uniqueIndex:idx_logreg" json:"task_id"`
	FilePath string `gorm:"size:512;uniqueIndex:idx_logreg" json:"file_path"`
	Inode    uint64 `json:"inode"`
	Offset   int64  `json:"offset"`
	// Fingerprint is the first 64 bytes of the file. When a file is rotated and
	// a new one appears at the same path, the offset no longer means anything —
	// the fingerprint tells us this is a different file and reading restarts.
	Fingerprint string    `gorm:"size:128" json:"fingerprint"`
	UpdatedAt   time.Time `json:"updated_at"`
}
