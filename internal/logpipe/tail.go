package logpipe

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// batch is one read worth of events plus the offset they end at. The pipeline
// advances the registry only after the output accepted the batch, so a crash
// replays the batch instead of losing the tail of a file.
type batch struct {
	task   *Task
	path   string
	inode  uint64
	fp     string
	events []Event
	endOff int64
}

// harvester reads one file from a saved offset until EOF, then returns. The
// manager restarts it on the next scan when the file grows, which keeps the
// number of goroutines proportional to files, not to file size.
type harvester struct {
	task    Task
	path    string
	reg     *Registry
	agent   string
	version string
}

func (h *harvester) run(ctx context.Context) ([]batch, error) {
	f, err := os.Open(h.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Only regular files: a fifo or a device node would block forever.
	if !st.Mode().IsRegular() {
		return nil, nil
	}
	if !withinPaths(h.task.Input.Paths, h.path) || excluded(h.task.Input.ExcludeFiles, h.path) {
		return nil, nil
	}
	if h.task.Input.IgnoreOlder > 0 && time.Since(st.ModTime()) > h.task.Input.IgnoreOlder {
		return nil, nil
	}

	prev := h.reg.Get(h.task.ID, h.path)
	fp := Fingerprint(f)
	start := prev.Offset
	switch {
	case prev.UpdatedAt.IsZero():
		// Never seen: filebeat 8 reads existing files from the beginning by
		// default (tail_files: false), so behave the same.
		start = 0
	case st.Size() < start:
		// Truncated (copytruncate rotation): the old offset is meaningless.
		start = 0
	case prev.Fingerprint != "" && fp != "" && prev.Fingerprint != fp:
		// A different file now sits at the same path after rename rotation.
		start = 0
	}
	if start > st.Size() {
		start = st.Size()
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}

	var (
		batches  []batch
		rd       = newLineReader(f, h.task.Input.Encoding)
		ml       = newMultiline(h.task.Input.Multiline)
		offset   = start
		curBatch = batch{task: &h.task, path: h.path, inode: inodeOf(st), fp: fp}
		maxBytes = int64(4 << 20) // 4MB per batch keeps memory bounded
	)

	for {
		select {
		case <-ctx.Done():
			if len(curBatch.events) > 0 {
				batches = append(batches, curBatch)
			}
			return batches, nil
		default:
		}

		lineStart := offset
		line, n, err := rd.ReadLine()
		if n > 0 {
			offset += int64(n)
		}
		if line != "" {
			if complete, ev, st, en := ml.push(line, lineStart, offset); complete {
				curBatch.events = append(curBatch.events, newEvent(h.task, h.path, st, ev, h.agent, h.version))
				curBatch.endOff = en
			}
			// A pending multiline block holds memory; flush it rather than let
			// one endless stack trace stall the output.
			if ml.pendingBytes() > 1<<20 {
				if ev, ok, st, en := ml.flush(offset); ok {
					curBatch.events = append(curBatch.events, newEvent(h.task, h.path, st, ev, h.agent, h.version))
					curBatch.endOff = en
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				if ev, ok, st, en := ml.flush(offset); ok {
					curBatch.events = append(curBatch.events, newEvent(h.task, h.path, st, ev, h.agent, h.version))
					curBatch.endOff = en
				}
			}
			break
		}
		if len(curBatch.events) >= 500 || curBatch.endOff-start >= maxBytes {
			batches = append(batches, curBatch)
			curBatch = batch{task: &h.task, path: h.path, inode: inodeOf(st), fp: fp}
		}
	}
	if len(curBatch.events) > 0 {
		batches = append(batches, curBatch)
	}
	return batches, nil
}

// lineReader splits the file on \n first and decodes each line afterwards, so
// the consumed byte count always matches the real file offset even when the
// encoding changes the byte length (GBK → UTF-8).
type lineReader struct {
	f   *os.File
	raw []byte
	eof bool
	enc string
}

func newLineReader(f *os.File, enc string) *lineReader {
	return &lineReader{f: f, enc: strings.ToLower(strings.TrimSpace(enc))}
}

// ReadLine returns one line without the trailing newline, the bytes consumed
// (including it), and io.EOF at the end. A trailing partial line is not
// returned: the writer may still be mid-line, so the offset must not advance.
func (l *lineReader) ReadLine() (string, int, error) {
	for {
		if i := bytes.IndexByte(l.raw, '\n'); i >= 0 {
			line := l.raw[:i]
			l.raw = l.raw[i+1:]
			return l.decode(line), i + 1, nil
		}
		if l.eof {
			if len(l.raw) > 0 && l.raw[len(l.raw)-1] == '\n' {
				// Defensive: a line terminated at EOF without content.
				l.raw = nil
				return "", 0, io.EOF
			}
			return "", 0, io.EOF
		}
		tmp := make([]byte, 64<<10)
		n, err := l.f.Read(tmp)
		if n > 0 {
			l.raw = append(l.raw, tmp[:n]...)
		}
		if err == io.EOF {
			l.eof = true
		} else if err != nil {
			return "", 0, err
		}
	}
}

func (l *lineReader) decode(b []byte) string {
	s := strings.TrimSuffix(string(b), "\r")
	switch l.enc {
	case "gbk", "gb18030", "gb2312":
		if out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), []byte(s)); err == nil {
			return string(out)
		}
	case "latin1", "iso-8859-1":
		r := make([]rune, 0, len(s))
		for i := 0; i < len(s); i++ {
			r = append(r, rune(s[i]))
		}
		return string(r)
	}
	return s
}

