package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// defaultFimPaths / criticalFimPaths 由平台文件提供：
//   - fim_paths_other.go  → Linux / macOS，/etc 下那几个关键文件
//   - fim_paths_windows.go → Windows，SAM/SYSTEM 等
// 放在这里硬编码 /etc 会让 Windows 首启就把一堆不存在的路径加进基线。

// FimService handles file integrity monitoring.
type FimService struct {
	baselineRepo *repository.FimBaselineRepo
	changeRepo   *repository.FimChangeRepo
	// setting reads a panel setting. Nil means "use the built-in defaults",
	// which keeps the service usable from tests and keeps the defaults working
	// on a database that has never been configured.
	setting func(key string) string
	// save persists settings; nil disables editing.
	save func(map[string]string) error
	// onChange fans a detected change out to the perception layer. Nil keeps FIM
	// a standalone checker (tests, and a panel with sentinel disabled).
	onChange func(model.FimChange)
	// bootstrapped marks that the first-run baseline has been built. Without it,
	// an empty baseline table makes every Check() rebuild, and a file that
	// appeared between two rounds would be absorbed into the fresh baseline
	// instead of being reported as new.
	bootstrapped bool
}

// NewFimService creates a FimService.
func NewFimService(baseline *repository.FimBaselineRepo, change *repository.FimChangeRepo) *FimService {
	return &FimService{baselineRepo: baseline, changeRepo: change}
}

// SetSettings wires the settings reader used to resolve monitored paths.
func (s *FimService) SetSettings(fn func(key string) string) { s.setting = fn }

// SetSaver wires the settings writer used by the paths editor.
func (s *FimService) SetSaver(fn func(map[string]string) error) { s.save = fn }

// SetOnChange wires the callback invoked for every detected change, so the
// perception layer can turn file tampering into a security signal.
func (s *FimService) SetOnChange(fn func(model.FimChange)) { s.onChange = fn }

// fimSetting reads a setting through the injected reader, tolerating nil.
func (s *FimService) fimSetting(key string) string {
	if s.setting == nil {
		return ""
	}
	return s.setting(key)
}

// MonitoredPaths returns the patterns currently in effect and which of them are
// treated as critical. An empty setting means the built-in default list — an
// operator who never opened this page must keep getting the standard coverage.
func (s *FimService) MonitoredPaths() (patterns []string, critical []string) {
	patterns = splitPatterns(s.fimSetting("fim_paths"))
	if len(patterns) == 0 {
		patterns = append([]string{}, defaultFimPaths...)
	}
	critical = splitPatterns(s.fimSetting("fim_critical_paths"))
	if len(critical) == 0 {
		critical = defaultCriticalList()
	}
	return patterns, critical
}

// defaultCriticalList renders the platform's critical-path set as a sorted slice.
// 设置页要回填成一行文本，map 得先变成有序列表，否则每次刷新顺序都在跳。
func defaultCriticalList() []string {
	out := make([]string, 0, len(criticalFimPaths))
	for p := range criticalFimPaths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// splitPatterns splits a comma separated list, dropping blanks.
func splitPatterns(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Rebuild builds/rebuilds the baseline from the configured patterns.
//
// It also drops baselines that no configured pattern covers any more. Without
// that, removing a path in the settings page would leave it monitored forever:
// the list would look right while the checker kept watching the old files.
//
// The test is "does some pattern still cover this path", not "did this round
// produce this path". A file that vanished is still covered by its literal
// pattern, so its baseline survives and Check can report the deletion — deleting
// the baseline instead would turn a disappearance into silence.
func (s *FimService) Rebuild() error {
	patterns, _ := s.MonitoredPaths()
	for _, pattern := range patterns {
		for _, path := range expandFimPattern(pattern) {
			sha, size, mod, err := sha256File(path)
			if err != nil {
				continue // skip if the file does not exist
			}
			_ = s.baselineRepo.Upsert(&model.FimBaseline{Path: path, SHA256: sha, Size: size, ModTime: mod, Enabled: true})
		}
	}
	existing, err := s.baselineRepo.List()
	if err != nil {
		return nil // the upserts already succeeded; pruning can wait for next round
	}
	for _, b := range existing {
		if !patternCoversAny(patterns, b.Path) {
			_ = s.baselineRepo.Delete(b.ID)
		}
	}
	return nil
}

// patternCoversAny reports whether any configured pattern still covers path.
func patternCoversAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if patternCovers(p, path) {
			return true
		}
	}
	return false
}

