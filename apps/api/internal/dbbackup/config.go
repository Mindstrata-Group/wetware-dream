// Package dbbackup is the weekly off-site database backup to S3 (Timeweb).
//
// The workflow and schedule live in the Go worker; the dump itself is done by
// the backup_database activity in the anonymizer's python worker (apps/anonymizer):
// it has pg_dump and boto3, and it is built on the server without an image pipeline.
// Every run is visible in the Temporal UI: key, size, what rotation deleted.
package dbbackup

import "os"

const (
	BackupScheduleID  = "mindstrata-db-backup"
	BackupWorkflowID  = "mindstrata-db-backup-run"
	BackupActivity    = "backup_database"
	defaultBackupCron = "30 4 * * 0" // Sunday 04:30
	defaultBackupTZ   = "Asia/Yekaterinburg"
)

type Config struct {
	ScheduleEnabled  bool
	ScheduleCron     string
	ScheduleTimeZone string
	// QueueSuffix separates the prod/staging Go queues and schedules on the shared
	// Temporal. The backup_database activity lives in the shared python queue
	// with prod credentials, so only prod enables the schedule.
	QueueSuffix string
}

func ConfigFromEnv() Config {
	return ConfigFromLookup(os.Getenv)
}

func ConfigFromLookup(lookup func(string) string) Config {
	return Config{
		// Off by default: only prod creates the schedule
		// (BACKUP_SCHEDULE_ENABLED=true in .env.prod). The staging DB is a copy
		// of prod; there is no point backing it up.
		ScheduleEnabled:  boolValue(lookup("BACKUP_SCHEDULE_ENABLED"), false),
		ScheduleCron:     stringValue(lookup("BACKUP_CRON"), defaultBackupCron),
		ScheduleTimeZone: stringValue(lookup("BACKUP_TIMEZONE"), defaultBackupTZ),
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
