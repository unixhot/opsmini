//go:build !windows

package service

// decodeCmdOutput is a no-op off Windows: ufw, firewall-cmd and iptables all
// write UTF-8 already.
func decodeCmdOutput(b []byte) string { return string(b) }