// Watches reports whether path falls under any currently monitored pattern.
//
// 容灾演练要靠它回答一个关键问题：「我把样本文件放在这里，会有人看见吗」。
// 如果不检查就落地，演练的结论会从「Agent 没发现」变成「根本没人看」——
// 前者是产品缺陷，后者是配置问题，两者混在一起会误导判断。
func (s *FimService) Watches(path string) bool {
	patterns, _ := s.MonitoredPaths()
	for _, p := range patterns {
		// A directory pattern such as /etc means "everything under it", but
		// patternCovers treats a literal as an exact path.
		if !strings.ContainsAny(p, "*?[") {
			if path == p || strings.HasPrefix(path, strings.TrimSuffix(p, "/")+"/") {
				return true
			}
			continue
		}
		if patternCovers(p, path) {
			return true
		}
	}
	return false
}

// patternCovers applies the same syntax expandFimPattern accepts: literal,
// standard glob, or ** for any depth.
func patternCovers(pattern, path string) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern == path
	}
	if idx := strings.Index(pattern, "**"); idx >= 0 {
		root := strings.TrimSuffix(pattern[:idx], "/")
		if root != "" && !strings.HasPrefix(path, root+"/") {
			return false
		}
		suffix := strings.TrimPrefix(pattern[idx+2:], "/")
		if suffix == "" {
			return true
		}
		ok, err := filepath.Match(suffix, filepath.Base(path))
		return err == nil && ok
	}
	ok, err := filepath.Match(pattern, path)
	return err == nil && ok
}

// fimMaxExpand bounds how many files one pattern may contribute.
//
// /etc/** on a busy host can match tens of thousands of files; hashing all of
// them every round would turn the integrity check into the panel's biggest CPU
// consumer. The cap keeps a broad pattern useful instead of harmful.
const fimMaxExpand = 500

// expandFimPattern turns one configured pattern into concrete file paths.
//
// A pattern without wildcards is returned as-is even when the file is missing:
// that is how "monitor this file, tell me if it appears or disappears" works.
// Standard glob syntax is used, plus ** for "any depth", which path/filepath
// does not support but operators reach for anyway.
func expandFimPattern(pattern string) []string {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{pattern}
	}
	if idx := strings.Index(pattern, "**"); idx >= 0 {
		return expandDoubleStar(pattern, idx)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	if len(matches) > fimMaxExpand {
		matches = matches[:fimMaxExpand]
	}
	return matches
}

