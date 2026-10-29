//go:build !linux && !darwin

package service

// diskUsage is unavailable on this platform; callers fall back to sizes only.
func diskUsage(string) (used, total int64, ok bool) { return 0, 0, false }

// isPseudoFS has no meaning where the filesystem table is not POSIX-like.
func isPseudoFS(string) bool { return false }
