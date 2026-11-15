//go:build linux

package service

import (
	"os"
	"strconv"
	"strings"
)

// listRoutes parses /proc/net/route: the only kernel interface that exposes the
// IPv4 routing table without shelling out.
func listRoutes() ([]RouteEntry, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var out []RouteEntry
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header and blank lines
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		flags, _ := strconv.ParseInt(fields[3], 16, 64)
		metric, _ := strconv.Atoi(fields[6])
		out = append(out, RouteEntry{
			Destination: hexToIPv4(fields[1]),
			Gateway:     hexToIPv4(fields[2]),
			Genmask:     hexToIPv4(fields[7]),
			Flags:       int(flags),
			Iface:       fields[0],
			Metric:      metric,
		})
	}
	return out, nil
}

// hexToIPv4 converts a little-endian 32-bit hex string from /proc/net/route to
// dotted-quad IP.
func hexToIPv4(hexStr string) string {
	v, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return hexStr
	}
	return strconv.Itoa(int(byte(v))) + "." +
		strconv.Itoa(int(byte(v>>8))) + "." +
		strconv.Itoa(int(byte(v>>16))) + "." +
		strconv.Itoa(int(byte(v>>24)))
}
