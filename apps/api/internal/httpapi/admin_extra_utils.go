package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultDailyMessageLimit int64 = 50

func normalizedDailyMessageLimit(value *int64) int64 {
	if value == nil || *value <= 0 {
		return defaultDailyMessageLimit
	}
	return *value
}

func protectedTariffGroupName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "участники" || name == "participants" || name == "members" || strings.Contains(name, "участник")
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	out := []int64{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func resolveFirstModeID(firstModeID *int64, modeIDs []int64) (*int64, bool) {
	modeIDs = uniquePositiveIDs(modeIDs)
	if len(modeIDs) == 0 {
		return nil, firstModeID == nil || *firstModeID <= 0
	}
	if firstModeID == nil || *firstModeID <= 0 {
		id := modeIDs[0]
		return &id, true
	}
	for _, modeID := range modeIDs {
		if modeID == *firstModeID {
			id := *firstModeID
			return &id, true
		}
	}
	return nil, false
}

func randomPromoCode() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "MS-" + strings.ToUpper(hex.EncodeToString(b))
}

func parseOptionalAdminTime(raw string) (*time.Time, error) {
	return parseOptionalAdminTimeWithDayBound(raw, false)
}

func parseOptionalAdminEndTime(raw string) (*time.Time, error) {
	return parseOptionalAdminTimeWithDayBound(raw, true)
}

func parseOptionalAdminTimeWithDayBound(raw string, endOfDay bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return &t, nil
	}
	return nil, fmt.Errorf("time must be RFC3339 or YYYY-MM-DD")
}

func promocodeApplyURL(r *http.Request, code string) string {
	base := publicWebBaseURL()
	if base == "" {
		proto := "https"
		if r.TLS == nil && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.")) {
			proto = "http"
		}
		base = fmt.Sprintf("%s://%s", proto, r.Host)
	}
	return fmt.Sprintf("%s/access?promo=%s", strings.TrimRight(base, "/"), code)
}

type nilResponseWriter struct{}

func (nilResponseWriter) Header() http.Header        { return http.Header{} }
func (nilResponseWriter) Write([]byte) (int, error)  { return 0, nil }
func (nilResponseWriter) WriteHeader(statusCode int) {}
