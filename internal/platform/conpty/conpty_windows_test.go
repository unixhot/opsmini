//go:build windows

package conpty

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestConsoleRoundTrip proves the ConPTY plumbing end to end: create the
// pseudo console, start a shell inside it, write a command and read its echo
// back. It is the only check that covers CreatePseudoConsole, the attribute
// list handoff and the pipe directions at once.
func TestConsoleRoundTrip(t *testing.T) {
	shell := DefaultShell()
	con, err := Start(context.Background(), shell, ShellEnv(shell), 100, 30)
	if err != nil {
		t.Fatalf("Start(%s): %v", shell, err)
	}
	defer con.Close()

	if err := con.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	go func() {
		time.Sleep(1500 * time.Millisecond)
		_, _ = con.Write([]byte("Write-Output OPSMINI_CONPTY_OK\r\n"))
		time.Sleep(1500 * time.Millisecond)
		_, _ = con.Write([]byte("exit\r\n"))
	}()

	deadline := time.Now().Add(15 * time.Second)
	var out strings.Builder
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		n, rerr := con.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			if strings.Contains(out.String(), "OPSMINI_CONPTY_OK") {
				return // success
			}
		}
		if rerr != nil {
			break
		}
	}
	t.Fatalf("marker not echoed; tail=%q", tail(out.String(), 500))
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
