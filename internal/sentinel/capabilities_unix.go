//go:build !windows

package sentinel

// Capabilities reports which perception sources work on this platform.
// Unix 上六路全通：/var/log 与 auth.log 都在预期位置。
func Capabilities() []Capability {
	return []Capability{
		{Key: "metric", Enabled: true},
		{Key: "process", Enabled: true},
		{Key: "network", Enabled: true},
		{Key: "log", Enabled: true},
		{Key: "auth", Enabled: true},
		{Key: "fim", Enabled: true},
	}
}
