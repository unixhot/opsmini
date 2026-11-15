//go:build !windows

package conpty

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"sync"

	"github.com/creack/pty"
)

type unixConsole struct {
	f    *os.File
	cmd  *exec.Cmd
	once sync.Once
}

func start(_ context.Context, shell string, env []string, cols, rows uint16) (Console, error) {
	cmd := exec.Command(shell)
	cmd.Env = env
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &unixConsole{f: f, cmd: cmd}, nil
}

func (c *unixConsole) Read(p []byte) (int, error)  { return c.f.Read(p) }
func (c *unixConsole) Write(p []byte) (int, error) { return c.f.Write(p) }

func (c *unixConsole) Resize(cols, rows uint16) error {
	return pty.Setsize(c.f, &pty.Winsize{Cols: cols, Rows: rows})
}

func (c *unixConsole) Close() error {
	c.once.Do(func() {
		_ = c.f.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
	})
	return nil
}

func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/bash"
}

// shellEnv derives HOME/USER/LOGNAME/SHELL from the account database, because a
// systemd unit does not set them: without them `cd` fails with "HOME not set",
// `~` does not expand, and vim/git silently skip their dotfiles.
func shellEnv(shell string) []string {
	uid := os.Getuid()
	home, username := "", ""
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		home, username = u.HomeDir, u.Username
	}
	if home == "" {
		if uid == 0 {
			home = "/root"
		} else {
			home = "/"
		}
	}
	if username == "" {
		if uid == 0 {
			username = "root"
		} else {
			username = strconv.Itoa(uid)
		}
	}

	overrides := map[string]string{
		"HOME":    home,
		"USER":    username,
		"LOGNAME": username,
		"SHELL":   shell,
		// TERM must be set or full-screen programs (top, vim) cannot render.
		"TERM": "xterm-256color",
		// C.UTF-8 ships with glibc, so it avoids setlocale warnings when
		// en_US.UTF-8 is not installed.
		"LANG": "C.UTF-8",
	}

	out := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		name := kv
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				name = kv[:i]
				break
			}
		}
		if _, replaced := overrides[name]; replaced {
			// execve keeps duplicate keys and libc getenv returns the first, so
			// appending alone would leave an empty inherited value in effect.
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
