package safety

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Quarantine moves a file into the quarantine area instead of deleting it.
//
// This is the single most important safety primitive in the design: an agent
// that cannot delete anything can still be wrong without being catastrophic.
func (m *Manager) Quarantine(incidentID, path, reason string) (*model.QuarantineItem, error) {
	clean := filepath.Clean(path)
	if clean == "/" || clean == "." || clean == "" {
		return nil, fmt.Errorf("拒绝隔离非法路径 %q", path)
	}
	if verdict, rule, why := m.CheckArg("quarantine_file", map[string]any{"path": clean}); verdict != "allow" {
		return nil, fmt.Errorf("%s（%s）", why, rule)
	}
	st, err := os.Lstat(clean)
	if err != nil {
		return nil, fmt.Errorf("目标不存在：%w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("暂不支持隔离目录，请指定文件")
	}
	dir, err := m.IncidentDir(incidentID, "quarantine")
	if err != nil {
		return nil, err
	}
	sum, size, err := hashFile(clean)
	if err != nil {
		return nil, err
	}
	stored := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(clean)))
	if err := copyFile(clean, stored); err != nil {
		return nil, err
	}
	// chmod 000 first (defangs an active webshell even if the move fails),
	// then remove the original: the copy in the quarantine dir is authoritative.
	_ = os.Chmod(clean, 0o000)
	owner := ""
	if fi, err := os.Stat(clean); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			owner = fmt.Sprintf("%d:%d", st.Uid, st.Gid)
		}
	}
	if err := os.Remove(clean); err != nil {
		return nil, fmt.Errorf("移入隔离区失败：%w", err)
	}
	item := &model.QuarantineItem{
		IncidentID:   incidentID,
		OriginalPath: clean,
		StoredPath:   stored,
		SHA256:       sum,
		Size:         size,
		Mode:         uint32(st.Mode().Perm()),
		Owner:        owner,
		Reason:       reason,
		CreatedAt:    time.Now(),
	}
	if m.DB != nil {
		if err := m.DB.Create(item).Error; err != nil {
			return item, err
		}
	}
	return item, nil
}

// RestoreQuarantine puts a quarantined file back where it came from.
func (m *Manager) RestoreQuarantine(id uint) (*model.QuarantineItem, error) {
	if m.DB == nil {
		return nil, fmt.Errorf("数据库不可用")
	}
	var item model.QuarantineItem
	if err := m.DB.First(&item, id).Error; err != nil {
		return nil, err
	}
	if item.RestoredAt != nil {
		return &item, fmt.Errorf("该文件已还原过")
	}
	if _, err := os.Stat(item.OriginalPath); err == nil {
		return &item, fmt.Errorf("原路径已存在同名文件，拒绝覆盖：%s", item.OriginalPath)
	}
	if err := copyFile(item.StoredPath, item.OriginalPath); err != nil {
		return &item, err
	}
	_ = os.Chmod(item.OriginalPath, os.FileMode(item.Mode))
	item.RestoredAt = nowPtr()
	m.DB.Model(&model.QuarantineItem{}).Where("id = ?", item.ID).
		Update("restored_at", item.RestoredAt)
	return &item, nil
}

// Snapshot stores a pre-change copy of a file or config.
func (m *Manager) Snapshot(incidentID, path, kind string) (*model.SnapshotRecord, error) {
	clean := filepath.Clean(path)
	dir, err := m.IncidentDir(incidentID, "snapshots")
	if err != nil {
		return nil, err
	}
	sum, size, err := hashFile(clean)
	if err != nil {
		return nil, err
	}
	stored := filepath.Join(dir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(clean)))
	if err := copyFile(clean, stored); err != nil {
		return nil, err
	}
	if kind == "" {
		kind = "file"
	}
	rec := &model.SnapshotRecord{
		IncidentID: incidentID, Path: clean, StoredPath: stored,
		Kind: kind, SHA256: sum, Size: size, CreatedAt: time.Now(),
	}
	if m.DB != nil {
		if err := m.DB.Create(rec).Error; err != nil {
			return rec, err
		}
	}
	return rec, nil
}

