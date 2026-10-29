package sentinel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Aggregation policy.
//
// Two shapes of noise motivated this:
//
//   - one object changing over and over (a service stuck in a restart loop, a
//     firewall listing that differs only in its header timestamp), and
//   - one scenario coming from many sources (an SSH brute force from a dozen
//     addresses).
//
// Both must collapse into a single incident that grows a counter instead of a
// wall of rows. The group key decides what "the same thing" means and the
// window decides how long a closed loop keeps folding.
type aggGroup struct {
	// Key is persisted on the incident and is the only join condition.
	Key string
	// Window is how far back an incident may have been last seen and still
	// accept a new signal. Zero means "fold for as long as it stays active":
	// the object keeps changing, the story is still the same story.
	Window time.Duration
	// Collect lists distinct subjects as members. Only useful when the group is
	// broader than one subject.
	Collect bool
	// Label is the aggregated subject name, empty to keep the signal's own.
	Label string
}

const (
	familyAuthAnomaly = "auth.anomaly"
	familyMetric      = "metric"
	familyLogGrowth   = "log_growth"
	// authBruteGroupKey folds every failed-login burst on this host into one
	// incident, whatever address it came from.
	authBruteGroupKey = "auth.anomaly|bruteforce"
	// memberCap bounds the stored member list so one row cannot grow without
	// limit; MemberTotal still reports the real number.
	memberCap = 50
	// authLoginWindow keeps successful-login anomalies per address per half day:
	// the same operator logging in at an odd hour again is the same thing to
	// look at, but tomorrow's login is a new observation.
	authLoginWindow = 12 * time.Hour
)

// foldWhileOpen are the families whose signal means "a condition still holds"
// rather than "something just happened": a CPU alert, a log file still growing.
//
// An alert nobody has handled is ONE alert, not one per window — nine separate
// "指标告警 · CPU 使用率" rows is exactly how a real alert becomes noise the
// operator learns to scroll past. So these fold for as long as the incident stays
// open, regardless of how far apart the signals are.
var foldWhileOpen = map[string]bool{
	familyMetric:    true,
	familyLogGrowth: true,
}

// ongoingFamilies is the narrower set that also changes how the incident is
// treated after a run: investigate once, and do not close it when the
// investigation ends (see IsOngoingGroup).
//
// metric only. A threshold condition keeps holding whether or not anyone looked,
// so re-running the analysis costs tokens for no new information. log_growth is
// deliberately NOT here: rolling the log over does end the condition, and a
// log-growth incident that refuses to close reads like a bug.
var ongoingFamilies = map[string]bool{
	familyMetric: true,
}

// IsOngoingGroup reports whether an incident covers a condition that is still
// happening (an attack in progress, or a threshold still exceeded) rather than a
// single finished event.
//
// Such an incident must not be closed by the end of an investigation: the attack
// has not stopped, so closing it only makes the next burst open a second row and
// the noise comes straight back. It also only gets investigated once —
// re-analysing the same unchanged situation every half hour burns tokens for
// nothing.
func IsOngoingGroup(groupKey string) bool {
	if groupKey == authBruteGroupKey {
		return true
	}
	family, _, _ := strings.Cut(groupKey, "|")
	return ongoingFamilies[family]
}

// groupOf decides how a signal is clustered. defWindow is the fallback window
// for families that keep the classic "same subject, short window" behaviour.
func groupOf(s model.Signal, defWindow time.Duration) aggGroup {
	switch s.Family {
	case model.FamilyChange:
		// Per object, and for as long as the incident is active: a config file
		// edited five times this hour is one change story. The agent is barred
		// from acting on this family anyway, so a long window costs nothing.
		return aggGroup{Key: "change|" + s.Subject}
	case familyMetric, familyLogGrowth:
		// Window 0 = fold while the incident stays active. A CPU rule that keeps
		// firing must not produce one incident per 10 minutes.
		return aggGroup{Key: s.Family + "|" + s.Subject}
	case familyAuthAnomaly:
		if s.Fingerprint == "auth.failed_burst" {
			return aggGroup{Key: authBruteGroupKey, Collect: true, Label: "SSH 爆破"}
		}
		// Successful logins stay per-address: "root logged in from somewhere new"
		// must never be diluted into a bucket.
		return aggGroup{Key: s.Family + "|" + s.Subject, Window: authLoginWindow}
	}
	return aggGroup{Key: s.Family + "|" + s.Subject, Window: defWindow}
}

// titleForGroup renders the incident title, which for a collected group has to
// be rebuilt as the member set grows ("涉及 13 个源 IP").
func (g aggGroup) titleForGroup(s model.Signal, total int) string {
	if g.Label != "" {
		if g.Collect && total > 1 {
			return fmt.Sprintf("%s · 涉及 %d 个源 IP", g.Label, total)
		}
		return g.Label
	}
	return titleFor(s)
}

// memberJSON encodes the first member of a group.
func memberJSON(subject string) string {
	if subject == "" {
		return ""
	}
	b, err := json.Marshal([]string{subject})
	if err != nil {
		return ""
	}
	return string(b)
}

// mergeMember folds subject into the stored member list. It returns the encoded
// list and the new total. Duplicates bump the total only when the list has
// already been truncated, otherwise total == len(list).
func mergeMember(stored string, total int, subject string) (string, int) {
	if subject == "" {
		return stored, total
	}
	var list []string
	if stored != "" {
		_ = json.Unmarshal([]byte(stored), &list)
	}
	for _, m := range list {
		if m == subject {
			if total < len(list) {
				total = len(list)
			}
			return stored, total
		}
	}
	if len(list) >= memberCap {
		// Keep counting, stop storing: the tail of a 5000-address attack is not
		// worth a row update per address.
		if total < len(list)+1 {
			total = len(list) + 1
		}
		return stored, total
	}
	list = append(list, subject)
	b, err := json.Marshal(list)
	if err != nil {
		return stored, total
	}
	if total < len(list) {
		total = len(list)
	}
	return string(b), total
}
