package sentinel

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// skipLogs are pseudo files (sparse or binary bookkeeping) that must never be
// treated as growing application logs.
var skipLogs = map[string]bool{
	"lastlog": true, "wtmp": true, "btmp": true, "utmp": true, "faillog": true,
	"dpkg.log": false,
}

type logFileState struct {
	size    int64
	inode   uint64
	base    map[int]*ewma // hour bucket -> rate baseline (bytes/min)
	strikes int
	lastRun time.Time
}

// logWatcher detects abnormal log growth.
//
// It samples file sizes on a fixed interval, converts the delta into bytes per
// minute, and compares it with a per-hour-bucket EWMA baseline. Two consecutive
// sigma breaches (or one hard threshold breach) raise a signal. When a signal is
// raised it also resolves who holds the file open, which is what decides whether
// the file may be touched at all.
type logWatcher struct {
	cfg    Config
	mu     sync.Mutex
	states map[string]*logFileState
	// db persists the per-hour baselines (model.CollectorBaseline). Optional:
	// without it the watcher still works, it just relearns on every restart.
	db *gorm.DB
	// lastSave throttles the write path. Sampling runs every 10s; writing a row
	// per sample per file would put a SQLite update behind every tick.
	lastSave time.Time
}

func newLogWatcher(cfg Config) *logWatcher {
	return &logWatcher{cfg: cfg, states: map[string]*logFileState{}}
}

// setStore enables baseline persistence and loads whatever was saved before.
// Baselines older than baselineTTL are ignored: they describe a host from another
// lifetime and would produce confident nonsense.
func (w *logWatcher) setStore(db *gorm.DB) {
	if db == nil {
		return
	}
	w.db = db
	var rows []model.CollectorBaseline
	cutoff := time.Now().Add(-baselineTTL)
	if err := db.Where("collector = ? AND updated_at > ?", "logwatch", cutoff).Find(&rows).Error; err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range rows {
		st := w.states[r.Subject]
		if st == nil {
			st = &logFileState{base: map[int]*ewma{}}
			w.states[r.Subject] = st
		}
		st.base[r.Bucket] = &ewma{Mean: r.Mean, Var: r.Variance, Samples: r.Samples, Alpha: 0.3}
	}
	if len(rows) > 0 {
		log.Printf("sentinel: logwatch resumed %d baselines", len(rows))
	}
}

// persist writes the current baselines back. Called at most once a minute.
func (w *logWatcher) persist() {
	if w.db == nil {
		return
	}
	w.mu.Lock()
	type row struct {
		subject string
		bucket  int
		e       ewma
	}
	var batch []row
	for subj, st := range w.states {
		for bucket, e := range st.base {
			if e.Samples < 3 {
				// Below three samples the value is noise; saving it would make the
				// next start look better informed than it is.
				continue
			}
			batch = append(batch, row{subj, bucket, *e})
		}
	}
	w.mu.Unlock()

	now := time.Now()
	for _, b := range batch {
		rec := model.CollectorBaseline{
			Collector: "logwatch", Subject: b.subject, Bucket: b.bucket,
			Mean: b.e.Mean, Variance: b.e.Var, Samples: b.e.Samples, UpdatedAt: now,
		}
		w.db.Where("collector = ? AND subject = ? AND bucket = ?", "logwatch", b.subject, b.bucket).
			Assign(rec).FirstOrCreate(&model.CollectorBaseline{})
	}
}

// baselineTTL is how long a persisted baseline stays usable. A week is long
// enough to survive a maintenance window and short enough that a host whose
// workload changed has its old expectations discarded.
const baselineTTL = 7 * 24 * time.Hour

func (w *logWatcher) Name() string { return "logwatch" }

func (w *logWatcher) Run(ctx context.Context, out func(Signal)) {
	interval := 10 * time.Second
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.scan(interval, out)
			if time.Since(w.lastSave) >= time.Minute {
				w.lastSave = time.Now()
				w.persist()
			}
		}
	}
}

