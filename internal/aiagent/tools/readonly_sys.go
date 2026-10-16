package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/service"
)

// readonlyTools returns the L0 tool set: observation only, never mutates the host.
func readonlyTools(d Deps) []aiagent.Tool {
	var out []aiagent.Tool
	if d.Sys != nil {
		out = append(out,
			sysOverviewTool(d),
			listProcessesTool(d),
			listPortsTool(d),
			listDisksTool(d),
			listNetworkTool(d),
			listServicesTool(d),
		)
	}
	if d.File != nil {
		out = append(out, dirUsageTool(d), readFileTool(d))
	}
	if d.Log != nil {
		out = append(out, searchLogsTool(d))
	}
	if d.Metrics != nil {
		out = append(out, queryMetricsTool(d))
	}
	if d.Docker != nil {
		out = append(out, dockerPSTool(d))
	}
	if d.Cron != nil || d.Crontab != nil {
		out = append(out, listCronTool(d))
	}
	if d.Website != nil {
		out = append(out, listWebsitesTool(d))
	}
	if d.Database != nil {
		out = append(out, listDatabasesTool(d))
	}
	if d.Security != nil {
		out = append(out, securityStatusTool(d))
	}
	if d.Skill != nil {
		out = append(out, searchSkillsTool(d))
	}
	return out
}

// sysOverviewTool reports the live host overview.
func sysOverviewTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "sys_overview",
		Title:       "系统概览",
		Description: "获取当前主机实时概览：CPU、内存、磁盘、负载、进程/端口/用户数量与在线时长。排查任何性能问题时应先调用本工具。",
		Risk:        aiagent.RiskReadonly,
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取主机实时概览" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			sum, err := d.Sys.MonitorSummary()
			if err != nil {
				return failed("采集系统概览失败：%v", err), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "主机名: %s\n", sum.Hostname)
			fmt.Fprintf(&b, "系统: %s %s (%s/%s)\n", sum.OS, sum.PlatformVer, sum.Platform, sum.KernelArch)
			fmt.Fprintf(&b, "CPU: %.1f%% / %d 核 (%s)\n", sum.CPUPercent, sum.CPUCores, sum.CPUModel)
			fmt.Fprintf(&b, "内存: %.1f%% 已用 %s / 共 %s\n", sum.MemPercent, fmtBytes(sum.MemUsed), fmtBytes(sum.MemTotal))
			fmt.Fprintf(&b, "磁盘(根分区): %.1f%% 已用 %s / 共 %s\n", sum.DiskPercent, fmtBytes(sum.DiskUsed), fmtBytes(sum.DiskTotal))
			fmt.Fprintf(&b, "负载: %.2f / %.2f / %.2f\n", sum.Load1, sum.Load5, sum.Load15)
			fmt.Fprintf(&b, "运行时长: %s\n", humanUptime(sum.Uptime))
			fmt.Fprintf(&b, "进程: %d 个（僵尸 %d）\n", sum.ProcessCount, sum.ZombieCount)
			fmt.Fprintf(&b, "主机用户: %d 个（可登录 %d）\n", sum.UserCount, sum.LoginUserCount)
			fmt.Fprintf(&b, "监听端口: %d 个（TCP %d / UDP %d）\n", sum.PortCount, sum.TcpPortCount, sum.UdpPortCount)
			if len(sum.IPs) > 0 {
				fmt.Fprintf(&b, "IP: %s\n", strings.Join(sum.IPs, ", "))
			}
			return textResult(b.String()), nil
		},
	}
}

