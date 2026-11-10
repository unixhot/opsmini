package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A directory containing a unix socket must still be downloadable. /tmp on a
// stock Alibaba Cloud host has aliyun_assist_service.sock; opening it returns
// ENXIO and used to fail the whole archive with a 500.
func TestCompressDirSkipsSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// fifo 比 socket 更狠：os.Open 会一直阻塞等写端，旧的实现会把整个
	// 下载请求挂死。用 fifo 来守「非常规文件必须跳过」这条。
	pipePath := filepath.Join(dir, "pipe.fifo")
	if err := syscall.Mkfifo(pipePath, 0o644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService("/")
	done := make(chan struct{})
	var tmp, name string
	var err error
	go func() { tmp, name, err = svc.CompressDir(dir); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CompressDir blocked on a FIFO — special files must be skipped, not opened")
	}
	if err != nil {
		t.Fatalf("CompressDir failed on dir with special file: %v", err)
	}
	defer os.Remove(tmp)
	if name == "" {
		t.Fatal("empty download name")
	}
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("bad zip: %v", err)
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	// 归档内的条目带顶层目录名（解压后得到 base/ 而不是一堆散文件）
	top := filepath.Base(dir)
	if !found[filepath.Join(top, "a.txt")] || !found[filepath.Join(top, "sub", "b.txt")] {
		t.Fatalf("regular files missing from archive: %v", found)
	}
	if found[filepath.Join(top, "pipe.fifo")] {
		t.Fatalf("fifo should be skipped, got archive entry")
	}
}

// 归档临时文件如果就落在被遍历的目录里，实现必须跳过它自己，否则 Walk
// 会边读边写那个还在增长的 zip（实测 13GB 且 5 秒涨 500MB）。
func TestCompressDirSkipsItsOwnArchive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewFileService("/")
	svc.SetTmpDir(dir) // 故意把归档放进被打包目录，复现 /tmp 场景

	done := make(chan struct{})
	var tmp, name string
	var err error
	go func() { tmp, name, err = svc.CompressDir(dir); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("CompressDir did not finish — it is packing its own archive")
	}
	if err != nil {
		t.Fatalf("CompressDir: %v", err)
	}
	defer os.Remove(tmp)
	if name == "" {
		t.Fatal("empty name")
	}
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("bad zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.Ext(f.Name) == ".zip" {
			t.Fatalf("archive contains an archive entry: %s", f.Name)
		}
	}
}

// 超过上限必须明确报错，而不是闷头把磁盘写满。
func TestCompressDirRespectsSizeCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewFileService("/")
	svc.maxCompress = 1024
	if _, _, err := svc.CompressDir(dir); err == nil {
		t.Fatal("expected size cap error, got nil")
	} else if !strings.Contains(err.Error(), "目录超过") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 下载根目录不能因为 /proc 里的文件读不了就整体 500。
//
// /proc/1/attr/apparmor/exec 能 stat、能 open，读的时候返回 EINVAL；
// /proc/kcore 显示成 128TB。打包时只要有一个这样的文件，整包就失败——
// 而这些内容本来也不该进归档。
func TestCompressDirSkipsPseudoFilesystems(t *testing.T) {
	// 路径判定与平台无关，先验证。
	if packSkipPseudo("/proc/1/attr/apparmor/exec") != true {
		t.Fatal("/proc 子路径应被视为伪文件系统")
	}
	if packSkipPseudo("/procfs-notes") != false {
		t.Fatal("路径边界判断错误：/procfs-notes 不是 /proc")
	}
	if packSkipPseudo("/etc/passwd") != false {
		t.Fatal("/etc/passwd 被误判")
	}
	if _, err := os.Stat("/proc"); err != nil {
		t.Skip("本机没有 /proc，跳过真实打包")
	}
	svc := NewFileService("/")
	done := make(chan struct{})
	var tmp, name string
	var err error
	go func() { tmp, name, err = svc.CompressDir("/"); close(done) }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		t.Fatal("打包根目录超时")
	}
	if err != nil {
		t.Fatalf("打包根目录失败: %v", err)
	}
	defer os.Remove(tmp)
	if name == "" {
		t.Fatal("下载名为空")
	}
	// 2GiB 上限下打包 / 必然超限，报错也算正常；但如果报的是「读 /proc 失败」
	// 那就还是老问题。
	if strings.Contains(err.Error(), "/proc") {
		t.Fatalf("仍在读取 /proc: %v", err)
	}
}
