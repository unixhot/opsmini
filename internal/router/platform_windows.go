//go:build windows

package router

// unsupportedPrefixes lists API path prefixes whose implementations depend on a
// Linux-only subsystem.
//
// On Windows those handlers would either shell out to commands that do not exist
// or query tables that stay empty for ever, so the panel answers 501 with a
// reason instead of pretending success. The frontend hides the matching menu
// entries when /api/v1/system/capabilities reports them missing.
func unsupportedPrefixes() []string {
	return []string{
		"/api/v1/websites",          // nginx / apache vhost management
		"/api/v1/databases",         // mysqld instance management
		"/api/v1/apps",              // one-click deploy runs bash install scripts
		"/api/v1/crontab",           // system crontab parsing
		// /var/log 的文本日志查看；Windows 的日志在事件日志（.evtx）里，需要
		// EvtQuery 读取，当前没有对应实现，给空列表会让人以为「本机没有日志」。
		"/api/v1/logs",
	}
}

// platformName is reported by the capabilities endpoint.
const platformName = "windows"