// listProcessesTool lists processes sorted by memory or CPU.
func listProcessesTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_processes",
		Title:       "进程列表",
		Description: "列出主机进程，可按内存或 CPU 占用排序，用于定位资源消耗大户。",
		Risk:        aiagent.RiskReadonly,
		Schema: schema(map[string]any{
			"sort":   pEnum("排序依据", "mem", "cpu", "pid"),
			"limit":  pInt("返回条数，默认 20，最大 200"),
			"filter": pStr("按进程名或命令行关键字过滤（可选）"),
		}),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("列出进程（排序 %s，最多 %d 条）", strDef(args, "sort", "mem"), intArg(args, "limit", 20))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			limit := intArg(args, "limit", 20)
			if limit <= 0 || limit > 200 {
				limit = 200
			}
			procs, err := d.Sys.Processes(0)
			if err != nil {
				return failed("获取进程列表失败：%v", err), nil
			}
			filter := strings.ToLower(str(args, "filter"))
			if filter != "" {
				kept := procs[:0]
				for _, p := range procs {
					if strings.Contains(strings.ToLower(p.Name), filter) || strings.Contains(strings.ToLower(p.Cmdline), filter) {
						kept = append(kept, p)
					}
				}
				procs = kept
			}
			switch strDef(args, "sort", "mem") {
			case "cpu":
				sort.Slice(procs, func(i, j int) bool { return procs[i].MemPercent > procs[j].MemPercent })
			case "pid":
				sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
			default:
				sort.Slice(procs, func(i, j int) bool { return procs[i].MemPercent > procs[j].MemPercent })
			}
			total := len(procs)
			if len(procs) > limit {
				procs = procs[:limit]
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 个进程，按 %s 排序显示前 %d 个：\n", total, strDef(args, "sort", "mem"), len(procs))
			b.WriteString("PID\t用户\t内存%\t进程名\t命令行\n")
			for _, p := range procs {
				fmt.Fprintf(&b, "%d\t%s\t%.1f\t%s\t%s\n", p.PID, p.User, p.MemPercent, p.Name, truncate(p.Cmdline, 120))
			}
			return textResult(b.String()), nil
		},
	}
}

// listPortsTool lists listening ports.
func listPortsTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_ports",
		Title:       "监听端口",
		Description: "列出主机正在监听的所有端口及其所属进程，用于排查端口占用与服务暴露情况。",
		Risk:        aiagent.RiskReadonly,
		Schema: schema(map[string]any{
			"port":     pInt("只查看指定端口（可选）"),
			"protocol": pEnum("协议过滤", "tcp", "udp"),
		}),
		Preview: func(map[string]any) string { return "读取监听端口列表" },
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			ports, err := d.Sys.Ports()
			if err != nil {
				return failed("获取端口列表失败：%v", err), nil
			}
			wantPort := intArg(args, "port", 0)
			wantProto := strings.ToLower(str(args, "protocol"))
			var b strings.Builder
			b.WriteString("协议\t端口\t监听地址\t进程\t命令行\n")
			n := 0
			for _, p := range ports {
				if wantPort > 0 && int(p.Port) != wantPort {
					continue
				}
				if wantProto != "" && strings.ToLower(p.Protocol) != wantProto {
					continue
				}
				fmt.Fprintf(&b, "%s\t%d\t%s\t%s\t%s\n", p.Protocol, p.Port, p.Address, p.Name, truncate(p.Cmdline, 100))
				n++
			}
			if n == 0 {
				return textResult("没有匹配的监听端口。"), nil
			}
			return textResult(fmt.Sprintf("共 %d 条监听记录：\n%s", n, b.String())), nil
		},
	}
}

// listDisksTool lists disk partitions.
func listDisksTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_disks",
		Title:       "磁盘分区",
		Description: "列出所有磁盘分区及其使用率，用于定位磁盘空间不足。",
		Risk:        aiagent.RiskReadonly,
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取磁盘分区使用情况" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			disks, err := d.Sys.Disks()
			if err != nil {
				return failed("获取磁盘信息失败：%v", err), nil
			}
			var b strings.Builder
			b.WriteString("设备\t挂载点\t文件系统\t总量\t已用\t可用\t使用率\n")
			for _, dk := range disks {
				fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%.1f%%\n",
					dk.Device, dk.Mountpoint, dk.Fstype,
					fmtBytes(dk.Total), fmtBytes(dk.Used), fmtBytes(dk.Free), dk.UsedPercent)
			}
			return textResult(b.String()), nil
		},
	}
}

// listNetworkTool lists network interfaces and routes.
func listNetworkTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_network",
		Title:       "网络接口与路由",
		Description: "列出网络接口（含流量统计）与内核路由表，用于排查网络连通性问题。",
		Risk:        aiagent.RiskReadonly,
		Schema:      schema(map[string]any{}),
		Preview:     func(map[string]any) string { return "读取网络接口与路由表" },
		Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
			var b strings.Builder
			if nics, err := d.Sys.Networks(); err == nil {
				b.WriteString("【网络接口】\n名称\t类型\tMTU\t接收\t发送\t地址\n")
				for _, n := range nics {
					fmt.Fprintf(&b, "%s\t%s\t%d\t%s\t%s\t%s\n", n.Name, n.Type, n.MTU,
						fmtBytes(n.BytesRecv), fmtBytes(n.BytesSent), strings.Join(n.Addrs, ", "))
				}
			}
			if routes, err := d.Sys.Routes(); err == nil && len(routes) > 0 {
				b.WriteString("\n【路由表】\n目标\t网关\t掩码\t接口\tmetric\n")
				for _, r := range routes {
					fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\n", r.Destination, r.Gateway, r.Genmask, r.Iface, r.Metric)
				}
			}
			return textResult(b.String()), nil
		},
	}
}

