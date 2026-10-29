package change

import (
	"path/filepath"
	"regexp"
	"strings"
)

// hardBlockedPaths are never read, never stored. This list lives in CODE on
// purpose: a configuration mistake (or a compromised panel) must not be able to
// start harvesting /etc/shadow or private keys. Extra patterns can be added via
// config, but nothing here can be removed.
var hardBlockedPaths = []string{
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/master.passwd",
	"/etc/ssl/private/",
	"/root/.ssh/",
	"/root/.aws/",
	"/root/.kube/",
	"/root/.docker/config.json",
	"/root/.bash_history",
	"/root/.mysql_history",
	"/proc/",
	"/sys/",
	"/dev/",
	"/data/opsmini/", // 自己的数据库，防自激
}

// hardBlockedExts are suffixes that indicate cryptographic material.
var hardBlockedExts = []string{".key", ".pem", ".p12", ".pfx", ".jks", ".keystore"}

// hardBlockedNames are path segments that usually hold credentials.
var hardBlockedNames = []string{".bash_history", ".ssh/", "id_rsa", "id_ed25519", ".env"}

// IsBlocked reports whether a path must be skipped entirely.
//
// extra may contain user-defined patterns (globs); it can only ADD to the
// blocklist, never remove from it.
func IsBlocked(path string, extra []string) bool {
	p := filepath.Clean(path)
	for _, b := range hardBlockedPaths {
		if p == strings.TrimSuffix(b, "/") || strings.HasPrefix(p, b) {
			return true
		}
	}
	lower := strings.ToLower(p)
	for _, ext := range hardBlockedExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	for _, n := range hardBlockedNames {
		if strings.Contains(lower, n) {
			return true
		}
	}
	for _, pat := range extra {
		if matchGlob(pat, p) {
			return true
		}
	}
	return false
}

// secretPatterns redact credentials embedded in configuration lines.
//
// Order matters: PEM blocks are matched first so that a private key spread over
// several lines is collapsed before the line-oriented rules run.
var secretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// PEM / certificate bodies (multi-line)
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`), "-----BEGIN <redacted:key>-----"},
	// key = value  (password/secret/token/api key ...)
	{regexp.MustCompile(`(?i)^(\s*["']?[\w.-]*(?:pass|passwd|password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential|auth)[\w.-]*["']?\s*[=:]\s*)(\S.*)$`), "${1}<redacted>"},
	// scheme://user:password@host
	{regexp.MustCompile(`(\b[a-zA-Z][a-zA-Z0-9+.-]*://[^:@/\s]+):[^@/\s]+@`), "${1}:<redacted>@"},
	// well-known token shapes
	{regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`), "<redacted:aws-key>"},
	{regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{16,}|ghp_[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})`), "<redacted:token>"},
	// mysql/redis style command lines
	{regexp.MustCompile(`(?i)(-p)[^\s]+`), "${1}<redacted>"},
}

// injectionPatterns neutralise text that tries to act as an instruction to the
// LLM. This is defence in depth, not a guarantee: the primary mitigation is that
// change bodies are not handed to the model by default at all.
var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bignore\s+(all\s+)?(the\s+)?(previous|above|prior)\b`),
	regexp.MustCompile(`(?i)\bdisregard\s+(all\s+)?(previous|above|prior)\b`),
	regexp.MustCompile(`(?i)\byou\s+(must|should|are\s+now|will\s+now)\b`),
	regexp.MustCompile(`(?i)\b(system|assistant|user)\s*:\s*`),
	regexp.MustCompile(`(?i)\bexecute\s+(the\s+following|this)\b`),
	regexp.MustCompile(`(?i)\brun\s+(the\s+following|this)\s+command\b`),
	regexp.MustCompile(`忽略(以上|之前|前面|所有)`),
	regexp.MustCompile(`立即执行|马上执行|必须执行`),
	regexp.MustCompile(`你现在是|你的新角色`),
}

// SanitizeLine redacts secrets and neutralises injection attempts in one line.
// The returned flags indicate what happened, for accounting and for surfacing
// "filtered N suspicious fragments" in the UI.
func SanitizeLine(line string) (out string, redacted bool, filtered int) {
	out = line
	for _, p := range secretPatterns {
		if p.re.MatchString(out) {
			out = p.re.ReplaceAllString(out, p.repl)
			redacted = true
		}
	}
	for _, re := range injectionPatterns {
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, "<filtered>")
			filtered++
		}
	}
	return out, redacted, filtered
}

// SanitizeText applies SanitizeLine to every line. Used before diffing and
// before storing any body: the order matters — sanitizing AFTER diffing would
// let the three lines of diff context leak the secret the redaction removed.
func SanitizeText(text string) (string, bool, int) {
	lines := strings.Split(text, "\n")
	redacted := false
	filtered := 0
	for i, l := range lines {
		var r bool
		var f int
		lines[i], r, f = SanitizeLine(l)
		if r {
			redacted = true
		}
		filtered += f
	}
	return strings.Join(lines, "\n"), redacted, filtered
}

// keyNamePattern extracts the "key = value" key of a line, used to build the
// Fields list ("which settings changed") without exposing values.
var keyNamePattern = regexp.MustCompile(`^\s*([A-Za-z_][\w.-]{1,48})\s*[=:]`)

// extractFields returns up to max key names whose VALUE changed between the two
// texts (added, removed, or same key with a different value).
//
// Comparing key sets alone was not enough: the most common config change is
// "same key, different value" (worker_processes 4 -> 1), which would report an
// empty field list exactly when the field name is the most useful clue.
// Names only, never values.
func extractFields(before, after string, max int) []string {
	keysOf := func(text string) map[string]string {
		m := make(map[string]string, 64)
		for _, line := range strings.Split(text, "\n") {
			if mm := keyNamePattern.FindStringSubmatch(line); mm != nil {
				m[mm[1]] = strings.TrimSpace(line)
			}
		}
		return m
	}
	kb, ka := keysOf(before), keysOf(after)
	var out []string
	seen := make(map[string]struct{})
	add := func(k string) {
		if len(out) >= max {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for k, va := range ka {
		vb, ok := kb[k]
		if !ok || vb != va {
			add(k)
		}
	}
	for k := range kb {
		if _, ok := ka[k]; !ok {
			add(k)
		}
	}
	return out
}
