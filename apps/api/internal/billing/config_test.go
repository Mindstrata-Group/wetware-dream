package billing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigFromLookupDefaults(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromLookup(func(string) string { return "" })

	require.Equal(t, "temporal-server:7233", cfg.TemporalAddress)
	require.Empty(t, cfg.DatabaseURL)
	require.Empty(t, cfg.YooKassaShopID)
	require.Empty(t, cfg.YooKassaSecretKey)
	require.True(t, cfg.RenewalScheduleEnabled)
	require.Equal(t, defaultRenewalCron, cfg.RenewalScheduleCron)
	require.Equal(t, defaultRenewalTimeZone, cfg.RenewalScheduleTimeZone)
}

func TestConfigFromLookupOverrides(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"TEMPORAL_ADDRESS":                 "temporal.local:7233",
		"DATABASE_URL":                     "postgres://user:pass@db/app",
		"YOOKASSA_SHOP_ID":                 "shop",
		"YOOKASSA_SECRET_KEY":              "secret",
		"BILLING_RENEWAL_SCHEDULE_ENABLED": "false",
		"BILLING_RENEWAL_CRON":             "15 4 * * *",
		"BILLING_RENEWAL_TIMEZONE":         "Europe/Moscow",
	}

	cfg := ConfigFromLookup(func(key string) string { return values[key] })

	require.Equal(t, "temporal.local:7233", cfg.TemporalAddress)
	require.Equal(t, "postgres://user:pass@db/app", cfg.DatabaseURL)
	require.Equal(t, "shop", cfg.YooKassaShopID)
	require.Equal(t, "secret", cfg.YooKassaSecretKey)
	require.False(t, cfg.RenewalScheduleEnabled)
	require.Equal(t, "15 4 * * *", cfg.RenewalScheduleCron)
	require.Equal(t, "Europe/Moscow", cfg.RenewalScheduleTimeZone)
}
