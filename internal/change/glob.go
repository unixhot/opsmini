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
	// 反斜杠只在 Windows 上是分隔符：Unix 上它是合法的文件名字符，也是
	// path.Match 里的转义符，无条件替换会改变 Unix 的匹配语义。
	return matchGlobFor(pattern, path, filepath.Separator == '\\')
}

// matchGlobFor is matchGlob with the separator convention passed in.
//
// 分成显式参数是为了让 Windows 分支能在 Linux/macOS 的 CI 上被测到：
// filepath.ToSlash 在 Separator=='/' 时直接返回原值（见 Go 源码
// internal/filepathlite/path.go），所以「归一化」这件事本身只在 Windows 编译时
// 存在，用它写死的测试在别的平台必然失败。
func matchGlobFor(pattern, path string, windows bool) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if windows {
		// "C:\a\b" -> "C:/a/b"。不归一化的话整串会被当成一个 segment，
		// "C:/proj/**" 永不命中，而 "**/*.tmp" 又会因为 "*" 能吞掉反斜杠
		// 而"碰巧"命中 —— 两种结果都不对。
		pattern = strings.ReplaceAll(pattern, `\`, "/")
		path = strings.ReplaceAll(path, `\`, "/")
	}
	// 两个平台都按 "/" 分段匹配。
	//
	// 不能用 filepath.Match：Windows 上分隔符是 "\\"，"/" 只是一个普通字符，
	// 于是 "*" 会跨目录匹配，"/var/log/*.log" 会命中 "/var/log/nginx/error.log"。
	// 面板里的规则和路径都是 POSIX 风格，用 path.Match 语义才一致。
	return matchSegments(splitPath(pattern), splitPath(path))
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
