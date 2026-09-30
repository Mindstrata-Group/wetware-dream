package anonymizer

import (
	"os"
	"strconv"
	"time"
)

const (
	DefaultTemporalAddress   = "temporal-server:7233"
	GoTaskQueue              = "anonymizer-go"
	PythonTaskQueue          = "anonymizer-python"
	AnonymizeTextsActivity   = "anonymize_texts"
	AnonymizationScheduleID  = "mindstrata-anonymize-messages"
	AnonymizationWorkflowID  = "mindstrata-anonymize-messages-workflow"
	defaultAnonymizationCron = "0 2 1 * *"
	defaultAnonymizationTZ   = "Asia/Yekaterinburg"
)

type Config struct {
	DatabaseURL         string
	TemporalAddress     string
	MaxBatchBytes       int // maximum total content size in a batch (bytes)
	MaxBatchRows        int // upper row limit for the window function (safety cap)
	SleepBetweenBatches time.Duration
	MessageAgeDays      int
	ScheduleEnabled     bool
	ScheduleCron        string
	ScheduleTimeZone    string
	// QueueSuffix separates the prod/staging Go queues and schedules on the shared
	// Temporal: "" on prod, "-staging" on staging. The Python queue stays
	// shared: anonymize_texts is a pure function with no database access.
	QueueSuffix string
}

// WorkflowConfig is the safe subset of the config written to Temporal history.
// Do not add DATABASE_URL or other secrets here.
type WorkflowConfig struct {
	SleepBetweenBatches time.Duration
}

func ConfigFromEnv() Config {
	return ConfigFromLookup(os.Getenv)
}

func ConfigFromLookup(lookup func(string) string) Config {
	workflowCfg := WorkflowConfigFromLookup(lookup)
	return Config{
		DatabaseURL:         lookup("DATABASE_URL"),
		TemporalAddress:     stringValue(lookup("TEMPORAL_ADDRESS"), DefaultTemporalAddress),
		MaxBatchBytes:       intValue(lookup("ANON_MAX_BATCH_BYTES"), 200*1024),
		MaxBatchRows:        intValue(lookup("ANON_MAX_BATCH_ROWS"), 2000),
		SleepBetweenBatches: workflowCfg.SleepBetweenBatches,
		MessageAgeDays:      intValue(lookup("ANON_MESSAGE_AGE_DAYS"), 30),
		ScheduleEnabled:     boolValue(lookup("ANON_SCHEDULE_ENABLED"), true),
		ScheduleCron:        stringValue(lookup("ANON_SCHEDULE_CRON"), defaultAnonymizationCron),
		ScheduleTimeZone:    stringValue(lookup("ANON_SCHEDULE_TZ"), defaultAnonymizationTZ),
		QueueSuffix:         lookup("TEMPORAL_QUEUE_SUFFIX"),
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

func WorkflowConfigFromEnv() WorkflowConfig {
	return WorkflowConfigFromLookup(os.Getenv)
}

func WorkflowConfigFromLookup(lookup func(string) string) WorkflowConfig {
	return WorkflowConfig{
		SleepBetweenBatches: time.Duration(intValue(lookup("ANON_SLEEP_MS"), 2000)) * time.Millisecond,
	}
}

func stringValue(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func intValue(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
