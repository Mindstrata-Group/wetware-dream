// Package dbmaintenance contains scheduled safe database operations.
package dbmaintenance

import "os"

const (
	ModesMarkdownScheduleID  = "mindstrata-normalize-modes-markdown"
	ModesMarkdownWorkflowID  = "mindstrata-normalize-modes-markdown-run"
	NormalizeModesActivity   = "NormalizeModesMarkdown"
	defaultModesMarkdownCron = "0 4 1 * *"
	defaultModesMarkdownTZ   = "Asia/Yekaterinburg"
)

type Config struct {
	ScheduleEnabled  bool
	ScheduleCron     string
	ScheduleTimeZone string
	QueueSuffix      string
}

func ConfigFromEnv() Config {
	return ConfigFromLookup(os.Getenv)
}

func ConfigFromLookup(lookup func(string) string) Config {
	return Config{
		ScheduleEnabled:  boolValue(lookup("MODES_MARKDOWN_NORMALIZE_ENABLED"), true),
		ScheduleCron:     stringValue(lookup("MODES_MARKDOWN_NORMALIZE_CRON"), defaultModesMarkdownCron),
		ScheduleTimeZone: stringValue(lookup("MODES_MARKDOWN_NORMALIZE_TZ"), defaultModesMarkdownTZ),
		QueueSuffix:      lookup("TEMPORAL_QUEUE_SUFFIX"),
	}
}

func boolValue(value string, fallback bool) bool {
	switch value {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return fallback
	}
}

func stringValue(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
