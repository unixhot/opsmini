//go:build !windows

package service

// defaultFimPaths are the key files monitored by default.
var defaultFimPaths = []string{
	"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/ssh/sshd_config",
	"/etc/crontab", "/etc/sudoers", "/etc/hosts", "/etc/resolv.conf",
}

// criticalFimPaths are key files whose changes are classified as critical.
var criticalFimPaths = map[string]bool{
	"/etc/passwd": true, "/etc/shadow": true, "/etc/sudoers": true,
}
