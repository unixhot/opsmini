//go:build windows

package service

import "strings"

// listDNSServers reads the per-interface DNS servers.
//
// Windows has no /etc/resolv.conf; the panel used to return null here, so the
// monitor page showed an empty DNS row. Get-DnsClientServerAddress is the API
// behind the adapter dialog and needs no locale parsing.
func listDNSServers() []string {
	type row struct {
		ServerAddresses []string `json:"ServerAddresses"`
	}
	var rows []row
	const q = `Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object ServerAddresses`
	if err := psJSON(q, &rows); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range rows {
		for _, ip := range r.ServerAddresses {
			ip = strings.TrimSpace(ip)
			if ip == "" || seen[ip] {
				continue
			}
			seen[ip] = true
			out = append(out, ip)
		}
	}
	return out
}
