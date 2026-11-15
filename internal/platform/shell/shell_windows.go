//go:build windows

package shell

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// psUTF8 switches PowerShell's output stream from the console OEM code page
// (CP936 on a Chinese Windows) to UTF-8.
//
// The panel decodes captured output as UTF-8. Without this prefix every 中文 line
// a command prints arrives as legacy bytes and turns into replacement
// characters, which reads as "the terminal is broken" rather than "the encoding
// was not negotiated".
const psUTF8 = `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;`

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
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", psUTF8+line)
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
		// 用 -Command "& '<file>'" 而不是 -File，只因前者能在执行脚本前先设好
		// 输出编码（-File 没有空间放前缀）。脚本自己的 exit N 仍然是进程退出码。
		quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
		return exec.CommandContext(ctx, "powershell",
			"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-Command", psUTF8+"& "+quoted)
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