func (w *logWatcher) scan(interval time.Duration, out func(Signal)) {
	files := w.candidates()
	now := time.Now()
	hour := now.Hour()

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, path := range files {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		inode := inodeOf(st)
		prev := w.states[path]
		if prev == nil || prev.inode != inode || st.Size() < prev.size {
			// First sighting or rotation: (re)start from the current size, but keep
			// the learned baselines — they belong to the file's role (an nginx log
			// that rotated at noon still behaves like an nginx log), not to the
			// inode. Dropping them here would silently undo the persistence.
			base := map[int]*ewma{}
			if prev != nil && prev.base != nil {
				base = prev.base
			}
			w.states[path] = &logFileState{
				size:    st.Size(),
				inode:   inode,
				base:    base,
				lastRun: now,
			}
			continue
		}
		delta := st.Size() - prev.size
		prev.size = st.Size()
		if delta <= 0 {
			prev.strikes = 0
			continue
		}

		perMin := float64(delta) * float64(time.Minute) / float64(interval)
		b := prev.base[hour]
		if b == nil {
			b = &ewma{Alpha: 0.3}
			prev.base[hour] = b
		}
		var deviation float64
		if b.Samples >= 5 {
			sigma := b.Sigma()
			if sigma < 4096 {
				sigma = 4096
			}
			deviation = (perMin - b.Mean) / sigma
		}
		// A hard threshold breach is an emergency even on a brand new file: do not
		// hide it behind the learning period. Everything else waits for a baseline.
		hard := int64(perMin) >= w.cfg.LogGrowthBytesPerMin
		if b.Samples < 20 && !hard {
			b.Update(perMin)
			prev.strikes = 0
			continue
		}
		soft := b.Samples >= 20 && deviation >= w.cfg.LogGrowthSigma
		if hard || soft {
			prev.strikes++
		} else {
			prev.strikes = 0
		}
		if prev.strikes < 2 && !hard {
			b.Update(perMin)
			continue
		}

		sev := model.SeverityMedium
		if hard || deviation >= 6 {
			sev = model.SeverityHigh
		}
		lines, _ := tailLines(path, 8<<10, 40)
		top, share, distinct := topTemplates(lines, 0)
		if isErr := hasErrorKeyword(lines); isErr && sev == model.SeverityMedium {
			sev = model.SeverityHigh
		}
		if share > 0.8 && len(lines) >= 10 {
			// One template dominating the tail is the signature of a loop.
			sev = model.SeverityHigh
		}
		pid, cmd := holderProcess(path)

		sig := newSignal(model.SignalKindLog, "logwatch", path, "log_growth",
			"log_growth:"+path, sev,
			"日志增长异常："+path+" 当前约 "+humanBytes(int64(perMin))+"/min").
			withDeviation(deviation).
			withTags("scenario:log_growth").
			withEvidence(map[string]any{
				"path":             path,
				"bytes_per_min":    int64(perMin),
				"baseline_per_min": int64(b.Mean),
				"deviation":        deviation,
				"holder_pid":       pid,
				"holder_cmd":       cmd,
				"template_top":     top,
				"template_share":   share,
				"template_count":   distinct,
				"tail":             lines,
			})
		out(sig)
		prev.strikes = 0
		b.Update(perMin)
	}
}

// candidates lists log files under the configured roots (bounded).
func (w *logWatcher) candidates() []string {
	var out []string
	for _, root := range w.cfg.LogPaths {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				// Do not descend into the journal binary store or archives.
				base := filepath.Base(path)
				if base == "journal" || base == "private" || base == "old" {
					return filepath.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() || info.Size() < 4096 {
				return nil
			}
			name := filepath.Base(path)
			if skipLogs[name] || strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".xz") ||
				strings.HasSuffix(name, ".1") || strings.HasSuffix(name, ".2") {
				return nil
			}
			if !strings.Contains(strings.ToLower(name), "log") {
				return nil
			}
			if len(out) >= 200 {
				return filepath.SkipAll
			}
			out = append(out, path)
			return nil
		})
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return itoa(n) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return itoa(n/div) + " " + units[exp]
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
