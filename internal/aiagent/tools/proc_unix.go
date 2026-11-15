//go:build !windows

package tools

import "syscall"

// processAlive reports whether pid still exists. Signal 0 performs the
// permission/existence check without delivering anything.
func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
