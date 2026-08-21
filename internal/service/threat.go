package service

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// ThreatService performs lightweight threat detection (suspicious processes/cron jobs/startup items).
type ThreatService struct {
	repo *repository.ThreatFindingRepo
}

// NewThreatService creates a ThreatService.
func NewThreatService(repo *repository.ThreatFindingRepo) *ThreatService {
	return &ThreatService{repo: repo}
}

var minerKeywords = []string{
	"xmrig", "minerd", "cpuminer", "kdevtmpfsi", "kinsing", "kthreaddi",
	"cryptonight", "stratum", "minergate", "nanominer", "xmr-stak",
}

// Scan runs a round of threat scanning and deduplicates findings into the database.
func (s *ThreatService) Scan() ([]model.ThreatFinding, error) {
	findings := append(s.scanProcesses(), s.scanCron()...)
	findings = append(findings, s.scanStartup()...)

	now := time.Now()
	saved := make([]model.ThreatFinding, 0, len(findings))
	for _, f := range findings {
		existing, err := s.repo.FindByKindName(f.Kind, f.Name)
		if err == nil && existing != nil {
			existing.LastSeen = now
			existing.Detail = f.Detail
			if existing.Status == "active" {
				existing.Severity = f.Severity
			}
			_ = s.repo.Update(existing)
			saved = append(saved, *existing)
			continue
		}
		f.FirstSeen = now
		f.LastSeen = now
		f.Status = "active"
		_ = s.repo.Create(&f)
		saved = append(saved, f)
	}
	return saved, nil
}

// List returns the threat list.
func (s *ThreatService) List(limit int) ([]model.ThreatFinding, error) {
	return s.repo.List(limit)
}

// Resolve marks a threat as handled.
func (s *ThreatService) Resolve(id uint) error {
	list, _ := s.repo.List(1000)
	for i := range list {
		if list[i].ID == id {
			now := time.Now()
			list[i].Status = "resolved"
			list[i].ResolvedAt = &now
			return s.repo.Update(&list[i])
		}
	}
	return nil
}

// scanProcesses detects mining processes and processes in suspicious paths.
func (s *ThreatService) scanProcesses() []model.ThreatFinding {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	var out []model.ThreatFinding
	for _, p := range procs {
		name, _ := p.Name()
		cmdline, _ := p.Cmdline()
		lower := strings.ToLower(name + " " + cmdline)
		matched := ""
		for _, k := range minerKeywords {
			if strings.Contains(lower, k) {
				matched = k
				break
			}
		}
		if matched != "" {
			out = append(out, model.ThreatFinding{
				Kind: "process", Name: name, Severity: "critical",
				Detail: "检测到挖矿关键词「" + matched + "」：" + cmdline,
			})
			continue
		}
		// suspicious path processes (executables under /tmp, /dev/shm)
		if exe, e := p.Exe(); e == nil && (strings.HasPrefix(exe, "/tmp/") || strings.HasPrefix(exe, "/dev/shm/")) {
			out = append(out, model.ThreatFinding{
				Kind: "process", Name: name, Severity: "warning",
				Detail: "进程可执行文件位于临时目录：" + exe,
			})
		}
	}
	return out
}

// scanCron detects suspicious cron jobs.
func (s *ThreatService) scanCron() []model.ThreatFinding {
	cronSvc := NewCrontabService()
	all := append(cronSvc.SystemJobs(), cronSvc.UserJobs()...)
	patterns := []string{"curl", "wget", "base64 -d", "/dev/shm"}
	var out []model.ThreatFinding
	for _, job := range all {
		lower := strings.ToLower(job.Command)
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				out = append(out, model.ThreatFinding{
					Kind: "cron", Name: job.Command, Severity: "warning",
					Detail: "计划任务包含可疑下载执行：" + job.Command,
				})
				break
			}
		}
	}
	return out
}

// scanStartup detects suspicious startup items.
func (s *ThreatService) scanStartup() []model.ThreatFinding {
	var out []model.ThreatFinding
	// suspicious commands in /etc/rc.local
	if data, err := os.ReadFile("/etc/rc.local"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "exit") {
				continue
			}
			lower := strings.ToLower(line)
			if strings.Contains(lower, "curl") || strings.Contains(lower, "wget") || strings.Contains(lower, "/tmp/") {
				out = append(out, model.ThreatFinding{
					Kind: "startup", Name: "rc.local", Severity: "warning",
					Detail: "可疑启动项：" + line,
				})
			}
		}
	}
	// suspicious items among systemd enabled services (scripts pointing to /tmp or /dev/shm)
	if data, err := exec.Command("systemctl", "list-unit-files", "--type=service", "--state=enabled", "--no-pager", "--no-legend").Output(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			lower := strings.ToLower(f[0])
			if strings.Contains(lower, "miner") || strings.Contains(lower, "xmrig") || strings.Contains(lower, "kdevtmpfs") {
				out = append(out, model.ThreatFinding{
					Kind: "startup", Name: f[0], Severity: "critical",
					Detail: "可疑自启动服务：" + f[0],
				})
			}
		}
	}
	return out
}
