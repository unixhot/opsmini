//go:build windows

package service

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultFileRoot returns the traversal boundary for file management.
//
// An empty string means "no boundary": the Windows panel manages whole volumes,
// which is the same practical scope the Linux build gets from a "/" root. There
// is no single root to anchor to — C:\ and D:\ are siblings, not children.
func defaultFileRoot() string { return "" }

// normalizeFileRoot maps a POSIX-style root onto this platform's model. The
// router passes "/", which on Windows means "the whole machine" rather than a
// path: there is no single directory containing C:// and D://.
func normalizeFileRoot(root string) string {
	if root == "" || root == "/" || root == `\` {
		return ""
	}
	return root
}

// defaultFileBrowsePath is where the file manager opens when no path is given.
func defaultFileBrowsePath() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d + string(filepath.Separator)
	}
	return `C:\`
}

// fileRoots lists the places the file manager lets the operator jump to. On
// Windows that is the set of mounted drives, because "/" has no meaning.
func fileRoots() []string {
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		p := string(c) + ":\\"
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, defaultFileBrowsePath())
	}
	return out
}

// withinFileRoot reports whether p is inside root. With an empty root only
// absolute paths are accepted, which keeps "C:\..." working and rejects the
// relative forms a traversal attempt would use.
func withinFileRoot(root, p string) bool {
	if root == "" {
		return filepath.IsAbs(p)
	}
	rel, err := filepath.Rel(filepath.Clean(root), p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
