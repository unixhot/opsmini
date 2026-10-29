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
