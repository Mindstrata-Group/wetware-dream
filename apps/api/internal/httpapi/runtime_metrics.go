package httpapi

import "expvar"

var (
	metricQuotaFastPathHits   = expvar.NewInt("chat_quota_fast_path_hits_total")
	metricQuotaSlowPathHits   = expvar.NewInt("chat_quota_slow_path_hits_total")
	metricUsersUpdateApplied  = expvar.NewInt("chat_users_update_applied_total")
	metricUsersUpdateSkipped  = expvar.NewInt("chat_users_update_skipped_total")
	metricDailyCounterIncrOps = expvar.NewInt("chat_daily_counter_increment_ops_total")
)

func trackUsersUpdate(rowsAffected int64) {
	if rowsAffected > 0 {
		metricUsersUpdateApplied.Add(1)
		return
	}
	metricUsersUpdateSkipped.Add(1)
}
