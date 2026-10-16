package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
)

// --- argument helpers ---

// str returns a string argument (empty when missing).
func str(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// strDef returns a string argument or a default.
func strDef(args map[string]any, key, def string) string {
	if v := str(args, key); v != "" {
		return v
	}
	return def
}

// intArg returns an integer argument or a default.
func intArg(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

// boolArg returns a boolean argument or a default.
func boolArg(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

// --- schema helpers ---

// schema builds a JSON Schema object definition.
func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// pStr declares a string property.
func pStr(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// pInt declares an integer property.
func pInt(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// pBool declares a boolean property.
func pBool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// pEnum declares an enumerated string property.
func pEnum(desc string, values ...string) map[string]any {
	items := make([]any, 0, len(values))
	for _, v := range values {
		items = append(items, v)
	}
	return map[string]any{"type": "string", "description": desc, "enum": items}
}

// --- result helpers ---

// textResult wraps plain text into a successful result.
func textResult(s string) aiagent.Result {
	return aiagent.Result{OK: true, Summary: s, Detail: s}
}

// jsonResult marshals a value into a result, keeping Summary compact.
func jsonResult(v any, summary string) aiagent.Result {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return aiagent.Result{OK: false, Summary: "序列化失败：" + err.Error()}
	}
	out := string(raw)
	return aiagent.Result{OK: true, Summary: out, Detail: summary}
}

// failed builds a non-fatal tool failure (the loop feeds it back to the model).
func failed(format string, a ...any) aiagent.Result {
	msg := fmt.Sprintf(format, a...)
	return aiagent.Result{OK: false, Summary: msg, Detail: msg}
}

// fmtBytes formats a byte count into a readable unit.
func fmtBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// runCmd runs a read-only helper command and returns its combined output.
func runCmd(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, name, args...).CombinedOutput()
	return string(out), err
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
