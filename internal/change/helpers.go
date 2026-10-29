package change

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// inodeOf returns the inode number, used to detect log rotation.
func inodeOf(fi os.FileInfo) uint64 {
	if fi == nil {
		return 0
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// shortHash is a stable 16-char digest, enough for change detection.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// runCmd executes an external command with a hard timeout.
//
// Binaries are always addressed by absolute path: a polluted PATH (or a dropped
// binary earlier in PATH) must not be able to hijack a detector.
func runCmd(timeout time.Duration, bin string, args ...string) (string, error) {
	if _, err := os.Stat(bin); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// firstExisting returns the first path that exists.
func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// cleanLines trims trailing spaces, drops tool comments and collapses the
// ipset/iptables counters so that a rule set can be compared without churning
// on packet counters or on the dump timestamp in the header.
func cleanLines(s string) string {
	var out []byte
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			start = i + 1
			line = stripToolComments(stripCounters(line))
			if line == "" {
				continue
			}
			out = append(out, []byte(line)...)
			out = append(out, '\n')
		}
	}
	return string(out)
}
