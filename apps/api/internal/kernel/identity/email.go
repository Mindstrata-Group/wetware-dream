// Package identity holds identity primitives shared by all modules (kernel).
package identity

import "strings"

// NormalizeEmail is the one canonical form of an email address used for
// lookups and uniqueness: trimmed and lower-cased.
func NormalizeEmail(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
