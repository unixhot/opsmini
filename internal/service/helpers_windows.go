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
// down" rather than "this one page is slow". runFirewallCmd has had the same
// bound for netsh since it was written.
const psTimeout = 10 * time.Second

// psForceUTF8 switches PowerShell's output stream from the console OEM code page
// (CP936 on a Chinese Windows) to UTF-8.
//
// This is not cosmetic. Without it every non-ASCII character a cmdlet prints
// arrives as legacy bytes; encoding/json rejects them and substitutes U+FFFD, so
// rule names, network adapter aliases and account descriptions reach the panel
// as "???". The bytes are destroyed on the Go side, not mis-rendered on the
// frontend, which means no amount of UI work can recover them.
const psForceUTF8 = `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;`

// psJSON runs a PowerShell expression and unmarshals its ConvertTo-Json output
// into dst. A single result comes back as an object rather than an array, so the
// payload is normalised to an array before decoding.
//
// Errors carry the command's stderr: a bare "exit status 1" sends the operator
// hunting for a permission problem when the real cause is an empty event log or
// a missing module.
func psJSON(expr string, dst any) error {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()

	script := psForceUTF8 + "@(" + expr + ") | ConvertTo-Json -Depth 4 -Compress"
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("PowerShell 执行超时（%s）", psTimeout)
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
