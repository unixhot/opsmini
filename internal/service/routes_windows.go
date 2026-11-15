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
		State             string `json:"State"`
	}
	// 接口状态单独取一次再按索引查表：Get-NetRoute 不带连接状态，而路由表里
	// 保留着已断开接口的路由，flags 恒为 UP 就是把断网说成通网。
	// 一次取全表，避免每条路由再往返一次 CIM 调用。
	const q = `$st = @{};` +
		` Get-NetIPInterface -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object { $st[[int]$_.InterfaceIndex] = [string]$_.ConnectionState };` +
		` Get-NetRoute -AddressFamily IPv4 | ForEach-Object {` +
		` [pscustomobject]@{ DestinationPrefix=$_.DestinationPrefix; NextHop=$_.NextHop; InterfaceAlias=$_.InterfaceAlias; RouteMetric=$_.RouteMetric; State=$st[[int]$_.InterfaceIndex] } }`
	var rows []row
	if err := psJSON(q, &rows); err != nil {
		return nil, err
	}

	out := make([]RouteEntry, 0, len(rows))
	for _, r := range rows {
		dest, mask := splitPrefix(r.DestinationPrefix)
		flags := 1 // up
		// 状态拿不到（空串）时保持乐观：宁可多显示一条可用路由，也不要因为
		// 查询失败把整张表说成 down。
		if r.State != "" && !strings.EqualFold(r.State, "Connected") {
			flags = 0
		}
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
