//go:build !windows

package shell

import (
	"context"
	"fmt"
	"os/exec"
)

// command runs line through /bin/sh, matching the behaviour the Agent has always
// had on Linux: `sh -c` so pipes, redirections and globs work as the user wrote
// them.
func command(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}

// script runs a script file. `shell` is plain sh, which is what a "shell script"
// means here; PowerShell and batch files have no interpreter on this platform.
func script(ctx context.Context, language, path string) *exec.Cmd {
	switch language {
	case "python":
		return exec.CommandContext(ctx, "python3", path)
	default:
		return exec.CommandContext(ctx, "sh", path)
	}
}

func validate(language string) error {
	switch language {
	case "shell", "python":
		return nil
	case "powershell":
		return fmt.Errorf("当前主机不支持 PowerShell 脚本")
	case "cmd":
		return fmt.Errorf("当前主机不支持 cmd 批处理脚本")
	default:
		return fmt.Errorf("不支持的脚本类型 %q", language)
	}
}

func extFor(language string) string {
	switch language {
	case "python":
		return ".py"
	default:
		return ".sh"
	}
}
