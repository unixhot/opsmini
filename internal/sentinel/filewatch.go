package sentinel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Web script extensions that should never appear out of nowhere in a document root.
var scriptExt = map[string]bool{
	".php": true, ".phtml": true, ".php5": true, ".jsp": true, ".jspx": true,
	".aspx": true, ".ashx": true, ".asp": true, ".cgi": true, ".pl": true,
	".py": true, ".sh": true,
}

// webshellPatterns are the content heuristics. Two or more hits (or one strong
// hit) turns a new file into a high-severity signal; a single weak hit stays
// informational so normal uploads do not page anyone.
var webshellPatterns = []struct {
	Needle string
	Strong bool
}{
	{"eval($_POST", true},
	{"eval($_REQUEST", true},
	{"eval($_GET", true},
	{"assert($_POST", true},
	{"base64_decode(", false},
	{"gzinflate(base64", true},
	{"str_rot13(", false},
	{"shell_exec(", false},
	{"passthru(", false},
	{"preg_replace(\"/e", true},
	{"ProcessStartInfo", false},
	{"Runtime.getRuntime().exec", false},
	{"Request.Item[", false},
	{"/dev/tcp/", true},
}

type fileState struct {
	mtime time.Time
	size  int64
}

type fileWatcher struct {
	cfg    Config
	mu     sync.Mutex
	seen   map[string]fileState
	keyMod time.Time
}

func newFileWatcher(cfg Config) *fileWatcher {
	return &fileWatcher{cfg: cfg, seen: map[string]fileState{}}
}

func (w *fileWatcher) Name() string { return "filewatch" }

func (w *fileWatcher) Run(ctx context.Context, out func(Signal)) {
	// First pass builds the inventory without emitting anything.
	w.scan(out, true)
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.scan(out, false)
		}
	}
}

func (w *fileWatcher) scan(out func(Signal), initial bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// authorized_keys changed -> authentication-relevant signal.
	keyPath := "/root/.ssh/authorized_keys"
	if st, err := os.Stat(keyPath); err == nil {
		if !w.keyMod.IsZero() && st.ModTime().After(w.keyMod) && !initial {
			out(newSignal(model.SignalKindFile, "filewatch/keys", keyPath, "auth.anomaly",
				"auth.key_changed", model.SeverityHigh,
				"authorized_keys 被修改："+keyPath).
				withTags("mitre:T1098.004", "scenario:ssh_backdoor").
				withEvidence(map[string]any{"path": keyPath, "mtime": st.ModTime().Format(time.RFC3339)}))
		}
		w.keyMod = st.ModTime()
	}

	count := 0
	for _, root := range w.cfg.WebRoots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			count++
			if count > 20000 {
				return filepath.SkipAll
			}
			if !scriptExt[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			prev, ok := w.seen[path]
			w.seen[path] = fileState{mtime: info.ModTime(), size: info.Size()}
			if initial || (ok && !info.ModTime().After(prev.mtime)) {
				return nil
			}
			strong, hits := scoreScript(path)
			if hits == 0 {
				return nil
			}
			sev := model.SeverityMedium
			fp := "file.new_script"
			if strong || hits >= 2 {
				sev = model.SeverityHigh
				fp = "webshell.file"
			}
			out(newSignal(model.SignalKindFile, "filewatch/webroot", path, "webshell",
				fp, sev,
				"Web 目录出现可疑脚本："+path+"（命中 "+itoa(int64(hits))+" 个特征）").
				withTags("mitre:T1505.003", "scenario:webshell").
				withEvidence(map[string]any{
					"path": path, "size": info.Size(),
					"mtime": info.ModTime().Format(time.RFC3339),
					"strong": strong, "hits": hits,
				}))
			return nil
		})
	}
}

// scoreScript reads the head of a file and counts webshell-ish markers.
func scoreScript(path string) (strong bool, hits int) {
	f, err := os.Open(path)
	if err != nil {
		return false, 0
	}
	defer f.Close()
	buf := make([]byte, 64<<10)
	n, _ := f.Read(buf)
	return scoreScriptBytes(buf[:n])
}

// scoreScriptBytes is the pure heuristic used by scoreScript (exported for tests).
func scoreScriptBytes(buf []byte) (strong bool, hits int) {
	body := string(buf)
	if len(body) == 0 {
		return false, 0
	}
	for _, p := range webshellPatterns {
		if strings.Contains(body, p.Needle) || strings.Contains(strings.ToLower(body), strings.ToLower(p.Needle)) {
			hits++
			if p.Strong {
				strong = true
			}
		}
	}
	return strong, hits
}
