//go:build windows

package sys

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// FileID returns a stable identity for a file that changes when the file is
// replaced. NTFS has no inode exposed through os.FileInfo, so the creation
// timestamp stands in for it: overwriting a file in place keeps it, replacing
// the file (log rotation) produces a new one — the same distinction inode
// numbers provide on Unix.
func FileID(fi os.FileInfo) uint64 {
	if fi == nil {
		return 0
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return uint64(d.CreationTime.HighDateTime)<<32 | uint64(d.CreationTime.LowDateTime)
	}
	return 0
}

// Owner returns the owning account of a file. Windows owns files through ACLs
// rather than uid/gid, which os.FileInfo does not expose; resolving the owner
// needs a security-descriptor query that no current caller justifies, so this
// is empty until something needs it.
func Owner(os.FileInfo) string { return "" }

// DiskUsage reports the used and total bytes of the volume holding path.
// GetDiskFreeSpaceExW reports free space as seen by the calling user, which is
// the Windows counterpart of Bavail.
func DiskUsage(path string) (used, total uint64, ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var freeToCaller, totalBytes, totalFree uint64
	r, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r == 0 {
		return 0, 0, false
	}
	return totalBytes - totalFree, totalBytes, true
}

// Kill terminates a process with TerminateProcess.
//
// Windows has no POSIX signals, so "polite" and "force" are the same call:
// there is no SIGTERM to ask a process to exit cleanly. Callers that need a
// graceful stop have to use the Service Control Manager or a window message.
func Kill(pid int, force bool) error {
	const processTerminate = 0x0001
	h, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return err
		}
		return ErrNoSuchProcess
	}
	defer syscall.CloseHandle(h)
	if err := syscall.TerminateProcess(h, 1); err != nil {
		return err
	}
	return nil
}

// StopProcess has no Windows equivalent. Suspending a process needs a handle to
// every thread (NtSuspendProcess is undocumented); there is no by-pid signal, so
// freeze/resume stay Linux-only.
func StopProcess(int) error { return ErrUnsupported }

// ResumeProcess has no Windows equivalent: there is no SIGSTOP/SIGCONT, and a
// suspended thread cannot be resumed by pid alone. Freeze/resume stay Linux-only.
func ResumeProcess(int) error { return ErrUnsupported }
