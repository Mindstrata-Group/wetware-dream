package httpapi

import (
	"net"
	"net/http"

	authapi "mindstrata-stage1/api/internal/httpapi/auth"
)

// clientIP returns the client's real IP. If a trusted proxy list is set and
// RemoteAddr is IN it, we trust X-Forwarded-For / X-Real-IP. Otherwise the
// headers are ignored (S-NEW-4: protection against XFF spoofing with a misconfigured proxy).
func clientIP(r *http.Request) string {
	return clientIPWithTrust(r, nil)
}

func clientIPWithTrust(r *http.Request, trustedProxies []*net.IPNet) string {
	return authapi.ClientIPWithTrust(r, trustedProxies)
}

// parseTrustedProxyCIDRs parses the IP/CIDR list from config into []*net.IPNet.
// An IP without a mask -> /32 (or /128 for IPv6).
func parseTrustedProxyCIDRs(raw []string) []*net.IPNet {
	return authapi.ParseTrustedProxyCIDRs(raw)
}

func requestIPHash(r *http.Request) string {
	return tokenHash(clientIP(r))
}

// K-NEW4-2: trustedClientIP is the IP checked against the trusted proxy chain (production).
// Legacy clientIP() is kept only for dev/tests where TrustedProxyIPs is empty.
func (h Handler) trustedClientIP(r *http.Request) string {
	return clientIPWithTrust(r, parseTrustedProxyCIDRs(h.TrustedProxyIPs))
}

// trustedRequestIPHash for use in production endpoints (rate limiter,
// audit log, cookie consent: places where spoofed XFF must NOT be trusted).
func (h Handler) trustedRequestIPHash(r *http.Request) string {
	return tokenHash(h.trustedClientIP(r))
}

func isHTTPSRequest(r *http.Request) bool {
	return authapi.IsHTTPSRequest(r)
}
