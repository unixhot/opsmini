//go:build linux

package service

import "syscall"

// Linux filesystem magic numbers we refuse to count. These are virtual trees: the
// sizes they report are addresses, not bytes on disk (/proc/kcore alone claims
// 128 TB), so adding them turns a 12 GB root into "140 TB".
const (
	magicProc     = 0x9fa0
	magicSysfs    = 0x62656572
	magicDevpts   = 0x1cd1
	magicCgroup   = 0x27e0eb
	magicCgroup2  = 0x63677270
	magicDebugfs  = 0x64626720
	magicSecurity = 0x73636673
	magicTracefs  = 0x74726163
	magicBpf      = 0xcafebabe
	magicNsfs     = 0x6e736673
	magicConfigfs = 0x62656570
	magicSelinux  = 0xf97cff8c
	magicPipefs   = 0x50495045
	magicMqueue   = 0x19800202
)

var pseudoMagic = map[int64]bool{
	magicProc: true, magicSysfs: true, magicDevpts: true,
	magicCgroup: true, magicCgroup2: true, magicDebugfs: true,
	magicSecurity: true, magicTracefs: true, magicBpf: true,
	magicNsfs: true, magicConfigfs: true, magicSelinux: true,
	magicPipefs: true, magicMqueue: true,
}

// diskUsage reports the used and total bytes of the filesystem holding path.
func diskUsage(path string) (used, total int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	total = int64(st.Blocks) * int64(st.Bsize)
	used = int64(st.Blocks-st.Bfree) * int64(st.Bsize)
	return used, total, true
}

// isPseudoFS reports whether path sits on a virtual filesystem. Checking the magic
// catches nested mounts (a container runtime mounting /proc deeper in the tree),
// which a hard-coded path list never would.
func isPseudoFS(path string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		// Unreadable means uninteresting: do not skip a real directory because of it.
		return false
	}
	return pseudoMagic[int64(st.Type)]
}
