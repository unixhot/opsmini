package safety

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Run-level rollback: a run is a transaction.
//
// A single tool call being reversible is not enough. A run that quarantines a file,
// blocks an address and truncates a log, then fails verification, leaves the host in
// a state that matches neither before nor after — and no human can reconstruct it
// from the chat transcript. This journal records each reversible write in order so the
// whole run can be undone as one unit, newest first.
//
// Scope is honest: it undoes what was recorded as undoable. `exec_command` cannot be
// undone, so it is journaled as non-undoable and reported as such — pretending
// otherwise would be worse than admitting the limit.

// Journal records one write so it can be undone later. refID points at the row that
// knows how to undo it (quarantine item, snapshot record, ...).
func (m *Manager) Journal(sessionID, op, target string, refID uint, undoable bool) {
	if m.DB == nil || sessionID == "" {
		return
	}
	var seq int64
	m.DB.Model(&model.ActionJournal{}).Where("session_id = ?", sessionID).Count(&seq)
	row := model.ActionJournal{
		SessionID: sessionID,
		Seq:       int(seq) + 1,
		Op:        op,
		Target:    target,
		RefID:     refID,
		Undoable:  undoable,
		CreatedAt: time.Now(),
	}
	if err := m.DB.Create(&row).Error; err != nil {
		// A missing journal row means a lost undo, so say so rather than swallow it.
		fmt.Printf("safety: journal %s/%s: %v\n", sessionID, op, err)
	}
}

// RollbackStep is one attempted undo.
type RollbackStep struct {
	Op      string `json:"op"`
	Target  string `json:"target"`
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped"`
	Detail  string `json:"detail"`
}

// RollbackSessions undoes every reversible write made by the given sessions, newest
// first. Non-undoable writes are reported as skipped so the caller can tell the user
// exactly what remains.
func (m *Manager) RollbackSessions(sessionIDs []string) []RollbackStep {
	if m.DB == nil || len(sessionIDs) == 0 {
		return nil
	}
	var rows []model.ActionJournal
	m.DB.Where("session_id IN ? AND undone = ?", sessionIDs, false).
		Order("id DESC").Find(&rows)

	steps := make([]RollbackStep, 0, len(rows))
	for _, r := range rows {
		step := m.undo(r)
		steps = append(steps, step)
		// Undone or skipped both mean "nothing more to try here"; only a real
		// failure stays pending so a later manual rollback can retry it.
		if step.OK || step.Skipped {
			m.DB.Model(&model.ActionJournal{}).Where("id = ?", r.ID).
				Updates(map[string]any{"undone": true, "undone_at": time.Now(), "undo_error": step.Detail})
		} else {
			m.DB.Model(&model.ActionJournal{}).Where("id = ?", r.ID).Update("undo_error", step.Detail)
		}
	}
	return steps
}

// undo performs one inverse operation.
func (m *Manager) undo(r model.ActionJournal) RollbackStep {
	step := RollbackStep{Op: r.Op, Target: r.Target}
	if !r.Undoable {
		step.Skipped = true
		step.Detail = "该操作不可撤销，需人工处理"
		return step
	}
	switch r.Op {
	case "quarantine":
		if _, err := m.RestoreQuarantine(r.RefID); err != nil {
			step.Detail = err.Error()
			return step
		}
		step.OK = true
		step.Detail = "已还原 " + r.Target
	case "block_ip":
		if err := m.UnblockIP(r.Target); err != nil {
			step.Detail = err.Error()
			return step
		}
		step.OK = true
		step.Detail = "已解封 " + r.Target
	case "truncate_log":
		if _, err := m.RestoreSnapshot(r.RefID); err != nil {
			step.Detail = err.Error()
			return step
		}
		step.OK = true
		step.Detail = "已从快照恢复 " + r.Target
	case "freeze_process":
		pid, err := strconv.Atoi(r.Target)
		if err != nil {
			step.Detail = "进程号无效：" + r.Target
			return step
		}
		if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
			if err == syscall.ESRCH {
				step.Skipped = true
				step.Detail = "进程已不存在"
				return step
			}
			step.Detail = err.Error()
			return step
		}
		step.OK = true
		step.Detail = "已恢复进程 " + r.Target
	default:
		step.Skipped = true
		step.Detail = "未知操作类型，需人工处理"
	}
	return step
}

// JournalFor returns the recorded writes for a set of sessions, oldest first, for
// display in the panel.
func (m *Manager) JournalFor(sessionIDs []string) []model.ActionJournal {
	if m.DB == nil || len(sessionIDs) == 0 {
		return nil
	}
	var rows []model.ActionJournal
	m.DB.Where("session_id IN ?", sessionIDs).Order("id").Find(&rows)
	return rows
}

// FileStillExists reports whether a path still exists — used by the shadow reviewer
// to score whether a proposed quarantine would still be needed.
func FileStillExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
