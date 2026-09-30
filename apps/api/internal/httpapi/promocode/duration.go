package promocode

import (
	"strconv"
	"strings"
	"time"
)

func ParseDays(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if v, err := strconv.Atoi(raw); err == nil {
		return v
	}

	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			continue
		}
		if digits.Len() > 0 {
			break
		}
	}
	if digits.Len() == 0 {
		return 0
	}
	v, _ := strconv.Atoi(digits.String())
	return v
}

func ApplyDuration(base time.Time, raw string, fallbackDays int) time.Time {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return base.AddDate(0, 0, fallbackDays)
	}

	years, months, days := 0, 0, 0
	fields := strings.Fields(raw)
	for i := 0; i+1 < len(fields); i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			continue
		}
		unit := fields[i+1]
		switch {
		case strings.HasPrefix(unit, "year"):
			years += n
		case strings.HasPrefix(unit, "mon"):
			months += n
		case strings.HasPrefix(unit, "week"):
			days += n * 7
		case strings.HasPrefix(unit, "day"):
			days += n
		}
	}
	if years == 0 && months == 0 && days == 0 {
		return base.AddDate(0, 0, fallbackDays)
	}
	return base.AddDate(years, months, days)
}
