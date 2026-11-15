//go:build windows

package store

// unsupportedAlertMetrics lists built-in alert rules whose metric cannot be
// collected on Windows, so seeding them only produces noise.
//
// 判定依据是「这个指标在 Windows 上有没有数据源」：
//   - 磁盘 inode      NTFS 没有 inode 概念，指标恒为 0
//   - CPU 温度        需要传感器驱动，Windows 上 gopsutil 读不到
//   - 僵尸进程        Windows 没有僵尸进程状态
//   - 只读文件系统    读 /proc/mounts，Windows 没有该文件
//   - 文件系统错误    dmesg 专用
//   - OOM 事件        Linux OOM killer 专有
//   - 硬件错误        EDAC / MCE，Linux 专有
//   - 上下文切换频率  /proc/stat 专有
//
// Docker 相关规则不在此列：Windows 可以装 Docker Desktop，缺的是"是否装了"的
// 判定，那个在 service/docker.go 里按端点是否存在处理。
func unsupportedAlertMetrics() map[string]bool {
	return map[string]bool{
		"磁盘 inode 使用率": true,
		"CPU 温度":       true,
		"僵尸进程数":        true,
		"只读文件系统数":      true,
		"文件系统错误数":      true,
		"OOM 事件数":      true,
		"硬件错误数":        true,
		"上下文切换频率":      true,
	}
}
