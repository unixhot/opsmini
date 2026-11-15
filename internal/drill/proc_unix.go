//go:build !windows

package drill

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// spawnBusyLoop starts a CPU spinner that restore can kill as a whole group.
//
// 用独立进程组（Setpgid）而不是单进程：`sh -c` 以后可能派生出子进程，
// 只杀父进程会留下孤儿继续吃 CPU —— 那种残留没人会发现，直到 CPU 曲线
// 连续一周不对劲。
func spawnBusyLoop() (int, error) {
	cmd := exec.Command("/bin/sh", "-c", "while :; do :; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

// killProcGroup terminates the spinner and anything it spawned.
func killProcGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// allocSpace reserves size bytes at path without writing them.
func allocSpace(ctx context.Context, path string, size int64) error {
	out, err := exec.CommandContext(ctx, "fallocate", "-l", strconv.FormatInt(size, 10), path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------- 演练用到的路径（Unix）----------

func drillLogPath() string  { return "/var/log/opsmini-drill-flood.log" }
func drillShellDir() string { return "/tmp/opsmini-drill-shell" }
func drillDiskPath() string { return "/var/tmp/opsmini-drill-fill.bin" }