// RestoreSnapshot rolls a file back to a stored snapshot.
func (m *Manager) RestoreSnapshot(id uint) (*model.SnapshotRecord, error) {
	if m.DB == nil {
		return nil, fmt.Errorf("数据库不可用")
	}
	var rec model.SnapshotRecord
	if err := m.DB.First(&rec, id).Error; err != nil {
		return nil, err
	}
	if rec.RolledBack {
		return &rec, fmt.Errorf("该快照已回滚过")
	}
	if err := copyFile(rec.StoredPath, rec.Path); err != nil {
		return &rec, err
	}
	rec.RolledBack = true
	rec.RolledBackAt = nowPtr()
	m.DB.Model(&model.SnapshotRecord{}).Where("id = ?", rec.ID).
		Updates(map[string]any{"rolled_back": true, "rolled_back_at": rec.RolledBackAt})
	return &rec, nil
}

// --- IP blocking -------------------------------------------------------------

// BlockIP adds a temporary firewall drop rule and records its expiry so the
// ban heals itself even if nobody ever unblocks it.
func (m *Manager) BlockIP(incidentID, ip string, ttl time.Duration, reason string) (*model.BlockedIP, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" || strings.ContainsAny(ip, " /;|&$`") {
		return nil, fmt.Errorf("非法 IP：%q", ip)
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	if err := runCmd("iptables", "-w", "-I", "INPUT", "-s", ip, "-j", "DROP"); err != nil {
		// Fall back to nftables when iptables is not installed.
		if err2 := runCmd("nft", "add", "rule", "inet", "filter", "input", "ip", "saddr", ip, "drop"); err2 != nil {
			return nil, fmt.Errorf("封禁失败（iptables: %v；nft: %v）", err, err2)
		}
	}
	rec := &model.BlockedIP{
		IP: ip, Reason: reason, IncidentID: incidentID,
		Source: "agent", ExpiresAt: time.Now().Add(ttl), CreatedAt: time.Now(),
	}
	if m.DB != nil {
		if err := m.DB.Create(rec).Error; err != nil {
			return rec, err
		}
	}
	return rec, nil
}

// UnblockIP removes the drop rule and marks the record released.
func (m *Manager) UnblockIP(ip string) error {
	var lastErr error
	if err := runCmd("iptables", "-w", "-D", "INPUT", "-s", ip, "-j", "DROP"); err != nil {
		if err2 := runCmd("nft", "delete", "rule", "inet", "filter", "input", "ip", "saddr", ip, "drop"); err2 != nil {
			lastErr = fmt.Errorf("解封失败（iptables: %v；nft: %v）", err, err2)
		}
	}
	if m.DB != nil {
		m.DB.Model(&model.BlockedIP{}).
			Where("ip = ? AND released_at IS NULL", ip).
			Updates(map[string]any{"released_at": time.Now()})
	}
	return lastErr
}

// SweepBlocks releases expired bans and returns how many were released.
func (m *Manager) SweepBlocks() int {
	if m.DB == nil {
		return 0
	}
	var list []model.BlockedIP
	m.DB.Where("released_at IS NULL AND expires_at < ?", time.Now()).Find(&list)
	n := 0
	for _, b := range list {
		if err := m.UnblockIP(b.IP); err == nil {
			n++
		}
	}
	return n
}

// --- verification ------------------------------------------------------------

// Assertion is one post-action check. It is intentionally a tiny DSL instead of
// a script: an agent cannot invent a check that always passes.
type Assertion struct {
	Type    string  `json:"type"`
	Path    string  `json:"path,omitempty"`
	Unit    string  `json:"unit,omitempty"`
	Port    int     `json:"port,omitempty"`
	Family  string  `json:"family,omitempty"`
	Pattern string  `json:"pattern,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Window  string  `json:"window,omitempty"`
}

// Verify runs the assertions and returns whether all passed plus details.
func (m *Manager) Verify(ctx context.Context, incidentID string, list []Assertion) (bool, []string) {
	var details []string
	ok := true
	for _, a := range list {
		pass, detail := m.checkOne(ctx, incidentID, a)
		if !pass {
			ok = false
		}
		details = append(details, fmt.Sprintf("%s: %v（%s）", a.Type, pass, detail))
	}
	return ok, details
}

