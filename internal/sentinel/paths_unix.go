//go:build !windows

package sentinel

// defaultLogPaths / defaultWebRoots 是 Unix 上的默认观测范围。
func defaultLogPaths() []string { return []string{"/var/log"} }

func defaultWebRoots() []string {
	return []string{"/var/www", "/var/www/html", "/www/wwwroot", "/data/www"}
}
