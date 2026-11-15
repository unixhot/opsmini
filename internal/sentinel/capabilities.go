package sentinel

// Capability 说明某一类感知能力在当前平台上是否真的在工作。
//
// 为什么要暴露这个：Windows 上没有 /var/log/auth.log，也没有 Linux 那种
// syslog；日志与登录两路采集器拿不到任何信号。界面上的「0 事件」会被读成
// 「一切正常」，实际是「根本没人看」——假阴性比误报危险得多，因为它让人
// 放心。把这些能力显式标出来，用户才知道该用什么、该忽略什么。
type Capability struct {
	// Key 是稳定的机器标识（log / auth / metric / process / network / fim），
	// 前端用它查本地化名称，不要用 Label 做判断。
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	// Reason 只在不可用时给出，说明为什么、以及能不能补救。
	Reason string `json:"reason,omitempty"`
}
