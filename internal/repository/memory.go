package repository

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// MemoryRepo stores durable agent memories.
type MemoryRepo struct {
	db *gorm.DB
}

// NewMemoryRepo returns a new MemoryRepo.
func NewMemoryRepo(db *gorm.DB) *MemoryRepo {
	return &MemoryRepo{db: db}
}

// Upsert writes a fact, replacing an existing one with the same subject AND fact.
//
// Re-stating a fact must not grow the table: the agent will re-learn "nginx config
// lives in /etc/nginx/conf.d" in many sessions, and 50 copies of it crowd out the
// 4 facts that actually vary.
func (r *MemoryRepo) Upsert(m *model.AgentMemory) error {
	var existing model.AgentMemory
	err := r.db.Where("subject = ? AND fact = ?", m.Subject, m.Fact).First(&existing).Error
	if err == nil {
		existing.Confidence = m.Confidence
		existing.ExpiresAt = m.ExpiresAt
		existing.SourceSession = m.SourceSession
		return r.db.Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return r.db.Create(m).Error
}

// List returns memories newest first, skipping expired rows.
func (r *MemoryRepo) List(limit int) ([]model.AgentMemory, error) {
	if limit <= 0 {
		limit = 200
	}
	var out []model.AgentMemory
	err := r.db.Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

// Count returns the number of live memories.
func (r *MemoryRepo) Count() (int64, error) {
	var n int64
	err := r.db.Model(&model.AgentMemory{}).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).Count(&n).Error
	return n, err
}

// Relevant picks memories to inject into a new run.
//
// Matching is intentionally simple: a memory is relevant when its subject appears
// in the question. Anything cleverer (embeddings, fuzzy scores) needs the very
// infrastructure a single-host panel does not have, and a wrong match is worse
// than no match — it makes the agent assert something unrelated with confidence.
//
// When fewer than limit subjects match, the newest high-confidence memories fill
// the remaining slots so a general question ("这台机器状态如何") still benefits
// from what was learned before.
func (r *MemoryRepo) Relevant(query string, limit int) ([]model.AgentMemory, error) {
	if limit <= 0 {
		limit = 5
	}
	all, err := r.List(limit * 8)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(query)
	hit := make([]model.AgentMemory, 0, limit)
	rest := make([]model.AgentMemory, 0, len(all))
	for _, m := range all {
		if m.Subject != "" && strings.Contains(lower, strings.ToLower(m.Subject)) {
			hit = append(hit, m)
			continue
		}
		rest = append(rest, m)
	}
	if len(hit) >= limit {
		return hit[:limit], nil
	}
	for _, m := range rest {
		if len(hit) >= limit {
			break
		}
		if m.Confidence < 0.5 {
			continue
		}
		hit = append(hit, m)
	}
	return hit, nil
}

// Delete removes one memory.
func (r *MemoryRepo) Delete(id uint) error {
	return r.db.Delete(&model.AgentMemory{}, id).Error
}

// Clear removes all memories and reports how many were dropped.
func (r *MemoryRepo) Clear() (int64, error) {
	res := r.db.Where("1 = 1").Delete(&model.AgentMemory{})
	return res.RowsAffected, res.Error
}

// Evict keeps the table under limit rows by dropping the least trustworthy and
// oldest entries first.
//
// Confidence before age on purpose: "I think the port might be 8080" should go
// before "the user told me the port is 8080" even if the latter is older.
func (r *MemoryRepo) Evict(limit int) error {
	if limit <= 0 {
		return nil
	}
	n, err := r.Count()
	if err != nil || n <= int64(limit) {
		return err
	}
	over := int(n) - limit
	var doomed []model.AgentMemory
	if err := r.db.Order("confidence ASC, created_at ASC").Limit(over).Find(&doomed).Error; err != nil {
		return err
	}
	ids := make([]uint, 0, len(doomed))
	for _, m := range doomed {
		ids = append(ids, m.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return r.db.Delete(&model.AgentMemory{}, ids).Error
}
