//go:build windows

package sentinel

import (
	"github.com/shirou/gopsutil/v4/process"
)

// Windows 上没有 /proc，进程信息改走 gopsutil（底层是 NtQuerySystemInformation
// / WMI）。字段语义对齐 Unix 版本，调用方不需要改。

// procCmdline returns the command line of a pid (space separated).
func procCmdline(pid int) string {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return ""
	}
	s, err := p.Cmdline()
	if err != nil {
		return ""
	}
	return s
}

// procComm returns the short process name.
func procComm(pid int) string {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return ""
	}
	name, err := p.Name()
	if err != nil {
		return ""
	}
	return name
}

// procPPID returns the parent pid.
func procPPID(pid int) int {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0
	}
	ppid, err := p.Ppid()
	if err != nil {
		return 0
	}
	return int(ppid)
}

// procUID has no Windows equivalent: the platform identifies security principals
// by SID, and the collectors' uid-based matching (web server accounts) does not
// apply. -1 means "unknown", which callers already treat as "no match".
func procUID(int) int { return -1 }

// userUID cannot be resolved without enumerating local accounts; the callers only
// use it for Unix web-server accounts, which do not exist here.
func userUID(string) int { return -1 }

// holderProcess finds the process holding a file open. Windows has no /proc/<pid>/fd;
// doing this properly needs handle enumeration (NtQuerySystemInformation with
// SystemHandleInformation) or Sysinternals handle.exe. logwatch — the only caller —
// is disabled on Windows (see Capabilities), so this stays unimplemented rather
// than shipping a half-working scan that says "nobody holds this file".
func holderProcess(string) (int, string) { return 0, "" }
