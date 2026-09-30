package main

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// Hygiene rules keep installation-specific data out of a public repository:
// a real server address or a real person's e-mail in a fixture is exactly how
// private data leaks, and it looks harmless at the moment of the commit.

var ipv4Literal = regexp.MustCompile(`(?:^|[^0-9.])((?:25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])(?:\.(?:25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])){3})(?:[^0-9.]|$)`)

// Well-known placeholder addresses that are fine in tests and docs.
var allowedPublicIPs = map[string]bool{
	"1.2.3.4":      true, // the classic "some client" in X-Forwarded-For tests
	"8.8.8.8":      true, // public resolver, used as a well-known external address
	"1.1.1.1":      true,
	"9.9.9.9":      true, // another public resolver
	"140.82.121.3": true, // github.com, used in the SSH host key test
}

// ipAllowed reports whether an IPv4 literal may appear in the repository.
func ipAllowed(s string) bool {
	if allowedPublicIPs[s] {
		return true
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return true // not an address at all
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || addr.IsLinkLocalUnicast() || addr.IsMulticast() {
		return true
	}
	for _, p := range docPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	b := addr.As4()
	return b[0] == 255 || b[0] == 0 // masks and "this network"
}

// RFC 5737 documentation ranges, the RFC 6598 shared range and public
// third-party service networks.
var docPrefixes = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("100.64.0.0/10"),
	// Published webhook source networks of the YooKassa payment provider:
	// the same for every installation, the webhook allowlist is tested on them.
	netip.MustParsePrefix("185.71.76.0/27"),
	netip.MustParsePrefix("185.71.77.0/27"),
	netip.MustParsePrefix("77.75.153.0/25"),
	netip.MustParsePrefix("77.75.154.128/25"),
	netip.MustParsePrefix("77.75.156.11/32"),
	netip.MustParsePrefix("77.75.156.35/32"),
}

var emailLiteral = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,})`)

// Domains reserved for examples (RFC 2606/6761) and service addresses that
// are not a person.
var allowedEmailDomains = []string{
	"example.com", "example.org", "example.net", "example.edu",
	".example", ".test", ".local", ".invalid", ".localhost",
	"users.noreply.github.com", "github.com", "iam.gserviceaccount.com",
}

func emailAllowed(domain string) bool {
	d := strings.ToLower(domain)
	for _, a := range allowedEmailDomains {
		if strings.HasPrefix(a, ".") {
			if strings.HasSuffix(d, a) {
				return true
			}
		} else if d == a || strings.HasSuffix(d, "."+a) {
			return true
		}
	}
	return false
}

// CheckHygiene scans one text file and returns violations as "file:line: why".
// Lines containing the marker "hygiene:allow" are skipped: the marker is
// visible in review, so every exception is a conscious decision.
func CheckHygiene(name, text string) []string {
	var problems []string
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "hygiene:allow") {
			continue
		}
		for _, m := range ipv4Literal.FindAllStringSubmatch(line, -1) {
			if !ipAllowed(m[1]) {
				problems = append(problems, fmt.Sprintf("%s:%d: public IP address %s (use 192.0.2.x or a hostname from config)", name, i+1, m[1]))
			}
		}
		for _, m := range emailLiteral.FindAllStringSubmatch(line, -1) {
			if !emailAllowed(m[1]) {
				problems = append(problems, fmt.Sprintf("%s:%d: e-mail on a real domain %q (use @example.com in fixtures and docs)", name, i+1, m[1]))
			}
		}
	}
	return problems
}
