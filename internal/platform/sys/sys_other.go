//go:build !linux && !darwin && !windows

package sys

import "os"

// FileID is unknowable where os.FileInfo does not expose a stable identity.
func FileID(os.FileInfo) uint64 { return 0 }

// Owner is unknowable where the account model is not POSIX-like.
func Owner(os.FileInfo) string { return "" }

// DiskUsage is unavailable on this platform; callers fall back to sizes only.
func DiskUsage(string) (used, total uint64, ok bool) { return 0, 0, false }

// Kill is unavailable on this platform.
func Kill(int, bool) error { return ErrUnsupported }

// StopProcess is unavailable on this platform.
func StopProcess(int) error { return ErrUnsupported }

// ResumeProcess is unavailable on this platform.
func ResumeProcess(int) error { return ErrUnsupported }
