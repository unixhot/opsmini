//go:build windows

package service

import (
	"testing"

	"golang.org/x/sys/windows"
)

// netsh writes the machine's ANSI code page, not UTF-8. The panel has to convert
// it, otherwise the operator gets mojibake exactly when a firewall command failed
// and the message matters most.
func TestDecodeCmdOutputLegacyCodePage(t *testing.T) {
	if got := decodeCmdOutput([]byte("firewall ok")); got != "firewall ok" {
		t.Fatalf("ASCII 原样透传失败：%q", got)
	}
	if windows.GetACP() != 936 {
		t.Skip("只有 GBK 系统才能构造这段用例")
	}
	// "防火墙" 的 GBK 字节
	gbk := []byte{0xb7, 0xc0, 0xbb, 0xf0, 0xc7, 0xbd}
	if got := decodeCmdOutput(gbk); got != "防火墙" {
		t.Fatalf("decodeCmdOutput = %q，期望 防火墙", got)
	}
}