// listServicesTool lists systemd units (read-only).
func listServicesTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "list_services",
		Title:       "系统服务",
		Description: "列出 systemd 服务及其运行状态（只读）。可通过 state 过滤 running/failed。",
		Risk:        aiagent.RiskReadonly,
		Schema: schema(map[string]any{
			"state":  pEnum("过滤状态", "running", "failed", "exited"),
			"filter": pStr("服务名关键字过滤（可选）"),
			"limit":  pInt("返回条数，默认 40"),
		}),
		Preview: func(map[string]any) string { return "读取 systemd 服务列表（只读）" },
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			out, err := runCmd(ctx, 15*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend", "--plain")
			if err != nil && strings.TrimSpace(out) == "" {
				return failed("当前系统不支持 systemctl 或执行失败：%v", err), nil
			}
			limit := intArg(args, "limit", 40)
			if limit <= 0 || limit > 300 {
				limit = 40
			}
			state := strings.ToLower(str(args, "state"))
			kw := strings.ToLower(str(args, "filter"))
			var rows []string
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				name, load, active, sub := fields[0], fields[1], fields[2], fields[3]
				if kw != "" && !strings.Contains(strings.ToLower(name), kw) {
					continue
				}
				if state != "" && !strings.Contains(strings.ToLower(sub), state) && !strings.Contains(strings.ToLower(active), state) {
					continue
				}
				if load == "not-found" {
					continue
				}
				rows = append(rows, fmt.Sprintf("%s\t%s\t%s", name, active, sub))
				if len(rows) >= limit {
					break
				}
			}
			if len(rows) == 0 {
				return textResult("没有匹配的系统服务。"), nil
			}
			return textResult("服务名\t状态\t子状态\n" + strings.Join(rows, "\n")), nil
		},
	}
}

// dirUsageTool reports the size of directories or the biggest children of a directory.
func dirUsageTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "dir_usage",
		Title:       "目录占用分析",
		Description: "统计指定目录的总大小，并列出占用最大的子目录/文件（相当于 du -sh *）。定位磁盘写满的首要工具。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "file.view",
		Schema: schema(map[string]any{
			"path": pStr("目录绝对路径，例如 /var/log"),
			"top":  pInt("列出占用最大的前 N 项，默认 15"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "统计目录占用：" + str(args, "path")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			if !filepath.IsAbs(path) {
				return failed("必须使用绝对路径"), nil
			}
			total, files, dirs, children, err := scanDir(path)
			if err != nil {
				return failed("统计目录 %s 失败：%v", path, err), nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "目录 %s 递归总大小 %s（文件 %d 个，子目录 %d 个）\n", path, fmtBytes(uint64(total)), files, dirs)
			top := intArg(args, "top", 15)
			if top <= 0 || top > 100 {
				top = 15
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Size > children[j].Size })
			if len(children) > top {
				children = children[:top]
			}
			if len(children) > 0 {
				b.WriteString("\n占用最大的条目：\n名称\t类型\t大小\t路径\n")
				for _, e := range children {
					kind := "文件"
					if e.IsDir {
						kind = "目录"
					}
					fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", e.Name, kind, fmtBytes(uint64(e.Size)), e.Path)
				}
			}
			return textResult(b.String()), nil
		},
	}
}

// dirChild is the aggregated size of one direct child of a directory.
type dirChild struct {
	Name  string
	Path  string
	IsDir bool
	Size  int64
}

