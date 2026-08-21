package service

import (
	"os"
	"path/filepath"
	"strings"
)

// SystemCronJob is a read-only task in the system/user crontab.
type SystemCronJob struct {
	Source   string `json:"source"`   // source file path
	User     string `json:"user"`     // owning user
	Schedule string `json:"schedule"` // cron time expression (5 fields or @reboot, etc.)
	Command  string `json:"command"`  // command to execute
}

// CrontabService reads the system crontab (/etc/crontab, /etc/cron.d/*, /var/spool/cron/*) read-only.
type CrontabService struct{}

// NewCrontabService creates a CrontabService.
func NewCrontabService() *CrontabService { return &CrontabService{} }

// SystemJobs reads system-level crontab (/etc/crontab + /etc/cron.d/*).
func (s *CrontabService) SystemJobs() []SystemCronJob {
	out := []SystemCronJob{}
	// /etc/crontab: 5 time fields + user + command
	if jobs := s.parseFile("/etc/crontab", true); jobs != nil {
		out = append(out, jobs...)
	}
	// /etc/cron.d/*: 5 time fields + user + command
	if entries, err := os.ReadDir("/etc/cron.d"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			p := filepath.Join("/etc/cron.d", e.Name())
			if jobs := s.parseFile(p, true); jobs != nil {
				out = append(out, jobs...)
			}
		}
	}
	return out
}

// UserJobs reads each user's crontab (/var/spool/cron/*).
func (s *CrontabService) UserJobs() []SystemCronJob {
	out := []SystemCronJob{}
	for _, dir := range []string{"/var/spool/cron", "/var/spool/cron/crontabs"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			user := e.Name()
			p := filepath.Join(dir, user)
			if jobs := s.parseFile(p, false); jobs != nil {
				out = append(out, jobs...)
			}
		}
	}
	return out
}

// parseFile parses a crontab file. hasUserField indicates whether a user column
// follows the time fields (/etc/crontab and /etc/cron.d/* have one, /var/spool/cron/* does not).
func (s *CrontabService) parseFile(path string, hasUserField bool) []SystemCronJob {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []SystemCronJob
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// skip environment variable lines (SHELL=/bin/bash etc.)
		if eq := strings.IndexByte(line, '='); eq > 0 && !strings.Contains(line[:eq], " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		job := SystemCronJob{
			Source:   path,
			Schedule: strings.Join(fields[:5], " "),
		}
		if hasUserField {
			job.User = fields[5]
			job.Command = strings.Join(fields[6:], " ")
		} else {
			job.User = filepath.Base(path)
			job.Command = strings.Join(fields[5:], " ")
		}
		if job.Command == "" {
			continue
		}
		out = append(out, job)
	}
	return out
}