// multiline implements filebeat's pattern/negate/match semantics.
type multiline struct {
	cfg   *Multiline
	re    *regexp.Regexp
	buf   []string
	bytes int
	// start is the file offset where the buffered event begins. log.offset must
	// point at the first line of the record, not at wherever the buffer happened
	// to be flushed, otherwise every event reports the offset of the next one.
	start int64
}

func newMultiline(cfg *Multiline) *multiline {
	if cfg == nil || cfg.Pattern == "" {
		return &multiline{}
	}
	re, err := regexp.Compile(cfg.Pattern)
	if err != nil {
		return &multiline{}
	}
	return &multiline{cfg: cfg, re: re}
}

// push adds a line and reports whether a complete event is ready, together with
// the [start,end) byte range the event covers.
func (m *multiline) push(line string, lineStart, lineEnd int64) (bool, string, int64, int64) {
	if m.cfg == nil || m.re == nil {
		return true, line, lineStart, lineEnd
	}
	matched := m.re.MatchString(line)
	if m.cfg.Negate {
		matched = !matched
	}
	// filebeat semantics, which are easy to get backwards:
	//   match: after  → a line that MATCHES the pattern continues the previous
	//                   event (pattern "^\s" merges stack traces)
	//   match: before → a line that MATCHES the pattern STARTS a new event
	//                   (the pattern marks the first line of each record)
	if m.cfg.Match == "before" {
		if matched && len(m.buf) > 0 {
			out, start := strings.Join(m.buf, "\n"), m.start
			m.reset()
			m.buf = append(m.buf, line)
			m.bytes += len(line)
			m.start = lineStart
			return true, out, start, lineStart
		}
		if len(m.buf) == 0 {
			m.start = lineStart
		}
		m.buf = append(m.buf, line)
		m.bytes += len(line)
		return false, "", 0, 0
	}
	if !matched && len(m.buf) > 0 {
		out, start := strings.Join(m.buf, "\n"), m.start
		m.reset()
		m.buf = append(m.buf, line)
		m.bytes += len(line)
		m.start = lineStart
		return true, out, start, lineStart
	}
	if len(m.buf) == 0 {
		m.start = lineStart
	}
	m.buf = append(m.buf, line)
	m.bytes += len(line)
	if m.cfg.MaxLines > 0 && len(m.buf) >= m.cfg.MaxLines {
		out := strings.Join(m.buf, "\n")
		start := m.start
		m.reset()
		return true, out, start, lineEnd
	}
	return false, "", 0, 0
}

// flush returns whatever is buffered (EOF or shutdown).
func (m *multiline) flush(at int64) (string, bool, int64, int64) {
	if len(m.buf) == 0 {
		return "", false, 0, 0
	}
	out, start := strings.Join(m.buf, "\n"), m.start
	m.reset()
	return out, true, start, at
}

func (m *multiline) pendingBytes() int { return m.bytes }

func (m *multiline) reset() {
	m.buf = nil
	m.bytes = 0
	m.start = 0
}

// --- path helpers --------------------------------------------------------

func inodeOf(st os.FileInfo) uint64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return s.Ino
	}
	return 0
}

// withinPaths checks the file against the task's globs (filebeat semantics).
func withinPaths(patterns []string, path string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
	}
	return false
}

func excluded(patterns []string, path string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
	}
	return false
}
