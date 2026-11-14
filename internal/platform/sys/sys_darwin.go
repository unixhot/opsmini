//go:build darwin

package sys

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// FileID returns the inode number, which changes when a file is replaced by a
// new one at the same path — the signal log-rotation detection relies on.
func FileID(fi os.FileInfo) uint64 {
	if fi == nil {
		return 0
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// Owner returns "uid:gid" of the file, as recorded in quarantine items.
func Owner(fi os.FileInfo) string {
	if fi == nil {
		return ""
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", st.Uid, st.Gid)
	}
	return ""
}

// DiskUsage reports the used and total bytes of the filesystem holding path.
// Free space counts Bavail (available to unprivileged users), matching df(1).
func DiskUsage(path string) (used, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	bsize := uint64(st.Bsize)
	total = st.Blocks * bsize
	free := st.Bavail * bsize
	if free > total {
		free = total
	}
	return total - free, total, true
}

// Kill terminates a process. force selects SIGKILL over the polite SIGTERM.
func Kill(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return ErrNoSuchProcess
		}
		return err
	}
	return nil
}

// StopProcess sends SIGSTOP, suspending the process while preserving its state.
func StopProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return ErrNoSuchProcess
		}
		return err
	}
	return nil
}

// ResumeProcess sends SIGCONT to a process previously stopped with SIGSTOP.
func ResumeProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return ErrNoSuchProcess
		}
		return err
	}
	return nil
}
