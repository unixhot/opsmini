//go:build windows

package service

import (
	"strconv"
	"strings"
)

// listRoutes reads the IPv4 routing table through Get-NetRoute.
//
// Reading /proc/net/route (the Unix implementation) made this endpoint return
// 500 on Windows. `route print` would need parsing of a localised table header;
// Get-NetRoute returns object properties whose names are not translated.
func listRoutes() ([]RouteEntry, error) {
	type row struct {
		DestinationPrefix string `json:"DestinationPrefix"`
		NextHop           string `json:"NextHop"`
		InterfaceAlias    string `json:"InterfaceAlias"`
		RouteMetric       int    `json:"RouteMetric"`
	}
	var rows []row
	const q = `Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceAlias,RouteMetric`
	if err := psJSON(q, &rows); err != nil {
		return nil, err
	}

	out := make([]RouteEntry, 0, len(rows))
	for _, r := range rows {
		dest, mask := splitPrefix(r.DestinationPrefix)
		flags := 1 // up
		if r.NextHop != "" && r.NextHop != "0.0.0.0" {
			flags |= 2 // gateway
		}
		out = append(out, RouteEntry{
			Destination: dest,
			Gateway:     r.NextHop,
			Genmask:     mask,
			Flags:       flags,
			Iface:       r.InterfaceAlias,
			Metric:      r.RouteMetric,
		})
	}
	return out, nil
}

// splitPrefix turns "192.168.1.0/24" into ("192.168.1.0", "255.255.255.0").
func splitPrefix(prefix string) (dest, mask string) {
	parts := strings.SplitN(prefix, "/", 2)
	dest = parts[0]
	if len(parts) < 2 {
		return dest, ""
	}
	bits, err := strconv.Atoi(parts[1])
	if err != nil || bits < 0 || bits > 32 {
		return dest, ""
	}
	var v uint32
	if bits > 0 {
		v = ^uint32(0) << (32 - bits)
	}
	return dest, strconv.Itoa(int(byte(v>>24))) + "." +
		strconv.Itoa(int(byte(v>>16))) + "." +
		strconv.Itoa(int(byte(v>>8))) + "." +
		strconv.Itoa(int(byte(v)))
}
