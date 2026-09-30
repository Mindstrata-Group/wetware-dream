package webhook

import "net"

// YooKassaAllowedCIDRs is the YooKassa webhook IP allowlist (May 2026).
var YooKassaAllowedCIDRs = func() []*net.IPNet {
	raw := []string{
		"185.71.76.0/27",
		"185.71.77.0/27",
		"77.75.153.0/25",
		"77.75.154.128/25",
		"77.75.156.11/32",
		"77.75.156.35/32",
		"2a02:5180::/32",
	}
	out := make([]*net.IPNet, 0, len(raw))
	for _, s := range raw {
		_, n, err := net.ParseCIDR(s)
		if err == nil && n != nil {
			out = append(out, n)
		}
	}
	return out
}()

func IsYooKassaIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range YooKassaAllowedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
