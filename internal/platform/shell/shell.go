// Package shell runs external commands the way the host's own shell would,
// which is the one thing that genuinely differs between the panel's targets:
// POSIX shells take `-c`, cmd.exe takes `/c`, and PowerShell takes `-Command`.
//
// Script languages are named by intent rather than by interpreter, so that the
// same request means "run this natively here":
//
//	shell       sh on Unix, PowerShell on Windows
//	powershell  PowerShell (unsupported on Unix)
//	cmd         cmd.exe batch (unsupported on Unix)
//	python      python3 on Unix, python on Windows
//
// The caller is responsible for writing the script to a file whose extension
// matches ExtFor(language).
package shell

import (
	"context"
	"os/exec"
)

// aliases maps accepted spellings onto the four canonical language names.
var aliases = map[string]string{
	"":        "shell",
	"sh":      "shell",
	"bash":    "shell",
	"pwsh":    "powershell",
	"ps1":     "powershell",
	"bat":     "cmd",
	"batch":   "cmd",
	"python3": "python",
	"py":      "python",
}

// NormalizeLanguage maps an alias to a canonical language name. An unknown
// value is returned unchanged so the caller can reject it.
func NormalizeLanguage(language string) string {
	if canon, ok := aliases[language]; ok {
		return canon
	}
	return language
}

// Validate reports whether this platform can run the already-normalized
// language. It exists so an unsupported request fails with a clear message
// instead of a confusing exec error.
func Validate(language string) error { return validate(language) }

// Command returns a command that runs line through the platform shell.
func Command(ctx context.Context, line string) *exec.Cmd { return command(ctx, line) }

// Script returns a command that runs the script file at path.
func Script(ctx context.Context, language, path string) *exec.Cmd {
	return script(ctx, language, path)
}

// ExtFor returns the file extension a script of this language needs.
func ExtFor(language string) string { return extFor(language) }
