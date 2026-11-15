//go:build windows

package screen

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentPid    = modkernel32.NewProc("GetCurrentProcessId")
	procProcessIdToSessn = modkernel32.NewProc("ProcessIdToSessionId")
)

// SessionLimited reports whether this process cannot see the interactive desktop.
//
// Windows isolates services in Session 0: a panel started as a service renders
// into a session with no visible desktop, so GDI capture returns a black frame
// forever. Reporting that up-front turns "why is the picture black" into a
// message that names the cause and the way out.
func SessionLimited() (bool, string) {
	pid, _, _ := procGetCurrentPid.Call()
	var sid uint32
	ok, _, _ := procProcessIdToSessn.Call(pid, uintptr(unsafe.Pointer(&sid)))
	if ok == 0 {
		return false, ""
	}
	if sid == 0 {
		return true, "面板以服务身份运行在 Session 0，看不到用户桌面。" +
			"请改为在已登录的桌面会话中启动 opsmini，或使用命令行终端。"
	}
	return false, ""
}
