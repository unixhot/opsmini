//go:build windows

package shell

import (
	"context"
	"fmt"
	"os/exec"
)

// command runs line through PowerShell.
//
// PowerShell rather than cmd.exe because the shell string reaching this layer
// comes from a human or a model describing an administrative task, and the
// modern Windows administrative vocabulary (Get-Service, Get-Process,
// Get-NetTCPConnection) lives there. -NoProfile keeps a user's profile script
// from changing behaviour or printing banners into the captured output;
// -NonInteractive prevents a prompt from hanging the Agent loop until timeout.
func command(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "powershell",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", line)
}

// script runs a script file with the interpreter that matches its language.
// `shell` writes a .ps1 (see extFor), so it runs under PowerShell; a caller that
// really wants batch has to say "cmd".
func script(ctx context.Context, language, path string) *exec.Cmd {
	switch language {
	case "python":
		return exec.CommandContext(ctx, "python", path)
	case "cmd":
		return exec.CommandContext(ctx, "cmd", "/c", path)
	default:
		return exec.CommandContext(ctx, "powershell",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	}
}

func validate(language string) error {
	switch language {
	case "shell", "powershell", "cmd", "python":
		return nil
	default:
		return fmt.Errorf("不支持的脚本类型 %q", language)
	}
}

func extFor(language string) string {
	switch language {
	case "python":
		return ".py"
	case "cmd":
		return ".cmd"
	default:
		return ".ps1"
	}
}
