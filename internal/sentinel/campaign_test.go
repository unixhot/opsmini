package sentinel

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newCampaignDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.Incident{}, &model.Campaign{}, &model.Signal{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedIncident(t *testing.T, db *gorm.DB, id, family, subject, severity string, at time.Time) model.Incident {
	t.Helper()
	inc := model.Incident{
		ID: id, Title: family + " · " + subject, Family: family, Subject: subject,
		GroupKey: family + "|" + subject, Severity: severity, Status: model.IncidentOpen,
		Count: 1, FirstSeen: at, LastSeen: at, CreatedAt: at, UpdatedAt: at,
	}
	if err := db.Create(&inc).Error; err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return inc
}

// A brute-force burst on its own must not open a chain: every internet-facing host
// sees failed logins, and turning that into a permanent "attack chain" would make the
// whole view worthless.
func TestCampaignDoesNotStartFromAuthNoise(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()
	inc := seedIncident(t, db, "i-auth-1", "auth.anomaly", "45.9.1.2", model.SeverityHigh, now)
	c.Attach(inc)

	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 0 {
		t.Fatalf("auth noise must not create a campaign, got %d", n)
	}
}

// A webshell opens a chain, and an outbound connection from the same host shortly
// after joins it — that is the exact story the feature exists for.
func TestCampaignChainsExecutionThenC2(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "i-web-1", "webshell", "/var/www/html/upload/x.php", model.SeverityHigh, now)
	c.Attach(web)

	var camp model.Campaign
	if err := db.First(&camp).Error; err != nil {
		t.Fatalf("webshell should open a campaign: %v", err)
	}

	netInc := seedIncident(t, db, "i-net-1", "net.anomaly", "45.9.1.2:4444", model.SeverityHigh, now.Add(6*time.Minute))
	c.Attach(netInc)

	var linked model.Incident
	db.First(&linked, "id = ?", "i-net-1")
	if linked.CampaignID != camp.ID {
		t.Fatalf("net.anomaly should join the chain, campaign=%q want %q", linked.CampaignID, camp.ID)
	}
	var after model.Campaign
	db.First(&after, "id = ?", camp.ID)
	if after.StageCount != 2 {
		t.Fatalf("stage count = %d, want 2", after.StageCount)
	}
	if after.Severity != model.SeverityCritical {
		// execution + c2 is explicitly the escalation pair.
		t.Fatalf("execution+c2 should escalate to critical, got %s", after.Severity)
	}
	if after.Narrative == "" {
		t.Fatal("narrative should be generated")
	}
}

// Operational noise must never be promoted into an attack stage: a CPU alert during a
// webshell investigation is a symptom, not a breach.
func TestCampaignIgnoresOperationalFamilies(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "i-web-2", "webshell", "/var/www/x.php", model.SeverityHigh, now)
	c.Attach(web)
	metric := seedIncident(t, db, "i-met-1", "metric", "cpu", model.SeverityHigh, now.Add(2*time.Minute))
	c.Attach(metric)

	var linked model.Incident
	db.First(&linked, "id = ?", "i-met-1")
	if linked.CampaignID != "" {
		t.Fatalf("metric must not join a campaign, got %q", linked.CampaignID)
	}
}

// A low-severity observation is not evidence of anything and must be skipped even if
// its family is part of the kill chain.
func TestCampaignSkipsLowSeverity(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()
	inc := seedIncident(t, db, "i-low-1", "proc.anomaly", "kworker", model.SeverityLow, now)
	c.Attach(inc)

	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 0 {
		t.Fatalf("low severity must not open a campaign, got %d", n)
	}
}

// Attaching twice must not double-count or duplicate stages (Attach runs on every
// signal fold, so idempotency is a correctness requirement, not a nicety).
func TestCampaignAttachIsIdempotent(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()
	inc := seedIncident(t, db, "i-web-3", "webshell", "/var/www/y.php", model.SeverityHigh, now)
	c.Attach(inc)

	var camp model.Campaign
	db.First(&camp)
	c.Attach(inc)
	c.Attach(inc)

	var after model.Campaign
	db.First(&after, "id = ?", camp.ID)
	if after.StageCount != 1 {
		t.Fatalf("stage count = %d, want 1", after.StageCount)
	}
	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 1 {
		t.Fatalf("campaigns = %d, want 1", n)
	}
}

// Quiet chains close; nothing is deleted, so history stays readable.
func TestCampaignSweepClosesQuietChains(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	old := time.Now().Add(-48 * time.Hour)
	inc := seedIncident(t, db, "i-web-4", "webshell", "/var/www/z.php", model.SeverityHigh, old)
	c.Attach(inc)

	if n := c.Sweep(12 * time.Hour); n != 1 {
		t.Fatalf("sweep closed %d, want 1", n)
	}
	var camp model.Campaign
	db.First(&camp)
	if camp.Status != model.CampaignClosed {
		t.Fatalf("status = %q, want closed", camp.Status)
	}
}

