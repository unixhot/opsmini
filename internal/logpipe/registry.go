package logpipe

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// Registry remembers the read position of every (task, file) pair.
//
// Offset advancement is driven by the pipeline AFTER the output accepted the
// batch, which gives at-least-once: a crash between publishing and saving
// re-sends the batch instead of losing it.
type Registry struct {
	db *gorm.DB

	mu    sync.Mutex
	cache map[string]*model.LogPipeRegistry
	dirty map[string]bool
}

func NewRegistry(db *gorm.DB) *Registry {
	return &Registry{db: db, cache: map[string]*model.LogPipeRegistry{}, dirty: map[string]bool{}}
}

func regKey(taskID, path string) string { return taskID + "\x00" + path }

// Get returns the saved position, or a zero one for a never-seen file.
func (r *Registry) Get(taskID, path string) model.LogPipeRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[regKey(taskID, path)]; ok {
		return *e
	}
	var row model.LogPipeRegistry
	if r.db != nil {
		if err := r.db.Where("task_id = ? AND file_path = ?", taskID, path).Limit(1).Find(&row).Error; err == nil && row.ID != 0 {
			r.cache[regKey(taskID, path)] = &row
			return row
		}
	}
	return model.LogPipeRegistry{TaskID: taskID, FilePath: path}
}

// Advance records a new confirmed offset.
func (r *Registry) Advance(taskID, path string, inode uint64, offset int64, fingerprint string) {
	r.mu.Lock()
	key := regKey(taskID, path)
	e := r.cache[key]
	if e == nil {
		e = &model.LogPipeRegistry{TaskID: taskID, FilePath: path}
		r.cache[key] = e
	}
	e.Inode = inode
	e.Offset = offset
	if fingerprint != "" {
		e.Fingerprint = fingerprint
	}
	e.UpdatedAt = time.Now()
	row := *e
	r.mu.Unlock()

	if r.db == nil {
		return
	}
	// Upsert on (task_id, file_path): SQLite has ON CONFLICT, but the GORM
	// portable path here is "update then insert if no rows".
	res := r.db.Model(&model.LogPipeRegistry{}).
		Where("task_id = ? AND file_path = ?", taskID, path).
		Updates(map[string]any{"inode": inode, "offset": offset, "fingerprint": row.Fingerprint, "updated_at": row.UpdatedAt})
	if res.Error != nil {
		log.Printf("logpipe: save registry %s: %v", path, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		if err := r.db.Create(&row).Error; err != nil {
			log.Printf("logpipe: create registry %s: %v", path, err)
		}
	}
}

// Forget drops positions of files that no longer belong to a task (rotation).
func (r *Registry) Forget(taskID, path string) {
	r.mu.Lock()
	delete(r.cache, regKey(taskID, path))
	r.mu.Unlock()
	if r.db != nil {
		r.db.Where("task_id = ? AND file_path = ?", taskID, path).Delete(&model.LogPipeRegistry{})
	}
}

// ForgetTask removes every position of a task; used when a task disappears.
func (r *Registry) ForgetTask(taskID string) {
	r.mu.Lock()
	for k := range r.cache {
		if len(k) > len(taskID) && k[:len(taskID)] == taskID {
			delete(r.cache, k)
		}
	}
	r.mu.Unlock()
	if r.db != nil {
		r.db.Where("task_id = ?", taskID).Delete(&model.LogPipeRegistry{})
	}
}

// Fingerprint reads the first 64 bytes; a rotated file at the same path has a
// different head, which is how we notice the offset no longer applies.
func Fingerprint(f *os.File) string {
	buf := make([]byte, 64)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return ""
	}
	sum := sha256.Sum256(buf[:n])
	return hex.EncodeToString(sum[:8])
}
