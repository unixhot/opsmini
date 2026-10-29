package change

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/model"
	"gorm.io/gorm"
)

// Store persists change events and provides the tamper-evident hash chain.
type Store struct {
	db *gorm.DB

	mu        sync.Mutex
	lastHash  string
	lastID    string
	diffQuota int64 // bytes; 0 = unlimited
}

// NewStore builds a store. It loads the tail of the chain so that a restart
// continues the chain instead of starting a new one (a new chain would look
// like a truncation and defeat the tamper detection).
func NewStore(db *gorm.DB, diffQuotaBytes int64) (*Store, error) {
	s := &Store{db: db, diffQuota: diffQuotaBytes}
	var tail model.ChangeEvent
	err := db.Order("created_at desc, rowid desc").Limit(1).Find(&tail).Error
	if err != nil {
		return nil, err
	}
	if tail.ID != "" {
		s.lastHash = tail.ChainHash
		s.lastID = tail.ID
	}
	return s, nil
}

// Append writes one change event plus its (sanitized) diff body.
//
// Repeats inside the same dedup bucket only bump Count: a `vim` session that
// writes a file five times should read as one change in the timeline, not five.
func (s *Store) Append(ev Event) (*model.ChangeEvent, error) {
	now := time.Now()
	rec := model.ChangeEvent{
		ID:         ev.ID(),
		Kind:       ev.Kind,
		Source:     ev.Source,
		ObjectType: ev.Object.Type,
		ObjectKey:  ev.Object.Key,
		Labels:     strings.Join(dedupeStrings(ev.Object.Labels), ","),
		ActorKind:  ev.Actor.Kind,
		ActorUser:  ev.Actor.User,
		ActorIP:    ev.Actor.IP,
		ActorHow:   ev.Actor.How,
		Confidence: orDefault(ev.Actor.Confidence, model.ConfidenceUnknown),
		Count:      1,
		At:         ev.At,
		DetectedAt: now,
	}
	if rec.At.IsZero() {
		rec.At = now
	}
	if ev.Diff != nil {
		rec.Added = ev.Diff.Added
		rec.Removed = ev.Diff.Removed
		rec.Fields = strings.Join(dedupeStrings(ev.Diff.Fields), ",")
		rec.HashFrom = ev.Diff.HashFrom
		rec.HashTo = ev.Diff.HashTo
		rec.Truncated = ev.Diff.Truncated
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// No time-bucket merging here.
	//
	// An earlier version collapsed repeats of the same object inside a 10 minute
	// bucket by bumping Count. That silently broke the hash chain: the merge
	// updated hash_to/added/removed, which are part of the digest, so every
	// merged row failed verification afterwards. It would also rewrite a row
	// whose hash a later row already references, breaking the chain downstream.
	//
	// Editor double-writes are already handled by the 200ms debounce in the file
	// source, so merging here bought almost nothing. Every distinct change now
	// gets its own row — which is also what an audit trail should look like.
	rec.PrevHash = s.lastHash
	rec.ChainHash = chainHash(rec.PrevHash, rec)
	if err := s.db.Create(&rec).Error; err != nil {
		// Idempotent write: the realtime watcher and the periodic scan can race on
		// the same file, producing the same id twice. The second write is not an
		// error, it is the same fact observed twice.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			var existing model.ChangeEvent
			if e := s.db.Where("id = ?", rec.ID).Limit(1).Find(&existing).Error; e == nil && existing.ID != "" {
				return &existing, nil
			}
		}
		return nil, err
	}
	s.lastHash = rec.ChainHash
	s.lastID = rec.ID
	s.appendDiffLocked(rec.ID, ev.Diff)
	return &rec, nil
}

// appendDiffLocked stores the diff body unless the quota is exhausted. When the
// quota is hit we keep writing metadata: "something changed" is the evidence
// that matters most, and dropping it to protect disk space would be backwards.
func (s *Store) appendDiffLocked(changeID string, d *Diff) {
	if d == nil || strings.TrimSpace(d.Body) == "" {
		return
	}
	if s.diffQuota > 0 {
		var used int64
		s.db.Model(&model.ChangeDiff{}).Select("COALESCE(SUM(LENGTH(body)),0)").Scan(&used)
		if used+int64(len(d.Body)) > s.diffQuota {
			return
		}
	}
	var seq int64
	s.db.Model(&model.ChangeDiff{}).Where("change_id = ?", changeID).Count(&seq)
	_ = s.db.Create(&model.ChangeDiff{
		ChangeID:  changeID,
		Seq:       int(seq),
		Body:      d.Body,
		Truncated: d.Truncated,
	}).Error
}

