package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A pattern without wildcards is returned as-is even when the file is missing:
// that is how "monitor this, tell me if it appears" works. Dropping it would
// make a deleted-then-restored file permanently invisible.
func TestExpandFimPatternLiteral(t *testing.T) {
	got := expandFimPattern("/etc/does-not-exist-xyz")
	if len(got) != 1 || got[0] != "/etc/does-not-exist-xyz" {
		t.Fatalf("got %v", got)
	}
}

func TestExpandFimPatternGlob(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.conf", "b.conf", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := expandFimPattern(filepath.Join(dir, "*.conf"))
	if len(got) != 2 {
		t.Fatalf("glob matched %d files, want 2: %v", len(got), got)
	}
}

// ** is not part of path/filepath syntax, but operators write it anyway; it must
// mean "any depth" rather than silently matching nothing.
func TestExpandFimPatternDoubleStar(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := []string{
		filepath.Join(root, "top.conf"),
		filepath.Join(nested, "deep.conf"),
		filepath.Join(nested, "skip.txt"),
	}
	for _, p := range targets {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := expandFimPattern(filepath.Join(root, "**", "*.conf"))
	if len(got) != 2 {
		t.Fatalf("** matched %d files, want 2: %v", len(got), got)
	}
	for _, p := range got {
		if filepath.Ext(p) != ".conf" {
			t.Fatalf("suffix filter ignored: %s", p)
		}
	}
}

// Expansion is capped so a broad pattern cannot turn every check round into a
// full-tree hash.
func TestExpandFimPatternCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < fimMaxExpand+10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.conf", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := expandFimPattern(filepath.Join(dir, "*.conf"))
	if len(got) > fimMaxExpand {
		t.Fatalf("expansion not capped: %d", len(got))
	}
}

// Without any setting the built-in list applies: a database that was never
// configured must keep the standard coverage.
func TestMonitoredPathsDefaults(t *testing.T) {
	s := &FimService{}
	patterns, critical := s.MonitoredPaths()
	if len(patterns) != len(defaultFimPaths) {
		t.Fatalf("patterns: got %d want %d", len(patterns), len(defaultFimPaths))
	}
	if len(critical) != len(defaultCriticalPaths) {
		t.Fatalf("critical: got %d want %d", len(critical), len(defaultCriticalPaths))
	}
}

// An empty setting is not "monitor nothing": it falls back to the defaults.
func TestMonitoredPathsEmptySettingFallsBack(t *testing.T) {
	s := &FimService{}
	s.SetSettings(func(key string) string {
		if key == "fim_paths" {
			return "  ,  " // blank list
		}
		return ""
	})
	patterns, _ := s.MonitoredPaths()
	if len(patterns) != len(defaultFimPaths) {
		t.Fatalf("blank setting should fall back to defaults, got %v", patterns)
	}
}

func TestMonitoredPathsCustom(t *testing.T) {
	s := &FimService{}
	s.SetSettings(func(key string) string {
		switch key {
		case "fim_paths":
			return "/etc/nginx/*.conf, /opt/app/config.yaml"
		case "fim_critical_paths":
			return "/opt/app/config.yaml"
		}
		return ""
	})
	patterns, critical := s.MonitoredPaths()
	if len(patterns) != 2 || patterns[0] != "/etc/nginx/*.conf" {
		t.Fatalf("patterns: %v", patterns)
	}
	if len(critical) != 1 || critical[0] != "/opt/app/config.yaml" {
		t.Fatalf("critical: %v", critical)
	}
}

// Critical patterns may be wildcards; a map lookup would classify them as
// warning without any visible error.
func TestLevelForWildcardCritical(t *testing.T) {
	s := &FimService{}
	s.SetSettings(func(key string) string {
		switch key {
		case "fim_paths":
			return "/etc/ssh/*_config"
		case "fim_critical_paths":
			return "/etc/ssh/*_config, /etc/passwd"
		}
		return ""
	})
	if got := s.levelFor("/etc/ssh/sshd_config"); got != "critical" {
		t.Fatalf("wildcard critical not matched: %s", got)
	}
	if got := s.levelFor("/etc/passwd"); got != "critical" {
		t.Fatalf("literal critical not matched: %s", got)
	}
	if got := s.levelFor("/etc/hosts"); got != "warning" {
		t.Fatalf("non-critical path classified as %s", got)
	}
}

// Removing a path must drop its baseline, otherwise the list looks right while
// the checker keeps watching the old file.
func TestPatternCoversPruning(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/etc/passwd", "/etc/passwd", true},
		{"/etc/passwd", "/etc/shadow", false},
		{"/etc/ssh/*_config", "/etc/ssh/sshd_config", true},
		{"/etc/ssh/*_config", "/etc/passwd", false},
		{"/etc/nginx/**/*.conf", "/etc/nginx/conf.d/a.conf", true},
		{"/etc/nginx/**/*.conf", "/etc/nginx/nginx.conf", true},
		{"/etc/nginx/**/*.conf", "/etc/apache2/a.conf", false},
		{"/etc/nginx/**", "/etc/nginx/conf.d/a.conf", true},
		{"/etc/nginx/**", "/etc/passwd", false},
	}
	for _, c := range cases {
		if got := patternCovers(c.pattern, c.path); got != c.want {
			t.Fatalf("patternCovers(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// A vanished file is still covered by its literal pattern, so its baseline must
// survive: Check needs it to report "file deleted" instead of staying silent.
func TestPatternCoversKeepsMissingFile(t *testing.T) {
	if !patternCoversAny([]string{"/etc/passwd", "/etc/ssh/*_config"}, "/etc/ssh/sshd_config") {
		t.Fatal("wildcard should still cover a path whose file is gone")
	}
	if patternCoversAny([]string{"/etc/passwd"}, "/etc/crontab") {
		t.Fatal("dropped path must not be covered any more")
	}
}
