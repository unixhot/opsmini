package change

import (
	"fmt"
	"strings"
)

// maxDiffLines caps how many lines we are willing to align. Alignment is
// O(n*m); 1500 lines costs roughly 9MB of scratch memory and a few
// milliseconds, which is fine for configuration files. Anything larger is
// reported as a coarse change (counts + truncated body) rather than a diff.
const maxDiffLines = 1500

// maxDiffBytes caps the stored body. A single 100MB "configuration" file must
// not be able to blow up the database.
const maxDiffBytes = 64 * 1024

// MakeDiff produces a unified-style diff between two already-sanitized texts.
//
// The body is intentionally produced from sanitized input: if we diffed first
// and sanitized afterwards, the three lines of context would carry the very
// secret the redaction removed.
func MakeDiff(before, after string) (body string, added, removed int, truncated bool) {
	bl := splitLines(before)
	al := splitLines(after)

	if len(bl) > maxDiffLines || len(al) > maxDiffLines {
		// Coarse mode: no alignment, just counts and a short excerpt.
		added, removed = coarseCounts(bl, al)
		body = fmt.Sprintf("(文件过大，仅统计变更：+%d −%d 行)\n", added, removed)
		return body, added, removed, true
	}

	ops := align(bl, al)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(bl), len(al)))
	for _, op := range ops {
		switch op.kind {
		case opSame:
			b.WriteString(" " + op.text + "\n")
		case opAdd:
			b.WriteString("+" + op.text + "\n")
			added++
		case opDel:
			b.WriteString("-" + op.text + "\n")
			removed++
		}
		if b.Len() > maxDiffBytes {
			truncated = true
			b.WriteString("... (截断)\n")
			return b.String(), added, removed, truncated
		}
	}
	return b.String(), added, removed, truncated
}

type opKind int

const (
	opSame opKind = iota
	opAdd
	opDel
)

type diffOp struct {
	kind opKind
	text string
}

// align runs a classic LCS over lines and emits same/add/del operations.
func align(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// dp[i][j] = LCS length of a[i:], b[j:]
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			ops = append(ops, diffOp{opSame, a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffOp{opDel, a[i]})
			i++
		} else {
			ops = append(ops, diffOp{opAdd, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{opDel, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{opAdd, b[j]})
	}
	return ops
}

// coarseCounts is the fallback for oversized files: compare line multisets.
func coarseCounts(a, b []string) (added, removed int) {
	count := make(map[string]int, len(a))
	for _, l := range a {
		count[l]++
	}
	for _, l := range b {
		if count[l] > 0 {
			count[l]--
		} else {
			added++
		}
	}
	for _, v := range count {
		if v > 0 {
			removed += v
		}
	}
	return added, removed
}

// splitLines splits on "\n" without producing a trailing empty element for a
// file that ends with a newline (that artifact would otherwise show up as a
// spurious diff line on every single change).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// diffStat is a cheap "did it really change" helper: identical content must not
// produce an event (touch, chmod, backup-and-restore all fall here).
func diffStat(before, after string) (added, removed int, changed bool) {
	if before == after {
		return 0, 0, false
	}
	body, a, r, _ := MakeDiff(before, after)
	return a, r, body != "" && (a > 0 || r > 0)
}
