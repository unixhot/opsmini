// Package sentinel is OpsMini's perception layer: collectors turn raw host
// observations into normalized Signals, the bus de-duplicates and persists
// them, and the aggregator folds related signals into Incidents.
//
// The package is deliberately dependency-light: it only touches /proc, /var/log
// and the SQLite database, so it keeps working on a minimal host without any
// external agent (no journald, no auditd, no eBPF).
package sentinel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/platform/sys"
)

// Signal wraps the persisted model with a small fluent builder used by the
// collectors (withEvidence / withTags / withDeviation).
type Signal struct{ model.Signal }

// newSignal fills the boilerplate fields of a signal.
func newSignal(kind, source, subject, family, fingerprint, severity, message string) Signal {
	now := time.Now()
	return Signal{model.Signal{
		ID:          strings.ReplaceAll(uuid.NewString(), "-", ""),
		Kind:        kind,
		Source:      source,
		Subject:     subject,
		Family:      family,
		Fingerprint: fingerprint,
		DedupKey:    fmt.Sprintf("%s|%s|%s", fingerprint, subject, now.Format("2006010215")),
		Severity:    severity,
		Message:     truncate(message, 500),
		ObservedAt:  now,
		CreatedAt:   now,
	}}
}

// withEvidence attaches a JSON evidence blob.
func (s Signal) withEvidence(v any) Signal {
	if b, err := json.Marshal(v); err == nil && len(b) <= 8192 {
		s.Evidence = string(b)
	}
	return s
}

// withTags attaches comma separated tags.
func (s Signal) withTags(tags ...string) Signal {
	s.Tags = truncate(strings.Join(tags, ","), 250)
	return s
}

func (s Signal) withDeviation(d float64) Signal {
	s.Deviation = d
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// tailLines reads the last maxLines lines within the last maxBytes bytes.
func tailLines(path string, maxBytes int64, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := st.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines, nil
}

var (
	reDigits = regexp.MustCompile(`\d+`)
	reHex    = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reUUID   = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	reIP     = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	reSpace  = regexp.MustCompile(`\s+`)
)

// normTemplate normalizes a log line into a template so that repeated messages
// collapse into one fingerprint ("job 42 failed after 3s" -> "job N failed after Ns").
func normTemplate(line string) string {
	s := line
	s = reUUID.ReplaceAllString(s, "<uuid>")
	s = reIP.ReplaceAllString(s, "<ip>")
	s = reHex.ReplaceAllString(s, "<hex>")
	s = reDigits.ReplaceAllString(s, "N")
	s = reSpace.ReplaceAllString(s, " ")
	return truncate(strings.TrimSpace(s), 160)
}

// topTemplates returns the most frequent templates and their share.
func topTemplates(lines []string, topN int) (string, float64, int) {
	if len(lines) == 0 {
		return "", 0, 0
	}
	counts := map[string]int{}
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		counts[normTemplate(ln)]++
	}
	if len(counts) == 0 {
		return "", 0, 0
	}
	best, bestN := "", 0
	for t, n := range counts {
		if n > bestN {
			best, bestN = t, n
		}
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if topN <= 0 {
		topN = len(counts)
	}
	_ = topN
	return best, float64(bestN) / float64(total), len(counts)
}

var errKeywords = []string{"error", "fatal", "panic", "exception", "no space", "denied", "refused", "timeout"}

func hasErrorKeyword(lines []string) bool {
	for _, ln := range lines {
		l := strings.ToLower(ln)
		for _, k := range errKeywords {
			if strings.Contains(l, k) {
				return true
			}
		}
	}
	return false
}

// 进程信息与文件持有者查询依赖 /proc，Windows 上没有这个文件系统。
// 平台实现见 proc_unix.go / proc_windows.go。

// --- EWMA baseline -----------------------------------------------------------

// ewma is a tiny exponentially weighted moving average with a variance estimate.
type ewma struct {
	Mean    float64
	Var     float64
	Samples int
	Alpha   float64
}

func (e *ewma) Update(v float64) {
	if e.Alpha == 0 {
		e.Alpha = 0.3
	}
	if e.Samples == 0 {
		e.Mean = v
		e.Samples = 1
		return
	}
	diff := v - e.Mean
	e.Mean += e.Alpha * diff
	e.Var = (1 - e.Alpha) * (e.Var + e.Alpha*diff*diff)
	e.Samples++
}

func (e *ewma) Sigma() float64 {
	s := sqrt(e.Var)
	return s
}

func sqrt(f float64) float64 {
	if f <= 0 {
		return 0
	}
	x := f
	for i := 0; i < 20; i++ {
		x = 0.5 * (x + f/x)
	}
	return x
}

// inodeOf returns a stable file identity (used to detect log rotation).
func inodeOf(fi os.FileInfo) uint64 { return sys.FileID(fi) }
