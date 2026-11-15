//go:build windows

package service

import "testing"

// netsh writes the console OEM code page, not UTF-8. The panel has to convert it,
// otherwise the operator gets mojibake exactly when a firewall command failed and
// the message matters most.
func TestDecodeCmdOutputLegacyCodePage(t *testing.T) {
	if got := decodeCmdOutput([]byte("firewall ok")); got != "firewall ok" {
		t.Fatalf("ASCII 原样透传失败：%q", got)
	}
	if cp, _, _ := procGetOEMCP.Call(); uint32(cp) != 936 {
		t.Skipf("只有 GBK 系统才能构造这段用例（当前 OEM 代码页 %d）", uint32(cp))
	}
	// "防火墙" 的 GBK 字节
	gbk := []byte{0xb7, 0xc0, 0xbb, 0xf0, 0xc7, 0xbd}
	if got := decodeCmdOutput(gbk); got != "防火墙" {
		t.Fatalf("decodeCmdOutput = %q，期望 防火墙", got)
	}
}
