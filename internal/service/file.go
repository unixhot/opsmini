package service

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// FileService handles file management; all paths are restricted within root to prevent directory traversal.
type FileService struct {
	root string
}

// NewFileService creates a FileService. root is the allowed root directory.
func NewFileService(root string) *FileService {
	if root == "" {
		root = "/"
	}
	return &FileService{root: filepath.Clean(root)}
}

// FileEntry is a file/directory entry.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mod_time"` // unix seconds
}

// File operation errors.
var (
	ErrPathEscape = errors.New("path escapes root directory")
	ErrNotExist   = errors.New("file or directory not exist")
	ErrNotEmpty   = errors.New("directory not empty")
)

// resolve normalizes a user-input path and validates that it stays within root.
func (s *FileService) resolve(p string) (string, error) {
	if p == "" {
		p = s.root
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.root, p)
	}
	p = filepath.Clean(p)
	root := filepath.Clean(s.root)
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", ErrPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathEscape
	}
	return p, nil
}

// List lists directory contents (directories first, sorted by name).
func (s *FileService) List(dir string) ([]FileEntry, error) {
	full, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileEntry{
			Name:    e.Name(),
			Path:    filepath.ToSlash(filepath.Join(dir, e.Name())),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// validateName checks that a file/directory name contains no path separators or "..", to prevent traversal.
func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrPathEscape
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return ErrPathEscape
	}
	return nil
}

// MakeDir creates a new directory.
func (s *FileService) MakeDir(dir, name string) error {
	parent, err := s.resolve(dir)
	if err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(parent, name), 0o755)
}

// Rename renames a file/directory.
func (s *FileService) Rename(path, newName string) error {
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
	if err := validateName(newName); err != nil {
		return err
	}
	target := filepath.Join(filepath.Dir(full), newName)
	target, err = s.resolve(target)
	if err != nil {
		return err
	}
	return os.Rename(full, target)
}

// Delete deletes a file or directory recursively.
func (s *FileService) Delete(path string) error {
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
	// 保护：不允许删除 root 目录本身（否则会把整个根目录清空）。
	if filepath.Clean(full) == filepath.Clean(s.root) {
		return errors.New("cannot delete the root directory")
	}
	return os.RemoveAll(full)
}

// SaveUploaded saves uploaded file content to the target path.
func (s *FileService) SaveUploaded(dir, name string, data []byte) error {
	parent, err := s.resolve(dir)
	if err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(parent, name), data, 0o644)
}

// Open opens a file for download, returning its absolute path and name (validates it is a regular file, not a directory).
func (s *FileService) Open(path string) (string, string, error) {
	full, err := s.resolve(path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return "", "", errors.New("cannot download a directory")
	}
	return full, filepath.Base(full), nil
}

// DirUsage reports the recursive size of a directory and the disk usage of its containing filesystem.
type DirUsage struct {
	Size        int64   `json:"size"`         // recursive bytes of the directory
	DiskUsed    int64   `json:"disk_used"`    // filesystem used bytes
	DiskTotal   int64   `json:"disk_total"`   // filesystem total bytes
	DiskPercent float64 `json:"disk_percent"` // 0-100
}

// DirUsage computes the directory's recursive size and its filesystem usage.
func (s *FileService) DirUsage(path string) (*DirUsage, error) {
	full, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("not a directory")
	}
	u := &DirUsage{}
	_ = filepath.Walk(full, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			u.Size += fi.Size()
		}
		return nil
	})
	var st syscall.Statfs_t
	if err := syscall.Statfs(full, &st); err == nil {
		u.DiskTotal = int64(st.Blocks) * int64(st.Bsize)
		u.DiskUsed = int64(st.Blocks-st.Bfree) * int64(st.Bsize)
		if u.DiskTotal > 0 {
			u.DiskPercent = float64(u.DiskUsed) / float64(u.DiskTotal) * 100
		}
	}
	return u, nil
}

// CompressDir zips a directory into a temporary file, returning the temp path and the download filename.
// The caller is responsible for removing the temp file after the download completes.
func (s *FileService) CompressDir(path string) (tempPath, downloadName string, err error) {
	full, err := s.resolve(path)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", errors.New("not a directory")
	}
	base := filepath.Base(full)
	if base == "." || base == "/" || base == "" || base == string(filepath.Separator) {
		base = "archive"
	}
	tmp, err := os.CreateTemp("", "opsmini-"+base+"-*.zip")
	if err != nil {
		return "", "", err
	}
	cleanup := func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}

	zw := zip.NewWriter(tmp)
	root := filepath.Clean(full)
	walkErr := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil // skip the root directory itself
		}
		rel, err := filepath.Rel(filepath.Dir(root), p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if fi.IsDir() {
			hdr.Name += "/"
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, f)
			f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if walkErr != nil {
		_ = zw.Close()
		cleanup()
		return "", "", walkErr
	}
	if err := zw.Close(); err != nil {
		cleanup()
		return "", "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", "", err
	}
	return tmp.Name(), base + ".zip", nil
}