// Filter narrows a change listing.
type Filter struct {
	Kind      string
	ObjectKey string
	Since     time.Time
	Until     time.Time
	Page      int
	PageSize  int
}

// List returns change events (metadata only) and the total count.
func (s *Store) List(f Filter) ([]model.ChangeEvent, int64, error) {
	q := s.db.Model(&model.ChangeEvent{})
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.ObjectKey != "" {
		q = q.Where("object_key LIKE ?", "%"+f.ObjectKey+"%")
	}
	if !f.Since.IsZero() {
		q = q.Where("at >= ?", f.Since)
	}
	if !f.Until.IsZero() {
		q = q.Where("at <= ?", f.Until)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := f.Page, f.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 200 {
		size = 50
	}
	var list []model.ChangeEvent
	err := q.Order("at desc").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// Get returns one event together with its diff bodies (already sanitized).
func (s *Store) Get(id string) (*model.ChangeEvent, []model.ChangeDiff, error) {
	var ev model.ChangeEvent
	if err := s.db.Where("id = ?", id).Limit(1).Find(&ev).Error; err != nil {
		return nil, nil, err
	}
	if ev.ID == "" {
		return nil, nil, errors.New("change not found")
	}
	var diffs []model.ChangeDiff
	if err := s.db.Where("change_id = ?", id).Order("seq asc").Find(&diffs).Error; err != nil {
		return nil, nil, err
	}
	return &ev, diffs, nil
}

// ListBetween returns events in a time window, used by the correlator.
func (s *Store) ListBetween(from, to time.Time, limit int) []model.ChangeEvent {
	if limit <= 0 {
		limit = 200
	}
	var list []model.ChangeEvent
	s.db.Where("at >= ? AND at <= ?", from, to).Order("at desc").Limit(limit).Find(&list)
	return list
}

// ChainStatus describes the tamper-evidence check result.
type ChainStatus struct {
	Checked  int      `json:"checked"`
	OK       bool     `json:"ok"`
	Breaks   []string `json:"breaks,omitempty"`
	LastHash string   `json:"last_hash"`
}

// VerifyChain walks the chain and reports any break.
//
// Honest limitation: this detects rows that were deleted or edited in place, but
// it cannot stop an attacker with root who rewrites the whole chain. Real
// tamper-evidence requires shipping the stream off-box (planned for v1.6).
func (s *Store) VerifyChain(limit int) ChainStatus {
	if limit <= 0 {
		limit = 2000
	}
	var list []model.ChangeEvent
	s.db.Order("created_at asc, rowid asc").Limit(limit).Find(&list)
	st := ChainStatus{OK: true}
	// Start from the anchor left behind by retention, not from "". Otherwise the first
	// record after any purge inherits a PrevHash that points at a deleted row and the
	// status would read "chain broken" forever.
	prev := s.chainAnchor()
	for i := range list {
		st.Checked++
		ev := list[i]
		if ev.PrevHash != prev {
			st.OK = false
			st.Breaks = append(st.Breaks, fmt.Sprintf("%s: prev hash mismatch", ev.ID))
		}
		if want := chainHash(ev.PrevHash, ev); want != ev.ChainHash {
			st.OK = false
			st.Breaks = append(st.Breaks, fmt.Sprintf("%s: content hash mismatch", ev.ID))
		}
		prev = ev.ChainHash
	}
	st.LastHash = prev
	if len(st.Breaks) > 20 {
		st.Breaks = st.Breaks[:20]
	}
	return st
}

// Purge deletes data older than the retention windows.
func (s *Store) Purge(metaDays, diffDays int) (int64, error) {
	now := time.Now()
	var diffs, metas int64
	if diffDays > 0 {
		cut := now.AddDate(0, 0, -diffDays)
		res := s.db.Where("created_at < ?", cut).Delete(&model.ChangeDiff{})
		diffs = res.RowsAffected
	}
	if metaDays > 0 {
		cut := now.AddDate(0, 0, -metaDays)
		// diffs of expired events go first so we do not leave orphans
		var ids []string
		s.db.Model(&model.ChangeEvent{}).Where("at < ?", cut).Pluck("id", &ids)
		if len(ids) > 0 {
			s.db.Where("change_id IN ?", ids).Delete(&model.ChangeDiff{})
		}
		// Remember where the chain stopped before dropping the rows: the surviving
		// records still point at them through PrevHash.
		var newest model.ChangeEvent
		if err := s.db.Where("at < ?", cut).Order("created_at desc, rowid desc").Limit(1).Find(&newest).Error; err == nil && newest.ChainHash != "" {
			s.setChainAnchor(newest.ChainHash)
		}
		res := s.db.Where("at < ?", cut).Delete(&model.ChangeEvent{})
		metas = res.RowsAffected
	}
	return diffs + metas, nil
}

// Usage reports storage consumption for the panel.
type Usage struct {
	Events    int64  `json:"events"`
	Diffs     int64  `json:"diffs"`
	DiffBytes int64  `json:"diff_bytes"`
	OldestAt  string `json:"oldest_at,omitempty"`
}

// Usage computes storage statistics.
func (s *Store) Usage() Usage {
	var u Usage
	s.db.Model(&model.ChangeEvent{}).Count(&u.Events)
	s.db.Model(&model.ChangeDiff{}).Count(&u.Diffs)
	s.db.Model(&model.ChangeDiff{}).Select("COALESCE(SUM(LENGTH(body)),0)").Scan(&u.DiffBytes)
	var oldest model.ChangeEvent
	if err := s.db.Order("at asc").Limit(1).Find(&oldest).Error; err == nil && oldest.ID != "" {
		u.OldestAt = oldest.At.Format(time.RFC3339)
	}
	return u
}

// --- chain anchor ----------------------------------------------------------

// EnsureChainAnchor backfills the anchor for databases written before the anchor
// existed. Such a store may already have had its oldest rows pruned, so its first
// surviving record points at a row that no longer exists — every later check would
// report "chain broken" although nothing was tampered with.
func (s *Store) EnsureChainAnchor() {
	if s.chainAnchor() != "" {
		return
	}
	var oldest model.ChangeEvent
	if err := s.db.Order("created_at asc, rowid asc").Limit(1).Find(&oldest).Error; err != nil || oldest.ID == "" {
		return
	}
	if oldest.PrevHash == "" {
		// The chain genuinely starts here; nothing was pruned.
		return
	}
	s.setChainAnchor(oldest.PrevHash)
}

const chainAnchorKey = "anchor"

// chainAnchor returns the ChainHash of the newest record removed by retention.
func (s *Store) chainAnchor() string {
	var m model.ChangeChainMeta
	if err := s.db.Where("key = ?", chainAnchorKey).Limit(1).Find(&m).Error; err != nil {
		return ""
	}
	return m.Value
}

// setChainAnchor records that key. Failures are silent: a missing anchor only makes
// the chain report one extra break, it never fabricates an OK.
func (s *Store) setChainAnchor(hash string) {
	rec := model.ChangeChainMeta{Key: chainAnchorKey, Value: hash, UpdatedAt: time.Now()}
	if err := s.db.Save(&rec).Error; err != nil {
		return
	}
}

// --- offsets ---------------------------------------------------------------

// Offset returns the stored read cursor for a log source.
func (s *Store) Offset(source string) (model.ChangeOffset, bool) {
	var o model.ChangeOffset
	if err := s.db.Where("source = ?", source).Limit(1).Find(&o).Error; err != nil || o.Source == "" {
		return o, false
	}
	return o, true
}

// SaveOffset persists a read cursor.
func (s *Store) SaveOffset(source string, inode uint64, offset int64) {
	rec := model.ChangeOffset{Source: source, Inode: inode, Offset: offset, UpdatedAt: time.Now()}
	if err := s.db.Save(&rec).Error; err != nil {
		log.Printf("change: save offset %s: %v", source, err)
	}
}

// --- ignore rules ----------------------------------------------------------

// IgnorePatterns returns user-editable ignore globs.
func (s *Store) IgnorePatterns() []string {
	var list []model.ChangeIgnore
	s.db.Find(&list)
	out := make([]string, 0, len(list))
	for _, it := range list {
		out = append(out, it.Pattern)
	}
	return out
}

// AddIgnore adds a glob.
func (s *Store) AddIgnore(pattern, note string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("pattern required")
	}
	return s.db.Create(&model.ChangeIgnore{Pattern: pattern, Note: note}).Error
}

// RemoveIgnore deletes a glob.
func (s *Store) RemoveIgnore(id uint) error {
	return s.db.Delete(&model.ChangeIgnore{}, id).Error
}

// ListIgnore returns the raw records for the settings page.
func (s *Store) ListIgnore() []model.ChangeIgnore {
	var list []model.ChangeIgnore
	s.db.Order("id asc").Find(&list)
	return list
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
