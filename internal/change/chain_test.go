package change

import (
	"testing"

	"github.com/opsmini/opsmini/internal/model"
)

// The second-layer rule is narrow by design. These tests pin the narrowness: a
// regression that lets ordinary config edits into a kill chain would turn every
// maintenance window into an "attack chain" banner.

func TestChainStageRequiresSuspiciousVerdict(t *testing.T) {
	// The same path, three verdicts: only "suspicious" may join a chain.
	cases := []struct {
		verdict string
		want    bool
	}{
		{model.ChangeVerdictSuspicious, true},
		{model.ChangeVerdictUnclear, false},
		{model.ChangeVerdictNormal, false},
		{"", false},
		{"whatever", false},
	}
	for _, c := range cases {
		stage, ok := ChainStage(model.ChangeKindConfig, "/root/.ssh/authorized_keys", c.verdict)
		if ok != c.want {
			t.Errorf("verdict %q: ok = %v, want %v", c.verdict, ok, c.want)
		}
		if ok && stage != "persistence" {
			t.Errorf("verdict %q: stage = %q, want persistence", c.verdict, stage)
		}
	}
}

func TestChainStagePersistencePaths(t *testing.T) {
	yes := []string{
		"/root/.ssh/authorized_keys",
		"/home/deploy/.ssh/authorized_keys",
		"/etc/sudoers",
		"/etc/sudoers.d/90-cloud",
		"/etc/passwd",
		"/etc/shadow",
		"/etc/pam.d/sshd",
		"/etc/ld.so.preload",
		"/etc/cron.d/backup",
		"/var/spool/cron/crontabs/root",
		"/etc/systemd/system/sshd.service",
		"/lib/systemd/system/x.service",
		"/etc/rc.local",
		"/etc/profile.d/x.sh",
		"/etc/bash.bashrc",
		"/etc/ssh/sshd_config",
		// trailing slash / doubled separators must not defeat the match
		"/etc/sudoers.d/",
		"//etc//cron.d/job",
	}
	for _, p := range yes {
		if _, ok := ChainStage(model.ChangeKindConfig, p, model.ChangeVerdictSuspicious); !ok {
			t.Errorf("%q should be a persistence stage", p)
		}
	}

	no := []string{
		"/etc/nginx/nginx.conf",
		"/etc/hosts",
		"/etc/resolv.conf",
		"/etc/sysctl.conf",
		"/etc/logrotate.d/app",
		"/var/log/app/app.log",
		"/etc/ssl/certs/ca.pem",
		"/root/.sshx/authorized_keys", // prefix must be a real path boundary
		"",
	}
	for _, p := range no {
		if stage, ok := ChainStage(model.ChangeKindConfig, p, model.ChangeVerdictSuspicious); ok {
			t.Errorf("%q must not be a chain stage (got %q)", p, stage)
		}
	}
}

func TestChainStageKinds(t *testing.T) {
	// An account or cron change is a persistence mechanism by definition, whatever
	// the path looks like.
	for _, kind := range []string{model.ChangeKindAccount, model.ChangeKindCron} {
		if _, ok := ChainStage(kind, "some-user", model.ChangeVerdictSuspicious); !ok {
			t.Errorf("kind %q should map to persistence", kind)
		}
	}
	// Package / service / firewall changes are operational, never a chain stage.
	for _, kind := range []string{model.ChangeKindPackage, model.ChangeKindService, model.ChangeKindFirewall} {
		if _, ok := ChainStage(kind, "/etc/passwd", model.ChangeVerdictSuspicious); ok {
			t.Errorf("kind %q must not map to a chain stage", kind)
		}
	}
}
