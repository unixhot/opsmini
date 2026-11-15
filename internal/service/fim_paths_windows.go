//go:build windows

package service

import (
	"os"
	"path/filepath"
)

// defaultFimPaths are the key files monitored by default on Windows.
//
// The Linux defaults (/etc/passwd, /etc/ssh/sshd_config) have no counterpart;
// these are the files whose tampering actually matters here: name resolution,
// the legacy INI that still gets parsed, and the user's PowerShell profile
// (a classic persistence location).
func fimCandidatePaths() []string {
	sysroot := os.Getenv("SystemRoot")
	if sysroot == "" {
		sysroot = `C:\Windows`
	}
	paths := []string{
		filepath.Join(sysroot, `System32\drivers\etc\hosts`),
		filepath.Join(sysroot, `System32\drivers\etc\networks`),
		filepath.Join(sysroot, "win.ini"),
	}
	if up := os.Getenv("USERPROFILE"); up != "" {
		paths = append(paths,
			filepath.Join(up, `Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`),
			filepath.Join(up, `Documents\PowerShell\Microsoft.PowerShell_profile.ps1`),
		)
	}

	// 计划任务：每个任务就是 Tasks 目录下的一个 XML 文件，是持久化最常用的落点
	// 之一。数量可能上百，只取前 80 个，避免 FIM 首轮哈希把启动拖慢。
	paths = append(paths, listFiles(filepath.Join(sysroot, `System32\Tasks`), 80)...)

	// 启动目录：快捷方式与脚本的实际存放位置（所有用户 + 当前用户）。
	if pd := os.Getenv("ProgramData"); pd != "" {
		paths = append(paths, filepath.Join(pd, `Microsoft\Windows\Start Menu\Programs\StartUp`))
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		paths = append(paths, filepath.Join(ad, `Microsoft\Windows\Start Menu\Programs\Startup`))
	}

	// 启动目录本身是目录，FIM 只哈希文件，这里展开成具体文件。
	var files []string
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			files = append(files, listFiles(p, 40)...)
			continue
		}
		files = append(files, p)
	}
	return files
}

// listFiles returns up to max regular files directly under dir (non-recursive).
// 目录不存在或无权限时返回空，调用方照常继续。
func listFiles(dir string, max int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, max)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
		if len(out) >= max {
			break
		}
	}
	return out
}

var defaultFimPaths = fimCandidatePaths()

// criticalFimPaths are key files whose changes are classified as critical.
// hosts is the one that can silently redirect every outbound name lookup.
var criticalFimPaths = map[string]bool{}
