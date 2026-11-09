package tools

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/model"
)

// memoryTools exposes the cross-session memory the agent writes for itself.
//
// These tools are classified RiskReadonly even though they insert rows. The
// classification is about blast radius on the host, and writing the agent's own
// notebook has none: it cannot restart a service, delete a file or open a port.
// Marking them as writes would put them behind a confirmation dialog on every
// single call, which in practice means the agent never records anything and the
// whole feature is wasted.
func memoryTools(d Deps) []aiagent.Tool {
	if d.Memories == nil {
		return nil
	}
	return []aiagent.Tool{rememberTool(d)}
}

// memoryMaxRows bounds the table. Five hundred one-line facts is far more than
// any single host needs, and keeps the injected prompt small even at the limit.
const memoryMaxRows = 500

// memoryTTLDays is how long a fact stays useful before it is worth re-verifying.
// Paths and service layouts rarely change; ports and versions do.
const memoryTTLDays = 180

func rememberTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:  "remember",
		Title: "记住事实",
		Description: `把一条「关于这台机器的事实」写入长期记忆，下次新会话可以直接复用，不必重新探测。

只在确认了稳定的事实时调用，例如：
- subject=nginx fact=站点配置在 /etc/nginx/conf.d/ 下，每个站点一个 .conf
- subject=备份目录 fact=备份统一放在 /data/backup，按日期分子目录
- subject=app-api fact=服务由 systemd 管理，unit 名 app-api.service，端口 8080

不要记录：临时状态（当前内存占用）、对话内容、猜测、凭据与密码。同一事实重复调用没有副作用。`,
		Risk: aiagent.RiskReadonly,
		Schema: schema(map[string]any{
			"subject":    pStr("事实的主体：服务名、路径、主机或组件名，用于下次匹配问题"),
			"fact":       pStr("一句话事实，具体到可以直接使用（路径、端口、unit 名）"),
			"confidence": pStr("置信度 0~1，用户明确告知的用 0.9 以上，自己观察推断的 0.6~0.7；默认 0.7"),
		}, "subject", "fact"),
		Preview: func(args map[string]any) string {
			return "记住：" + str(args, "subject") + " — " + str(args, "fact")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			subject := strings.TrimSpace(str(args, "subject"))
			fact := strings.TrimSpace(str(args, "fact"))
			if subject == "" || fact == "" {
				return failed("必须提供 subject 与 fact"), nil
			}
			if len([]rune(fact)) > 400 {
				return failed("fact 太长（%d 字），请压缩到 400 字以内的一句话", len([]rune(fact))), nil
			}
			conf := 0.7
			if v := strings.TrimSpace(str(args, "confidence")); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					conf = f
				}
			}
			if conf < 0 {
				conf = 0
			}
			if conf > 1 {
				conf = 1
			}
			expires := time.Now().AddDate(0, 0, memoryTTLDays)
			m := &model.AgentMemory{
				Subject:       truncateRunes(subject, 120),
				Fact:          fact,
				SourceSession: ctx.SessionID,
				Confidence:    conf,
				ExpiresAt:     &expires,
			}
			if err := d.Memories.Upsert(m); err != nil {
				return failed("写入记忆失败：%v", err), nil
			}
			// Evict after insert rather than on a timer: the table only grows here,
			// so this is the only moment the bound can be exceeded.
			if err := d.Memories.Evict(memoryMaxRows); err != nil {
				// Losing an eviction is not worth failing the call the model made;
				// the next successful call will trim the excess.
				return textResult(fmt.Sprintf("已记住：%s — %s（清理旧记忆时出错：%v）", subject, fact, err)), nil
			}
			return textResult(fmt.Sprintf("已记住：%s — %s", subject, fact)), nil
		},
	}
}

// truncateRunes cuts a string to n runes without splitting a multi-byte
// character. Subject names are usually one word, but paths can be long.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