func (m *Manager) checkOne(ctx context.Context, incidentID string, a Assertion) (bool, string) {
	switch a.Type {
	case "service_active":
		out, err := exec.CommandContext(ctx, "systemctl", "is-active", a.Unit).Output()
		if err != nil {
			return false, "systemctl is-active 失败: " + err.Error()
		}
		s := strings.TrimSpace(string(out))
		return s == "active", "状态 " + s
	case "port_listen":
		return portListening(a.Port), "端口 " + fmt.Sprint(a.Port)
	case "process_absent":
		found := ""
		entries, _ := os.ReadDir("/proc")
		for _, e := range entries {
			if len(found) > 0 {
				break
			}
			var pid int
			if _, err := fmt.Sscanf(e.Name(), "%d", &pid); err != nil {
				continue
			}
			comm := procName(pid)
			if a.Pattern != "" && strings.Contains(comm, a.Pattern) {
				found = comm
			}
		}
		return found == "", "匹配到 " + found
	case "no_new_signal":
		if m.DB == nil {
			return true, "无数据库，跳过"
		}
		window := 10 * time.Minute
		if a.Window != "" {
			if d, err := time.ParseDuration(a.Window); err == nil {
				window = d
			}
		}
		// Count only signals that still belong to an UNHANDLED incident:
		//   - this incident's own signals are the reason we ran, not a regression;
		//   - a previous, already closed incident of the same family is history.
		// Signals without an incident (brand new, not yet folded) count as new.
		var rows []model.Signal
		q := m.DB.Model(&model.Signal{}).
			Where("family = ? AND observed_at > ?", a.Family, time.Now().Add(-window))
		if incidentID != "" {
			q = q.Where("incident_id <> ?", incidentID)
		}
		q.Find(&rows)

		seen := map[string]bool{}
		pending := 0
		for _, r := range rows {
			if r.IncidentID == "" {
				pending++
				continue
			}
			if seen[r.IncidentID] {
				continue
			}
			seen[r.IncidentID] = true
			var inc struct {
				Status   string
				Severity string
			}
			m.DB.Model(&model.Incident{}).Select("status, severity").
				Where("id = ?", r.IncidentID).Scan(&inc)
			switch inc.Status {
			case model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning:
				// Informational observations (a normal login, a routine file change)
				// must not make verification fail: they are below every trigger
				// threshold by definition.
				if model.SeverityRank(inc.Severity) >= model.SeverityRank(model.SeverityMedium) {
					pending++
				}
			}
		}
		return pending == 0, fmt.Sprintf("%s 内仍有 %d 个未处理的同类事件", window, pending)
	case "disk_usage_lt":
		used, total := diskUsage(a.Path)
		if total == 0 {
			return false, "无法读取磁盘用量"
		}
		pct := float64(used) / float64(total) * 100
		return pct < a.Value, fmt.Sprintf("%.1f%% < %.0f%%", pct, a.Value)
	}
	return false, "未知断言类型"
}

// --- helpers -----------------------------------------------------------------

// statfs is a thin wrapper so the rest of the file stays syscall-agnostic.
func statfs(path string, st *syscall.Statfs_t) error { return syscall.Statfs(path, st) }

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func runCmd(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// portListening checks /proc/net/tcp* for a LISTEN socket on the given port.
func portListening(port int) bool {
	want := fmt.Sprintf(":%04X", port)
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(b), "\n") {
			fields := strings.Fields(ln)
			if len(fields) < 4 {
				continue
			}
			if fields[3] != "0A" { // 0A = LISTEN
				continue
			}
			if strings.HasSuffix(fields[1], want) {
				return true
			}
		}
	}
	return false
}

// diskUsage returns used and total bytes for the filesystem holding path.
func diskUsage(path string) (uint64, uint64) {
	var st syscall.Statfs_t
	if err := statfs(path, &st); err != nil {
		return 0, 0
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	return total - free, total
}
