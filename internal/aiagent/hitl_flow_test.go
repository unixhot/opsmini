package aiagent

import (
	"testing"

	"github.com/opsmini/opsmini/internal/model"
)

// TestSuspendOnConfirm pins the decision that separates "wait for a human" from
// "refuse and carry on". Getting it backwards is silent either way: a scheduled
// task would hang until its deadline, or an incident page would never offer the
// approve button.
func TestSuspendOnConfirm(t *testing.T) {
	cases := []struct {
		name string
		opt  RunOptions
		want bool
	}{
		{"interactive chat suspends", RunOptions{}, true},
		{"scheduled task refuses", RunOptions{Unattended: true}, false},
		{"event assist suspends", RunOptions{Unattended: true, SuspendOnConfirm: true}, true},
		// SuspendOnConfirm alone is meaningless without Unattended, but if someone
		// sets both it must still suspend rather than silently change behaviour.
		{"both set", RunOptions{Unattended: true, SuspendOnConfirm: true}, true},
	}
	for _, c := range cases {
		if got := suspendOnConfirm(c.opt); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestAwaitingApprovalIsDistinct ensures the new lifecycle states stay distinct
// from the existing ones. Merging them would make "needs you" indistinguishable
// from "still working" everywhere the status is compared.
func TestAwaitingApprovalIsDistinct(t *testing.T) {
	states := []string{
		model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning,
		model.IncidentAwaitingApproval, model.IncidentResolved,
		model.IncidentSkipped, model.IncidentIgnored, model.IncidentTakenOver,
		model.IncidentFailed,
	}
	seen := map[string]bool{}
	for _, s := range states {
		if seen[s] {
			t.Fatalf("duplicate incident state %q", s)
		}
		seen[s] = true
	}
	if model.IncidentAwaitingApproval == model.IncidentRunning {
		t.Fatal("awaiting_approval must not collapse into running")
	}
	if model.IncidentSkipped == model.IncidentResolved {
		t.Fatal("skipped must not collapse into resolved")
	}
}
