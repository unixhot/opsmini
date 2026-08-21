package service

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrLogNotFound indicates the log file does not exist.
var ErrLogNotFound = errors.New("log file not found")

// ErrLogNotAllowed indicates the log path is not in the allowlist.
var ErrLogNotAllowed = errors.New("log path not allowed")

// LogSource is a readable log source.
type LogSource struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // system / website
	Size int64  `json:"size"`
}

// LogService provides read-only access to system/website logs.
type LogService struct {
	candidates []LogSource // allowlist: only these log files may be read
}

// NewLogService creates a log service with a predefined allowlist of common log files.
func NewLogService() *LogService {
	return &LogService{candidates: []LogSource{
		{Name: "syslog", Path: "/var/log/syslog", Type: "system"},
		{Name: "messages", Path: "/var/log/messages", Type: "system"},
		{Name: "auth.log", Path: "/var/log/auth.log", Type: "system"},
		{Name: "secure", Path: "/var/log/secure", Type: "system"},
		{Name: "kern.log", Path: "/var/log/kern.log", Type: "system"},
		{Name: "system.log", Path: "/var/log/system.log", Type: "system"},
		{Name: "Nginx access.log", Path: "/var/log/nginx/access.log", Type: "website"},
		{Name: "Nginx error.log", Path: "/var/log/nginx/error.log", Type: "website"},
		{Name: "Apache access.log", Path: "/var/log/apache2/access.log", Type: "website"},
		{Name: "Apache error.log", Path: "/var/log/apache2/error.log", Type: "website"},
	}}
}

// List returns the log sources that actually exist.
func (s *LogService) List() []LogSource {
	out := make([]LogSource, 0, len(s.candidates))
	for _, c := range s.candidates {
		if fi, err := os.Stat(c.Path); err == nil && !fi.IsDir() {
			c.Size = fi.Size()
			out = append(out, c)
		}
	}
	return out
}

// Tail reads the last lines lines of an allowlisted log file (at most 1MB).
func (s *LogService) Tail(path string, lines int) (string, error) {
	if !s.allowed(path) {
		return "", ErrLogNotAllowed
	}
	if lines <= 0 || lines > 1000 {
		lines = 100
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrLogNotFound
		}
		return "", err
	}
	defer f.Close()

	// read at most the last 1MB of the file to avoid large files exhausting memory.
	const maxRead = 1 << 20
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := fi.Size()
	offset := int64(0)
	if size > maxRead {
		offset = size - maxRead
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return "", err
	}

	var all []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n"), nil
}

// allowed checks whether the path is in the candidate allowlist.
func (s *LogService) allowed(path string) bool {
	p := filepath.Clean(path)
	for _, c := range s.candidates {
		if filepath.Clean(c.Path) == p {
			return true
		}
	}
	return false
}
