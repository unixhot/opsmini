// Package backup implements the local backup/restore engine.
//
// Two promises shape the design:
//   - a backup can be one click, because it only reads and writes;
//   - a restore cannot be a bare one click: it overwrites production data, so it
//     always takes a pre-restore snapshot, verifies, and rolls back on failure.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Object is one thing to back up.
type Object struct {
	Kind string `json:"kind"` // dir | mysql | redis
	Path string `json:"path"` // directory for dir, socket/host for mysql
	Name string `json:"name"` // logical name, also the archive file stem
	// MySQL specifics
	Database string `json:"database,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"` // never written to the manifest
}

// Job is a backup definition (persisted in the cron job's config).
type Job struct {
	Name      string   `json:"name"`
	Objects   []Object `json:"objects"`
	TargetDir string   `json:"target_dir"` // where snapshots are written
	Retention int      `json:"retention"`  // keep N snapshots, 0 = keep all
	// Remote targets to upload the finished snapshot to (names registered via
	// SetStores). Uploads happen after the snapshot is complete on disk, so a
	// failed upload never leaves a half-written backup on the remote side.
	Remote []string `json:"remote,omitempty"`
	// KeepLocal controls whether the local copy survives a successful upload.
	// Default true: the local copy is what makes a restore fast.
	KeepLocal *bool `json:"keep_local,omitempty"`
}

// Manifest describes one snapshot.
type Manifest struct {
	Name      string          `json:"name"`
	Job       string          `json:"job"`
	CreatedAt time.Time       `json:"created_at"`
	Hostname  string          `json:"hostname"`
	Version   string          `json:"version"`
	Objects   []ManifestEntry `json:"objects"`
	TotalSize int64           `json:"total_size"`
}

// ManifestEntry is one archived object.
type ManifestEntry struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Note   string `json:"note,omitempty"`
}

// Manager runs backups and restores.
type Manager struct {
	root    string // default target root: <data>/backups
	version string
	// mu guards stores; the backup engine itself is stateless per run.
	mu sync.RWMutex
	// stores are the configured off-host targets, keyed by name.
	stores map[string]RemoteStore
	// RunCmd is injectable so tests do not need mysqldump.
	RunCmd func(name string, args ...string) ([]byte, error)
}

// NewManager builds a manager rooted at the given directory.
func NewManager(root, version string) *Manager {
	_ = os.MkdirAll(root, 0o700)
	return &Manager{root: root, version: version, stores: map[string]RemoteStore{}, RunCmd: func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}}
}

// Root returns the default snapshot directory.
func (m *Manager) Root() string { return m.root }

