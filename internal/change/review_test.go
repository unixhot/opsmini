package change

import (
	"testing"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

func changeAt(kind, source, object, actor, ip, how, confidence string) Event {
	ev := NewEvent(kind, source, Object{Key: object}, time.Date(2026, 10, 29, 14, 0, 0, 0, time.Local))
	ev.Actor = Actor{User: actor, IP: ip, How: how, Confidence: confidence}
	return ev
}

// A package upgrade during business hours is the most common change on any host and
// must never cost a model call.
func TestReviewRoutinePackageUpgradeIsNormal(t *testing.T) {
	ev := changeAt(model.ChangeKindPackage, "pkg", "/etc/nginx/nginx.conf", "root", "", "pkg", model.ConfidenceConfirmed)
	rev, reasons := ScoreChange(ev, ReviewContext{ActorHistory: 5}, ev.At)

	if rev.Verdict != model.ChangeVerdictNormal {
		t.Fatalf("verdict = %s, want normal（score %.2f, reasons %v）", rev.Verdict, rev.Score, reasons)
	}
}

// The canonical persistence trick: an account added to /etc/passwd by an unknown
// actor. If this is not flagged, the feature has no reason to exist.
func TestReviewUnknownActorOnPasswdIsSuspicious(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/passwd", "", "", "", model.ConfidenceUnknown)
	rev, reasons := ScoreChange(ev, ReviewContext{}, ev.At)

	if rev.Verdict != model.ChangeVerdictSuspicious {
		t.Fatalf("verdict = %s, want suspicious（score %.2f, reasons %v）", rev.Verdict, rev.Score, reasons)
	}
	if len(reasons) == 0 {
		t.Fatal("a verdict without reasons cannot be argued with")
	}
}

// Root over SSH from a public address editing sshd_config at 2am: every signal points
// the same way, so the score should be well past the threshold.
func TestReviewRemoteRootOffHoursScoresHigh(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/ssh/sshd_config", "root", "45.9.1.2", "ssh", model.ConfidenceConfirmed)
	ev.At = time.Date(2026, 10, 29, 2, 30, 0, 0, time.Local)
	rev, reasons := ScoreChange(ev, ReviewContext{}, ev.At)

	if rev.Verdict != model.ChangeVerdictSuspicious {
		t.Fatalf("verdict = %s, want suspicious（score %.2f, reasons %v）", rev.Verdict, rev.Score, reasons)
	}
	if rev.Score < 0.85 {
		t.Fatalf("score = %.2f, want >= 0.85 for this combination", rev.Score)
	}
}

// Co-occurrence is the strongest single signal: nobody edits systemd units during a
// webshell investigation by coincidence.
func TestReviewNearbyIncidentEscalates(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/systemd/system/backup.service", "deploy", "10.0.0.8", "ssh", model.ConfidenceConfirmed)
	base, _ := ScoreChange(ev, ReviewContext{ActorHistory: 3}, ev.At)
	withEvent, reasons := ScoreChange(ev, ReviewContext{ActorHistory: 3, NearbyIncidents: 2}, ev.At)

	if withEvent.Score <= base.Score {
		t.Fatalf("nearby incident should raise the score: %.2f -> %.2f", base.Score, withEvent.Score)
	}
	if withEvent.Verdict == model.ChangeVerdictNormal {
		t.Fatalf("verdict = %s, want at least unclear（score %.2f, reasons %v）", withEvent.Verdict, withEvent.Score, reasons)
	}
}

// A file edited five times in an hour is worth a look even when everything else looks
// routine — someone is fighting it.
func TestReviewFlappingRaisesScore(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/resolv.conf", "deploy", "10.0.0.8", "ssh", model.ConfidenceConfirmed)
	base, _ := ScoreChange(ev, ReviewContext{ActorHistory: 9}, ev.At)
	flap, reasons := ScoreChange(ev, ReviewContext{ActorHistory: 9, RecentSame: 4}, ev.At)

	if flap.Score <= base.Score {
		t.Fatalf("flapping should raise the score: %.2f -> %.2f", base.Score, flap.Score)
	}
	if flap.Verdict == model.ChangeVerdictNormal {
		t.Fatalf("verdict = %s, want unclear or suspicious（reasons %v）", flap.Verdict, reasons)
	}
}

// A medium-path edit by an identified internal operator is "unclear" at most: worth
// listing, not worth a model call.
func TestReviewInternalOperatorOnConfigIsNotSuspicious(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/nginx/conf.d/app.conf", "deploy", "10.0.0.31", "ssh", model.ConfidenceConfirmed)
	rev, reasons := ScoreChange(ev, ReviewContext{ActorHistory: 12}, ev.At)

	if rev.Verdict == model.ChangeVerdictSuspicious {
		t.Fatalf("verdict = %s（score %.2f, reasons %v）, want normal/unclear", rev.Verdict, rev.Score, reasons)
	}
}

// The score must never leave 0..1, whatever combination arrives.
func TestReviewScoreIsBounded(t *testing.T) {
	ev := changeAt(model.ChangeKindConfig, "file", "/etc/sudoers", "", "45.9.1.2", "ssh", model.ConfidenceInferred)
	ev.At = time.Date(2026, 10, 31, 3, 0, 0, 0, time.Local) // Saturday 03:00
	rev, _ := ScoreChange(ev, ReviewContext{RecentSame: 50, NearbyIncidents: 9}, ev.At)
	if rev.Score < 0 || rev.Score > 1 {
		t.Fatalf("score out of range: %.3f", rev.Score)
	}
	if rev.Verdict != model.ChangeVerdictSuspicious {
		t.Fatalf("verdict = %s, want suspicious", rev.Verdict)
	}
}

func TestIsPrivateIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"10.1.2.3": true, "192.168.1.1": true, "172.16.0.9": true, "172.31.5.5": true,
		"127.0.0.1": true, "45.9.1.2": false, "172.32.0.1": false, "8.8.8.8": false,
	} {
		if got := isPrivateIP(ip); got != want {
			t.Errorf("isPrivateIP(%s) = %v, want %v", ip, got, want)
		}
	}
}
