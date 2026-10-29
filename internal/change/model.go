// Package change implements change awareness: detecting, normalizing, storing
// and correlating modifications made to a host.
//
// Design invariants (do not break these):
//
//  1. A change is a FACT, not a judgement. Collectors only record what/when/
//     from-what-to-what. Whether it is a problem is decided by policies and the
//     agent, never here.
//  2. Change events never trigger actions. They enter the signal bus with
//     family="change" and severity=info, and the trigger layer forces any such
//     incident into shadow mode (see internal/trigger).
//  3. Content is untrusted. File bodies may contain secrets and prompt-injection
//     payloads; they are sanitized BEFORE storage and diffed only after
//     sanitization.
package change

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Object is the thing that changed.
type Object struct {
	Type   string   // file | package | unit | user | rule | job
	Key    string   // absolute path / package name / unit name / username / rule id / cron id
	Labels []string // tags used for correlation (service names, ports, domains)
}

// Actor is who (or what) made the change. Attribution is best-effort; see
// Confidence in model.Confidence* constants.
type Actor struct {
	Kind   string // ssh_session | panel | agent | cron | package_manager | system | unknown
	User   string // OS user
	IP     string // source IP when known
	How    string // vim | sed | apt | systemctl | panel | ...
	Confidence string
}

// Diff describes the shape of the change without carrying the body.
type Diff struct {
	Changed   bool
	Added     int
	Removed   int
	Fields    []string // changed key names (sanitized)
	HashFrom  string
	HashTo    string
	Body      string // sanitized unified diff; stored separately
	Truncated bool
}

// Event is the in-memory representation used by collectors before it is
// converted into model.ChangeEvent for storage.
type Event struct {
	Kind   string
	Source string
	Object Object
	Actor  Actor
	Diff   *Diff
	At     time.Time
}

// NewEvent builds an event with sensible defaults.
func NewEvent(kind, source string, obj Object, at time.Time) Event {
	return Event{
		Kind:   kind,
		Source: source,
		Object: obj,
		At:     at,
		Actor:  Actor{Kind: "unknown", Confidence: model.ConfidenceUnknown},
	}
}

// Fingerprint is the normalized identity of the change target, used for trend
// aggregation ("this file has been changed 12 times this month").
func (e Event) Fingerprint() string {
	return fmt.Sprintf("change.%s:%s", e.Kind, e.Object.Key)
}

// DedupKey collapses repeated changes of the same object inside one time bucket
// (10 minutes) into a single row with an increasing Count.
func (e Event) DedupKey() string {
	return fmt.Sprintf("%s:%d", e.Fingerprint(), e.At.Truncate(10*time.Minute).Unix())
}

// Summary is the one-line human readable message shown in the event list.
func (e Event) Summary() string {
	switch e.Kind {
	case model.ChangeKindConfig:
		if e.Diff != nil && e.Diff.Changed {
			return fmt.Sprintf("%s 被修改（+%d −%d）", baseName(e.Object.Key), e.Diff.Added, e.Diff.Removed)
		}
		return fmt.Sprintf("%s 被修改", baseName(e.Object.Key))
	case model.ChangeKindPackage:
		return fmt.Sprintf("软件包变更：%s", e.Object.Key)
	case model.ChangeKindService:
		return fmt.Sprintf("服务状态变更：%s", e.Object.Key)
	case model.ChangeKindAccount:
		return fmt.Sprintf("账号/权限变更：%s", e.Object.Key)
	case model.ChangeKindFirewall:
		return "防火墙规则变更"
	case model.ChangeKindCron:
		return fmt.Sprintf("定时任务变更：%s", e.Object.Key)
	}
	return fmt.Sprintf("%s 变更：%s", e.Kind, e.Object.Key)
}

// ID derives a stable-ish identifier from the dedup key plus the exact time, so
// two changes of the same file in different buckets get different ids while a
// retry of the same detection does not create duplicates.
func (e Event) ID() string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", e.DedupKey(), e.At.UnixNano())))
	return "chg_" + hex.EncodeToString(h[:8])
}

// chainHash computes the tamper-evident hash of one event row.
func chainHash(prev string, ev model.ChangeEvent) string {
	payload := strings.Join([]string{
		prev,
		ev.ID,
		ev.Kind,
		ev.ObjectKey,
		ev.HashFrom,
		ev.HashTo,
		fmt.Sprintf("%d", ev.Added),
		fmt.Sprintf("%d", ev.Removed),
		fmt.Sprintf("%d", ev.At.Unix()),
	}, "|")
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}
