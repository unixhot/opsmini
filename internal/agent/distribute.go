package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// Task distribution from the platform: log collection tasks and plugin packages.
//
// The Agent is the executing side and owns no policy. It receives a finished
// artefact (a filebeat YAML, a plugin archive), installs it, confirms that it is
// actually running, and reports back. Two design choices follow from that:
//
//   - Packages are pulled by the Agent over HTTP instead of pushed through the
//     gRPC stream. A plugin archive is a few megabytes; streaming it would mix
//     the plugin lifecycle with the file-distribution channel and give the Server
//     a reason to buffer whole files.
//   - Every result reports the *observed* state, not the requested one. "Config
//     written" and "task collecting" are different facts, and only the second one
//     is worth showing an operator.

// LogControl is the subset of the log pipeline the platform may drive.
type LogControl interface {
	// ApplyTask installs a task file. status is applied / unchanged / failed.
	ApplyTask(filename string, content []byte, sha string, strict bool) (status string, warnings []string, err error)
	// RemoveTask deletes a task file (missing file is not an error).
	RemoveTask(filename string) error
	// ReloadNow rescans the task directory.
	ReloadNow()
	// TaskByFile reports the live state of the task loaded from a file.
	TaskByFile(filename string) (LogState, bool)
}

// PluginControl is the subset of the plugin manager the platform may drive.
type PluginControl interface {
	// Install verifies digest and signature, then installs from a local archive.
	Install(pkgPath, wantSHA, sig string) (id, version string, err error)
	Remove(id string, purge bool) error
	Enable(id string, on bool) error
	Restart(id string) error
	// State reports the current state of a plugin, if present.
	State(id string) (state, version, lastErr string, ok bool)
}

// LogState mirrors the fields of a log task the platform cares about.
type LogState struct {
	TaskID        string
	State         string
	FilesOpen     int32
	BytesRead     int64
	EventsSent    int64
	EventsDropped int64
	QueueDepth    int64
	LastError     string
	Output        string
}

// controls bundles the optional subsystems a Server task may address.
type controls struct {
	log    LogControl
	plugin PluginControl
}

// SetControls wires the subsystems that platform tasks act on. Called once by the
// router after they exist; nil subsystems simply make their tasks fail with a
// clear message instead of panicking.
func (c *Client) SetControls(logCtl LogControl, pluginCtl PluginControl) {
	c.ctrl = controls{log: logCtl, plugin: pluginCtl}
}

// SetControls is the same for the lifecycle manager, so a restart of the
// connection keeps the wiring.
func (m *Manager) SetControls(logCtl LogControl, pluginCtl PluginControl) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctrl = controls{log: logCtl, plugin: pluginCtl}
}

// handleLogTask applies one log task and reports the result.
func (c *Client) handleLogTask(ctx context.Context, t *agentpb.LogTask, sendCh chan<- *agentpb.AgentMessage) {
	res := &agentpb.LogTaskResult{JobId: t.JobId, TaskId: t.TaskId}
	switch strings.ToLower(t.Action) {
	case "apply":
		if c.ctrl.log == nil {
			res.Status, res.Error = "failed", "本机未启用日志管道"
			break
		}
		status, warns, err := c.ctrl.log.ApplyTask(t.Filename, t.Content, t.Sha256, t.Strict)
		res.Status, res.Warnings = status, warns
		if err != nil {
			res.Status, res.Error = "failed", err.Error()
		}
	case "remove":
		if c.ctrl.log == nil {
			res.Status, res.Error = "failed", "本机未启用日志管道"
			break
		}
		if err := c.ctrl.log.RemoveTask(t.Filename); err != nil {
			res.Status, res.Error = "failed", err.Error()
		} else {
			res.Status = "removed"
		}
	case "reload":
		if c.ctrl.log == nil {
			res.Status, res.Error = "failed", "本机未启用日志管道"
			break
		}
		c.ctrl.log.ReloadNow()
		res.Status = "applied"
	case "status":
		if c.ctrl.log == nil {
			res.Status, res.Error = "failed", "本机未启用日志管道"
			break
		}
		st, ok := c.ctrl.log.TaskByFile(t.Filename)
		if !ok {
			res.Status = "failed"
			res.Error = "任务未加载"
			break
		}
		res.Status = "running"
		res.State = &agentpb.LogTaskState{
			TaskId:        st.TaskID,
			State:         st.State,
			FilesOpen:     st.FilesOpen,
			BytesRead:     st.BytesRead,
			EventsSent:    st.EventsSent,
			EventsDropped: st.EventsDropped,
			QueueDepth:    st.QueueDepth,
			LastError:     st.LastError,
			Output:        st.Output,
		}
	default:
		res.Status, res.Error = "failed", "未知动作："+t.Action
	}
	sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_LogResult{LogResult: res}}
}

