package plugin

import (
	"os"
	"os/exec"
)

// linkCurrentDir points <dir>/current at a release directory.
//
// POSIX 上是一次 symlink，Windows 上不行：建符号链接需要
// SeCreateSymbolicLinkPrivilege（管理员或开发者模式），普通用户下会失败，
// 表现为"插件装了但 current 不存在"，紧接着启动就报路径不存在。
//
// Windows 上按代价从低到高退让：
//  1. symlink（有权限时最省空间）
//  2. 目录联接 junction（mklink /J，不需要特权）
//  3. 直接复制一份（最坏情况，占双份磁盘，但功能完整）
func linkCurrentDir(rel, cur string) error {
	if err := os.Symlink(rel, cur); err == nil {
		return nil
	}
	if err := exec.Command("cmd", "/c", "mklink", "/J", cur, rel).Run(); err == nil {
		return nil
	}
	return copyTree(rel, cur)
}
