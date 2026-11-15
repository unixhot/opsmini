//go:build windows

package service

import (
	"unicode/utf8"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// procGetOEMCP 取控制台 OEM 代码页。x/sys/windows 只导出了 GetACP（ANSI），
// 而控制台程序按 OEM 写输出，两者在非中文系统上并不相同。
var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")

// decodeCmdOutput converts output from a native Windows tool (netsh and friends)
// into UTF-8.
//
// netsh.exe is not PowerShell: it writes bytes in the machine's ANSI code page,
// not UTF-8. Go strings are UTF-8, so on a Chinese Windows every firewall error
// message arrives unreadable — precisely when the operator needs to read it.
// PowerShell-based paths get UTF-8 from [Console]::OutputEncoding instead; this
// covers the paths that call a native exe directly.
func decodeCmdOutput(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	enc := oemEncoding()
	if enc == nil {
		return string(b)
	}
	out, _, err := transform.Bytes(enc.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

// oemEncoding maps the console OEM code page onto a decoder.
//
// 用 OEM 而不是 ANSI（GetACP）：netsh 这类控制台程序写的是 OEM 代码页。中文系统
// 上两者都是 936，看不出差别，但俄语（ANSI 1251 / OEM 866）会解错。
// x/sys/windows 没有导出 GetOEMCP，直接调 kernel32。
//
// 只覆盖本项目会遇到的代码页。其余返回 nil —— 原样透传是诚实的，猜一个代码页
// 只会产出"看起来很像真的"的乱码。
func oemEncoding() encoding.Encoding {
	cp, _, _ := procGetOEMCP.Call()
	switch uint32(cp) {
	case 936:
		return simplifiedchinese.GBK
	case 950:
		return traditionalchinese.Big5
	case 932:
		return japanese.ShiftJIS
	case 949:
		return korean.EUCKR
	case 866:
		return charmap.CodePage866
	case 850:
		return charmap.CodePage850
	case 437:
		return charmap.CodePage437
	default:
		return nil
	}
}
