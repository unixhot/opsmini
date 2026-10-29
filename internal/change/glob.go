package change

import (
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
	// Fast path: no "**" means the standard semantics apply unchanged.
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}
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
		ok, err := filepath.Match(pat[0], seg[0])
		if err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}
