package service

import (
	"fmt"
	"time"

	"github.com/opsmini/opsmini/internal/repository"
)

// ModuleScore is the score of a single security module.
type ModuleScore struct {
	Score   int    `json:"score"`
	Max     int    `json:"max"`
	Summary string `json:"summary"`
}

// SecurityOverview is the security overview and scoring.
type SecurityOverview struct {
	Score    int         `json:"score"`
	Grade    string      `json:"grade"`
	Baseline ModuleScore `json:"baseline"`
	Login    ModuleScore `json:"login"`
	Firewall ModuleScore `json:"firewall"`
	Fim      ModuleScore `json:"fim"`
	Threat   ModuleScore `json:"threat"`
}

// SecurityService aggregates the security overview and scoring.
type SecurityService struct {
	baselineRepo  *repository.BaselineResultRepo
	fimChangeRepo *repository.FimChangeRepo
	threatRepo    *repository.ThreatFindingRepo
	fw            *FirewallService
	login         *LoginSecurityService
}

// NewSecurityService creates a SecurityService.
func NewSecurityService(
	baseline *repository.BaselineResultRepo,
	fimChange *repository.FimChangeRepo,
	threat *repository.ThreatFindingRepo,
	fw *FirewallService,
	login *LoginSecurityService,
) *SecurityService {
	return &SecurityService{baselineRepo: baseline, fimChangeRepo: fimChange, threatRepo: threat, fw: fw, login: login}
}

// Overview computes the security score and per-module summaries.
func (s *SecurityService) Overview() (*SecurityOverview, error) {
	ov := &SecurityOverview{}

	// baseline (out of 40)
	ov.Baseline = ModuleScore{Max: 40}
	if results, err := s.baselineRepo.LatestScanID(); err == nil && results != "" {
		items, _ := s.baselineRepo.ListByScanID(results)
		if len(items) > 0 {
			totalWeight, got := 0, 0.0
			for _, it := range items {
				totalWeight += it.Weight
				switch it.Status {
				case "pass":
					got += float64(it.Weight)
				case "warn":
					got += float64(it.Weight) * 0.5
				}
			}
			if totalWeight > 0 {
				ov.Baseline.Score = int(40 * got / float64(totalWeight))
			}
			pass, fail := 0, 0
			for _, it := range items {
				if it.Status == "pass" {
					pass++
				} else if it.Status == "fail" {
					fail++
				}
			}
			ov.Baseline.Summary = summaryf("%d 项通过 / %d 项未通过", pass, fail)
		}
	} else {
		ov.Baseline.Summary = "尚未执行体检"
	}

	// login security (out of 20)
	ov.Login = ModuleScore{Max: 20, Score: 20}
	if report, err := s.login.Analyze(); err == nil {
		ov.Login.Score = 20
		if n := len(report.BruteForce); n > 0 {
			ov.Login.Score = 20 - n*5
			if ov.Login.Score < 0 {
				ov.Login.Score = 0
			}
			ov.Login.Summary = summaryf("发现 %d 个疑似暴力破解目标", n)
		} else if report.FailedCount24h > 0 {
			ov.Login.Score = 16
			ov.Login.Summary = summaryf("24 小时内 %d 次失败登录", report.FailedCount24h)
		} else {
			ov.Login.Summary = "未发现暴力破解"
		}
	} else {
		ov.Login.Summary = "无法读取认证日志"
	}

	// firewall (out of 15)
	ov.Firewall = ModuleScore{Max: 15}
	if s.fw.Status().Enabled {
		ov.Firewall.Score = 15
		ov.Firewall.Summary = "防火墙已启用"
	} else {
		ov.Firewall.Summary = "防火墙未启用"
	}

	// FIM (out of 15)
	ov.Fim = ModuleScore{Max: 15, Score: 15}
	if n, err := s.fimChangeRepo.CountRecent(time.Now().Add(-24 * time.Hour)); err == nil && n > 0 {
		// roughly deduct points by change level (simplified to 3 per change)
		ov.Fim.Score = 15 - int(n)*3
		if ov.Fim.Score < 0 {
			ov.Fim.Score = 0
		}
		ov.Fim.Summary = summaryf("24 小时内 %d 处关键文件变更", n)
	} else {
		ov.Fim.Summary = "关键文件未发生变更"
	}

	// threat (out of 10)
	ov.Threat = ModuleScore{Max: 10, Score: 10}
	threats, _ := s.threatRepo.ListActive()
	if len(threats) > 0 {
		for _, t := range threats {
			switch t.Severity {
			case "critical":
				ov.Threat.Score -= 5
			case "warning":
				ov.Threat.Score -= 2
			default:
				ov.Threat.Score -= 1
			}
		}
		if ov.Threat.Score < 0 {
			ov.Threat.Score = 0
		}
		ov.Threat.Summary = summaryf("%d 个未处理威胁", len(threats))
	} else {
		ov.Threat.Summary = "未发现威胁"
	}

	ov.Score = ov.Baseline.Score + ov.Login.Score + ov.Firewall.Score + ov.Fim.Score + ov.Threat.Score
	ov.Grade = gradeOf(ov.Score)
	return ov, nil
}

// summaryf formats a security finding summary string.
func summaryf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// gradeOf maps a 0-100 security score to a grade label.
func gradeOf(score int) string {
	switch {
	case score >= 90:
		return "优秀"
	case score >= 75:
		return "良好"
	case score >= 60:
		return "一般"
	default:
		return "危险"
	}
}
