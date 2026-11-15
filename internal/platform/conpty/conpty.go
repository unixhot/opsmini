// Package conpty starts an interactive shell attached to a pseudo terminal.
//
// Unix gets a pty (creack/pty); Windows gets a ConPTY, the Win10 1809+
// console host that lets a console program believe it owns a terminal. Both are
// exposed as a single duplex stream plus a resize operation, so the WebSocket
// terminal handler does not branch on platform.
package conpty

import (
	"context"
	"io"
	"os/exec"
)

// Console is an interactive shell's terminal: write goes to the shell's input,
// read comes from its output, and Resize only affects programs that query the
// window size.
type Console interface {
	io.ReadWriteCloser
	// Resize sets the terminal size in character cells.
	Resize(cols, rows uint16) error
}

// DefaultShell returns the shell to start when the caller does not name one.
func DefaultShell() string { return defaultShell() }

// ShellEnv returns the environment to hand to shell.
func ShellEnv(shell string) []string { return shellEnv(shell) }

// Start launches shell with an initial window size and returns its console.
// Closing the console terminates the shell.
func Start(ctx context.Context, shell string, env []string, cols, rows uint16) (Console, error) {
	return start(ctx, shell, env, cols, rows)
}

// lookPath is shared by the platform implementations.
func lookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	return p, err == nil
}
