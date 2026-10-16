package aiagent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// ErrSessionNotFound is returned when a session id does not exist.
var ErrSessionNotFound = errors.New("会话不存在")

// Store persists agent conversations in SQLite. Keeping the transcript in the
// database (instead of only in the browser) is what makes multi-turn memory and
// resumed runs possible.
type Store struct {
	repo *repository.AIRepo
	// keepVerbatim is the number of newest messages kept verbatim while
	// compressing the transcript; older ones are folded into a summary.
	keepVerbatim int
	// summarize is the optional LLM-backed compressor. When nil, old messages are
	// dropped with a short deterministic marker instead.
	summarize func(prompt string) (string, error)
}

// NewStore creates a Store.
func NewStore(repo *repository.AIRepo, summarize func(string) (string, error)) *Store {
	return &Store{repo: repo, keepVerbatim: 24, summarize: summarize}
}

// Repository exposes the underlying repository (used by the API layer).
func (s *Store) Repository() *repository.AIRepo { return s.repo }

// EnsureSession returns the session, creating it when id is empty or unknown.
func (s *Store) EnsureSession(id string, userID uint, title string) (*model.AISession, error) {
	if id != "" {
		if sess, err := s.repo.GetSession(id); err == nil {
			return sess, nil
		}
	}
	newID := id
	if newID == "" {
		newID = newSessionID()
	}
	if title == "" {
		title = "新会话"
	}
	sess := &model.AISession{ID: newID, UserID: userID, Title: truncateText(title, 60)}
	if err := s.repo.CreateSession(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// GetSession loads a session owned by userID.
func (s *Store) GetSession(id string, userID uint) (*model.AISession, error) {
	sess, err := s.repo.GetSession(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	if sess.UserID != userID {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// ListSessions returns the user's sessions.
func (s *Store) ListSessions(userID uint, limit int) ([]model.AISession, error) {
	return s.repo.ListSessions(userID, limit)
}

// DeleteSession removes a session owned by userID.
func (s *Store) DeleteSession(id string, userID uint) error {
	if _, err := s.GetSession(id, userID); err != nil {
		return err
	}
	return s.repo.DeleteSession(id)
}

// History returns the stored transcript of a session as LLM messages.
func (s *Store) History(sessionID string, limit int) ([]Message, error) {
	rows, err := s.repo.ListMessages(sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromModel(r))
	}
	return out, nil
}

// Append writes messages to the transcript.
func (s *Store) Append(sessionID string, msgs ...Message) error {
	if len(msgs) == 0 {
		return nil
	}
	rows := make([]model.AIMessage, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, toModel(m))
	}
	return s.repo.AppendMessages(sessionID, rows)
}

// Touch updates the session bookkeeping after a run.
func (s *Store) Touch(sess *model.AISession, steps, tools int) {
	if sess == nil {
		return
	}
	sess.Steps += steps
	sess.ToolsUsed += tools
	_ = s.repo.UpdateSession(sess)
}

// Count returns the number of stored messages in a session.
func (s *Store) Count(sessionID string) int64 {
	n, err := s.repo.CountMessages(sessionID)
	if err != nil {
		return 0
	}
	return n
}

// Compress folds the oldest part of the transcript into a single summary message
// so long conversations do not blow up the model context.
//
// It keeps the newest keepVerbatim messages untouched and replaces everything
// older with one system message describing what happened before.
func (s *Store) Compress(sessionID string) error {
	keep := s.keepVerbatim
	if keep <= 0 {
		keep = 24
	}
	// Reserve one slot for the summary message itself.
	dropped, err := s.repo.TrimMessages(sessionID, keep)
	if err != nil || len(dropped) == 0 {
		return err
	}

	summary := s.buildSummary(dropped)
	if summary == "" {
		return nil
	}
	row := model.AIMessage{Role: model.AIRoleSystem, Content: summary}
	rows := []model.AIMessage{row}
	if err := s.repo.AppendMessages(sessionID, rows); err != nil {
		return err
	}
	return nil
}

// buildSummary turns dropped messages into a compact digest.
func (s *Store) buildSummary(dropped []model.AIMessage) string {
	var transcript string
	for _, m := range dropped {
		switch m.Role {
		case model.AIRoleUser:
			transcript += "用户：" + truncateText(m.Content, 300) + "\n"
		case model.AIRoleAssistant:
			if m.Content != "" {
				transcript += "助手：" + truncateText(m.Content, 300) + "\n"
			}
			if m.ToolCalls != "" {
				transcript += "调用工具：" + truncateText(m.ToolCalls, 200) + "\n"
			}
		case model.AIRoleTool:
			transcript += "工具结果：" + truncateText(m.Content, 300) + "\n"
		case model.AIRoleSystem:
			transcript += truncateText(m.Content, 200) + "\n"
		}
	}
	if s.summarize != nil {
		if out, err := s.summarize(transcript); err == nil && out != "" {
			return "【历史对话摘要】" + out
		}
	}
	return "【历史对话摘要】早期对话已折叠，摘要如下：\n" + truncateText(transcript, 2000)
}

// toModel converts an LLM message into a persisted row.
func toModel(m Message) model.AIMessage {
	row := model.AIMessage{
		Role:       m.Role,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
		Name:       m.Name,
	}
	if len(m.ToolCalls) > 0 {
		if b, err := json.Marshal(m.ToolCalls); err == nil {
			row.ToolCalls = string(b)
		}
	}
	return row
}

// fromModel converts a persisted row back into an LLM message.
func fromModel(r model.AIMessage) Message {
	m := Message{
		Role:       r.Role,
		Content:    r.Content,
		ToolCallID: r.ToolCallID,
		Name:       r.Name,
	}
	if r.ToolCalls != "" {
		var calls []ToolCall
		if err := json.Unmarshal([]byte(r.ToolCalls), &calls); err == nil {
			m.ToolCalls = calls
		}
	}
	return m
}

// newSessionID generates a short unique session identifier.
func newSessionID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("s%d", time.Now().UnixNano())
	}
	return "s" + strconv.FormatInt(time.Now().Unix(), 36) + hex.EncodeToString(buf[:])
}
