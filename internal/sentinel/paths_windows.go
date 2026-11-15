//go:build windows

package sentinel

// defaultLogPaths / defaultWebRoots 在 Windows 上留空。
//
// 原来这里无条件用 /var/log 与 /var/www：启动日志会打印一串在这台机器上根本
// 不存在的路径，logwatch 也会按这些路径做增长基线（永远没有样本）。日志内容
// 实际在事件日志里，需要 EvtQuery 才能读，不属于当前的文本 tail 实现。
func defaultLogPaths() []string { return nil }

func defaultWebRoots() []string { return nil }
