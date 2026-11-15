package change

import "testing"

func TestMatchGlobSingleLevel(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		// filepath.Match semantics preserved when "**" is absent.
		{"/var/log/*.log", "/var/log/syslog.log", true},
		{"/var/log/*.log", "/var/log/nginx/error.log", false},
		{"/etc/nginx/*", "/etc/nginx/nginx.conf", true},
		{"/etc/nginx/*", "/etc/nginx/sites/a.conf", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// "**" is what people actually type (rsync, gitignore). Matching it is the whole
// point of this function.
func TestMatchGlobDoubleStar(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/var/log/**", "/var/log/syslog", true},
		{"/var/log/**", "/var/log/nginx/error.log", true},
		{"/var/log/**", "/var/log/a/b/c/d.log", true},
		{"/var/log/**", "/var/logs/syslog", false},
		{"/var/log/**/*.log", "/var/log/a/b/x.log", true},
		{"/var/log/**/*.log", "/var/log/a/b/x.conf", false},
		{"**/nginx.conf", "/etc/nginx/nginx.conf", true},
		{"**/*.swp", "/tmp/deep/dir/file.swp", true},
		{"/etc/**/conf.d/*.conf", "/etc/nginx/conf.d/a.conf", true},
		{"/etc/**/conf.d/*.conf", "/etc/nginx/conf.d/nested/a.conf", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// A malformed rule must not match everything: an ignore that silently swallows the
// whole host is worse than one that does nothing.
func TestMatchGlobMalformedIsFalse(t *testing.T) {
	for _, bad := range []string{"", "   ", "/var/log/["} {
		if matchGlob(bad, "/var/log/syslog") {
			t.Errorf("malformed pattern %q should not match", bad)
		}
	}
}

// Windows paths arrive as filepath.Clean output ("C:\a\b"). With the Windows
// separator convention both sides are normalised to "/" so that a rule written
// with directories works there too; before that fix every such rule matched
// nothing.
//
// 用 matchGlobFor(..., true) 而不是 matchGlob：反斜杠在 Unix 上不是分隔符，
// 只有把约定显式传进来，这条用例才能在 Linux/macOS 的 CI 上跑到 Windows 分支。
func TestMatchGlobWindowsBackslashPaths(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{`C:/proj/**`, `C:\proj\a.log`, true},
		{`C:/proj/**`, `C:\proj\sub\a.log`, true},
		{`C:/proj/**`, `C:\proj2\a.log`, false},
		{`*/cache/*`, `C:\Users\me\cache\a.tmp`, false}, // 三段模式，五段路径，本来就该不匹配
		{`**/cache/*`, `C:\Users\me\cache\a.tmp`, true},
		{`**/cache/*`, `C:\Users\me\cache\sub\a.tmp`, false},
		{`**/*.tmp`, `C:\Users\me\proj\a.tmp`, true},
		{`**/*.tmp`, `C:\Users\me\proj\a.log`, false},
		{`C:\proj\**`, `C:\proj\sub\a.log`, true},
		{`/var/log/**`, `C:\var\log\a.log`, false},
	}
	for _, c := range cases {
		if got := matchGlobFor(c.pattern, c.path, true); got != c.want {
			t.Errorf("matchGlobFor(%q, %q, windows) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// Unix 上反斜杠是普通文件名字符（也是 path.Match 的转义符），不能被当成分隔符；
// 否则一个叫 "a\b.log" 的文件会被误判成两层路径。
func TestMatchGlobUnixKeepsBackslash(t *testing.T) {
	if got := matchGlobFor(`*`, `a\b.log`, false); !got {
		t.Error(`"a\b.log" 应作为单个 segment 被 "*" 命中`)
	}
	if got := matchGlobFor(`*/b.log`, `a\b.log`, false); got {
		t.Error(`Unix 上反斜杠不是分隔符，"*/b.log" 不应命中 "a\b.log"`)
	}
}
