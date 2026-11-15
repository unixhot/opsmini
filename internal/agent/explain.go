package agent

import "strings"

// ExplainConnectError turns a gRPC dial/register failure into a sentence an
// operator can act on. The raw error text is kept for the log; the settings
// page shows this instead, because "error reading server preface: http2: frame
// too large" tells nobody which field to change.
//
// The first case is by far the most common: the panel is pointed at OpsAnt's
// web port (8010) instead of the OpsMini Server gRPC port (4507). Both are on
// the same host, so the address looks plausible and only the protocol differs.
func ExplainConnectError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "frame header looked like an HTTP/1.1 header"),
		strings.Contains(s, "frame too large"):
		return "该端口返回的是 HTTP 响应，不是 gRPC。这里要填 OpsAnt 的 OpsMini Server 地址（默认 4507），不要填 Web 端口 8010。"
	case strings.Contains(s, "invalid token"):
		return "Token 不正确：需要与 OpsAnt 的 OPSMINI_TOKEN 完全一致。"
	case strings.Contains(s, "registration rejected"):
		return "OpsAnt 拒绝了本次注册，请核对 Token 与主机配额。"
	case strings.Contains(s, "connection refused"):
		return "目标端口没有在监听，确认 OpsAnt 已启动、端口已映射到宿主机。"
	case strings.Contains(s, "DeadlineExceeded"), strings.Contains(s, "i/o timeout"):
		return "连接超时：检查网络连通性与防火墙/安全组是否放通该端口。"
	case strings.Contains(s, "no such host"):
		return "域名解析失败，请改用 IP 或检查 DNS。"
	}
	return ""
}
