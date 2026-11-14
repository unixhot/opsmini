//go:build !windows

package router

// unsupportedPrefixes is empty off Windows: every module has an implementation.
func unsupportedPrefixes() []string { return nil }

// platformName is reported by the capabilities endpoint.
const platformName = "unix"
