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

// Task distribution from the platform: plugin packages.
//
// The Agent is the executing side and owns no policy. It receives a finished
// artefact (a plugin archive), installs it, confirms that it is actually
// running, and reports back. Two design choices follow from that:
//
//   - Packages are pulled by the Agent over HTTP instead of pushed through the
//     gRPC stream. A plugin archive is a few megabytes; streaming it would mix
//     the plugin lifecycle with the file-distribution channel and give the Server
//     a reason to buffer whole files.
//   - Every result reports the *observed* state, not the requested one. "Package
//     installed" and "plugin running" are different facts, and only the second
//     one is worth showing an operator.
//
// The built-in log pipeline was removed in v1.4.3; its LogTask handler went with
// it, so a Server that still sends one falls through to the unknown-field path.

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

// controls bundles the optional subsystems a Server task may address.
type controls struct {
	plugin PluginControl
}

// SetControls wires the subsystems that platform tasks act on. Called once by the
// router after they exist; a nil subsystem simply makes its tasks fail with a
// clear message instead of panicking.
func (c *Client) SetControls(pluginCtl PluginControl) {
	c.ctrl = controls{plugin: pluginCtl}
}

// SetControls is the same for the lifecycle manager, so a restart of the
// connection keeps the wiring.
func (m *Manager) SetControls(pluginCtl PluginControl) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctrl = controls{plugin: pluginCtl}
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
