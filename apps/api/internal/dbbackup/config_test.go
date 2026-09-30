package dbbackup

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromLookup(func(string) string { return "" })

	require.False(t, cfg.ScheduleEnabled, "по умолчанию схедула выключена — включает только прод")
	require.Equal(t, "30 4 * * 0", cfg.ScheduleCron)
	require.Equal(t, "Asia/Yekaterinburg", cfg.ScheduleTimeZone)
}

func TestConfigFromEnvValues(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"BACKUP_SCHEDULE_ENABLED": "true",
		"BACKUP_CRON":             "0 5 * * 1",
		"BACKUP_TIMEZONE":         "Europe/Moscow",
	}
	cfg := ConfigFromLookup(func(key string) string { return env[key] })

	require.True(t, cfg.ScheduleEnabled)
	require.Equal(t, "0 5 * * 1", cfg.ScheduleCron)
	require.Equal(t, "Europe/Moscow", cfg.ScheduleTimeZone)
}
