//go:build !windows

package service

import (
	"os"
	"strings"
)

// listDNSServers reads /etc/resolv.conf (Linux and macOS both ship one).
func listDNSServers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var dns []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			dns = append(dns, f[1])
		}
	}
	return dns
}
