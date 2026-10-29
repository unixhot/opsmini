package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Off-host storage.
//
// Local snapshots protect against fat fingers, not against losing the machine. The
// remote layer exists for the second case, so it is deliberately simple: upload a
// finished snapshot directory, list what is there, download one back, delete one.
// No incremental format, no repository — a snapshot stays a plain directory of
// files that can be copied out with any tool even if OpsMini is gone.

// RemoteStore moves snapshots to and from an off-host location.
type RemoteStore interface {
	// Describe is a short human-readable identifier shown in the panel.
	Describe() string
	// PutDir uploads a complete snapshot directory.
	PutDir(ctx context.Context, localDir, name string) error
	// GetDir downloads one snapshot into dstParent/name.
	GetDir(ctx context.Context, name, dstParent string) error
	// List returns the snapshots that exist remotely.
	List(ctx context.Context) ([]RemoteSnapshot, error)
	// Delete removes one remote snapshot.
	Delete(ctx context.Context, name string) error
}

// TargetSpec declares one remote target (from config.yaml or a job).
type TargetSpec struct {
	Name string      `json:"name"`
	Type string      `json:"type"` // s3 | sftp
	S3   *S3Config   `json:"s3,omitempty"`
	SFTP *SFTPConfig `json:"sftp,omitempty"`
}

// NewRemote builds a RemoteStore from a spec.
func NewRemote(spec TargetSpec) (RemoteStore, error) {
	switch strings.ToLower(spec.Type) {
	case "s3":
		if spec.S3 == nil {
			return nil, fmt.Errorf("目标 %s 缺少 s3 配置", spec.Name)
		}
		return newS3Store(*spec.S3)
	case "sftp":
		if spec.SFTP == nil {
			return nil, fmt.Errorf("目标 %s 缺少 sftp 配置", spec.Name)
		}
		return newSFTPStore(*spec.SFTP)
	}
	return nil, fmt.Errorf("不支持的备份目标类型：%s", spec.Type)
}

// SetStores registers the named remote stores available for jobs and restores.
func (m *Manager) SetStores(stores map[string]RemoteStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stores = stores
}

// Stores lists the configured remote target names.
func (m *Manager) Stores() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.stores))
	for n := range m.stores {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// store resolves a target name.
func (m *Manager) store(name string) (RemoteStore, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.stores[name]
	if !ok {
		return nil, fmt.Errorf("未配置名为 %s 的备份目标", name)
	}
	return st, nil
}

// ListRemote lists the snapshots on a named target.
func (m *Manager) ListRemote(ctx context.Context, name string) ([]RemoteSnapshot, error) {
	st, err := m.store(name)
	if err != nil {
		return nil, err
	}
	list, err := st.List(ctx)
	if err != nil {
		return nil, err
	}
	// Newest first: names carry a timestamp suffix, so a string sort is enough.
	sort.Slice(list, func(i, j int) bool { return list[i].Name > list[j].Name })
	return list, nil
}

// Fetch pulls one remote snapshot into the local backup root and returns its
// directory. A snapshot already present locally is reused, so restoring twice does
// not re-download gigabytes.
func (m *Manager) Fetch(ctx context.Context, storeName, snapshot string) (string, error) {
	if err := validSnapshotName(snapshot); err != nil {
		return "", err
	}
	local := filepath.Join(m.root, snapshot)
	if _, err := os.Stat(filepath.Join(local, "manifest.json")); err == nil {
		return local, nil
	}
	st, err := m.store(storeName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return "", err
	}
	// Download beside the final location and rename when complete: an interrupted
	// download must not look like a usable snapshot.
	tmp := filepath.Join(m.root, ".incoming-"+snapshot)
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := st.GetDir(ctx, snapshot, tmp); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(tmp, snapshot, "manifest.json")); err != nil {
		return "", fmt.Errorf("远端快照 %s 不完整（缺少 manifest.json）", snapshot)
	}
	if err := os.Rename(filepath.Join(tmp, snapshot), local); err != nil {
		return "", err
	}
	return local, nil
}

// uploadSnapshot pushes a finished snapshot and applies the retention policy on the
// remote side. prune keeps the same "keep N newest" rule as local snapshots, so a
// daily job cannot silently fill a bucket.
func (m *Manager) uploadSnapshot(ctx context.Context, st RemoteStore, dir, name string, keep int) error {
	if err := st.PutDir(ctx, dir, name); err != nil {
		return err
	}
	if keep <= 0 {
		return nil
	}
	list, err := st.List(ctx)
	if err != nil {
		// Retention failing must not fail the backup: the data is safely uploaded,
		// and the next run will try again.
		return nil
	}
	names := make([]string, 0, len(list))
	for _, s := range list {
		names = append(names, s.Name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i, n := range names {
		if i < keep {
			continue
		}
		_ = st.Delete(ctx, n)
	}
	return nil
}
