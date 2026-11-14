// Package sys isolates the few operating-system details the panel's business
// logic needs but that are not portable across Go targets.
//
// Four primitives only, each with a per-platform implementation behind a build
// tag:
//
//	FileID         stable file identity, used to detect log rotation
//	Owner          owning account of a file, recorded in quarantine items
//	DiskUsage      used and total bytes of the volume holding a path
//	Kill           terminate a process
//	StopProcess    suspend a process without killing it
//	ResumeProcess  continue a stopped process
//
// The Linux and Darwin implementations keep the previous behaviour exactly. On
// Windows the equivalents are either different in kind (an ACL instead of
// uid/gid, no inode at all) or absent (no SIGSTOP/SIGCONT). Callers must
// therefore read a zero FileID, an empty Owner and ErrUnsupported as "this
// platform cannot answer" — never as a real value.
package sys

import "errors"

var (
	// ErrUnsupported means the platform has no equivalent for the operation.
	// Callers should degrade (skip the action, hide the field) rather than fail.
	ErrUnsupported = errors.New("operation not supported on this platform")

	// ErrNoSuchProcess means the target process has already exited.
	ErrNoSuchProcess = errors.New("process does not exist")
)
