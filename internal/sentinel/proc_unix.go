//go:build !windows

package sentinel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procCmdline returns the command line of a pid (space separated).
func procCmdline(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(b), "\x00", " ")
	return strings.TrimSpace(s)
}

// procComm returns the short process name.
func procComm(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// procPPID returns the parent pid.
func procPPID(pid int) int {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	// pid (comm) state ppid ... ; comm may contain spaces, so cut at the last ')'.
	s := string(b)
	i := strings.LastIndex(s, ")")
	if i < 0 {
		return 0
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(fields[1])
	return n
}

// procUID returns the real uid of a pid.
func procUID(pid int) int {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return -1
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(ln, "Uid:") {
			f := strings.Fields(ln)
			if len(f) >= 2 {
				n, _ := strconv.Atoi(f[1])
				return n
			}
		}
	}
	return -1
}

// holderProcess finds the process that currently has the file open for writing.
// This is what decides whether a log file may be touched at all.
func holderProcess(path string) (pid int, cmdline string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, ""
	}
	want, _ := filepath.EvalSymlinks(path)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", p)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if target == path || (want != "" && target == want) {
				return p, procCmdline(p)
			}
		}
	}
	return 0, ""
}

// userUID resolves a user name to uid by scanning /etc/passwd.
func userUID(name string) int {
	b, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return -1
	}
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Split(ln, ":")
		if len(f) >= 3 && f[0] == name {
			n, _ := strconv.Atoi(f[2])
			return n
		}
	}
	return -1
}
