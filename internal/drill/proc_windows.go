//go:build windows

package drill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opsmini/opsmini/internal/platform/sys"
)

// ErrCPUDrillUnsupported 表示当前平台无法制造持续的 CPU 饱和。
//
// 演练的前提是「起的负载能可靠收回去」。Windows 上没有 sh/while 这种
// 轻量空转，也没有 systemd-run 这样能一次性收掉整组的机制：用 cmd 起
// 循环进程，杀掉父进程后子进程可能继续跑，而 CPU 占用本身就是它唯一的
// 症状——残留一个空转进程比不做演练更糟。所以这里明确拒绝，而不是
// 假装成功：界面会显示「该演练在当前平台不可用」，这比一条绿的假报告有用。
var ErrCPUDrillUnsupported = errors.New("Windows 上暂不支持 CPU 打满演练（无法保证可靠回收负载进程）")

func spawnBusyLoop() (int, error) {
	return 0, ErrCPUDrillUnsupported
}

// killProcGroup terminates a single process. Windows has no process groups in
// the Unix sense; the CPU drill does not run here, so this is only a safety net.
func killProcGroup(pid int) {
	_ = sys.Kill(pid, true)
}

// allocSpace reserves size bytes at path.
//
// fsutil 不会真的写数据，和 fallocate 一样只占簇，因此不会拖慢磁盘。
// 参数顺序是「文件 大小」，文件必须不存在，否则直接报错——正好符合演练
// 每次都要新建填充文件的用法。
func allocSpace(ctx context.Context, path string, size int64) error {
	out, err := exec.CommandContext(ctx, "fsutil", "file", "createnew", path,
		strconv.FormatInt(size, 10)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------- 演练用到的路径（Windows）----------
//
// 没有 /var/log、/tmp、/var/tmp：都落在临时目录下。日志暴涨演练仍然要写进
// logwatch 正在监控的目录才有意义，Windows 上的监控目录由日志采集配置决定，
// 默认取 TEMP 也是唯一能保证可写的位置。

func drillLogPath() string  { return filepath.Join(os.TempDir(), "opsmini-drill-flood.log") }
func drillShellDir() string { return filepath.Join(os.TempDir(), "opsmini-drill-shell") }
func drillDiskPath() string { return filepath.Join(os.TempDir(), "opsmini-drill-fill.bin") }
