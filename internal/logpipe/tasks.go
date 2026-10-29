package logpipe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Task distribution.
//
// The platform owns task files; OpsMini only writes what it is given. Two rules
// shape the implementation:
//
//  1. Idempotent. A dispatch that repeats the same content must not rewrite the
//     file, because a rewrite changes the hash and restarts the harvester, losing
//     the multiline buffer and reopening every file.
//  2. Verifiable. "File written" is not "task running". The content is parsed
//     before it is installed, and a task that fails to parse leaves the previous
//     version in place instead of taking log collection down.

// TaskFileValid reports whether a filename is acceptable for tasks.d.
//
// Only a bare name ending in .yml/.yaml: a filename is the one field a remote
// caller could use to write outside the directory, and the platform has no reason
// to send a path.
func TaskFileValid(name string) error {
	if name == "" {
		return errors.New("任务文件名为空")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("任务文件名不能包含路径：%s", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("任务文件名不能以点开头：%s", name)
	}
	if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
		return fmt.Errorf("任务文件必须以 .yml 或 .yaml 结尾：%s", name)
	}
	return nil
}

// ApplyTask writes one task file and reloads the pipeline.
//
// Returns "unchanged" when the on-disk content already has the requested digest.
func (m *Manager) ApplyTask(filename string, content []byte, wantSHA string, strict bool) (status string, warnings []string, err error) {
	if err := TaskFileValid(filename); err != nil {
		return "failed", nil, err
	}
	if len(content) == 0 {
		return "failed", nil, errors.New("任务内容为空")
	}
	// The caller may send a digest to prove the transport did not corrupt the
	// payload; verify it when present.
	sum := sha256.Sum256(content)
	got := hex.EncodeToString(sum[:])
	if wantSHA != "" && !strings.EqualFold(wantSHA, got) {
		return "failed", nil, fmt.Errorf("内容摘要不匹配：期望 %s，实际 %s", wantSHA, got)
	}

	// A task arriving is itself the request to collect: wake the pipeline up before
	// the reload below, otherwise the task loads with no sender behind it and
	// nothing ever reaches the output.
	m.Enable()

	if err := os.MkdirAll(m.cfg.TasksDir, 0o755); err != nil {
		return "failed", nil, err
	}
	path := filepath.Join(m.cfg.TasksDir, filename)

	prev, prevErr := os.ReadFile(path)
	if prevErr == nil {
		prevSum := sha256.Sum256(prev)
		if hex.EncodeToString(prevSum[:]) == got {
			// Same content: nothing to do. Reporting "applied" here would make the
			// platform believe a reload happened.
			return "unchanged", nil, nil
		}
	}

	// Parse before installing so a broken task never reaches the directory.
	tasks, ws, perr := ParseTask(content, path)
	if perr != nil {
		return "failed", nil, perr
	}
	if len(tasks) == 0 {
		return "failed", nil, errors.New("配置中没有可用的 input")
	}
	if strict && len(ws) > 0 {
		return "failed", ws, fmt.Errorf("strict 模式下存在不受支持的字段：%s", strings.Join(ws, "; "))
	}

	if err := writeFileAtomic(path, content, 0o644); err != nil {
		return "failed", nil, err
	}
	m.reload()

	// Confirm the task actually loaded. A file that parses can still be dropped by
	// the pipeline (duplicate id, unreadable path), and silently reporting success
	// for a task that is not collecting is the exact failure this protocol exists
	// to prevent.
	loaded := false
	m.mu.RLock()
	for _, t := range tasks {
		if _, ok := m.tasks[t.ID]; ok {
			loaded = true
			break
		}
	}
	m.mu.RUnlock()
	if !loaded {
		// Roll back so the host keeps collecting whatever it collected before.
		if prevErr == nil {
			_ = writeFileAtomic(path, prev, 0o644)
		} else {
			_ = os.Remove(path)
		}
		m.reload()
		return "failed", ws, errors.New("任务未能加载，已恢复上一版配置")
	}
	return "applied", ws, nil
}

// RemoveTask deletes one task file and reloads. Removing a file that is not there
// is not an error: dispatch may be retried after a network failure.
func (m *Manager) RemoveTask(filename string) error {
	if err := TaskFileValid(filename); err != nil {
		return err
	}
	path := filepath.Join(m.cfg.TasksDir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.reload()
	log.Printf("logpipe: 任务文件 %s 已移除", filename)
	return nil
}

// TaskByFile finds the running task loaded from a given file name.
func (m *Manager) TaskByFile(filename string) (TaskStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rt := range m.tasks {
		if filepath.Base(rt.task.File) != filename {
			continue
		}
		st := statsOf(rt.task.ID)
		return TaskStatus{
			TaskID:        rt.task.ID,
			File:          filepath.Base(rt.task.File),
			Paths:         rt.task.Input.Paths,
			State:         rt.state,
			BytesRead:     atomic.LoadInt64(&st.bytesRead),
			EventsSent:    atomic.LoadInt64(&st.eventsSent),
			EventsDropped: atomic.LoadInt64(&st.eventsDropped),
			UpdatedAt:     rt.task.LoadedAt,
		}, true
	}
	return TaskStatus{}, false
}

// writeFileAtomic writes content to path via a temporary file in the same
// directory, so a crash mid-write cannot leave a truncated task file behind.
func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".task-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
