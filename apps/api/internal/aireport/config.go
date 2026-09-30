// Package aireport computes and once a day pushes to Uptime Kuma a summary of
// the AI providers: how many messages each provider served and how many times
// the resilience chain (ai_resilience.go) failed on an attempt or was exhausted
// entirely. The data source is the ai_provider_events table.
package aireport

import "os"

const (
	ScheduleID     = "mindstrata-ai-provider-daily-report"
	WorkflowID     = "mindstrata-ai-provider-daily-report-run"
	ReportActivity = "SendAIProviderDailyReport"

	defaultCron = "0 6 * * *"
	defaultTZ   = "Asia/Yekaterinburg"
)

type Config struct {
	ScheduleEnabled  bool
	ScheduleCron     string
	ScheduleTimeZone string
	QueueSuffix      string
	// EnvLabel tags the message in Kuma ("prod"/"staging"): both
	// environments report to the same push monitor.
	EnvLabel string
}

func ConfigFromEnv() Config {
	return ConfigFromLookup(os.Getenv)
}

func ConfigFromLookup(lookup func(string) string) Config {
	return Config{
		ScheduleEnabled:  boolValue(lookup("AI_REPORT_ENABLED"), true),
		ScheduleCron:     stringValue(lookup("AI_REPORT_CRON"), defaultCron),
		ScheduleTimeZone: stringValue(lookup("AI_REPORT_TZ"), defaultTZ),
		QueueSuffix:      lookup("TEMPORAL_QUEUE_SUFFIX"),
		EnvLabel:         stringValue(lookup("AI_REPORT_ENV_LABEL"), envLabelFromSuffix(lookup("TEMPORAL_QUEUE_SUFFIX"))),
	}
}

func envLabelFromSuffix(suffix string) string {
	if suffix == "" {
		return "prod"
	}
	return "staging"
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
