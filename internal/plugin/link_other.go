//go:build !windows

package plugin

import "os"

// linkCurrentDir points <dir>/current at a release directory with a symlink.
// 这是 Unix 上的常规做法：切换是原子的，也不占额外磁盘。
func linkCurrentDir(rel, cur string) error {
	return os.Symlink(rel, cur)
}
