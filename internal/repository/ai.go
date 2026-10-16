package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AIRepo provides data access for AI Agent sessions, messages and pending approvals.
type AIRepo struct {
	db *gorm.DB
}

// NewAIRepo returns a new AIRepo.
func NewAIRepo(db *gorm.DB) *AIRepo {
	return &AIRepo{db: db}
}

// --- sessions ---

// CreateSession inserts a session.
func (r *AIRepo) CreateSession(s *model.AISession) error {
	return r.db.Create(s).Error
}

// UpdateSession saves a session.
func (r *AIRepo) UpdateSession(s *model.AISession) error {
	return r.db.Save(s).Error
}

// GetSession returns a session by id.
func (r *AIRepo) GetSession(id string) (*model.AISession, error) {
	var s model.AISession
	if err := r.db.First(&s, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSessions returns the sessions of a user plus the scheduler-owned ones
// (user_id = 0, i.e. scheduled AI task reports), most recently updated first.
func (r *AIRepo) ListSessions(userID uint, limit int) ([]model.AISession, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []model.AISession
	err := r.db.Where("user_id = ? OR user_id = 0", userID).
		Order("updated_at desc").Limit(limit).Find(&list).Error
	return list, err
}

// DeleteSession removes a session together with its messages and approvals.
func (r *AIRepo) DeleteSession(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&model.AIMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&model.AIPendingApproval{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.AISession{}, "id = ?", id).Error
	})
}

// DeleteSessionsBefore removes sessions (and their children) not updated since t.
func (r *AIRepo) DeleteSessionsBefore(t time.Time) (int64, error) {
	var ids []string
	if err := r.db.Model(&model.AISession{}).Where("updated_at < ?", t).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id IN ?", ids).Delete(&model.AIMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id IN ?", ids).Delete(&model.AIPendingApproval{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&model.AISession{}).Error
	})
	return int64(len(ids)), err
}

// --- messages ---

// AppendMessages inserts a batch of messages, assigning sequential Seq values.
func (r *AIRepo) AppendMessages(sessionID string, msgs []model.AIMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	var maxSeq int
	_ = r.db.Model(&model.AIMessage{}).Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq).Error
	for i := range msgs {
		msgs[i].SessionID = sessionID
		msgs[i].Seq = maxSeq + i + 1
	}
	return r.db.Create(&msgs).Error
}

// ListMessages returns the messages of a session in chronological order.
func (r *AIRepo) ListMessages(sessionID string, limit int) ([]model.AIMessage, error) {
	q := r.db.Where("session_id = ?", sessionID).Order("seq asc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var list []model.AIMessage
	err := q.Find(&list).Error
	return list, err
}

// CountMessages returns the number of messages in a session.
func (r *AIRepo) CountMessages(sessionID string) (int64, error) {
	var n int64
	err := r.db.Model(&model.AIMessage{}).Where("session_id = ?", sessionID).Count(&n).Error
	return n, err
}

// DeleteMessagesFrom removes messages with Seq greater than seq (used when a run
// is rolled back after a rejected approval).
func (r *AIRepo) DeleteMessagesFrom(sessionID string, seq int) error {
	return r.db.Where("session_id = ? AND seq > ?", sessionID, seq).Delete(&model.AIMessage{}).Error
}

// TrimMessages keeps only the newest keep messages of a session and returns the
// removed ones (used by memory compression).
func (r *AIRepo) TrimMessages(sessionID string, keep int) ([]model.AIMessage, error) {
	var all []model.AIMessage
	if err := r.db.Where("session_id = ?", sessionID).Order("seq asc").Find(&all).Error; err != nil {
		return nil, err
	}
	if len(all) <= keep {
		return nil, nil
	}
	drop := all[:len(all)-keep]
	ids := make([]uint, 0, len(drop))
	for _, m := range drop {
		ids = append(ids, m.ID)
	}
	if err := r.db.Delete(&model.AIMessage{}, "id IN ?", ids).Error; err != nil {
		return nil, err
	}
	return drop, nil
}

// --- pending approvals ---

// CreateApproval inserts a pending approval.
func (r *AIRepo) CreateApproval(a *model.AIPendingApproval) error {
	return r.db.Create(a).Error
}

// GetApproval returns an approval by id.
func (r *AIRepo) GetApproval(id string) (*model.AIPendingApproval, error) {
	var a model.AIPendingApproval
	if err := r.db.First(&a, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateApproval saves an approval.
func (r *AIRepo) UpdateApproval(a *model.AIPendingApproval) error {
	return r.db.Save(a).Error
}

// ListPendingApprovals returns the pending approvals of a session.
func (r *AIRepo) ListPendingApprovals(sessionID string) ([]model.AIPendingApproval, error) {
	var list []model.AIPendingApproval
	err := r.db.Where("session_id = ? AND status = ?", sessionID, model.ApprovalPending).
		Order("created_at asc").Find(&list).Error
	return list, err
}

// ExpireApprovals marks every pending approval whose deadline has passed as expired.
func (r *AIRepo) ExpireApprovals(now time.Time) (int64, error) {
	res := r.db.Model(&model.AIPendingApproval{}).
		Where("status = ? AND expires_at < ?", model.ApprovalPending, now).
		Update("status", model.ApprovalExpired)
	return res.RowsAffected, res.Error
}
