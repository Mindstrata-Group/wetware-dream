package auth

import (
	"net"
	"net/http"
	"strings"
)

// ClientIPWithTrust returns the request client IP, honoring forwarding headers
// only when the direct peer is in trustedProxies. An empty trusted proxy list
// preserves the legacy/dev behavior of trusting X-Forwarded-For/X-Real-IP.
func ClientIPWithTrust(r *http.Request, trustedProxies []*net.IPNet) string {
	directHost := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		directHost = host
	}
	trustXFF := len(trustedProxies) == 0
	if !trustXFF {
		if ip := net.ParseIP(directHost); ip != nil {
			for _, n := range trustedProxies {
				if n.Contains(ip) {
					trustXFF = true
					break
				}
			}
		}
	}
	if trustXFF {
		if v := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); v != "" {
			parts := strings.Split(v, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
	}
	return directHost
}

// ParseTrustedProxyCIDRs parses IP/CIDR strings from config. Bare IPs are
// converted to /32 or /128 networks.
func ParseTrustedProxyCIDRs(raw []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s += "/32"
				} else {
					s += "/128"
				}
			}
		}
		if _, n, err := net.ParseCIDR(s); err == nil && n != nil {
			out = append(out, n)
		}
	}
	return out
}

func IsHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