// Two execution-stage events minutes apart belong to one chain. Opening a second
// chain for each extra webshell/suspicious process is exactly the noise the feature
// exists to remove.
func TestCampaignMergesConcurrentSameStage(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "m-web", "webshell", "/var/www/a/shell.php", model.SeverityHigh, now.Add(-40*time.Minute))
	c.Attach(web)
	proc := seedIncident(t, db, "m-proc", "proc.anomaly", "/tmp/.x/sh", model.SeverityHigh, now.Add(-36*time.Minute))
	c.Attach(proc)

	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 1 {
		t.Fatalf("campaigns = %d, want 1 (same-stage events must merge)", n)
	}
	var linked model.Incident
	db.First(&linked, "id = ?", "m-proc")
	if linked.CampaignID == "" {
		t.Fatal("second execution-stage incident did not join the chain")
	}
}

// A later incident sitting at an *earlier* kill-chain stage is usually an unrelated
// scan. Without this rule a nightly port scan rewrites the story of a real intrusion.
func TestCampaignRejectsStageRegressionWithoutEvidence(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "r-web", "webshell", "/var/www/b/x.php", model.SeverityHigh, now.Add(-30*time.Minute))
	c.Attach(web)
	netInc := seedIncident(t, db, "r-net", "net.anomaly", "45.1.2.3:4444", model.SeverityHigh, now.Add(-25*time.Minute))
	c.Attach(netInc)

	// auth.anomaly = initial_access, an earlier stage, arriving after c2.
	auth := seedIncident(t, db, "r-auth", "auth.anomaly", "45.9.9.9", model.SeverityHigh, now.Add(-18*time.Minute))
	c.Attach(auth)

	var linked model.Incident
	db.First(&linked, "id = ?", "r-auth")
	if linked.CampaignID != "" {
		t.Fatalf("stage regression must not attach without shared evidence, got %q", linked.CampaignID)
	}
}

// A shared indicator is strong enough to justify folding a stage regression back in:
// the same source address showing up again is the same actor.
func TestCampaignAllowsStageRegressionWithSharedIndicator(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "s-web", "webshell", "/var/www/c/x.php", model.SeverityHigh, now.Add(-30*time.Minute))
	c.Attach(web)
	netInc := seedIncident(t, db, "s-net", "net.anomaly", "45.1.2.3:4444", model.SeverityHigh, now.Add(-25*time.Minute))
	c.Attach(netInc)

	auth := seedIncident(t, db, "s-auth", "auth.anomaly", "45.1.2.3", model.SeverityHigh, now.Add(-18*time.Minute))
	c.Attach(auth)

	var linked model.Incident
	db.First(&linked, "id = ?", "s-auth")
	if linked.CampaignID == "" {
		t.Fatal("shared source address should tie the regression back to the chain")
	}
}

// Backfill exists so an upgrade is not a blank page: incidents recorded before the
// feature existed must still form chains, measured against their own timestamps.
func TestCampaignBackfillUsesIncidentTime(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	// Three hours old, outside the live 2h window relative to "now".
	seedIncident(t, db, "b-web", "webshell", "/var/www/d/x.php", model.SeverityHigh, now.Add(-3*time.Hour))
	seedIncident(t, db, "b-net", "net.anomaly", "45.1.2.3:4444", model.SeverityHigh, now.Add(-3*time.Hour+6*time.Minute))

	if n := c.Backfill(24 * time.Hour); n != 2 {
		t.Fatalf("backfill attached %d, want 2", n)
	}
	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 1 {
		t.Fatalf("campaigns = %d, want 1 (backfill should chain them)", n)
	}
}

// --- second-layer correlation: a suspicious change as the persistence stage ---

// stubChangeStage returns a fixed stage/ok for every change incident, so the
// campaigner's own rules can be tested without the change package.
func stubChangeStage(stage string, ok bool) func(model.Incident) (string, bool) {
	return func(inc model.Incident) (string, bool) {
		if inc.Family != model.FamilyChange {
			return "", false
		}
		return stage, ok
	}
}

func changeIncident(t *testing.T, db *gorm.DB, id, subject string, at time.Time) model.Incident {
	t.Helper()
	return seedIncident(t, db, id, model.FamilyChange, subject, model.SeverityHigh, at)
}