// handlePluginTask performs one plugin action and reports the result.
func (c *Client) handlePluginTask(ctx context.Context, t *agentpb.PluginTask, sendCh chan<- *agentpb.AgentMessage) {
	res := &agentpb.PluginResult{JobId: t.JobId, PluginId: t.PluginId}
	fail := func(err error) {
		res.Status, res.Error = "failed", err.Error()
		sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_PluginResult{PluginResult: res}}
	}
	if c.ctrl.plugin == nil {
		fail(fmt.Errorf("本机未启用插件子系统"))
		return
	}
	switch strings.ToLower(t.Action) {
	case "install", "upgrade":
		pkg, err := c.downloadPackage(ctx, t.SourceUrl, t.Sha256)
		if err != nil {
			fail(err)
			return
		}
		defer os.Remove(pkg)
		id, ver, err := c.ctrl.plugin.Install(pkg, t.Sha256, t.Signature)
		if err != nil {
			fail(err)
			return
		}
		res.PluginId, res.Version = id, ver
		// Installed but disabled is not running; say which one it is.
		if err := c.ctrl.plugin.Enable(id, true); err != nil {
			res.Status, res.Error = "failed", "安装成功但启动失败："+err.Error()
			break
		}
		res.Status = "ok"
		res.State = "running"
	case "remove":
		purge := !t.KeepData
		if err := c.ctrl.plugin.Remove(t.PluginId, purge); err != nil {
			fail(err)
			return
		}
		res.Status, res.State = "ok", "removed"
	case "enable", "disable":
		on := strings.EqualFold(t.Action, "enable")
		if err := c.ctrl.plugin.Enable(t.PluginId, on); err != nil {
			fail(err)
			return
		}
		res.Status = "ok"
		if on {
			res.State = "running"
		} else {
			res.State = "stopped"
		}
	case "restart":
		if err := c.ctrl.plugin.Restart(t.PluginId); err != nil {
			fail(err)
			return
		}
		res.Status, res.State = "ok", "running"
	case "status":
		st, ver, lastErr, ok := c.ctrl.plugin.State(t.PluginId)
		if !ok {
			res.Status, res.Error = "failed", "插件未安装"
			break
		}
		res.Status, res.State, res.Version = "ok", st, ver
		if lastErr != "" {
			res.Error = lastErr
		}
	default:
		fail(fmt.Errorf("未知动作：%s", t.Action))
		return
	}
	sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_PluginResult{PluginResult: res}}
}

// downloadPackage fetches a plugin archive to a temporary file and verifies its
// digest. The digest is checked here as well as in the plugin manager so a
// truncated transfer is rejected before anything is unpacked.
func (c *Client) downloadPackage(ctx context.Context, rawURL, wantSHA string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("插件包地址为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("插件包地址无效：%w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("插件包地址必须是 http/https：%s", rawURL)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载插件包失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载插件包失败：HTTP %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "opsmini-plugin-*.tar.gz")
	if err != nil {
		return "", err
	}
	name := f.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 512<<20)); err != nil {
		f.Close()
		os.Remove(name)
		return "", fmt.Errorf("写入插件包失败：%w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" && !strings.EqualFold(wantSHA, got) {
		os.Remove(name)
		return "", fmt.Errorf("插件包摘要不匹配：期望 %s，实际 %s", wantSHA, got)
	}
	log.Printf("agent: 插件包已下载 %s (%s)", filepath.Base(name), got[:12])
	return name, nil
}
