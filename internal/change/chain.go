package change

import (
	"strings"

	"github.com/opsmini/opsmini/internal/model"
)

// Second-layer correlation: a *suspicious* change as a stage of an attack chain.
//
// The rule that change events never open a chain stays as it is. "Someone edited
// nginx.conf" is a fact about operations, and promoting it to an attack stage would
// turn normal maintenance into a breach report — the exact failure mode that makes
// operators stop reading correlation output.
//
// What is worth linking is narrower and much more specific: a change that the rules
// already judged suspicious, aimed at a mechanism that survives a reboot or a
// re-login, occurring right after something was seen executing. In that shape the
// change is not "a config edit", it is the persistence step of an intrusion, and
// grouping it with the webshell that preceded it is the difference between reading
// three unrelated symptoms and reading one story.
//
// Everything here is a pure function of (kind, path, verdict) so it can be unit
// tested without a database, and so an operator can argue with it.

// persistencePaths are the mechanisms that re-establish access or code execution
// after the intrusion's original foothold is gone. Deliberately narrower than
// sensitivePaths: /etc/hosts or /etc/sysctl.conf are sensitive but do not persist
// anything on their own, and including them would let ordinary network tuning join
// an attack chain.
var persistencePaths = []string{
	// credentials and privilege escalation
	"/etc/passwd", "/etc/shadow", "/etc/gshadow", "/etc/group",
	"/etc/sudoers", "/etc/sudoers.d", "/etc/security",
	// remote login
	"/etc/ssh/sshd_config", "/root/.ssh", "/home", "/etc/pam.d",
	// code injection into every process
	"/etc/ld.so.preload", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	// scheduled and boot-time execution
	"/etc/crontab", "/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily",
	"/var/spool/cron", "/etc/systemd/system", "/lib/systemd/system",
	"/usr/lib/systemd/system", "/etc/rc.local", "/etc/init.d", "/etc/rc.d",
	// login-time scripts
	"/etc/profile", "/etc/profile.d", "/etc/bash.bashrc", "/etc/bashrc",
	"/etc/environment", "/etc/zsh",
	// kernel modules
	"/etc/modules", "/etc/modprobe.d", "/etc/modules-load.d",
}

// ChainStage reports whether this change can be a kill-chain stage, and which one.
//
// Only "persistence" is ever returned. A change cannot be initial access (that is a
// login, not an edit), cannot be execution (the collector would have seen a process),
// and cannot be c2 (that is network traffic). Anything else returns false, which
// means "stays standalone" — the normal, honest answer.
func ChainStage(kind, objectKey, verdict string) (string, bool) {
	// Unreviewed is not the same as cleared, but it is also not evidence: without a
	// suspicious verdict this is just a recorded edit.
	if verdict != model.ChangeVerdictSuspicious {
		return "", false
	}
	switch kind {
	case model.ChangeKindAccount, model.ChangeKindCron:
		// An account or a scheduled job IS a persistence mechanism by definition,
		// whatever the path looks like.
		return "persistence", true
	case model.ChangeKindConfig:
		if pathUnderAny(strings.TrimSpace(objectKey), persistencePaths) {
			return "persistence", true
		}
	}
	return "", false
}
