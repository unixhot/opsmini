//go:build windows

package service

import "github.com/opsmini/opsmini/internal/platform/sys"

// diskUsage reports the used and total bytes of the volume holding path.
func diskUsage(path string) (used, total int64, ok bool) {
	u, t, ok := sys.DiskUsage(path)
	if !ok {
		return 0, 0, false
	}
	return int64(u), int64(t), true
}

// isPseudoFS has no Windows counterpart. The Linux implementation exists to skip
// /proc, /sys and cgroup trees, whose reported sizes are addresses rather than
// bytes; Windows exposes no such virtual mounts under a path.
func isPseudoFS(string) bool { return false }
