package sentinel

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:sentinel-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.Signal{}, &model.Incident{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestBusDeduplicatesWithinWindow(t *testing.T) {
	db := newDB(t)
	bus := NewBus(db, time.Hour)
	var got int
	bus.Subscribe(func(model.Signal) { got++ })

	sig := newSignal(model.SignalKindLog, "test", "/var/log/app.log", "log_growth",
		"log_growth:/var/log/app.log", model.SeverityHigh, "boom").Signal
	bus.Publish(sig)
	bus.Publish(sig) // same dedup key: must be dropped
	// Drain the queue synchronously (the bus goroutine is not started in tests).
	for i := 0; i < 2; i++ {
		select {
		case s := <-bus.in:
			bus.handle(s)
		default:
		}
	}
	if got != 1 {
		t.Fatalf("expected 1 accepted signal, got %d", got)
	}
	var n int64
	db.Model(&model.Signal{}).Count(&n)
	if n != 1 {
		t.Fatalf("expected 1 persisted signal, got %d", n)
	}
}

func TestAggregatorFoldsIntoSingleIncident(t *testing.T) {
	db := newDB(t)
	agg := NewAggregator(db, 10*time.Minute)

	s1 := newSignal(model.SignalKindAuth, "test", "203.0.113.9", "auth.anomaly",
		"auth.accepted", model.SeverityMedium, "login 1").Signal
	s1.ID = "sig00000000000000000000000000001"
	inc1, created1 := agg.Add(s1)

	s2 := newSignal(model.SignalKindAuth, "test", "203.0.113.9", "auth.anomaly",
		"auth.accepted", model.SeverityCritical, "login 2").Signal
	s2.ID = "sig00000000000000000000000000002"
	inc2, created2 := agg.Add(s2)

	if !created1 || created2 {
		t.Fatalf("second signal must fold into the first incident (created1=%v created2=%v)", created1, created2)
	}
	if inc1.ID != inc2.ID {
		t.Fatalf("incident id changed: %s vs %s", inc1.ID, inc2.ID)
	}
	if inc2.Severity != model.SeverityCritical {
		t.Fatalf("severity must escalate to critical, got %s", inc2.Severity)
	}
	if inc2.Count != 2 {
		t.Fatalf("count must be 2, got %d", inc2.Count)
	}
}

func TestTemplateNormalization(t *testing.T) {
	a := normTemplate("job 42 failed after 3s from 10.0.0.1")
	b := normTemplate("job 77 failed after 9s from 10.0.0.2")
	if a != b {
		t.Fatalf("templates should collapse: %q vs %q", a, b)
	}
}

func TestFileScoreDetectsWebshell(t *testing.T) {
	strong, hits := scoreScriptBytes([]byte("<?php eval($_POST['c']); ?>"))
	if !strong || hits < 1 {
		t.Fatalf("expected a strong webshell hit, got strong=%v hits=%d", strong, hits)
	}
}
