//go:build windows

package service

import (
	"os"
	"path/filepath"
)

// defaultFimPaths are the key files monitored by default on Windows.
//
// The Linux defaults (/etc/passwd, /etc/ssh/sshd_config) have no counterpart;
// these are the files whose tampering actually matters here: name resolution,
// the legacy INI that still gets parsed, and the user's PowerShell profile
// (a classic persistence location).
func fimCandidatePaths() []string {
	sysroot := os.Getenv("SystemRoot")
	if sysroot == "" {
		sysroot = `C:\Windows`
	}
	paths := []string{
		filepath.Join(sysroot, `System32\drivers\etc\hosts`),
		filepath.Join(sysroot, `System32\drivers\etc\networks`),
		filepath.Join(sysroot, "win.ini"),
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		paths = append(paths,
			filepath.Join(up, `Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`),
			filepath.Join(up, `Documents\PowerShell\Microsoft.PowerShell_profile.ps1`),
		)
	}
	return paths
}

var defaultFimPaths = fimCandidatePaths()

// criticalFimPaths are key files whose changes are classified as critical.
// hosts is the one that can silently redirect every outbound name lookup.
var criticalFimPaths = map[string]bool{}
