//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultFileRoot returns the traversal boundary for file management.
func defaultFileRoot() string { return "/" }

// normalizeFileRoot keeps the caller's boundary as given.
func normalizeFileRoot(root string) string {
	if root == "" {
		return "/"
	}
	return root
}

// defaultFileBrowsePath is where the file manager opens when no path is given.
// root's home is the conventional starting point for a server panel.
func defaultFileBrowsePath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "/"
}

// fileRoots lists the places the file manager lets the operator jump to.
func fileRoots() []string {
	roots := []string{"/"}
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		roots = append([]string{home}, roots...)
	}
	return roots
}

// withinFileRoot reports whether p is inside root.
func withinFileRoot(root, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