// expandDoubleStar handles the "root/**/suffix" form by walking root and
// matching the suffix against each candidate.
func expandDoubleStar(pattern string, idx int) []string {
	root := strings.TrimSuffix(pattern[:idx], "/")
	if root == "" {
		root = "/"
	}
	suffix := strings.TrimPrefix(pattern[idx+2:], "/")
	out := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Unreadable directories are skipped: one permission problem must not
			// abort coverage of everything else under the root.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= fimMaxExpand {
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		if suffix == "" {
			out = append(out, path)
			return nil
		}
		ok, merr := filepath.Match(suffix, filepath.Base(path))
		if merr == nil && ok {
			out = append(out, path)
		}
		return nil
	})
	return out
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
	if n == 0 && !s.bootstrapped {
		// 首轮静默建基线。不能直接 return：如果被监控的文件一个都还不存在，
		// rebuild 之后依然是 0 条基线，直接返回会让下面的新增文件检测永远
		// 跑不起来——而那正是「文件后来才出现」这一最需要报的场景。
		//
		// 但只建一次：每轮都重建的话，两轮之间新出现的文件会被并进新基线，
		// 于是「新增」永远变成「一直就有」。
		_ = s.Rebuild()
		s.bootstrapped = true
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
				Level: s.levelFor(b.Path), Message: "文件被删除",
			}))
			continue
		}
		if sha != b.SHA256 {
			c := s.record(b, model.FimChange{
				Path: b.Path, OldSHA256: b.SHA256, NewSHA256: sha, Size: size, ModTime: mod,
				Level: s.levelFor(b.Path), Message: "文件内容发生变化",
			})
			changes = append(changes, c)
			// update the baseline to the new value to avoid repeated alerts
			b.SHA256 = sha
			b.Size = size
			b.ModTime = mod
			_ = s.baselineRepo.Upsert(&b)
		}
	}

	// 新增文件也必须报。
	//
	// 基线只覆盖「建基线时已经存在的文件」，所以旧实现里一个刚被上传上来的
	// webshell 是完全隐形的——恰恰是文件完整性监控最该抓的东西。变更和删除
	// 都能看见，唯独新增看不见，等于漏掉了一半。
	known := make(map[string]bool, len(baselines))
	for _, b := range baselines {
		known[b.Path] = true
	}
	patterns, _ := s.MonitoredPaths()
	added := 0
	for _, pattern := range patterns {
		if added >= fimMaxNewPerRound {
			break
		}
		for _, path := range expandFimPattern(pattern) {
			if known[path] || added >= fimMaxNewPerRound {
				continue
			}
			sha, size, mod, err := sha256File(path)
			if err != nil {
				continue // 还没出现、或者没权限：不是「新增」
			}
			c := s.record(model.FimBaseline{Path: path}, model.FimChange{
				Path: path, NewSHA256: sha, Size: size, ModTime: mod,
				Level: s.levelFor(path), Message: "新增文件",
			})
			changes = append(changes, c)
			// 立刻落基线，否则下一轮会把同一个文件再报一次，60 秒一条会很吵。
			_ = s.baselineRepo.Upsert(&model.FimBaseline{
				Path: path, SHA256: sha, Size: size, ModTime: mod, Enabled: true,
			})
			known[path] = true
			added++
		}
	}
	return changes
}

// fimMaxNewPerRound bounds how many "new file" events one round may produce.
// A directory pattern can match hundreds of files at once (a package upgrade,
// a log rotation burst); reporting all of them turns the event list into noise
// and buries the one that matters.
const fimMaxNewPerRound = 20

// record persists a FIM change event and returns it.
func (s *FimService) record(b model.FimBaseline, c model.FimChange) model.FimChange {
	_ = s.changeRepo.Create(&c)
	if s.onChange != nil {
		s.onChange(c)
	}
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

// levelFor returns the severity level for a monitored path.
//
// Critical patterns may be wildcards, so matching goes through filepath.Match
// rather than a map lookup: "/etc/ssh/*_config" is a reasonable thing to write
// and a map would silently classify it as warning.
func (s *FimService) levelFor(path string) string {
	_, critical := s.MonitoredPaths()
	for _, pat := range critical {
		if pat == path {
			return "critical"
		}
		if strings.ContainsAny(pat, "*?[") {
			if ok, err := filepath.Match(pat, path); err == nil && ok {
				return "critical"
			}
		}
	}
	return "warning"
}

// SavePaths persists the monitored patterns and the critical subset.
func (s *FimService) SavePaths(paths, critical string) error {
	if s.save == nil {
		return fmt.Errorf("设置存储未接入")
	}
	return s.save(map[string]string{
		"fim_paths":          paths,
		"fim_critical_paths": critical,
	})
}

// ResolvedPath is one configured pattern and what it expands to right now.
type ResolvedPath struct {
	Pattern string `json:"pattern"`
	Files   int    `json:"files"`
	Sample  string `json:"sample,omitempty"`
}

// ResolveForDisplay expands patterns for the settings page so an operator can
// see, before saving, whether a wildcard actually matches anything.
func (s *FimService) ResolveForDisplay(patterns []string) []ResolvedPath {
	out := make([]ResolvedPath, 0, len(patterns))
	for _, p := range patterns {
		files := expandFimPattern(p)
		item := ResolvedPath{Pattern: p, Files: len(files)}
		if len(files) > 0 {
			item.Sample = files[0]
		}
		out = append(out, item)
	}
	return out
}