// The story this exists for: a webshell lands, and eight minutes later something
// writes an authorized_keys entry. Two symptoms, one intrusion.
func TestCampaignChangeJoinsAfterExecution(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	c.ChangeStage = stubChangeStage("persistence", true)
	now := time.Now()

	web := seedIncident(t, db, "i-web-10", "webshell", "/var/www/html/x.php", model.SeverityHigh, now)
	c.Attach(web)
	var camp model.Campaign
	db.First(&camp)

	chg := changeIncident(t, db, "i-chg-10", "/root/.ssh/authorized_keys", now.Add(8*time.Minute))
	if !c.Attach(chg) {
		t.Fatal("a persistence change after an execution event must join the chain")
	}
	var linked model.Incident
	db.First(&linked, "id = ?", "i-chg-10")
	if linked.CampaignID != camp.ID {
		t.Fatalf("change campaign = %q, want %q", linked.CampaignID, camp.ID)
	}
	var after model.Campaign
	db.First(&after, "id = ?", camp.ID)
	if after.StageCount != 2 {
		t.Fatalf("stage count = %d, want 2 (execution+persistence)", after.StageCount)
	}
	if !strings.Contains(after.Stages, "persistence") {
		t.Fatalf("stages = %s, want persistence included", after.Stages)
	}
}

// A change is a fact record: it must never open a chain on its own, however
// suspicious the rules think it is. Otherwise every maintenance edit becomes an
// "attack chain of one".
func TestCampaignChangeNeverSeeds(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	c.ChangeStage = stubChangeStage("persistence", true)

	chg := changeIncident(t, db, "i-chg-11", "/root/.ssh/authorized_keys", time.Now())
	if c.Attach(chg) {
		t.Fatal("a change must not open a chain")
	}
	var n int64
	db.Model(&model.Campaign{}).Count(&n)
	if n != 0 {
		t.Fatalf("campaigns = %d, want 0", n)
	}
}

// Persisting only means something after a foothold: a chain that has not seen
// anything execute or call out is not the right home for it.
func TestCampaignChangeSkipsChainsWithoutFoothold(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	c.ChangeStage = stubChangeStage("persistence", true)
	now := time.Now()

	// A webshell would seed, so build the chain by hand with a persistence-only stage.
	camp := model.Campaign{
		ID: "cp-noexec", Title: "疑似攻击链", Severity: model.SeverityHigh,
		Status: model.CampaignOpen, Score: 0.3, StageCount: 1, IncidentCount: 1,
		Stages:    encodeStrings([]string{"persistence"}),
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now, FirstSeen: now.Add(-time.Hour), LastSeen: now.Add(-5 * time.Minute),
	}
	db.Create(&camp)

	chg := changeIncident(t, db, "i-chg-12", "/etc/cron.d/x", now)
	if c.Attach(chg) {
		t.Fatal("a chain without execution/c2 must not accept a persistence change")
	}
}

// Being close in time matters: "changed sudoers yesterday evening" and "webshell at
// 2am" are not one story, so the base weight alone must stay below the threshold.
func TestCampaignChangeRequiresProximity(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	c.ChangeStage = stubChangeStage("persistence", true)
	now := time.Now()

	web := seedIncident(t, db, "i-web-13", "webshell", "/var/www/html/z.php", model.SeverityHigh, now)
	c.Attach(web)

	far := changeIncident(t, db, "i-chg-13", "/etc/sudoers", now.Add(100*time.Minute))
	if c.Attach(far) {
		t.Fatal("a change 100 minutes later must not join the chain")
	}
	near := changeIncident(t, db, "i-chg-13b", "/etc/sudoers", now.Add(20*time.Minute))
	if !c.Attach(near) {
		t.Fatal("a change 20 minutes later should join the chain")
	}
}

// With no rule injected, changes behave exactly as before: standalone.
func TestCampaignChangeWithoutRuleStaysStandalone(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	now := time.Now()

	web := seedIncident(t, db, "i-web-14", "webshell", "/var/www/html/w.php", model.SeverityHigh, now)
	c.Attach(web)
	chg := changeIncident(t, db, "i-chg-14", "/root/.ssh/authorized_keys", now.Add(5*time.Minute))
	if c.Attach(chg) {
		t.Fatal("without ChangeStage a change must not join any chain")
	}
}

// A ruled-out change (ok=false from the rule) must stay out even when it is right
// after an execution event: that is the "someone edited nginx.conf" case.
func TestCampaignChangeNotSuspiciousStaysOut(t *testing.T) {
	db := newCampaignDB(t)
	c := NewCampaigner(db)
	c.ChangeStage = stubChangeStage("", false)
	now := time.Now()

	web := seedIncident(t, db, "i-web-15", "webshell", "/var/www/html/v.php", model.SeverityHigh, now)
	c.Attach(web)
	chg := changeIncident(t, db, "i-chg-15", "/etc/nginx/nginx.conf", now.Add(3*time.Minute))
	if c.Attach(chg) {
		t.Fatal("a non-suspicious change must not join the chain")
	}
}
