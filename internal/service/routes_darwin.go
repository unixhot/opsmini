//go:build darwin

package service

import (
	"os/exec"
	"strings"
)

// listRoutes parses `netstat -rn` on macOS.
//
// The previous implementation read /proc/net/route unconditionally, which does
// not exist on macOS either — the routes endpoint was returning 500 there too.
// netstat output has no netmask column, so Genmask stays empty and the CIDR is
// carried in Destination as printed (e.g. "192.168.1").
func listRoutes() ([]RouteEntry, error) {
	out, err := exec.Command("netstat", "-rn", "-f", "inet").Output()
	if err != nil {
		return nil, err
	}
	var routes []RouteEntry
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		// Skip the two header rows.
		if f[0] == "Destination" || strings.HasPrefix(f[0], "Internet") {
			continue
		}
		dest := f[0]
		if dest == "default" {
			dest = "0.0.0.0"
		}
		flags := 0
		if strings.Contains(f[2], "G") {
			flags |= 2 // gateway
		}
		if strings.Contains(f[2], "U") {
			flags |= 1 // up
		}
		routes = append(routes, RouteEntry{
			Destination: dest,
			Gateway:     f[1],
			Flags:       flags,
			Iface:       f[3],
		})
	}
	return routes, nil
}
