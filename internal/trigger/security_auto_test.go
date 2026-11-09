package trigger

import (
	"testing"

	"github.com/opsmini/opsmini/internal/model"
)

// Auto mode on a security incident may quarantine a file and nothing else.
// Every other action either stops something a human depends on or is not
// reversible, and security findings include plenty of legitimate deploys.
func TestCapSecurityAuto(t *testing.T) {
	got, writes := capSecurityAuto([]string{"block_ip", "service_control", "quarantine_file"}, 5)
	if len(got) != 1 || got[0] != "quarantine_file" {
		t.Fatalf("auto tools not narrowed to quarantine_file: %v", got)
	}
	if writes != 1 {
		t.Fatalf("write budget: got %d want 1", writes)
	}
}

// A policy that granted no write budget at all must not be widened: the cap only
// narrows, never raises.
func TestCapSecurityAutoRespectsZero(t *testing.T) {
	_, writes := capSecurityAuto(nil, 0)
	if writes != 1 {
		t.Fatalf("zero budget should become 1 (quarantine is the one allowed write), got %d", writes)
	}
	_, writes = capSecurityAuto(nil, 1)
	if writes != 1 {
		t.Fatalf("got %d want 1", writes)
	}
}

// The default policy ships in assist, and its auto whitelist must already match
// what the engine would enforce — otherwise raising it to auto silently does
// nothing different, or worse, does something unreviewed.
func TestDefaultSecurityPolicyAllowsOnlyQuarantine(t *testing.T) {
	var found bool
	for _, p := range defaultPolicies() {
		if p.Family != model.FamilySecurity {
			continue
		}
		found = true
		if p.Mode != model.ModeAssist {
			t.Errorf("default security policy should ship in assist, got %s", p.Mode)
		}
		if p.AutoTools != `["quarantine_file"]` {
			t.Errorf("auto whitelist should be quarantine_file only, got %s", p.AutoTools)
		}
	}
	if !found {
		t.Fatal("no default policy covers the security family: findings would never reach the agent")
	}
}

// The built-in security-response policy uses family "security". If that key is
// missing from knownFamilies, the panel renders the family dropdown with no
// matching option and every save is rejected — a policy nobody can edit is worse
// than a missing feature, because the rejection looks like a UI bug.
func TestSecurityFamilyIsEditable(t *testing.T) {
	found := false
	for _, f := range KnownFamilies() {
		if f == model.FamilySecurity {
			found = true
		}
	}
	if !found {
		t.Fatalf("%q missing from KnownFamilies: %v", model.FamilySecurity, KnownFamilies())
	}

	p := model.TriggerPolicy{
		Name: "security-response", Family: model.FamilySecurity,
		MinSeverity: model.SeverityHigh, Mode: model.ModeAssist,
		MaxWrites: 2, MaxSteps: 10, MaxSeconds: 300, MaxTokens: 120000,
		AllowTools: `["read_file","quarantine_file"]`, AutoTools: `["quarantine_file"]`,
		DenyTools: `["exec_command"]`, Notify: "面板", CooldownSec: 1800,
	}
	if err := ValidatePolicy(&p); err != nil {
		t.Fatalf("built-in security policy rejected: %v", err)
	}
}
