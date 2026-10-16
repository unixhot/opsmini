package aiagent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// Human-in-the-loop errors.
var (
	// ErrApprovalNotFound is returned when the approval id is unknown.
	ErrApprovalNotFound = errors.New("审批记录不存在")
	// ErrApprovalExpired is returned when the approval deadline has passed.
	ErrApprovalExpired = errors.New("审批已超时，请重新发起")
	// ErrApprovalResolved is returned when the approval was already handled.
	ErrApprovalResolved = errors.New("该审批已处理")
)

// Approver manages suspended tool calls that are waiting for a human decision.
// The interrupt record is persisted, so a page reload or a restart does not lose
// the pending confirmation.
type Approver struct {
	repo *repository.AIRepo
	ttl  time.Duration
}

// NewApprover creates an Approver. ttl defaults to 10 minutes.
func NewApprover(repo *repository.AIRepo, ttl time.Duration) *Approver {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Approver{repo: repo, ttl: ttl}
}

// TTL returns the approval validity window.
func (a *Approver) TTL() time.Duration { return a.ttl }

// Create records a suspended tool call.
func (a *Approver) Create(sessionID string, userID uint, t *Tool, call ToolCall, args map[string]any, preview string) (*model.AIPendingApproval, error) {
	raw, _ := json.Marshal(args)
	rec := &model.AIPendingApproval{
		ID:         newApprovalID(),
		SessionID:  sessionID,
		UserID:     userID,
		ToolName:   t.Name,
		ToolCallID: call.ID,
		Args:       string(raw),
		Risk:       int(t.Risk),
		Preview:    preview,
		Status:     model.ApprovalPending,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(a.ttl),
	}
	if err := a.repo.CreateApproval(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// Load returns a pending approval owned by userID. An out-of-date record is
// marked expired before the error is returned.
func (a *Approver) Load(id string, userID uint) (*model.AIPendingApproval, error) {
	rec, err := a.repo.GetApproval(id)
	if err != nil {
		return nil, ErrApprovalNotFound
	}
	if rec.UserID != userID {
		return nil, ErrApprovalNotFound
	}
	switch rec.Status {
	case model.ApprovalPending:
		// still open
	case model.ApprovalExpired:
		return nil, ErrApprovalExpired
	default:
		return nil, ErrApprovalResolved
	}
	if time.Now().After(rec.ExpiresAt) {
		rec.Status = model.ApprovalExpired
		_ = a.repo.UpdateApproval(rec)
		return nil, ErrApprovalExpired
	}
	return rec, nil
}

// Resolve marks the approval as approved or rejected.
func (a *Approver) Resolve(rec *model.AIPendingApproval, approved bool) error {
	if approved {
		rec.Status = model.ApprovalApproved
	} else {
		rec.Status = model.ApprovalRejected
	}
	return a.repo.UpdateApproval(rec)
}

// Pending returns the open approvals of a session.
func (a *Approver) Pending(sessionID string) ([]model.AIPendingApproval, error) {
	return a.repo.ListPendingApprovals(sessionID)
}

// Sweep marks every approval whose deadline has passed as expired and returns
// how many records were affected.
func (a *Approver) Sweep() int64 {
	n, _ := a.repo.ExpireApprovals(time.Now())
	return n
}

// Args decodes the stored arguments of an approval.
func (a *Approver) Args(rec *model.AIPendingApproval) map[string]any {
	out := map[string]any{}
	if rec == nil || rec.Args == "" {
		return out
	}
	_ = json.Unmarshal([]byte(rec.Args), &out)
	return out
}

// newApprovalID generates a random approval identifier.
func newApprovalID() string {
	var buf [10]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "ap" + time.Now().Format("20060102150405.000000")
	}
	return "ap" + hex.EncodeToString(buf[:])
}