// Run creates one snapshot and applies the retention policy.
//
// The named return values exist solely so the deferred history write can see the
// outcome: failed runs leave no snapshot behind, so without a record they would be
// completely invisible.
func (m *Manager) Run(job Job) (man *Manifest, err error) {
	rec := RunRecord{Job: strings.TrimSpace(job.Name), StartedAt: time.Now()}
	defer func() {
		rec.EndedAt = time.Now()
		rec.OK = err == nil
		if err != nil {
			rec.Error = err.Error()
		}
		if man != nil {
			rec.Snapshot = man.Name
			rec.Size = man.TotalSize
			rec.Objects = len(man.Objects)
		}
		m.record(rec)
	}()

	job.Name = strings.TrimSpace(job.Name)
	if job.Name == "" {
		return nil, errors.New("备份任务缺少名称")
	}
	if len(job.Objects) == 0 {
		return nil, errors.New("备份任务没有对象")
	}
	// Validate every object before creating the snapshot directory. Without this a
	// typo in the second of five paths fails after four archives have already been
	// written, leaving a half-finished snapshot to clean up.
	if err := validateObjects(job.Objects); err != nil {
		return nil, err
	}
	target := job.TargetDir
	if target == "" {
		target = m.root
	}
	hostname, _ := os.Hostname()
	name := fmt.Sprintf("%s-%s", sanitize(job.Name), time.Now().Format("20060102-150405"))
	// Two backups inside the same second would otherwise share a directory and
	// silently overwrite each other.
	dir := filepath.Join(target, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%s-%d", sanitize(job.Name), time.Now().Format("20060102-150405"), i)
		dir = filepath.Join(target, name)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
		return nil, err
	}

	man = &Manifest{
		Name: name, Job: job.Name, CreatedAt: time.Now(), Hostname: hostname, Version: m.version,
	}
	for _, obj := range job.Objects {
		entry, err := m.backupObject(obj, dir)
		if err != nil {
			// A failed object invalidates the whole snapshot: a partial backup that
			// looks complete is worse than no backup at all.
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("%s: %w", obj.Name, err)
		}
		man.Objects = append(man.Objects, *entry)
		man.TotalSize += entry.Size
	}
	body, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), body, 0o600); err != nil {
		return nil, err
	}
	m.applyRetention(target, job.Retention)

	// Upload after the snapshot is complete on disk, never during: a failed upload
	// must not leave a partial backup on the remote side.
	for _, storeName := range job.Remote {
		st, serr := m.store(storeName)
		if serr != nil {
			return man, serr
		}
		if uerr := m.uploadSnapshot(context.Background(), st, dir, name, job.Retention); uerr != nil {
			return man, fmt.Errorf("上传到 %s 失败：%w", storeName, uerr)
		}
	}
	if len(job.Remote) > 0 && job.KeepLocal != nil && !*job.KeepLocal {
		// The whole reason to drop the local copy is disk space; the remote copy has
		// already been verified by a successful upload.
		_ = os.RemoveAll(dir)
	}
	return man, nil
}

// validateObjects is the backup preflight: it answers "can this job run at all"
// without touching the disk. Messages name the offending object so a cron failure
// notification is actionable on its own.
func validateObjects(objs []Object) error {
	if len(objs) == 0 {
		return errors.New("备份任务没有对象")
	}
	for _, obj := range objs {
		label := obj.Name
		if label == "" {
			label = obj.Path
		}
		switch obj.Kind {
		case "dir":
			if strings.TrimSpace(obj.Path) == "" {
				return fmt.Errorf("%s：目录路径为空", label)
			}
			st, err := os.Stat(obj.Path)
			if os.IsNotExist(err) {
				return fmt.Errorf("%s：路径不存在 %s", label, obj.Path)
			}
			if err != nil {
				return fmt.Errorf("%s：无法访问 %s（%v）", label, obj.Path, err)
			}
			if !st.IsDir() {
				return fmt.Errorf("%s：%s 是文件，备份对象只能是目录（请填它所在的目录）", label, obj.Path)
			}
		case "mysql":
			if strings.TrimSpace(obj.Database) == "" {
				return fmt.Errorf("%s：未指定数据库名（写成 mysql:库名）", label)
			}
		case "redis":
			if strings.TrimSpace(obj.Path) == "" {
				return fmt.Errorf("%s：未指定 Redis 快照路径（写成 redis:/var/lib/redis/dump.rdb）", label)
			}
		default:
			return fmt.Errorf("%s：不支持的对象类型 %q（支持目录、mysql:库名、redis:路径）", label, obj.Kind)
		}
	}
	return nil
}

