//go:build !windows

package service

// baselineUnixOnly selects which item set BaselineService runs: the Unix set is
// defined inline in baseline.go and baseline_extra.go.
const baselineUnixOnly = true

// platformItems is empty off Windows; the Unix checks live in items() itself.
func platformItems() []checkItem { return nil }