// scanDir walks a directory once and aggregates the recursive size of each
// direct child. maxScanFiles bounds the work so a huge tree cannot stall a run.
func scanDir(path string) (total int64, files, dirs int, children []dirChild, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	if !info.IsDir() {
		return info.Size(), 1, 0, nil, nil
	}
	const maxScanFiles = 400000
	idxOf := map[string]int{}
	sep := string(filepath.Separator)
	scanned := 0
	walkErr := filepath.WalkDir(path, func(p string, e fs.DirEntry, werr error) error {
		if werr != nil || p == path {
			return nil
		}
		scanned++
		if scanned > maxScanFiles {
			return filepath.SkipAll
		}
		rel, rerr := filepath.Rel(path, p)
		if rerr != nil {
			return nil
		}
		first := rel
		if i := strings.Index(rel, sep); i >= 0 {
			first = rel[:i]
		}
		ci, ok := idxOf[first]
		if !ok {
			childPath := filepath.Join(path, first)
			children = append(children, dirChild{Name: first, Path: childPath, IsDir: e.IsDir()})
			ci = len(children) - 1
			idxOf[first] = ci
		}
		if e.IsDir() {
			dirs++
			return nil
		}
		files++
		var sz int64
		if fi, ierr := e.Info(); ierr == nil {
			sz = fi.Size()
		}
		total += sz
		children[ci].Size += sz
		return nil
	})
	return total, files, dirs, children, walkErr
}

// readFileTool reads a text file (bounded).
func readFileTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "read_file",
		Title:       "读取文件",
		Description: "读取服务器上的文本文件内容（自动限制大小）。可用于查看配置文件、日志片段。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "file.view",
		Schema: schema(map[string]any{
			"path":      pStr("文件绝对路径"),
			"max_lines": pInt("最多读取行数，默认 200"),
			"tail":      pBool("true 表示读取文件末尾（适合日志），默认 false"),
		}, "path"),
		Preview: func(args map[string]any) string { return "读取文件 " + str(args, "path") },
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			if !filepath.IsAbs(path) {
				return failed("必须使用绝对路径"), nil
			}
			st, err := os.Stat(path)
			if err != nil {
				return failed("文件不存在或无法访问：%v", err), nil
			}
			if st.IsDir() {
				return failed("%s 是目录，请使用 dir_usage 或 list_files", path), nil
			}
			const maxBytes = 512 << 10
			f, err := os.Open(path)
			if err != nil {
				return failed("打开文件失败：%v", err), nil
			}
			defer f.Close()
			size := st.Size()
			start := int64(0)
			tail := boolArg(args, "tail", false)
			if tail && size > maxBytes {
				start = size - maxBytes
			}
			if _, err := f.Seek(start, 0); err != nil {
				return failed("定位文件失败：%v", err), nil
			}
			buf := make([]byte, maxBytes)
			n, _ := f.Read(buf)
			content := string(buf[:n])
			if start > 0 {
				if idx := strings.Index(content, "\n"); idx >= 0 {
					content = content[idx+1:]
				}
			}
			maxLines := intArg(args, "max_lines", 200)
			if maxLines <= 0 || maxLines > 5000 {
				maxLines = 200
			}
			lines := strings.Split(content, "\n")
			truncated := false
			if len(lines) > maxLines {
				if tail {
					lines = lines[len(lines)-maxLines:]
				} else {
					lines = lines[:maxLines]
				}
				truncated = true
			}
			head := fmt.Sprintf("文件 %s（大小 %s，共 %d 行）", path, fmtBytes(uint64(size)), len(strings.Split(content, "\n")))
			if truncated {
				head += "，已截断显示"
			}
			body := head + "：\n" + strings.Join(lines, "\n")
			return aiagent.Result{OK: true, Summary: body, Detail: body}, nil
		},
	}
}

