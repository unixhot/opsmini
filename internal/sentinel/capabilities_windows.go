//go:build windows

package sentinel

// Capabilities reports which perception sources work on this platform.
//
// Windows 上丢的是两路，而且都是「不报错、只是永远没数据」的那种：
//   - 日志：文本日志采集指向 /var/log，Windows 上不存在；真正的内容在事件日志
//     （.evtx）里，要用 EvtQuery 逐条读，当前的 tail 式实现套不上去。
//   - 登录审计：失败登录在安全事件日志（Event ID 4625），同样需要事件日志 API。
//
// 进程、网络、指标、文件完整性照常工作：前三者走 gopsutil，FIM 的默认路径
// 由 fim_paths_windows.go 提供（hosts / win.ini / PowerShell profile）。
func Capabilities() []Capability {
	return []Capability{
		{Key: "metric", Enabled: true},
		{Key: "process", Enabled: true},
		{Key: "network", Enabled: true},
		{Key: "log", Enabled: false,
			Reason: "Windows 的日志在事件日志（.evtx）里，需要 EvtQuery 读取；当前版本仍按文本 tail 实现，因此不会产生日志信号"},
		{Key: "auth", Enabled: false,
			Reason: "失败登录记录在安全事件日志（Event ID 4625），同样依赖事件日志 API；当前版本不采集，不会产生登录异常事件"},
		{Key: "fim", Enabled: true},
	}
}
