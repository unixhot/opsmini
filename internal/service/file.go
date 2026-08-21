package service

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Delete deletes a file/directory (the directory must be empty).
func (s *FileService) Delete(path string) error {
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
	return os.Remove(full)
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
