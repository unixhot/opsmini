//go:build windows

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// psTimeout bounds a single PowerShell round trip.
//
// Without it a slow module load or a stalled Group Policy script leaves the HTTP
// handler waiting until the browser gives up, which reads as "the panel is
// down" rather than "this one page is slow". Callers whose query is known to be
// expensive ask for a larger bound via psJSONWithin instead of raising it for
// everything.
const psTimeout = 10 * time.Second

// psForceUTF8 switches PowerShell's output stream from the console OEM code page
// (CP936 on a Chinese Windows) to UTF-8.
//
// This is not cosmetic. Without it every non-ASCII character a cmdlet prints
// arrives as legacy bytes; encoding/json rejects them and substitutes U+FFFD, so
// rule names, network adapter aliases and account descriptions reach the panel
// as "???". The bytes are destroyed on the Go side, not mis-rendered on the
// frontend, which means no amount of UI work can recover them.
//
// 包一层 try/catch：setter 底层是 SetConsoleOutputCP，进程没有控制台时（被 NSSM、
// CREATE_NO_WINDOW 或 DETACHED_PROCESS 拉起）会抛 "The handle is invalid"，
// 不捕获就会让整条命令失败 —— 为了修乱码反而把功能整体打死，代价不对等。
// 失败时退回旧行为（可能乱码），至少功能还在。
const psForceUTF8 = `try{[Console]::OutputEncoding=[System.Text.Encoding]::UTF8}catch{};`

// psJSON runs a PowerShell expression and unmarshals its ConvertTo-Json output
// into dst, using the default timeout.
func psJSON(expr string, dst any) error { return psJSONWithin(expr, dst, psTimeout) }

// psJSONWithin is psJSON with an explicit timeout, for queries that are known to
// be slow (firewall rule enumeration, security event log parsing).
//
// A single result comes back as an object rather than an array, so the payload is
// normalised to an array before decoding.
//
// Errors carry the command's stderr: a bare "exit status 1" sends the operator
// hunting for a permission problem when the real cause is an empty event log or
// a missing module.
func psJSONWithin(expr string, dst any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	script := psForceUTF8 + "@(" + expr + ") | ConvertTo-Json -Depth 4 -Compress"
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("PowerShell 执行超时（%s）", timeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w：%s", err, msg)
		}
		return err
	}

	body := bytes.TrimSpace(stdout.Bytes())
	if len(body) == 0 {
		return nil
	}
	if body[0] != '[' {
		body = append(append([]byte("["), body...), ']')
	}
	return json.Unmarshal(body, dst)
}
