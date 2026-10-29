//go:build darwin

package service

import "syscall"

// diskUsage reports the used and total bytes of the filesystem holding path.
func diskUsage(path string) (used, total int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	total = int64(st.Blocks) * int64(st.Bsize)
	used = int64(st.Blocks-st.Bfree) * int64(st.Bsize)
	return used, total, true
}

// isPseudoFS is Linux-only in practice: macOS has no /proc or /sys, and no reliable
// per-mount flag worth encoding here.
func isPseudoFS(string) bool { return false }
