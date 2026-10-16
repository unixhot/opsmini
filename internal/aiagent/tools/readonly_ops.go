package tools

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
)

// dockerPSTool lists containers and their resource usage.
func dockerPSTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "docker_ps",
		Title:       "容器列表",
		Description: "列出 Docker 容器（含运行状态、端口、CPU/内存占用）。也可用 action=images/volumes/networks 查看镜像、数据卷与网络。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "container.view",
		Schema: schema(map[string]any{
			"action": pEnum("查看对象", "containers", "images", "volumes", "networks"),
		}),
		Preview: func(args map[string]any) string { return "读取 Docker " + strDef(args, "action", "containers") },
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			switch strDef(args, "action", "containers") {
			case "images":
				imgs, err := d.Docker.Images()
				if err != nil {
					return failed("获取镜像列表失败：%v", err), nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "共 %d 个镜像：\n镜像\t大小\t创建时间\n", len(imgs))
				for _, im := range imgs {
					fmt.Fprintf(&b, "%s\t%s\t%s\n", strings.Join(im.RepoTags, ", "), fmtBytes(uint64(im.Size)), time.Unix(im.Created, 0).Format("2006-01-02 15:04"))
				}
				return textResult(b.String()), nil
			case "volumes":
				vols, err := d.Docker.Volumes()
				if err != nil {
					return failed("获取数据卷失败：%v", err), nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "共 %d 个数据卷：\n名称\t驱动\t挂载点\n", len(vols))
				for _, v := range vols {
					fmt.Fprintf(&b, "%s\t%s\t%s\n", v.Name, v.Driver, v.Mountpoint)
				}
				return textResult(b.String()), nil
			case "networks":
				nets, err := d.Docker.Networks()
				if err != nil {
					return failed("获取网络列表失败：%v", err), nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "共 %d 个网络：\n名称\t驱动\t范围\n", len(nets))
				for _, n := range nets {
					fmt.Fprintf(&b, "%s\t%s\t%s\n", n.Name, n.Driver, n.Scope)
				}
				return textResult(b.String()), nil
			}
			cs, err := d.Docker.Containers()
			if err != nil {
				return failed("获取容器列表失败：%v", err), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 个容器：\n名称\t状态\t镜像\t端口\tCPU%%\t内存\n", len(cs))
			for _, c := range cs {
				mem := fmtBytes(c.MemUsage)
				if c.MemLimit > 0 {
					mem += "/" + fmtBytes(c.MemLimit)
				}
				fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%.1f\t%s\n", c.Name, c.Status, c.Image, c.Ports, c.CPUPercent, mem)
			}
			return textResult(b.String()), nil
		},
	}
}

// listCronTool lists scheduled tasks.
func listCronTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_cron_jobs",
		Title:       "计划任务",
		Description: "列出面板计划任务与系统 crontab 条目，用于排查定时任务异常、重复执行等问题。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "cron.view",
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取计划任务列表" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			var b strings.Builder
			if d.Cron != nil {
				jobs, err := d.Cron.List()
				if err != nil {
					return failed("获取面板计划任务失败：%v", err), nil
				}
				fmt.Fprintf(&b, "【面板计划任务】共 %d 条\n", len(jobs))
				for _, j := range jobs {
					state := "停用"
					if j.Enabled {
						state = "启用"
					}
					fmt.Fprintf(&b, "- %s [%s] %s：%s\n", j.Name, state, j.Schedule, truncate(j.Command, 150))
				}
			}
			if d.Crontab != nil {
				sys := d.Crontab.SystemJobs()
				usr := d.Crontab.UserJobs()
				fmt.Fprintf(&b, "\n【系统 crontab】共 %d 条\n", len(sys))
				for _, j := range sys {
					fmt.Fprintf(&b, "- %s (%s) %s：%s\n", j.Source, j.User, j.Schedule, truncate(j.Command, 120))
				}
				fmt.Fprintf(&b, "\n【用户 crontab】共 %d 条\n", len(usr))
				for _, j := range usr {
					fmt.Fprintf(&b, "- %s (%s) %s：%s\n", j.Source, j.User, j.Schedule, truncate(j.Command, 120))
				}
			}
			return textResult(b.String()), nil
		},
	}
}

