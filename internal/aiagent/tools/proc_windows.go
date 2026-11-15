//go:build windows

package tools

import "syscall"

// stillActive 是 GetExitCodeProcess 表示「进程还活着」的魔数（STILL_ACTIVE）。
const stillActive = 259

// processQueryLimitedInformation 是 OpenProcess 的访问权位（0x1000）。
// Go 的 syscall 包没有导出它，x/sys/windows 里有，但为了一个常量不值得
// 引入整包——数值来自 Win32 头文件 PROCESS_QUERY_LIMITED_INFORMATION。
const processQueryLimitedInformation = 0x1000

// processAlive reports whether pid still exists.
//
// Windows 没有 signal 0 那种「只探测不投递」的做法：拿一个查询句柄再看退出码。
// 打不开句柄说明进程已经没了、或者没有权限——两种情况都按「不在跑」处理，
// 因为这里唯一的用途是决定要不要提示用户「进程可能忽略了 SIGTERM」。
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
