package sentinel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// changeGroupKey is what the change fan-out produces for one object.
func changeSignalFor(subject string, at time.Time) model.Signal {
	s := newSignal(model.SignalKindChange, "change.service", subject, model.FamilyChange,
		"change|service|"+subject, model.SeverityInfo, "服务状态变更").Signal
	s.ObservedAt = at
	return s
}

func bruteSignalFor(ip string, at time.Time) model.Signal {
	s := newSignal(model.SignalKindAuth, "authwatch/sshd", ip, "auth.anomaly",
		"auth.failed_burst", model.SeverityMedium, "登录失败激增：来自 "+ip).Signal
	s.Fingerprint = "auth.failed_burst"
	s.ObservedAt = at
	return s
}

// A service stuck in a restart loop must produce one incident with a rising
// counter, not one incident per restart — even when the restarts are spread far
// apart, because the object is still the same object.
func TestServiceFlapFoldsAcrossLongGaps(t *testing.T) {
	db := newDB(t)
	agg := NewAggregator(db, 10*time.Minute)

	base := time.Now().Add(-time.Hour)
	var first model.Incident
	for i := 0; i < 5; i++ {
		s := changeSignalFor("zabbix-agent", base.Add(time.Duration(i)*30*time.Minute))
		s.ID = "chg_sig_" + string(rune('a'+i))
		inc, created := agg.Add(s)
		if i == 0 {
			if !created {
				t.Fatalf("first signal must create the incident")
			}
			first = inc
			continue
		}
		if created {
			t.Fatalf("signal %d opened a second incident: %+v", i, inc)
		}
		if inc.ID != first.ID {
			t.Fatalf("signal %d landed on %s, want %s", i, inc.ID, first.ID)
		}
	}

	var incidents []model.Incident
	db.Where("family = ?", model.FamilyChange).Find(&incidents)
	if len(incidents) != 1 {
		t.Fatalf("expected 1 aggregate incident, got %d", len(incidents))
	}
	if incidents[0].Count != 5 {
		t.Fatalf("expected count=5, got %d", incidents[0].Count)
	}
	if incidents[0].GroupKey != "change|zabbix-agent" {
		t.Fatalf("unexpected group key %q", incidents[0].GroupKey)
	}
}

// A brute force from a dozen addresses is one attack: one incident whose member
// list grows, titled with the number of sources.
func TestBruteForceFoldsAcrossSources(t *testing.T) {
	db := newDB(t)
	agg := NewAggregator(db, 10*time.Minute)

	ips := []string{"80.94.92.31", "80.94.92.28", "2.57.122.245"}
	now := time.Now()
	for i, ip := range ips {
		s := bruteSignalFor(ip, now.Add(time.Duration(i)*time.Minute))
		s.ID = "auth_sig_" + string(rune('a'+i))
		if _, created := agg.Add(s); i > 0 && created {
			t.Fatalf("source %s opened a second incident", ip)
		}
	}

	var incidents []model.Incident
	db.Where("family = ?", "auth.anomaly").Find(&incidents)
	if len(incidents) != 1 {
		t.Fatalf("expected 1 aggregate incident, got %d", len(incidents))
	}
	inc := incidents[0]
	if inc.Count != 3 {
		t.Fatalf("expected count=3, got %d", inc.Count)
	}
	if inc.MemberTotal != 3 {
		t.Fatalf("expected member_total=3, got %d", inc.MemberTotal)
	}
	if inc.Title != "SSH 爆破 · 涉及 3 个源 IP" {
		t.Fatalf("unexpected title %q", inc.Title)
	}
	if inc.Subject != "SSH 爆破" {
		t.Fatalf("unexpected subject %q", inc.Subject)
	}
	var members []string
	if err := json.Unmarshal([]byte(inc.Members), &members); err != nil {
		t.Fatalf("members is not JSON: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %v", members)
	}
}

// A handled incident must not swallow the future: once a human ignored it, the
// next signal has to open a fresh row so it can be noticed again.
func TestIgnoredIncidentStartsFresh(t *testing.T) {
	db := newDB(t)
	agg := NewAggregator(db, 10*time.Minute)

	now := time.Now()
	s1 := bruteSignalFor("80.94.92.31", now)
	s1.ID = "auth_sig_x"
	inc, _ := agg.Add(s1)
	db.Model(&model.Incident{}).Where("id = ?", inc.ID).Update("status", model.IncidentIgnored)

	s2 := bruteSignalFor("80.94.92.31", now.Add(time.Minute))
	s2.ID = "auth_sig_y"
	next, created := agg.Add(s2)
	if !created {
		t.Fatalf("a signal after an ignored incident must open a new one")
	}
	if next.ID == inc.ID {
		t.Fatalf("signal was folded back into the ignored incident")
	}
}

// Distinct successful logins must stay per-address: folding "root logged in
// from a new place" into a bucket would hide it.
func TestAcceptedLoginsStayPerAddress(t *testing.T) {
	db := newDB(t)
	agg := NewAggregator(db, 10*time.Minute)

	now := time.Now()
	for i, ip := range []string{"10.0.0.1", "10.0.0.2"} {
		s := newSignal(model.SignalKindAuth, "authwatch/sshd", ip, "auth.anomaly",
			"auth.accepted", model.SeverityMedium, "登录："+ip).Signal
		s.Fingerprint = "auth.accepted"
		s.ID = "auth_login_" + string(rune('a'+i))
		s.ObservedAt = now
		if _, created := agg.Add(s); !created {
			t.Fatalf("login from %s must not be folded into %s", ip, "another address")
		}
	}
}
