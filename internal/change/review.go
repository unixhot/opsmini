package change

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Rule-based change review.
//
// Why rules first: most changes are decidable without a model. "dpkg upgraded nginx
// during business hours" is not suspicious, and paying a model to say so 200 times a
// day is both slow and expensive. The rule pass is deterministic, explainable, free,
// and every reason it emits is a sentence an operator can argue with. Only what it
// cannot settle goes to the agent.
//
// The verdict is advisory. Nothing here can change a file.

// Weights. Documented on purpose: an operator who disagrees with a verdict needs to
// see which term pushed it over the line, and a future maintainer needs to know that
// these are tuned, not derived.
const (
	wSensitivePath = 0.45 // 改的是 /etc/passwd 这类高危对象
	wMediumPath    = 0.20
	wLowPath       = -0.10
	wNoActor       = 0.25 // 谁改的都不知道
	wInferredActor = 0.10
	wPublicActorIP = 0.30 // 从公网地址改的
	wSSHRoot       = 0.15
	wPackage       = -0.35 // 包管理器干的，通常正常
	wOffHours      = 0.15
	wWeekend       = 0.05
	wFlapping      = 0.15 // 一小时内同一对象反复改
	wNearbyEvent   = 0.35 // 同期有高危事件
	wFirstByActor  = 0.10 // 这个操作者第一次改这个对象
)

// Thresholds. suspicious costs tokens, so the bar is deliberately high: a false
// "normal" just means a human sees it in the list anyway, while a false "suspicious"
// burns a model call and teaches operators to ignore the badge.
const (
	thresholdSuspicious = 0.55
	thresholdUnclear    = 0.30
)

// ReviewContext is the history the score depends on, gathered separately so the
// scoring itself stays a pure function that can be tested without a database.
type ReviewContext struct {
	// RecentSame counts other changes to the same object in the last hour.
	RecentSame int
	// ActorHistory counts earlier changes to this object by the same actor.
	ActorHistory int
	// NearbyIncidents counts high/critical non-change incidents within 30 minutes.
	NearbyIncidents int
}

// sensitivePaths are the files whose modification is itself an event worth reading,
// whatever the actor looks like: an account added to /etc/passwd is exactly how
// persistence is established.
var sensitivePaths = []string{
	"/etc/passwd", "/etc/shadow", "/etc/gshadow", "/etc/group",
	"/etc/sudoers", "/etc/sudoers.d",
	"/etc/ssh/sshd_config", "/root/.ssh/authorized_keys", "/etc/ssh",
	"/etc/pam.d", "/etc/ld.so.preload", "/etc/ld.so.conf",
	"/etc/crontab", "/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily",
	"/var/spool/cron", "/etc/systemd/system", "/lib/systemd/system",
	"/etc/rc.local", "/etc/profile", "/etc/bash.bashrc", "/etc/environment",
	"/etc/security", "/etc/modules", "/etc/init.d",
}

// mediumPaths are configuration that matters but is edited routinely.
var mediumPaths = []string{
	"/etc/nginx", "/etc/apache2", "/etc/httpd", "/etc/my.cnf", "/etc/mysql",
	"/etc/redis", "/etc/php", "/etc/hosts", "/etc/resolv.conf", "/etc/fstab",
	"/etc/sysctl.conf", "/etc/logrotate.d", "/etc/docker", "/etc/ssl",
}

// noisyPaths change as a side effect of normal package management.
var noisyPaths = []string{
	"/var/lib/dpkg", "/var/lib/rpm", "/var/cache", "/var/lib/apt",
	"/var/log", "/usr/share", "/usr/lib", "/usr/local/lib",
}

