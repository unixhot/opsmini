package sentinel

import (
	"context"
	"log"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// RetentionPolicy is how long each kind of record is kept. Everything is
// measured in days; 0 means "keep forever".
//
// The split matters:
//   - signals are high volume and cheap to lose, keep them short;
//   - action logs are the audit trail, keep them long;
//   - incidents are the narrative (what happened, what was done, the report),
//     keep them in between and never delete one that is still open.
type RetentionPolicy struct {
	SignalDays     int `json:"signal_days"`
	IncidentDays   int `json:"incident_days"`
	ActionDays     int `json:"action_days"`
	RunDays        int `json:"run_days"`
	QuarantineDays int `json:"quarantine_days"`
	BlockDays      int `json:"block_days"`
}

// DefaultRetention is what a fresh install uses.
func DefaultRetention() RetentionPolicy {
	return RetentionPolicy{
		SignalDays:     14,
		IncidentDays:   90,
		ActionDays:     180,
		RunDays:        90,
		QuarantineDays: 30,
		BlockDays:      30,
	}
}

// normalize clamps the values to a sane range so a typo cannot wipe history.
func (p *RetentionPolicy) normalize() {
	clamp := func(v int, min int) int {
		if v == 0 {
			return 0 // 0 = keep forever
		}
		if v < min {
			return min
		}
		if v > 3650 {
			return 3650
		}
		return v
	}
	p.SignalDays = clamp(p.SignalDays, 1)
	p.IncidentDays = clamp(p.IncidentDays, 7)
	p.ActionDays = clamp(p.ActionDays, 7)
	p.RunDays = clamp(p.RunDays, 7)
	p.QuarantineDays = clamp(p.QuarantineDays, 1)
	p.BlockDays = clamp(p.BlockDays, 1)
}

// RetentionStat describes one table for the UI.
type RetentionStat struct {
	Table  string     `json:"table"`
	Rows   int64      `json:"rows"`
	Oldest *time.Time `json:"oldest,omitempty"`
	Days   int        `json:"days"`
}

// PurgeResult reports how many rows each table lost.
type PurgeResult struct {
	Signals     int64 `json:"signals"`
	Incidents   int64 `json:"incidents"`
	Actions     int64 `json:"actions"`
	Runs        int64 `json:"runs"`
	Quarantine  int64 `json:"quarantine"`
	Blocks      int64 `json:"blocks"`
	Total       int64 `json:"total"`
	DryRun      bool  `json:"dry_run"`
}

// closedStatuses are the incident states that are safe to age out. Open,
// triggered and running incidents are never deleted: they are live evidence.
var closedStatuses = []string{
	model.IncidentResolved, model.IncidentIgnored,
	model.IncidentTakenOver, model.IncidentFailed,
}

// RetentionStats returns the current row count and oldest record per table.
func (m *Manager) RetentionStats() []RetentionStat {
	if m.db == nil {
		return nil
	}
	out := []RetentionStat{}
	add := func(table string, model any, col string, days int) {
		var n int64
		m.db.Model(model).Count(&n)
		st := RetentionStat{Table: table, Rows: n, Days: days}
		// Query the oldest row directly: scanning MIN(col) into a time is fragile
		// across drivers, ordering by the column is not.
		if n > 0 {
			var t time.Time
			if err := m.db.Model(model).Select(col).Order(col + " ASC").Limit(1).Scan(&t).Error; err == nil && !t.IsZero() {
				tt := t
				st.Oldest = &tt
			}
		}
		out = append(out, st)
	}
	p := m.Retention
	add("signals", &model.Signal{}, "observed_at", p.SignalDays)
	add("incidents", &model.Incident{}, "created_at", p.IncidentDays)
	add("actions", &model.ActionLog{}, "created_at", p.ActionDays)
	add("runs", &model.AgentRun{}, "started_at", p.RunDays)
	add("quarantine", &model.QuarantineItem{}, "created_at", p.QuarantineDays)
	add("blocks", &model.BlockedIP{}, "created_at", p.BlockDays)
	return out
}

// Purge deletes rows older than the retention window. dryRun counts without
// deleting, which is what the UI shows before a manual cleanup.
func (m *Manager) Purge(what string, dryRun bool) (PurgeResult, error) {
	res := PurgeResult{DryRun: dryRun}
	if m.db == nil {
		return res, nil
	}
	p := m.Retention
	now := time.Now()
	all := what == "" || what == "all"

	if all || what == "signals" {
		if p.SignalDays > 0 {
			cut := now.AddDate(0, 0, -p.SignalDays)
			// Only drop signals whose incident is already closed (or that never
			// became one). Signals of open incidents are evidence.
			sub := m.db.Model(&model.Incident{}).Select("id").Where("status IN ?", closedStatuses)
			q := m.db.Model(&model.Signal{}).Where("observed_at < ?", cut).
				Where("incident_id = '' OR incident_id IN (?)", sub)
			var n int64
			if dryRun {
				q.Count(&n)
				res.Signals = n
			} else {
				res.Signals = m.db.Where("observed_at < ?", cut).
					Where("incident_id = '' OR incident_id IN (?)", sub).
					Delete(&model.Signal{}).RowsAffected
			}
		}
	}

	if all || what == "incidents" {
		if p.IncidentDays > 0 {
			cut := now.AddDate(0, 0, -p.IncidentDays)
			q := m.db.Model(&model.Incident{}).Where("status IN ?", closedStatuses).
				Where("COALESCE(closed_at, updated_at) < ?", cut)
			var ids []string
			q.Pluck("id", &ids)
			if len(ids) > 0 {
				if dryRun {
					res.Incidents = int64(len(ids))
				} else {
					// Orphan the signals first so they are not left pointing at a
					// missing incident, then remove the incidents.
					m.db.Model(&model.Signal{}).Where("incident_id IN ?", ids).Update("incident_id", "")
					res.Incidents = m.db.Where("id IN ?", ids).Delete(&model.Incident{}).RowsAffected
				}
			}
		}
	}

	if all || what == "actions" {
		if p.ActionDays > 0 {
			cut := now.AddDate(0, 0, -p.ActionDays)
			if dryRun {
				var n int64
				m.db.Model(&model.ActionLog{}).Where("created_at < ?", cut).Count(&n)
				res.Actions = n
			} else {
				res.Actions = m.db.Where("created_at < ?", cut).Delete(&model.ActionLog{}).RowsAffected
			}
		}
	}

	if all || what == "runs" {
		if p.RunDays > 0 {
			cut := now.AddDate(0, 0, -p.RunDays)
			sub := m.db.Model(&model.Incident{}).Select("id").Where("status IN ?", closedStatuses)
			if dryRun {
				var n int64
				m.db.Model(&model.AgentRun{}).Where("started_at < ?", cut).
					Where("incident_id = '' OR incident_id IN (?)", sub).Count(&n)
				res.Runs = n
			} else {
				res.Runs = m.db.Where("started_at < ?", cut).
					Where("incident_id = '' OR incident_id IN (?)", sub).
					Delete(&model.AgentRun{}).RowsAffected
			}
		}
	}

	if all || what == "quarantine" {
		if p.QuarantineDays > 0 {
			cut := now.AddDate(0, 0, -p.QuarantineDays)
			// Only the bookkeeping row of an already restored item goes away; the
			// file itself was moved back at restore time.
			q := m.db.Model(&model.QuarantineItem{}).
				Where("restored_at IS NOT NULL AND restored_at < ?", cut)
			var n int64
			if dryRun {
				q.Count(&n)
				res.Quarantine = n
			} else {
				res.Quarantine = m.db.Where("restored_at IS NOT NULL AND restored_at < ?", cut).
					Delete(&model.QuarantineItem{}).RowsAffected
			}
		}
	}

	if all || what == "blocks" {
		if p.BlockDays > 0 {
			cut := now.AddDate(0, 0, -p.BlockDays)
			q := m.db.Model(&model.BlockedIP{}).
				Where("released_at IS NOT NULL AND released_at < ?", cut)
			var n int64
			if dryRun {
				q.Count(&n)
				res.Blocks = n
			} else {
				res.Blocks = m.db.Where("released_at IS NOT NULL AND released_at < ?", cut).
					Delete(&model.BlockedIP{}).RowsAffected
			}
		}
	}

	res.Total = res.Signals + res.Incidents + res.Actions + res.Runs + res.Quarantine + res.Blocks
	return res, nil
}

// startRetention runs the cleanup once an hour. The first tick happens after a
// short delay so a restart does not immediately fight with startup work.
func (m *Manager) startRetention(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		first := time.After(3 * time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			case <-first:
				m.sweep()
			case <-t.C:
				m.sweep()
			}
		}
	}()
}

func (m *Manager) sweep() {
	res, err := m.Purge("all", false)
	if err != nil {
		log.Printf("sentinel: retention purge failed: %v", err)
		return
	}
	if res.Total > 0 {
		log.Printf("sentinel: retention removed signals=%d incidents=%d actions=%d runs=%d quarantine=%d blocks=%d",
			res.Signals, res.Incidents, res.Actions, res.Runs, res.Quarantine, res.Blocks)
	}
}
