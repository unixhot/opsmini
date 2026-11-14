//go:build windows

package service

import (
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows/registry"

	"github.com/opsmini/opsmini/internal/model"
)

// scanAll runs the Windows threat checks.
//
// The Unix scanners inspect /proc, /etc/rc.local and crontab; none of those
// exist here. These three look at the places persistence and malware actually
// land on Windows: the Run keys, the process table, and Defender's state.
func (s *ThreatService) scanAll() []model.ThreatFinding {
	var out []model.ThreatFinding
	out = append(out, s.scanRunKeys()...)
	out = append(out, s.scanProcessNames()...)
	out = append(out, s.scanDefender()...)
	return out
}

// scanRunKeys flags autostart entries whose command line looks like a loader.
//
// The rule is deliberately conservative because rundll32 and %TEMP% are used by
// plenty of legitimate software — a Logitech driver updater starts with
// rundll32, a wallpaper agent lives in %TEMP%. Reporting those as critical would
// train the operator to ignore the page. An entry is reported only when it
// carries a command with no benign explanation (encoded PowerShell, an
// in-memory download cradle), or when a script host runs out of a temporary
// directory. A bare rundll32 call is never reported.
func (s *ThreatService) scanRunKeys() []model.ThreatFinding {
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce`},
	}

	// Commands that are malicious regardless of context.
	strong := []string{
		"-encodedcommand", " -enc ", "downloadstring", "invoke-expression", "iex(",
		"certutil -urlcache", "certutil -decode", "bitsadmin /transfer",
		"mshta http", "mshta javascript", "regsvr32 /s /u /i:http",
	}
	// Interpreter hosts that are only suspicious next to a temp path.
	scriptHosts := []string{"powershell", "pwsh", "wscript", "cscript", "mshta"}
	tempMarkers := []string{`\temp\`, `\tmp\`, `\appdata\local\temp\`}

	var out []model.ThreatFinding
	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := key.ReadValueNames(0)
		for _, name := range names {
			val, _, verr := key.GetStringValue(name)
			if verr != nil {
				continue
			}
			lower := strings.ToLower(val)

			severity, hit := "", ""
			for _, pat := range strong {
				if strings.Contains(lower, pat) {
					severity, hit = "critical", pat
					break
				}
			}
			if severity == "" {
				hasHost, hasTemp := false, false
				for _, h := range scriptHosts {
					if strings.Contains(lower, h) {
						hasHost = true
						break
					}
				}
				for _, t := range tempMarkers {
					if strings.Contains(lower, t) {
						hasTemp = true
						hit = t
						break
					}
				}
				if hasHost && hasTemp {
					severity = "warning"
				}
			}
			if severity == "" {
				continue
			}
			out = append(out, model.ThreatFinding{
				Kind:     "startup",
				Name:     name,
				Severity: severity,
				Detail: fmt.Sprintf("自启动项 %s（%s）命中特征 %q：%s",
					name, k.path, strings.TrimSpace(hit), val),
			})
		}
		key.Close()
	}
	return out
}

// scanProcessNames matches the process table against names that are almost
// never legitimate on a business host: credential dumpers, reverse-shell
// helpers and lateral-movement tools.
func (s *ThreatService) scanProcessNames() []model.ThreatFinding {
	known := map[string]string{
		"mimikatz.exe": "凭据窃取工具",
		"procdump.exe": "常被用于导出 lsass 内存",
		"psexec.exe":   "远程执行工具",
		"psexesvc.exe": "远程执行服务端",
		"nc.exe":       "网络工具，常被用作反弹 shell",
		"ncat.exe":     "网络工具，常被用作反弹 shell",
		"netcat.exe":   "网络工具，常被用作反弹 shell",
		"wce.exe":      "凭据窃取工具",
		"gsecdump.exe": "凭据窃取工具",
	}

	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	var out []model.ThreatFinding
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		desc, hit := known[strings.ToLower(name)]
		if !hit {
			continue
		}
		exe, _ := p.Exe()
		out = append(out, model.ThreatFinding{
			Kind:     "process",
			Name:     name,
			Severity: "critical",
			Detail:   fmt.Sprintf("%s（PID %d）%s，路径：%s", name, p.Pid, desc, exe),
		})
	}
	return out
}

// scanDefender reports disabled real-time protection. Tamper protection hides
// the key entirely, and absence is not evidence of tampering, so only an
// explicit 1 is reported.
func (s *ThreatService) scanDefender() []model.ThreatFinding {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows Defender\Real-Time Protection`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("DisableRealtimeMonitoring")
	if err != nil || v != 1 {
		return nil
	}
	return []model.ThreatFinding{{
		Kind:     "config",
		Name:     "Defender 实时防护",
		Severity: "critical",
		Detail:   "Windows Defender 实时防护已被关闭，主机失去主要恶意软件拦截能力",
	}}
}
