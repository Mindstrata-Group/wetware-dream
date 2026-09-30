package anonymizer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigFromLookupDefaults(t *testing.T) {
	t.Parallel()

	cfg := ConfigFromLookup(func(string) string { return "" })

	require.Empty(t, cfg.DatabaseURL)
	require.Equal(t, DefaultTemporalAddress, cfg.TemporalAddress)
	require.Equal(t, 200*1024, cfg.MaxBatchBytes)
	require.Equal(t, 2000, cfg.MaxBatchRows)
	require.Equal(t, 2*time.Second, cfg.SleepBetweenBatches)
	require.Equal(t, 30, cfg.MessageAgeDays)
}

func TestConfigFromLookupOverridesAndInvalidValues(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"DATABASE_URL":          "postgres://user:pass@db/app",
		"TEMPORAL_ADDRESS":      "temporal.local:7233",
		"ANON_MAX_BATCH_BYTES":  "102400",
		"ANON_MAX_BATCH_ROWS":   "500",
		"ANON_SLEEP_MS":         "250",
		"ANON_MESSAGE_AGE_DAYS": "14",
	}
	cfg := ConfigFromLookup(func(key string) string { return values[key] })
	require.Equal(t, "postgres://user:pass@db/app", cfg.DatabaseURL)
	require.Equal(t, "temporal.local:7233", cfg.TemporalAddress)
	require.Equal(t, 102400, cfg.MaxBatchBytes)
	require.Equal(t, 500, cfg.MaxBatchRows)
	require.Equal(t, 250*time.Millisecond, cfg.SleepBetweenBatches)
	require.Equal(t, 14, cfg.MessageAgeDays)
	require.Equal(t, WorkflowConfig{SleepBetweenBatches: 250 * time.Millisecond}, WorkflowConfigFromLookup(func(key string) string { return values[key] }))

	invalid := ConfigFromLookup(func(key string) string {
		switch key {
		case "ANON_MAX_BATCH_BYTES", "ANON_MAX_BATCH_ROWS", "ANON_SLEEP_MS", "ANON_MESSAGE_AGE_DAYS":
			return "-1"
		default:
			return ""
		}
	})
	require.Equal(t, 200*1024, invalid.MaxBatchBytes)
	require.Equal(t, 2000, invalid.MaxBatchRows)
	require.Equal(t, 2*time.Second, invalid.SleepBetweenBatches)
	require.Equal(t, 30, invalid.MessageAgeDays)
	require.Equal(t, WorkflowConfig{SleepBetweenBatches: 2 * time.Second}, WorkflowConfigFromLookup(func(string) string { return "" }))
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("TEMPORAL_ADDRESS", "temporal-env:7233")
	t.Setenv("ANON_MAX_BATCH_BYTES", "327680")
	t.Setenv("ANON_MAX_BATCH_ROWS", "1000")
	t.Setenv("ANON_SLEEP_MS", "123")
	t.Setenv("ANON_MESSAGE_AGE_DAYS", "60")

	cfg := ConfigFromEnv()
	require.Equal(t, "postgres://env", cfg.DatabaseURL)
	require.Equal(t, "temporal-env:7233", cfg.TemporalAddress)
	require.Equal(t, 327680, cfg.MaxBatchBytes)
	require.Equal(t, 1000, cfg.MaxBatchRows)
	require.Equal(t, 123*time.Millisecond, cfg.SleepBetweenBatches)
	require.Equal(t, 60, cfg.MessageAgeDays)
	require.Equal(t, WorkflowConfig{SleepBetweenBatches: 123 * time.Millisecond}, WorkflowConfigFromEnv())
}
