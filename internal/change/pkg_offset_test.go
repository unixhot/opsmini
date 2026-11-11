package change

import "testing"

// 空库首启不能把安装历史当变更重放。
//
// 旧实现：文件 >64KiB 才从末尾前 64KiB 起读，否则从 0 读。刚装好的机器
// dpkg.log 往往只有几十 KB，于是整份历史被当成「刚刚发生」——实测首启一次
// 冒出 65 条包变更。
func TestLogStartOffsetFirstRunReadsNothing(t *testing.T) {
	for _, size := range []int64{0, 1, 1024, 30 << 10, 64 << 10, 200 << 10} {
		if got := logStartOffset(size, false, false); got != size {
			t.Fatalf("首次读取 size=%d 时起点应为文件末尾 %d，得到 %d", size, size, got)
		}
	}
}

// 轮转后新文件从 0 开始：那里面才是真正新发生的事。
func TestLogStartOffsetRotationStartsAtZero(t *testing.T) {
	if got := logStartOffset(4096, true, true); got != 0 {
		t.Fatalf("轮转后应从 0 读，得到 %d", got)
	}
}

// 正常续读保持调用方给的偏移（返回 -1 表示"不覆盖"）。
func TestLogStartOffsetContinuation(t *testing.T) {
	if got := logStartOffset(8192, true, false); got != -1 {
		t.Fatalf("续读不应改变偏移，得到 %d", got)
	}
}
