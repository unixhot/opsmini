//go:build !windows

package service

import (
	"errors"
	"os"
)

// Unix 上不存在 Windows 防火墙后端，这里给出明确的错误而不是空实现：
// 一旦真被调到，日志里能看出是后端判断出了问题，而不是规则"删成功了"。

func windowsFirewallRules() []FirewallRule { return nil }

func windowsAllowPort(int, string) error {
	return errors.New("当前系统不是 Windows，没有 Windows 防火墙后端")
}

func windowsDenyPort(int, string) error {
	return errors.New("当前系统不是 Windows，没有 Windows 防火墙后端")
}

func windowsDeleteRule(string) error {
	return errors.New("当前系统不是 Windows，没有 Windows 防火墙后端")
}

func windowsEnableFirewall() error { return errors.New("当前系统不是 Windows") }

func windowsDisableFirewall() error { return errors.New("当前系统不是 Windows") }

// remoteAdminPort keeps SSH open on Unix.
func remoteAdminPort() int { return 22 }

// isPrivileged reports whether the process runs as root.
func isPrivileged() bool { return os.Geteuid() == 0 }
