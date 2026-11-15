//go:build windows

package service

import (
	"unicode/utf8"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

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
	enc := ansiEncoding()
	if enc == nil {
		return string(b)
	}
	out, _, err := transform.Bytes(enc.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

// ansiEncoding maps the system ANSI code page onto a decoder.
//
// Only the code pages this project actually meets are covered. Anything else is
// returned as nil — passing the bytes through unchanged is honest, whereas
// guessing a code page would produce confident-looking nonsense.
func ansiEncoding() encoding.Encoding {
	switch windows.GetACP() {
	case 936:
		return simplifiedchinese.GBK
	case 950:
		return traditionalchinese.Big5
	case 932:
		return japanese.ShiftJIS
	case 949:
		return korean.EUCKR
	default:
		return nil
	}
}