// listWebsitesTool lists website records.
func listWebsitesTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_websites",
		Title:       "网站列表",
		Description: "列出面板中登记的网站（域名、根目录、运行环境等）。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "website.view",
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取网站列表" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			list, err := d.Website.List()
			if err != nil {
				return failed("获取网站列表失败：%v", err), nil
			}
			if len(list) == 0 {
				return textResult("面板中还没有登记网站。"), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 个网站：\n域名\t根目录\t运行环境\t状态\tSSL\n", len(list))
			for _, w := range list {
				state := "已停止"
				if w.Status == 1 {
					state = "运行中"
				}
				ssl := "否"
				if w.SSL {
					ssl = "是"
				}
				runtime := w.Runtime
				if w.PHPVersion != "" {
					runtime += " " + w.PHPVersion
				}
				fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", w.Domain, w.Dir, runtime, state, ssl)
			}
			return textResult(b.String()), nil
		},
	}
}

// listDatabasesTool lists database records managed by the panel.
func listDatabasesTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_databases",
		Title:       "数据库列表",
		Description: "列出面板登记的数据库实例及其连接信息（不含密码明文）。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "database.view",
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取数据库列表" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			list, err := d.Database.List()
			if err != nil {
				return failed("获取数据库列表失败：%v", err), nil
			}
			if len(list) == 0 {
				return textResult("面板中还没有登记数据库。"), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 个数据库记录：\n名称\t类型\t字符集\t状态\n", len(list))
			for _, db := range list {
				state := "已停止"
				if db.Status == 1 {
					state = "运行中"
				}
				fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", db.Name, db.Type, db.Charset, state)
			}
			return textResult(b.String()), nil
		},
	}
}

// securityStatusTool reports the host security overview.
func securityStatusTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "security_status",
		Title:       "安全概览",
		Description: "获取主机安全评分与各模块（基线、登录安全、防火墙、文件完整性、威胁）摘要。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "security.view",
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取主机安全概览" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			ov, err := d.Security.Overview()
			if err != nil {
				return failed("获取安全概览失败：%v", err), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "安全评分：%d 分（等级 %s）\n", ov.Score, ov.Grade)
			fmt.Fprintf(&b, "- 基线检查：%d/%d %s\n", ov.Baseline.Score, ov.Baseline.Max, ov.Baseline.Summary)
			fmt.Fprintf(&b, "- 登录安全：%d/%d %s\n", ov.Login.Score, ov.Login.Max, ov.Login.Summary)
			fmt.Fprintf(&b, "- 防火墙：%d/%d %s\n", ov.Firewall.Score, ov.Firewall.Max, ov.Firewall.Summary)
			fmt.Fprintf(&b, "- 文件完整性：%d/%d %s\n", ov.Fim.Score, ov.Fim.Max, ov.Fim.Summary)
			fmt.Fprintf(&b, "- 威胁检测：%d/%d %s\n", ov.Threat.Score, ov.Threat.Max, ov.Threat.Summary)
			return textResult(b.String()), nil
		},
	}
}

// searchSkillsTool searches the SkillHub marketplace.
func searchSkillsTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "search_skills",
		Title:       "搜索技能",
		Description: "在 SkillHub 技能市场搜索可安装的技能（Skill）。当用户想扩展能力、寻找现成技能时调用。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "skill.view",
		Schema: schema(map[string]any{
			"keyword": pStr("搜索关键词，例如 pdf、周报、excel"),
			"limit":   pInt("返回条数，默认 5"),
		}, "keyword"),
		Preview: func(args map[string]any) string { return "搜索技能：" + str(args, "keyword") },
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			kw := str(args, "keyword")
			if kw == "" {
				return failed("必须提供 keyword 参数"), nil
			}
			limit := intArg(args, "limit", 5)
			if limit <= 0 || limit > 20 {
				limit = 5
			}
			list, err := d.Skill.Search(kw, "", "score", 1, limit)
			if err != nil {
				return failed("搜索技能失败：%v", err), nil
			}
			if len(list) == 0 {
				return textResult("没有搜索到相关技能。"), nil
			}
			sort.SliceStable(list, func(i, j int) bool { return list[i].Downloads > list[j].Downloads })
			var b strings.Builder
			fmt.Fprintf(&b, "搜索到 %d 个技能：\n", len(list))
			for _, s := range list {
				state := "未安装"
				if s.Installed {
					state = "已安装"
				}
				fmt.Fprintf(&b, "- %s（slug: %s，%s，下载 %d，收藏 %d）\n  %s\n", s.Name, s.Slug, state, s.Downloads, s.Stars, truncate(s.Description, 120))
			}
			return textResult(b.String()), nil
		},
	}
}
