package chat

import "strings"

func NormalizeResponseMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "live":
		return "live"
	default:
		return "test"
	}
}