// searchLogsTool searches a log file for a pattern.
func searchLogsTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "search_logs",
		Title:       "日志检索",
		Description: "查看系统日志（syslog/auth.log/nginx 等），可按关键字或正则过滤。排查报错、登录失败、服务异常时使用。",
		Risk:        aiagent.RiskReadonly,
		Perm:        "log.view",
		Schema: schema(map[string]any{
			"name":    pStr("日志名称，例如 syslog、auth.log、Nginx error.log（可先用 list 查看可用列表）"),
			"pattern": pStr("过滤关键字或正则（可选）"),
			"lines":   pInt("最多返回行数，默认 100"),
		}),
		Preview: func(args map[string]any) string {
			return "检索日志 " + strDef(args, "name", "syslog")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			sources := d.Log.List()
			if len(sources) == 0 {
				return failed("没有可读的日志文件"), nil
			}
			name := str(args, "name")
			if name == "" || name == "list" {
				var names []string
				for _, s := range sources {
					names = append(names, fmt.Sprintf("%s(%s)", s.Name, fmtBytes(uint64(s.Size))))
				}
				return textResult("可用日志：" + strings.Join(names, "、") + "\n请用 name 参数指定要查看的日志。"), nil
			}
			var chosen *service.LogSource
			for i := range sources {
				if strings.EqualFold(sources[i].Name, name) || strings.EqualFold(filepath.Base(sources[i].Path), name) {
					chosen = &sources[i]
					break
				}
			}
			if chosen == nil {
				for i := range sources {
					if strings.Contains(strings.ToLower(sources[i].Name), strings.ToLower(name)) {
						chosen = &sources[i]
						break
					}
				}
			}
			if chosen == nil {
				return failed("未找到名为 %s 的日志，请先不带 name 调用本工具查看可用列表", name), nil
			}
			lines := intArg(args, "lines", 100)
			if lines <= 0 || lines > 2000 {
				lines = 100
			}
			// Read more than requested so filtering still returns enough context.
			raw, err := d.Log.Tail(chosen.Path, 3000)
			if err != nil {
				return failed("读取日志 %s 失败：%v", chosen.Name, err), nil
			}
			pattern := str(args, "pattern")
			var re *regexp.Regexp
			if pattern != "" {
				re, _ = regexp.Compile("(?i)" + pattern)
			}
			var matched []string
			for _, line := range strings.Split(raw, "\n") {
				if line == "" {
					continue
				}
				if re != nil && !re.MatchString(line) {
					continue
				}
				matched = append(matched, line)
			}
			if len(matched) > lines {
				matched = matched[len(matched)-lines:]
			}
			if len(matched) == 0 {
				return textResult(fmt.Sprintf("日志 %s 中没有匹配 %q 的内容（已扫描最近 3000 行）。", chosen.Name, pattern)), nil
			}
			body := fmt.Sprintf("日志 %s（路径 %s），匹配 %d 行：\n%s", chosen.Name, chosen.Path, len(matched), strings.Join(matched, "\n"))
			return aiagent.Result{OK: true, Summary: truncate(body, 6000), Detail: body}, nil
		},
	}
}

// queryMetricsTool returns recent metric samples.
func queryMetricsTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "query_metrics",
		Title:       "历史指标",
		Description: "查询最近的性能趋势采样点（CPU/内存/负载/网络速率），用于判断问题是突发还是持续，以及发生在什么时候。",
		Risk:        aiagent.RiskReadonly,
		Schema: schema(map[string]any{
			"window": pEnum("时间窗口", "5m", "30m", "1h", "6h", "24h"),
			"points": pInt("返回的采样点数量，默认 30"),
		}),
		Preview: func(args map[string]any) string {
			return "查询最近 " + strDef(args, "window", "30m") + " 的性能指标"
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			window := strDef(args, "window", "30m")
			var dur time.Duration
			switch window {
			case "5m":
				dur = 5 * time.Minute
			case "1h":
				dur = time.Hour
			case "6h":
				dur = 6 * time.Hour
			case "24h":
				dur = 24 * time.Hour
			default:
				dur = 30 * time.Minute
			}
			pts := d.Metrics.Since(dur)
			if len(pts) == 0 {
				return textResult("该时间窗口内没有采集到指标数据。"), nil
			}
			n := intArg(args, "points", 30)
			if n <= 0 {
				n = 30
			}
			step := 1
			if len(pts) > n {
				step = len(pts) / n
			}
			var b strings.Builder
			fmt.Fprintf(&b, "窗口 %s，共 %d 个采样点（每 %d 点取 1）：\n时间\tCPU%%\t内存%%\t负载1\n", window, len(pts), step)
			var maxCPU, maxMem float64
			for i := 0; i < len(pts); i += step {
				p := pts[i]
				if p.CPU > maxCPU {
					maxCPU = p.CPU
				}
				if p.Mem > maxMem {
					maxMem = p.Mem
				}
				fmt.Fprintf(&b, "%s\t%.1f\t%.1f\t%.2f\n", time.Unix(p.Time, 0).Format("15:04:05"), p.CPU, p.Mem, p.Load1)
			}
			fmt.Fprintf(&b, "\n峰值：CPU %.1f%%，内存 %.1f%%\n", maxCPU, maxMem)
			return textResult(b.String()), nil
		},
	}
}

// humanUptime formats seconds into a readable duration.
func humanUptime(sec uint64) string {
	day := sec / 86400
	hour := sec % 86400 / 3600
	min := sec % 3600 / 60
	if day > 0 {
		return fmt.Sprintf("%d 天 %d 小时 %d 分", day, hour, min)
	}
	if hour > 0 {
		return fmt.Sprintf("%d 小时 %d 分", hour, min)
	}
	return fmt.Sprintf("%d 分钟", min)
}
