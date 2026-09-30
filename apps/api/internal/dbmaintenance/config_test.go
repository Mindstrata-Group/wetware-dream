package dbmaintenance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigFromLookup_Defaults(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromLookup(func(string) string { return "" })

	require.True(t, cfg.ScheduleEnabled)
	require.Equal(t, "0 4 1 * *", cfg.ScheduleCron)
	require.Equal(t, "Asia/Yekaterinburg", cfg.ScheduleTimeZone)
}

func TestConfigFromLookup_Overrides(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"MODES_MARKDOWN_NORMALIZE_ENABLED": "false",
		"MODES_MARKDOWN_NORMALIZE_CRON":    "15 5 2 * *",
		"MODES_MARKDOWN_NORMALIZE_TZ":      "Europe/Moscow",
		"TEMPORAL_QUEUE_SUFFIX":            "-staging",
	}
	cfg := ConfigFromLookup(func(key string) string { return values[key] })

	require.False(t, cfg.ScheduleEnabled)
	require.Equal(t, "15 5 2 * *", cfg.ScheduleCron)
	require.Equal(t, "Europe/Moscow", cfg.ScheduleTimeZone)
	require.Equal(t, "-staging", cfg.QueueSuffix)
}