func (m *Manager) backupObject(obj Object, dir string) (*ManifestEntry, error) {
	switch obj.Kind {
	case "dir":
		if obj.Path == "" {
			return nil, errors.New("目录路径为空")
		}
		st, statErr := os.Stat(obj.Path)
		if os.IsNotExist(statErr) {
			return nil, fmt.Errorf("路径不存在: %s", obj.Path)
		}
		if statErr != nil {
			return nil, fmt.Errorf("无法访问 %s: %w", obj.Path, statErr)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("%s 是文件，备份对象只能是目录", obj.Path)
		}
		file := filepath.Join(dir, "data", sanitize(orDefault(obj.Name, filepath.Base(obj.Path)))+".tar.gz")
		size, sum, err := archiveDir(obj.Path, file)
		if err != nil {
			return nil, err
		}
		return &ManifestEntry{Kind: "dir", Name: obj.Name, Source: obj.Path, File: relTo(file, dir), Size: size, SHA256: sum}, nil
	case "mysql":
		if obj.Database == "" {
			return nil, errors.New("未指定数据库名")
		}
		args := []string{"--single-transaction", "--routines", "--triggers", "--databases", obj.Database}
		if obj.User != "" {
			args = append([]string{"-u" + obj.User}, args...)
		}
		out, err := m.RunCmd("mysqldump", args...)
		if err != nil {
			return nil, fmt.Errorf("mysqldump 失败: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		file := filepath.Join(dir, "data", sanitize(obj.Database)+".sql.gz")
		size, sum, err := writeGz(file, out)
		if err != nil {
			return nil, err
		}
		return &ManifestEntry{Kind: "mysql", Name: obj.Name, Source: obj.Database, File: relTo(file, dir), Size: size, SHA256: sum}, nil
	case "redis":
		// BGSAVE writes dump.rdb next to the server's dir; we copy the file the
		// caller points at (default /var/lib/redis/dump.rdb).
		src := orDefault(obj.Path, "/var/lib/redis/dump.rdb")
		if _, err := os.Stat(src); err != nil {
			return nil, fmt.Errorf("Redis 快照不存在: %s（先执行 BGSAVE 或指对路径）", src)
		}
		file := filepath.Join(dir, "data", "redis.rdb.gz")
		size, sum, err := gzFile(src, file)
		if err != nil {
			return nil, err
		}
		return &ManifestEntry{Kind: "redis", Name: obj.Name, Source: src, File: relTo(file, dir), Size: size, SHA256: sum}, nil
	}
	return nil, fmt.Errorf("不支持的备份类型 %q", obj.Kind)
}

// List returns snapshots newest first.
func (m *Manager) List(targetDir string) ([]Manifest, error) {
	dir := orDefault(targetDir, m.root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		man, err := readManifest(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, *man)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// RestoreOpts controls a restore.
type RestoreOpts struct {
	SnapshotDir string // absolute snapshot directory
	Confirm     string // must equal the snapshot name
	// DryRun performs preflight only.
	DryRun bool
}

// RestoreResult reports what a restore did.
type RestoreResult struct {
	SnapshotName   string   `json:"snapshot"`
	Preflight      []string `json:"preflight"`
	Restored       []string `json:"restored"`
	SnapshotBackup string   `json:"pre_restore_backup,omitempty"`
	RolledBack     bool     `json:"rolled_back"`
}

// Restore performs the three-gate flow: preflight, pre-restore snapshot then
// apply; any failure rolls back from that snapshot.
func (m *Manager) Restore(opt RestoreOpts) (*RestoreResult, error) {
	man, err := readManifest(opt.SnapshotDir)
	if err != nil {
		return nil, fmt.Errorf("快照不可读: %w", err)
	}
	if opt.Confirm != man.Name {
		return nil, fmt.Errorf("确认名称不匹配：请输入快照名 %q", man.Name)
	}
	res := &RestoreResult{SnapshotName: man.Name}

	// --- gate 1: preflight -------------------------------------------------
	for _, e := range man.Objects {
		switch e.Kind {
		case "dir":
			parent := filepath.Dir(e.Source)
			if _, err := os.Stat(parent); err != nil {
				return nil, fmt.Errorf("目标父目录不存在: %s", parent)
			}
			if err := checkWritable(parent); err != nil {
				return nil, fmt.Errorf("目标目录不可写: %s (%v)", parent, err)
			}
			res.Preflight = append(res.Preflight, "目录可写: "+e.Source)
		case "mysql":
			if _, err := exec.LookPath("mysql"); err != nil {
				return nil, errors.New("未找到 mysql 客户端，无法恢复数据库")
			}
			res.Preflight = append(res.Preflight, "mysql 客户端可用")
		case "redis":
			res.Preflight = append(res.Preflight, "redis 快照将覆盖 "+e.Source)
		}
		if _, err := os.Stat(filepath.Join(opt.SnapshotDir, e.File)); err != nil {
			return nil, fmt.Errorf("快照文件缺失: %s", e.File)
		}
	}
	if opt.DryRun {
		return res, nil
	}

	// --- gate 2: pre-restore snapshot -------------------------------------
	ts := time.Now().Format("20060102-150405")
	if dirs := dirObjects(man); len(dirs) > 0 {
		name := fmt.Sprintf("pre-restore-%s", ts)
		pre := filepath.Join(m.root, name)
		if err := os.MkdirAll(filepath.Join(pre, "data"), 0o700); err != nil {
			return nil, err
		}
		preMan := &Manifest{Name: name, Job: "pre-restore", CreatedAt: time.Now(), Hostname: man.Hostname, Version: m.version}
		for _, e := range dirs {
			file := filepath.Join(pre, "data", sanitize(e.Name)+".tar.gz")
			size, sum, err := archiveDir(e.Source, file)
			if err != nil {
				return nil, fmt.Errorf("恢复前快照失败（未做任何修改）: %w", err)
			}
			preMan.Objects = append(preMan.Objects, ManifestEntry{Kind: "dir", Name: e.Name, Source: e.Source, File: relTo(file, pre), Size: size, SHA256: sum})
		}
		body, _ := json.MarshalIndent(preMan, "", "  ")
		_ = os.WriteFile(filepath.Join(pre, "manifest.json"), body, 0o600)
		res.SnapshotBackup = pre
	}

	// --- gate 3: apply ----------------------------------------------------
	rollback := func(reason error) (*RestoreResult, error) {
		log.Printf("backup: 恢复失败，开始回滚: %v", reason)
		if res.SnapshotBackup == "" {
			return res, fmt.Errorf("恢复失败且没有回滚点: %w", reason)
		}
		if rbErr := m.restoreSnapshot(res.SnapshotBackup, ""); rbErr != nil {
			return res, fmt.Errorf("恢复失败: %v；回滚也失败: %v（快照在 %s）", reason, rbErr, res.SnapshotBackup)
		}
		res.RolledBack = true
		return res, fmt.Errorf("恢复失败，已自动回滚: %w", reason)
	}

	for _, e := range man.Objects {
		if err := m.restoreEntry(opt.SnapshotDir, e); err != nil {
			return rollback(err)
		}
		res.Restored = append(res.Restored, e.Name)
	}

	// --- verify -----------------------------------------------------------
	for _, e := range man.Objects {
		if e.Kind != "dir" {
			continue
		}
		got, err := dirDigest(e.Source)
		if err != nil {
			return rollback(fmt.Errorf("校验 %s 失败: %w", e.Source, err))
		}
		if got == "" {
			return rollback(fmt.Errorf("校验 %s 失败：目录为空", e.Source))
		}
	}
	return res, nil
}

// restoreSnapshot applies every dir entry of a snapshot without the gate flow
// (used for rollback and for plain restores from the API).
func (m *Manager) restoreSnapshot(dir, confirm string) error {
	man, err := readManifest(dir)
	if err != nil {
		return err
	}
	if confirm != "" && confirm != man.Name {
		return fmt.Errorf("确认名称不匹配")
	}
	for _, e := range man.Objects {
		if err := m.restoreEntry(dir, e); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) restoreEntry(snapshotDir string, e ManifestEntry) error {
	src := filepath.Join(snapshotDir, e.File)
	switch e.Kind {
	case "dir":
		// Replace the directory contents atomically enough: extract into a temp
		// dir next to it, then swap. The old contents are already snapshotted.
		parent := filepath.Dir(e.Source)
		tmp, err := os.MkdirTemp(parent, ".restore-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := extractTarGz(src, tmp); err != nil {
			return err
		}
		backupOld := e.Source + ".pre-restore-" + time.Now().Format("150405")
		if _, err := os.Stat(e.Source); err == nil {
			if err := os.Rename(e.Source, backupOld); err != nil {
				return err
			}
		}
		if err := os.Rename(tmp, e.Source); err != nil {
			// Put the original back if the swap failed.
			_ = os.Rename(backupOld, e.Source)
			return err
		}
		_ = os.RemoveAll(backupOld)
		return nil
	case "mysql":
		if _, err := exec.LookPath("mysql"); err != nil {
			return errors.New("未找到 mysql 客户端")
		}
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		cmd := exec.Command("mysql")
		cmd.Stdin = zr
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mysql 恢复失败: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "redis":
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		dst, err := os.Create(e.Source)
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = io.Copy(dst, zr)
		return err
	}
	return fmt.Errorf("不支持的恢复类型 %q", e.Kind)
}

// applyRetention keeps the newest N snapshots.
func (m *Manager) applyRetention(dir string, keep int) {
	if keep <= 0 {
		return
	}
	list, err := m.List(dir)
	if err != nil || len(list) <= keep {
		return
	}
	for _, man := range list[keep:] {
		_ = os.RemoveAll(filepath.Join(dir, man.Name))
	}
}

// --- helpers --------------------------------------------------------------

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var man Manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return nil, err
	}
	if man.Name == "" {
		return nil, errors.New("manifest 缺少 name")
	}
	return &man, nil
}

func archiveDir(src, dst string) (int64, string, error) {
	f, err := os.Create(dst)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	mw := io.MultiWriter(f, h)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)

	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		_, err = io.Copy(tw, io.LimitReader(fh, 1<<40))
		return err
	})
	if err != nil {
		return 0, "", err
	}
	if err := tw.Close(); err != nil {
		return 0, "", err
	}
	if err := gz.Close(); err != nil {
		return 0, "", err
	}
	st, _ := os.Stat(dst)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("归档内路径非法: %s", hdr.Name)
		}
		target := filepath.Join(dst, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func writeGz(path string, data []byte) (int64, string, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	mw := io.MultiWriter(f, h)
	gz := gzip.NewWriter(mw)
	if _, err := gz.Write(data); err != nil {
		return 0, "", err
	}
	if err := gz.Close(); err != nil {
		return 0, "", err
	}
	st, _ := os.Stat(path)
	var size int64
	if st != nil {
		size = st.Size()
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func gzFile(src, dst string) (int64, string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, "", err
	}
	return writeGz(dst, data)
}

// dirDigest returns a cheap fingerprint of a directory (file count + total size)
// used as the post-restore health check.
func dirDigest(path string) (string, error) {
	var count int64
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d-%d", count, total), nil
}

func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".opsmini-w-")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func dirObjects(man *Manifest) []ManifestEntry {
	var out []ManifestEntry
	for _, e := range man.Objects {
		if e.Kind == "dir" {
			out = append(out, e)
		}
	}
	return out
}

func relTo(file, base string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return file
	}
	return filepath.ToSlash(rel)
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_", "..", "_")
	s = repl.Replace(s)
	if s == "" {
		return "backup"
	}
	return s
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// SnapshotDir resolves a snapshot name under the default root, refusing escapes.
func validSnapshotName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return errors.New("非法快照名")
	}
	return nil
}

func (m *Manager) SnapshotDir(name string) (string, error) {
	if err := validSnapshotName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(m.root, name)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("快照不存在: %s", name)
	}
	return dir, nil
}

// ParseObjects turns the panel's line syntax into typed objects.
//
//	/var/www/html              directory
//	mysql:opsmini              database
//	redis:/var/lib/redis/dump.rdb
//
// It lives here (not in the API layer) so a scheduled backup task parses its
// object list exactly the same way a manual run does — two parsers would drift.
func ParseObjects(raw, dbUser, dbPass string) []Object {
	var out []Object
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "mysql:"):
			name := strings.TrimSpace(strings.TrimPrefix(line, "mysql:"))
			if name == "" {
				continue
			}
			out = append(out, Object{Kind: "mysql", Database: name, Name: "mysql-" + name, User: dbUser, Password: dbPass})
		case strings.HasPrefix(line, "redis:"):
			path := strings.TrimSpace(strings.TrimPrefix(line, "redis:"))
			out = append(out, Object{Kind: "redis", Path: path, Name: "redis"})
		default:
			out = append(out, Object{Kind: "dir", Path: line, Name: strings.Trim(strings.ReplaceAll(line, "/", "_"), "_")})
		}
	}
	return out
}
