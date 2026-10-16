package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
)

// RemoteClient talks to the OpsMini Server control-plane HTTP API (typically
// running on the OpsAnt host next to the gRPC data plane). Through it the panel
// can drive every other host that has an OpsMini Agent enrolled, without SSH.
type RemoteClient struct {
	// BaseURL returns the control-plane base URL, e.g. http://47.104.172.52:4508.
	BaseURL func() string
	// Token is an optional bearer token for the control plane.
	Token func() string
	// DataDir returns a local directory used to store pulled files.
	DataDir func() string
	client  *http.Client
}

// NewRemoteClient creates a RemoteClient.
func NewRemoteClient(baseURL, token, dataDir func() string) *RemoteClient {
	return &RemoteClient{
		BaseURL: baseURL,
		Token:   token,
		DataDir: dataDir,
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

// Configured reports whether a control-plane address has been set.
func (c *RemoteClient) Configured() bool {
	return c != nil && c.BaseURL != nil && strings.TrimSpace(c.BaseURL()) != ""
}

// RemoteAgent is one enrolled host reported by the control plane.
type RemoteAgent struct {
	Hostname    string    `json:"hostname"`
	Version     string    `json:"version"`
	IP          string    `json:"ip"`
	PanelPort   int32     `json:"panel_port"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	Online      bool      `json:"online"`
}

// ExecuteResult is the outcome of a remote command or script.
type ExecuteResult struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	ExitCode   int32  `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
}

// ListAgents returns every host currently enrolled on the control plane.
func (c *RemoteClient) ListAgents(ctx context.Context) ([]RemoteAgent, error) {
	var out struct {
		Agents []RemoteAgent `json:"agents"`
	}
	if err := c.do(ctx, http.MethodGet, "/agents", nil, &out); err != nil {
		return nil, err
	}
	return out.Agents, nil
}

// Execute dispatches a command or a script to a remote host.
func (c *RemoteClient) Execute(ctx context.Context, hostname, command, script, language string, timeout int) (*ExecuteResult, error) {
	body := map[string]any{"hostname": hostname}
	if command != "" {
		body["command"] = command
	}
	if script != "" {
		body["script"] = script
		body["language"] = language
	}
	if timeout > 0 {
		body["timeout"] = timeout
	}
	var out ExecuteResult
	if err := c.do(ctx, http.MethodPost, "/execute", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PullFile fetches a file from a remote host and stores it locally.
func (c *RemoteClient) PullFile(ctx context.Context, hostname, path string) (localPath string, size int, md5sum string, err error) {
	var out struct {
		Path          string `json:"path"`
		Size          int    `json:"size"`
		MD5           string `json:"md5"`
		ContentBase64 string `json:"content_base64"`
	}
	if err := c.do(ctx, http.MethodPost, "/pull", map[string]any{"hostname": hostname, "path": path}, &out); err != nil {
		return "", 0, "", err
	}
	data, derr := base64.StdEncoding.DecodeString(out.ContentBase64)
	if derr != nil {
		return "", 0, "", fmt.Errorf("解码文件内容失败：%w", derr)
	}
	dir := "/var/lib/opsmini/data/pulled"
	if c.DataDir != nil && c.DataDir() != "" {
		dir = filepath.Join(c.DataDir(), "pulled")
	}
	if mkerr := os.MkdirAll(dir, 0o755); mkerr != nil {
		return "", 0, "", mkerr
	}
	name := fmt.Sprintf("%s-%s", hostname, strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_"))
	local := filepath.Join(dir, name)
	if werr := os.WriteFile(local, data, 0o644); werr != nil {
		return "", 0, "", werr
	}
	return local, out.Size, out.MD5, nil
}

// PushFile writes content to a path on a remote host. Long transfers are encoded
// as base64 inside a single command, so the control plane needs no extra API.
func (c *RemoteClient) PushFile(ctx context.Context, hostname, dest string, content []byte, mode string) error {
	if len(content) > maxRemotePushBytes {
		return fmt.Errorf("远程下发内容上限 %s，当前 %s", fmtBytes(maxRemotePushBytes), fmtBytes(uint64(len(content))))
	}
	if !filepath.IsAbs(dest) {
		return errors.New("目标路径必须是绝对路径")
	}
	encoded := base64.StdEncoding.EncodeToString(content)
	dir := filepath.Dir(dest)
	cmd := fmt.Sprintf("mkdir -p %s && base64 -d > %s <<'OPSMINI_EOF'\n%s\nOPSMINI_EOF\n", shellQuote(dir), shellQuote(dest), encoded)
	if mode != "" {
		cmd += fmt.Sprintf("chmod %s %s\n", shellQuote(mode), shellQuote(dest))
	}
	res, err := c.Execute(ctx, hostname, cmd, "", "", 120)
	if err != nil {
		return err
	}
	if res.Status != "success" {
		return fmt.Errorf("下发失败（%s）：%s", res.Status, truncate(res.Stderr, 300))
	}
	return nil
}

// maxRemotePushBytes caps a single content push (the payload travels inside a shell command).
const maxRemotePushBytes = 512 << 10

// do performs a control-plane request and decodes the JSON response.
func (c *RemoteClient) do(ctx context.Context, method, path string, body any, out any) error {
	if !c.Configured() {
		return errors.New("未配置远程 OpsMini Server 控制面地址")
	}
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL()), "/")
	if _, err := url.Parse(base); err != nil {
		return fmt.Errorf("远程 Server 地址无效：%w", err)
	}
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != nil {
		if t := strings.TrimSpace(c.Token()); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求远程 Server 失败：%w", err)
	}
	defer resp.Body.Close()
	raw := make([]byte, 0, 4096)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			raw = append(raw, buf[:n]...)
			if len(raw) > 64<<20 {
				return errors.New("远程响应过大")
			}
		}
		if rerr != nil {
			break
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("远程 Server 返回 %d：%s", resp.StatusCode, truncate(string(raw), 300))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("解析远程响应失败：%w", err)
		}
	}
	return nil
}

// shellQuote wraps a value in single quotes for safe use in a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// remoteTools returns the multi-host tool set (M8-E).
func remoteTools(d Deps) []aiagent.Tool {
	rc := d.Remote
	if rc == nil || !rc.Configured() {
		return nil
	}
	remote := func(args map[string]any) string { return str(args, "host") }
	return []aiagent.Tool{
		{
			Name:        "list_remote_hosts",
			Title:       "远程主机列表",
			Description: "列出通过 OpsMini Server 纳管的所有远程主机及其在线状态。需要操作其它主机前应先调用本工具确认主机名。",
			Risk:        aiagent.RiskReadonly,
			Schema:      schema(map[string]any{}),
			Preview:     func(map[string]any) string { return "读取远程主机列表" },
			Execute: func(ctx aiagent.ExecContext, _ map[string]any) (aiagent.Result, error) {
				agents, err := rc.ListAgents(ctx)
				if err != nil {
					return failed("读取远程主机失败：%v", err), nil
				}
				if len(agents) == 0 {
					return textResult("当前没有已纳管的远程主机。"), nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "共 %d 台远程主机：\n主机名\t状态\tIP\t版本\t面板端口\t最后心跳\n", len(agents))
				for _, a := range agents {
					state := "离线"
					if a.Online {
						state = "在线"
					}
					fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%d\t%s\n", a.Hostname, state, a.IP, a.Version, a.PanelPort,
						a.LastSeen.Format("2006-01-02 15:04:05"))
				}
				return textResult(b.String()), nil
			},
		},
		{
			Name:        "remote_exec",
			Title:       "远程执行",
			Description: "在一台远程主机上执行命令或脚本（免 SSH，通过 OpsMini Agent 通道）。执行前请先用 list_remote_hosts 确认主机名。",
			Risk:        aiagent.RiskHighWrite,
			Perm:        "ai.agent.write",
			Schema: schema(map[string]any{
				"host":     pStr("远程主机名（须与 list_remote_hosts 返回的一致）"),
				"command":  pStr("要执行的 Shell 命令（与 script 二选一）"),
				"script":   pStr("要执行的脚本内容（与 command 二选一）"),
				"language": pEnum("脚本语言", "shell", "python"),
				"timeout":  pInt("超时秒数，默认 60，最大 600"),
			}, "host"),
			Preview: func(args map[string]any) string {
				if cmd := str(args, "command"); cmd != "" {
					return fmt.Sprintf("在远程主机 %s 上执行命令：`%s`", remote(args), cmd)
				}
				return fmt.Sprintf("在远程主机 %s 上执行脚本（%d 字节）", remote(args), len(str(args, "script")))
			},
			Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
				host := remote(args)
				if host == "" {
					return failed("必须提供 host 参数"), nil
				}
				command, script := str(args, "command"), str(args, "script")
				if command == "" && script == "" {
					return failed("command 与 script 至少提供一个"), nil
				}
				timeout := intArg(args, "timeout", 60)
				if timeout <= 0 || timeout > 600 {
					timeout = 60
				}
				res, err := rc.Execute(ctx, host, command, script, strDef(args, "language", "shell"), timeout)
				if err != nil {
					return failed("远程执行失败：%v", err), nil
				}
				var b strings.Builder
				fmt.Fprintf(&b, "主机 %s 执行结果：%s（退出码 %d，耗时 %dms）\n", host, res.Status, res.ExitCode, res.DurationMs)
				if res.Stdout != "" {
					b.WriteString("--- stdout ---\n" + truncate(res.Stdout, maxExecOutput) + "\n")
				}
				if res.Stderr != "" {
					b.WriteString("--- stderr ---\n" + truncate(res.Stderr, 16<<10) + "\n")
				}
				return aiagent.Result{OK: res.Status == "success", Summary: b.String(), Detail: b.String()}, nil
			},
		},
		{
			Name:        "remote_pull_file",
			Title:       "远程拉取文件",
			Description: "从远程主机拉取一个文件并保存到本面板的数据目录（默认 /var/lib/opsmini/data/pulled）。",
			Risk:        aiagent.RiskLowWrite,
			Perm:        "ai.agent.write",
			Schema: schema(map[string]any{
				"host": pStr("远程主机名"),
				"path": pStr("远程主机上的文件绝对路径"),
			}, "host", "path"),
			Preview: func(args map[string]any) string {
				return fmt.Sprintf("从远程主机 %s 拉取 %s", remote(args), str(args, "path"))
			},
			Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
				host, path := remote(args), str(args, "path")
				if host == "" || path == "" {
					return failed("host 与 path 均为必填"), nil
				}
				local, size, sum, err := rc.PullFile(ctx, host, path)
				if err != nil {
					return failed("拉取失败：%v", err), nil
				}
				return textResult(fmt.Sprintf("已从 %s 拉取 %s（%s，md5 %s）到本地 %s。",
					host, path, fmtBytes(uint64(size)), sum, local)), nil
			},
		},
		{
			Name:        "remote_push_file",
			Title:       "远程下发文件",
			Description: "把内容写入远程主机上的指定路径（自动创建目录，可选权限）。单次内容上限 512KB。",
			Risk:        aiagent.RiskHighWrite,
			Perm:        "ai.agent.write",
			Schema: schema(map[string]any{
				"host":    pStr("远程主机名"),
				"path":    pStr("远程主机上的目标绝对路径"),
				"content": pStr("要写入的内容"),
				"mode":    pStr("文件权限，例如 0644（可选）"),
			}, "host", "path", "content"),
			Preview: func(args map[string]any) string {
				return fmt.Sprintf("向远程主机 %s 写入文件 %s（%d 字节）", remote(args), str(args, "path"), len(str(args, "content")))
			},
			Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
				host, path := remote(args), str(args, "path")
				if host == "" || path == "" {
					return failed("host 与 path 均为必填"), nil
				}
				content := str(args, "content")
				if err := rc.PushFile(ctx, host, path, []byte(content), str(args, "mode")); err != nil {
					return failed("下发失败：%v", err), nil
				}
				return textResult(fmt.Sprintf("已向 %s 写入 %s（%d 字节）。", host, path, len(content))), nil
			},
		},
	}
}
