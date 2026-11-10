package service

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileService handles file management; all paths are restricted within root to prevent directory traversal.
type FileService struct {
	root string
	// tmpDir is where directory archives are built. It must be able to live
	// outside the tree being packed — see CompressDir.
	tmpDir string
	// maxCompress bounds the total bytes packed into one archive. Without it a
	// download of a directory that keeps growing fills the disk before anyone
	// notices.
	maxCompress int64
}

// DefaultMaxCompress is the per-archive size ceiling (2 GiB).
const DefaultMaxCompress int64 = 2 << 30

// NewFileService creates a FileService. root is the allowed root directory.
func NewFileService(root string) *FileService {
	if root == "" {
		root = "/"
	}
	return &FileService{root: filepath.Clean(root), tmpDir: os.TempDir(), maxCompress: DefaultMaxCompress}
}

// SetTmpDir moves archive scratch files to dir (typically <data>/tmp). The
// directory is created on demand: a missing scratch dir must not turn every
// directory download into a 500.
func (s *FileService) SetTmpDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	s.tmpDir = dir
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
	// Partial is true when the walk hit its entry budget, so Size is a lower bound
	// rather than the exact total. Silently reporting a wrong number would be worse
	// than admitting the cut-off.
	Partial bool `json:"partial"`
}

// maxWalkEntries bounds one directory-size request. /proc, /sys and huge data
// directories can hold millions of entries; without a cap a single request can
// occupy a worker for minutes and a user can trigger it repeatedly.
const maxWalkEntries = 200000

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
	seen := 0
	_ = filepath.Walk(full, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil {
			return nil
		}
		if fi.IsDir() {
			if isPseudoFS(p) {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if seen > maxWalkEntries {
			u.Partial = true
			return filepath.SkipAll
		}
		u.Size += fi.Size()
		return nil
	})
	if used, total, ok := diskUsage(full); ok {
		u.DiskUsed, u.DiskTotal = used, total
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
	tmp, err := os.CreateTemp(s.tmpDir, "opsmini-"+base+"-*.zip")
	if err != nil && s.tmpDir != "" {
		// 配置的临时目录被删或不可写：退回系统临时目录，而不是让下载失败。
		tmp, err = os.CreateTemp("", "opsmini-"+base+"-*.zip")
	}
	if err != nil {
		return "", "", err
	}
	cleanup := func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}
	tmpStat, err := tmp.Stat()
	if err != nil {
		cleanup()
		return "", "", err
	}

	zw := zip.NewWriter(tmp)
	root := filepath.Clean(full)
	var packed int64
	maxCompress := s.maxCompress
	if maxCompress <= 0 {
		maxCompress = DefaultMaxCompress
	}
	walkErr := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil // skip the root directory itself
		}
		// /proc、/sys、/dev 不是真实文件：读它们要么返回 EINVAL（/proc/1/attr/*），
		// 要么是无穷无尽的流（/proc/kcore 显示为 128TB）。下载根目录时只要碰到一个
		// 就读失败，整包 500——而这些内容本来也没有备份或下载的意义。
		if fi.IsDir() && packSkipPseudo(p) {
			return filepath.SkipDir
		}
		if packSkipPseudo(p) {
			return nil
		}
		// 打包 /tmp 时归档文件本身就在被遍历的目录里：不跳过它就会边读边写、
		// 无限增长（实测 13GB 仍在涨，5 秒 +500MB，直接把磁盘写满）。
		if os.SameFile(fi, tmpStat) {
			return nil
		}
		// 同一个目录里可能还有上一次中断留下的归档，一起跳过。
		if !fi.IsDir() && strings.HasPrefix(filepath.Base(p), "opsmini-"+base+"-") &&
			strings.HasSuffix(p, ".zip") {
			return nil
		}
		rel, err := filepath.Rel(filepath.Dir(root), p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// 目录里混进 unix socket / fifo / 设备文件是常态（/tmp 下的
		// aliyun_assist_service.sock、各种 .pid fifo）。它们既不能被打包，
		// 打开还会报 "no such device or address" —— 一个这样的文件不该让
		// 整个目录的下载变成 500。打不开的普通文件同理：跳过而不是失败。
		var src *os.File
		if !fi.IsDir() {
			if !fi.Mode().IsRegular() {
				return nil
			}
			packed += fi.Size()
			if packed > maxCompress {
				return fmt.Errorf("目录超过 %d MB，请选择子目录再下载", maxCompress>>20)
			}
			src, err = os.Open(p)
			if err != nil {
				return nil
			}
			defer src.Close()
			// 能打开不代表能读。挂载点文件（/run/snapd/ns/*.mnt）和 procfs 里的
			// 一些条目 open 成功、第一次 read 就返回 EINVAL；等 io.Copy 里才发现
			// 就已经写进 zip 了，撤不回来，只能整包失败。先探一个字节。
			var probe [1]byte
			if n, perr := src.Read(probe[:]); perr != nil && perr != io.EOF {
				return nil
			} else if _, serr := src.Seek(0, io.SeekStart); serr != nil {
				return nil
			} else if n == 0 && fi.Size() > 0 {
				return nil
			}
		}

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
		if src != nil {
			if _, err := io.Copy(w, src); err != nil {
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

// pseudoFSRoots are kernel pseudo filesystems that must never be walked when
// packing a directory. Matching is on the path boundary so /procfs-notes is not
// mistaken for /proc.
var pseudoFSRoots = []string{"/proc", "/sys", "/dev", "/run/udev"}

// packSkipPseudo reports whether p belongs to a kernel pseudo filesystem that
// must not be walked when packing a directory. Named differently from the
// isPseudoFS in disk_*.go: that one answers "is this mount point a pseudo fs"
// for disk accounting, this one is about what to leave out of an archive.
func packSkipPseudo(p string) bool {
	clean := filepath.Clean(p)
	for _, root := range pseudoFSRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}
