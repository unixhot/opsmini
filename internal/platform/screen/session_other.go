//go:build !windows

package screen

// SessionLimited is always false off Windows: X11 / Quartz sessions are not
// isolated the way Windows services are.
func SessionLimited() (bool, string) { return false, "" }
