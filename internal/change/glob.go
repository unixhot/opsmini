package change

import (
	"path"
	"path/filepath"
	"strings"
)

// matchGlob matches a path against a pattern, with one addition to Go's
// filepath.Match: "**" matches across path separators.
//
// Why: filepath.Match's "*" never crosses "/", and it has no "**" at all. Every
// operator who types "/var/log/**" — the convention from rsync, gitignore and
// every log-rotation tool — would silently match nothing, then conclude the ignore
// feature is broken. Supporting "**" is cheaper than teaching that syntax away.
//
// Semantics: "*" matches within one segment, "**" matches zero or more segments.
func matchGlob(pattern, path string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	// 两个平台都按 "/" 分段匹配。
	//
	// 不能用 filepath.Match：Windows 上分隔符是 "\\"，"/" 只是一个普通字符，
	// 于是 "*" 会跨目录匹配，"/var/log/*.log" 会命中 "/var/log/nginx/error.log"。
	// 面板里的规则和路径都是 POSIX 风格，用 path.Match 语义才一致。
	//
	// 两边都先归一化成 "/" 再分段。调用方传进来的路径是 filepath.Clean 的结果，
	// Windows 上是 "C:\a\b"；不归一化的话整串会被当成一个 segment，于是：
	//   - "C:/proj/**" 永不命中（段数对不上）；
	//   - "**/*.tmp" 反而"碰巧"命中，因为 "*" 能吞掉含反斜杠的整串。
	// 前者让用户的忽略规则完全失效，后者让规则的语义变成另一个东西。
	return matchSegments(splitPath(filepath.ToSlash(pattern)), splitPath(filepath.ToSlash(path)))
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true // "**" at the end swallows the rest, including nothing
			}
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], seg[0])
		if err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}
