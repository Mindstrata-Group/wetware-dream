package billing

import (
	"os"
	"strings"
	"time"
)

const (
	TaskQueue                     = "billing-renewals"
	DailyRenewalsScheduleID       = "mindstrata-yookassa-due-renewals"
	DailyRenewalsWorkflowID       = "mindstrata-yookassa-due-renewals-workflow"
	StartBillingRunActivity       = "StartBillingRun"
	ReconcileInvoicesActivity     = "ReconcilePendingYooKassaInvoices"
	RunDueRenewalsActivity        = "RunDueYooKassaRenewals"
	MarkExhaustedRenewalsActivity = "MarkExhaustedRenewals"
	FinishBillingRunActivity      = "FinishBillingRun"
	defaultRenewalCron            = "0 3 * * *"
	defaultRenewalTimeZone        = "Asia/Yekaterinburg"
	defaultRenewalCatchupWindow   = 6 * time.Hour
	defaultRenewalRunTimeout      = 30 * time.Minute
	defaultRenewalTaskTimeout     = 30 * time.Second
	defaultRenewalScheduleEnable  = true
)

type Config struct {
	TemporalAddress         string
	DatabaseURL             string
	YooKassaShopID          string
	YooKassaSecretKey       string
	RenewalScheduleEnabled  bool
	RenewalScheduleCron     string
	RenewalScheduleTimeZone string
	// QueueSuffix separates the prod/staging queues and schedules on the shared
	// Temporal: "" on prod, "-staging" on staging. Without it the staging worker
	// picked up prod billing tasks and ran them against the staging database.
	QueueSuffix string
}

func ConfigFromEnv() Config {
	return ConfigFromLookup(os.Getenv)
}

func ConfigFromLookup(lookup func(string) string) Config {
	return Config{
		TemporalAddress:         stringValue(lookup("TEMPORAL_ADDRESS"), "temporal-server:7233"),
		DatabaseURL:             lookup("DATABASE_URL"),
		YooKassaShopID:          lookup("YOOKASSA_SHOP_ID"),
		YooKassaSecretKey:       lookup("YOOKASSA_SECRET_KEY"),
		RenewalScheduleEnabled:  boolValue(lookup("BILLING_RENEWAL_SCHEDULE_ENABLED"), defaultRenewalScheduleEnable),
		RenewalScheduleCron:     stringValue(lookup("BILLING_RENEWAL_CRON"), defaultRenewalCron),
		RenewalScheduleTimeZone: stringValue(lookup("BILLING_RENEWAL_TIMEZONE"), defaultRenewalTimeZone),
		QueueSuffix:             strings.TrimSpace(lookup("TEMPORAL_QUEUE_SUFFIX")),
	}
}

func stringValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func boolValue(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