// ScoreChange judges one change. Pure: same input, same verdict, no I/O.
func ScoreChange(ev Event, ctx ReviewContext, now time.Time) (model.ChangeReview, []string) {
	var reasons []string
	score := 0.0

	obj := ev.Object.Key
	sev := ""

	switch {
	case pathUnderAny(obj, sensitivePaths):
		score += wSensitivePath
		sev = "高危路径：" + obj
	case pathUnderAny(obj, mediumPaths):
		score += wMediumPath
		sev = "重要配置：" + obj
	case pathUnderAny(obj, noisyPaths):
		score += wLowPath
		sev = "包管理/系统目录，通常由软件包操作产生"
	}
	if sev != "" {
		reasons = append(reasons, sev)
	}

	// Who: the single most useful signal, and the one most often missing.
	actor := strings.TrimSpace(ev.Actor.User)
	if ev.Kind == model.ChangeKindPackage || ev.Source == "pkg" {
		score += wPackage
		reasons = append(reasons, "由包管理器产生")
	}
	switch {
	case actor == "" && ev.Actor.Confidence == model.ConfidenceUnknown:
		score += wNoActor
		reasons = append(reasons, "无法确认操作者")
	case actor == "":
		score += wInferredActor
		reasons = append(reasons, "操作者未确定，仅由旁证推断")
	case ev.Actor.Confidence == model.ConfidenceInferred:
		score += wInferredActor
		reasons = append(reasons, "操作者为推断值（"+actor+"）")
	}
	if ip := strings.TrimSpace(ev.Actor.IP); ip != "" && !isPrivateIP(ip) {
		score += wPublicActorIP
		reasons = append(reasons, "来自公网地址 "+ip)
	}
	if actor == "root" && ev.Actor.How == "ssh" {
		score += wSSHRoot
		reasons = append(reasons, "root 通过 SSH 直接修改")
	}

	// When: off-hours editing of a sensitive file is a classic tell, but only a
	// minor one on its own — plenty of teams deploy at night.
	hour := now.Hour()
	if hour < 7 || hour >= 20 {
		score += wOffHours
		reasons = append(reasons, fmt.Sprintf("非工作时段（%02d 时）", hour))
	}
	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		score += wWeekend
		reasons = append(reasons, "周末变更")
	}

	// Repetition: a file edited five times in an hour is either a fight with a typo
	// or an attacker covering tracks. Either way it deserves a look.
	if ctx.RecentSame >= 3 {
		score += wFlapping
		reasons = append(reasons, fmt.Sprintf("1 小时内同一对象已变更 %d 次", ctx.RecentSame+1))
	}

	// Co-occurrence is the strongest available evidence: nobody edits sshd_config at
	// the same moment a webshell lands by coincidence.
	if ctx.NearbyIncidents > 0 {
		score += wNearbyEvent
		reasons = append(reasons, fmt.Sprintf("30 分钟内有 %d 个高危事件", ctx.NearbyIncidents))
	}

	if ctx.ActorHistory == 0 && actor != "" && pathUnderAny(obj, sensitivePaths) {
		score += wFirstByActor
		reasons = append(reasons, "该操作者首次修改此对象")
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	verdict := model.ChangeVerdictNormal
	switch {
	case score >= thresholdSuspicious:
		verdict = model.ChangeVerdictSuspicious
	case score >= thresholdUnclear:
		verdict = model.ChangeVerdictUnclear
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "未发现可疑信号")
	}

	return model.ChangeReview{
		ChangeID: ev.ID(),
		Verdict:  verdict,
		Score:    score,
		Source:   "rule",
	}, reasons
}

// Reviewer scores incoming changes and remembers the verdict.
type Reviewer struct {
	store *Store
	// Window bounds how far back "nearby incident" looks.
	Window time.Duration
}

// NewReviewer builds a reviewer over a store.
func NewReviewer(store *Store) *Reviewer {
	return &Reviewer{store: store, Window: 30 * time.Minute}
}

// Review scores one change, persists the verdict and returns it.
//
// Called on the persistence goroutine of the watcher: three small indexed queries,
// no model call, so it does not slow collection down.
func (r *Reviewer) Review(ev Event) (model.ChangeReview, error) {
	if r == nil || r.store == nil || r.store.db == nil {
		return model.ChangeReview{}, fmt.Errorf("reviewer 不可用")
	}
	now := time.Now()
	window := r.Window
	if window <= 0 {
		window = 30 * time.Minute
	}
	id := ev.ID()

	var recent int64
	r.store.db.Model(&model.ChangeEvent{}).
		Where("object_key = ? AND at > ? AND id <> ?", ev.Object.Key, now.Add(-time.Hour), id).
		Count(&recent)

	var history int64
	if actor := strings.TrimSpace(ev.Actor.User); actor != "" {
		r.store.db.Model(&model.ChangeEvent{}).
			Where("object_key = ? AND actor_user = ? AND id <> ?", ev.Object.Key, actor, id).
			Count(&history)
	}

	var nearby int64
	r.store.db.Model(&model.Incident{}).
		Where("family <> ? AND severity IN ?", model.FamilyChange, []string{model.SeverityHigh, model.SeverityCritical}).
		Where("last_seen > ? AND first_seen < ?", now.Add(-window), now.Add(window)).
		Count(&nearby)

	rev, reasons := ScoreChange(ev, ReviewContext{
		RecentSame:      int(recent),
		ActorHistory:    int(history),
		NearbyIncidents: int(nearby),
	}, now)

	raw, _ := json.Marshal(reasons)
	rev.Reasons = string(raw)
	rev.CreatedAt = now

	// Upsert: a change is scored once, but a retry of the same event must not fail
	// on the unique index.
	var existing model.ChangeReview
	if err := r.store.db.First(&existing, "change_id = ?", rev.ChangeID).Error; err == nil {
		rev.ID = existing.ID
		r.store.db.Model(&model.ChangeReview{}).Where("id = ?", existing.ID).
			Updates(map[string]any{"verdict": rev.Verdict, "score": rev.Score, "reasons": rev.Reasons})
	} else if err := r.store.db.Create(&rev).Error; err != nil {
		return rev, err
	}
	return rev, nil
}

func pathUnderAny(p string, prefixes []string) bool {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	for _, pre := range prefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// isPrivateIP is deliberately crude: it only needs to separate "an address on this
// machine or our own network" from "somewhere on the internet".
func isPrivateIP(ip string) bool {
	if ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		return true
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	var a, b int
	if _, err := fmt.Sscanf(parts[0], "%d", &a); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &b); err != nil {
		return false
	}
	switch {
	case a == 10:
		return true
	case a == 192 && b == 168:
		return true
	case a == 172 && b >= 16 && b <= 31:
		return true
	case a == 169 && b == 254:
		return true
	}
	return false
}
