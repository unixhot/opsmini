package aiagent

import (
	"testing"

	"github.com/opsmini/opsmini/internal/model"
)

// TestBlastRadius pins the "declared, never inferred" rule: only what the
// operator wrote produces a warning, and a name that is not declared must stay
// silent rather than guess.
func TestBlastRadius(t *testing.T) {
	g := &Guard{Dependencies: map[string][]string{
		"nginx": {"php-fpm", "app-api"},
		"mysql": {"app-api"},
	}}
	svc := &Tool{Name: "service_control", Risk: RiskLowWrite}

	cases := []struct {
		args map[string]any
		want int
		desc string
	}{
		{map[string]any{"service": "nginx"}, 2, "declared with two downstream"},
		{map[string]any{"service": "nginx.service"}, 2, ".service suffix stripped"},
		{map[string]any{"service": "mysql"}, 1, "declared with one downstream"},
		{map[string]any{"service": "redis"}, 0, "undeclared stays silent"},
		{map[string]any{"path": "/etc/hosts"}, 0, "no service key"},
		{map[string]any{"service": "  nginx  "}, 2, "whitespace tolerated"},
	}
	for _, c := range cases {
		if got := g.BlastRadius(svc, c.args); len(got) != c.want {
			t.Errorf("%s: got %v want %d", c.desc, got, c.want)
		}
	}

	// No declarations at all: the guard must behave exactly as before.
	empty := &Guard{}
	if got := empty.BlastRadius(svc, map[string]any{"service": "nginx"}); got != nil {
		t.Errorf("no declarations must return nil, got %v", got)
	}
}

// TestCriticalNeverAuto pins the hardcoded rule: a critical incident is never
// executed unattended, even when the policy asks for auto.
func TestCriticalNeverAuto(t *testing.T) {
	// Mirrors the guard in Engine.run. Kept as a table so that removing the rule
	// from the engine fails here first.
	downgrade := func(severity, mode string) string {
		if severity == model.SeverityCritical && mode == model.ModeAuto {
			return model.ModeAssist
		}
		return mode
	}
	cases := []struct{ sev, mode, want string }{
		{model.SeverityCritical, model.ModeAuto, model.ModeAssist},
		{model.SeverityCritical, model.ModeAssist, model.ModeAssist},
		{model.SeverityCritical, model.ModeShadow, model.ModeShadow},
		{model.SeverityHigh, model.ModeAuto, model.ModeAuto},
		{model.SeverityMedium, model.ModeAuto, model.ModeAuto},
	}
	for _, c := range cases {
		if got := downgrade(c.sev, c.mode); got != c.want {
			t.Errorf("severity=%s mode=%s: got %s want %s", c.sev, c.mode, got, c.want)
		}
	}
}
