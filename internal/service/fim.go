package service

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// defaultFimPaths are the key files monitored by default.
var defaultFimPaths = []string{
	"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/ssh/sshd_config",
	"/etc/crontab", "/etc/sudoers", "/etc/hosts", "/etc/resolv.conf",
}

// criticalFimPaths are key files whose changes are classified as critical.
var criticalFimPaths = map[string]bool{
	"/etc/passwd": true, "/etc/shadow": true, "/etc/sudoers": true,
}

// FimService handles file integrity monitoring.
type FimService struct {
	baselineRepo *repository.FimBaselineRepo
	changeRepo   *repository.FimChangeRepo
}

// NewFimService creates a FimService.
func NewFimService(baseline *repository.FimBaselineRepo, change *repository.FimChangeRepo) *FimService {
	return &FimService{baselineRepo: baseline, changeRepo: change}
}

// Rebuild builds/rebuilds the baseline (reads default key files and computes SHA256).
func (s *FimService) Rebuild() error {
	for _, path := range defaultFimPaths {
		sha, size, mod, err := sha256File(path)
		if err != nil {
			continue // skip if the file does not exist
		}
		_ = s.baselineRepo.Upsert(&model.FimBaseline{Path: path, SHA256: sha, Size: size, ModTime: mod, Enabled: true})
	}
	return nil
}

// ListBaselines returns all baseline items.
func (s *FimService) ListBaselines() ([]model.FimBaseline, error) {
	return s.baselineRepo.List()
}

// RemoveBaseline removes a baseline item.
func (s *FimService) RemoveBaseline(id uint) error {
	return s.baselineRepo.Delete(id)
}

// Check compares against the baseline, records change events, and returns this round's changes.
func (s *FimService) Check() []model.FimChange {
	n, _ := s.baselineRepo.Count()
	if n == 0 {
		_ = s.Rebuild() // silently build baseline on first run
		return nil
	}
	baselines, err := s.baselineRepo.List()
	if err != nil {
		return nil
	}
	var changes []model.FimChange
	for _, b := range baselines {
		if !b.Enabled {
			continue
		}
		sha, size, mod, err := sha256File(b.Path)
		if err != nil {
			changes = append(changes, s.record(b, model.FimChange{
				Path: b.Path, OldSHA256: b.SHA256, NewSHA256: "",
				Level: levelFor(b.Path), Message: "文件被删除",
			}))
			continue
		}
		if sha != b.SHA256 {
			c := s.record(b, model.FimChange{
				Path: b.Path, OldSHA256: b.SHA256, NewSHA256: sha, Size: size, ModTime: mod,
				Level: levelFor(b.Path), Message: "文件内容发生变化",
			})
			changes = append(changes, c)
			// update the baseline to the new value to avoid repeated alerts
			b.SHA256 = sha
			b.Size = size
			b.ModTime = mod
			_ = s.baselineRepo.Upsert(&b)
		}
	}
	return changes
}

// record persists a FIM change event and returns it.
func (s *FimService) record(b model.FimBaseline, c model.FimChange) model.FimChange {
	_ = s.changeRepo.Create(&c)
	return c
}

// ListChanges returns the most recent limit change events.
func (s *FimService) ListChanges(limit int) ([]model.FimChange, error) {
	return s.changeRepo.List(limit)
}

// sha256File computes a file's SHA256, size, and modification time.
func sha256File(path string) (string, int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, 0, err
	}
	fi, err := f.Stat()
	if err != nil {
		return "", 0, 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), fi.Size(), fi.ModTime().Unix(), nil
}

// levelFor returns the severity level for a monitored path ("critical" for high-value files, otherwise "warning").
func levelFor(path string) string {
	if criticalFimPaths[path] {
		return "critical"
	}
	return "warning"
}
