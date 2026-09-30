package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

type adminAuditClassification struct {
	Section   string
	Sensitive bool
}

var adminAuditActionClassifications = map[string]adminAuditClassification{
	"admin.access.grant":        {Section: "access", Sensitive: true},
	"admin.access.reset_limits": {Section: "access", Sensitive: true},
	"admin.access.revoke":       {Section: "access", Sensitive: true},
	"admin.ai_settings":         {Section: "orchestration", Sensitive: true},
	"admin.ai_gateway.create":   {Section: "orchestration", Sensitive: true},
	"admin.ai_gateway.update":   {Section: "orchestration", Sensitive: true},
	"admin.ai_gateway.archive":  {Section: "orchestration", Sensitive: true},
	"admin.ai_gateway.restore":  {Section: "orchestration", Sensitive: true},
	"admin.ai_gateway.move":     {Section: "orchestration", Sensitive: false},
	"admin.analytics_settings":  {Section: "system", Sensitive: false},
	// "Department N" practicum: the task bank is edited by the teacher with a key,
	// without a service account, so the audit marks it separately.
	"admin.game_task.import":  {Section: "system", Sensitive: false},
	"admin.game_result.reset": {Section: "system", Sensitive: true},
	// Service management through MCP agents (mcp_admin.go). Everything that changes
	// data is marked sensitive: these actions run without a human at the keyboard.
	"admin.mcp.access_grant":  {Section: "access", Sensitive: true},
	"admin.mcp.access_revoke": {Section: "access", Sensitive: true},
	"admin.mcp.promo_create":  {Section: "access", Sensitive: true},
	"admin.mcp.promo_update":  {Section: "access", Sensitive: true},
	// A machine with a reverse SSH tunnel is a way into the network, so registering
	// and revoking one is marked on par with granting access.
	"admin.mcp.machine_register":              {Section: "access", Sensitive: true},
	"admin.mcp.machine_forget":                {Section: "access", Sensitive: true},
	"admin.billing.access_recovery.search":    {Section: "billing", Sensitive: true},
	"admin.billing.access_recovery.transfer":  {Section: "billing", Sensitive: true},
	"admin.billing.yookassa.create_payment":   {Section: "billing", Sensitive: true},
	"admin.broadcast.schedule":                {Section: "notifications", Sensitive: true},
	"admin.dialog_summary_prompt":             {Section: "orchestration", Sensitive: false},
	"admin.lead_summary_prompt":               {Section: "orchestration", Sensitive: false},
	"admin.export.messages.read":              {Section: "exports", Sensitive: true},
	"admin.export.messages.summary":           {Section: "exports", Sensitive: true},
	"admin.mode.copy":                         {Section: "modes", Sensitive: false},
	"admin.mode.create":                       {Section: "modes", Sensitive: false},
	"admin.mode.delete":                       {Section: "modes", Sensitive: false},
	"admin.mode.detach_paid_tariffs":          {Section: "modes", Sensitive: true},
	"admin.mode.guardrail":                    {Section: "orchestration", Sensitive: true},
	"admin.mode.history_write_failed":         {Section: "modes", Sensitive: false},
	"admin.mode.patch":                        {Section: "modes", Sensitive: false},
	"admin.mode.restore":                      {Section: "modes", Sensitive: true},
	"admin.notification.send":                 {Section: "notifications", Sensitive: true},
	"admin.notification.test":                 {Section: "notifications", Sensitive: true},
	"admin.notification.history_write_failed": {Section: "notifications", Sensitive: false},
	"admin.notification_template.upsert":      {Section: "notifications", Sensitive: false},
	"admin.orchestration_prompt":              {Section: "orchestration", Sensitive: true},
	"admin.promocode.activate":                {Section: "promocodes", Sensitive: false},
	"admin.promocode.bulk_deactivate":         {Section: "promocodes", Sensitive: true},
	"admin.promocode.create":                  {Section: "promocodes", Sensitive: true},
	"admin.promocode.deactivate":              {Section: "promocodes", Sensitive: false},
	"admin.promocode.update":                  {Section: "promocodes", Sensitive: true},
	"admin.site_content.flush":                {Section: "content", Sensitive: false},
	"admin.site_content.update":               {Section: "content", Sensitive: false},
	"admin.site_media.upload":                 {Section: "content", Sensitive: false},
	"admin.site_media.delete":                 {Section: "content", Sensitive: false},
	"admin.stats.read":                        {Section: "stats", Sensitive: false},
	"admin.status.read":                       {Section: "system", Sensitive: false},
	"admin.summary_prompt.create":             {Section: "exports", Sensitive: true},
	"admin.summary_prompt.delete":             {Section: "exports", Sensitive: true},
	"admin.summary_prompt.patch":              {Section: "exports", Sensitive: true},
	"admin.tariff.archive":                    {Section: "tariffs", Sensitive: false},
	"admin.tariff.create":                     {Section: "tariffs", Sensitive: false},
	"admin.tariff.destroy":                    {Section: "tariffs", Sensitive: true},
	"admin.tariff.patch":                      {Section: "tariffs", Sensitive: false},
	"admin.tariff.set_modes_ai":               {Section: "tariffs", Sensitive: false},
	"admin.tariff_group.create":               {Section: "tariffs", Sensitive: false},
	"admin.tariff_group.delete":               {Section: "tariffs", Sensitive: false},
	"admin.tariff_group.patch":                {Section: "tariffs", Sensitive: false},
	"admin.user.create":                       {Section: "users", Sensitive: true},
	// Personal-data access log (data-protection AC-13/AC-16).
	"admin.user.read":                       {Section: "users", Sensitive: true},
	"admin.dialog.read":                     {Section: "dialogs", Sensitive: true},
	"admin.data_protection.gateway_country": {Section: "privacy", Sensitive: true},
	"admin.user.patch":                      {Section: "users", Sensitive: true},
	"admin.yookassa_config":                 {Section: "billing", Sensitive: true},
}

var adminAuditRouteClassifications = map[string]adminAuditClassification{
	"/api/admin/status":                             {Section: "system", Sensitive: false},
	"/api/admin/stats":                              {Section: "stats", Sensitive: false},
	"/api/admin/users":                              {Section: "users", Sensitive: true},
	"/api/admin/users/":                             {Section: "users", Sensitive: true},
	"/api/admin/access":                             {Section: "access", Sensitive: true},
	"/api/admin/modes":                              {Section: "modes", Sensitive: false},
	"/api/admin/modes/guardrail":                    {Section: "orchestration", Sensitive: true},
	"/api/admin/modes/model-stats":                  {Section: "modes", Sensitive: false},
	"/api/admin/ai-models":                          {Section: "modes", Sensitive: false},
	"/api/admin/modes/":                             {Section: "modes", Sensitive: false},
	"/api/admin/tariffs":                            {Section: "tariffs", Sensitive: false},
	"/api/admin/tariffs/":                           {Section: "tariffs", Sensitive: false},
	"/api/admin/tariff-groups":                      {Section: "tariffs", Sensitive: false},
	"/api/admin/tariff-groups/":                     {Section: "tariffs", Sensitive: false},
	"/api/admin/promocodes":                         {Section: "promocodes", Sensitive: true},
	"/api/admin/promocodes/bulk-deactivate":         {Section: "promocodes", Sensitive: true},
	"/api/admin/promocodes/":                        {Section: "promocodes", Sensitive: true},
	"/api/admin/orchestration-prompt":               {Section: "orchestration", Sensitive: true},
	"/api/admin/dialog-summary-prompt":              {Section: "orchestration", Sensitive: true},
	"/api/admin/lead-summary-prompt":                {Section: "orchestration", Sensitive: true},
	"/api/admin/ai-settings":                        {Section: "orchestration", Sensitive: true},
	"/api/admin/ai-gateways":                        {Section: "orchestration", Sensitive: true},
	"/api/admin/ai-gateways/":                       {Section: "orchestration", Sensitive: true},
	"/api/admin/analytics-settings":                 {Section: "system", Sensitive: false},
	"/api/admin/site-content":                       {Section: "content", Sensitive: false},
	"/api/admin/site-content/flush":                 {Section: "content", Sensitive: false},
	"/api/admin/site-media":                         {Section: "content", Sensitive: false},
	"/api/admin/notifications/templates":            {Section: "notifications", Sensitive: false},
	"/api/admin/notifications/test":                 {Section: "notifications", Sensitive: true},
	"/api/admin/notifications/send":                 {Section: "notifications", Sensitive: true},
	"/api/admin/notifications/preview":              {Section: "notifications", Sensitive: true},
	"/api/admin/notifications/history":              {Section: "notifications", Sensitive: false},
	"/api/admin/exports/messages":                   {Section: "exports", Sensitive: true},
	"/api/admin/broadcasts":                         {Section: "notifications", Sensitive: true},
	"/api/admin/summary-prompts":                    {Section: "exports", Sensitive: true},
	"/api/admin/summary-prompts/":                   {Section: "exports", Sensitive: true},
	"/api/admin/dialogs":                            {Section: "dialogs", Sensitive: true},
	"/api/admin/dialogs/":                           {Section: "dialogs", Sensitive: true},
	"/api/admin/payments":                           {Section: "billing", Sensitive: true},
	"/api/admin/payments/access-recovery":           {Section: "billing", Sensitive: true},
	"/api/admin/payments/yookassa/config":           {Section: "billing", Sensitive: true},
	"/api/admin/payments/yookassa/create":           {Section: "billing", Sensitive: true},
	"/api/admin/payments/subscriptions/revoke":      {Section: "billing", Sensitive: true},
	"/api/admin/payments/yookassa/test-charge":      {Section: "billing", Sensitive: true},
	"/api/admin/payments/yookassa/renewals/run":     {Section: "billing", Sensitive: true},
	"/api/admin/payments/yookassa/run-due-renewals": {Section: "billing", Sensitive: true},
	"/api/admin/client-logs":                        {Section: "system", Sensitive: false},
	"/api/admin/data-protection":                    {Section: "privacy", Sensitive: true},
	"/api/admin/data-protection/gateway-country":    {Section: "privacy", Sensitive: true},
	"/api/admin/data-protection/access-log":         {Section: "privacy", Sensitive: true},
}

func classifyAdminAudit(action, targetType, path string) adminAuditClassification {
	if class, ok := adminAuditActionClassifications[action]; ok {
		return class
	}
	if class, ok := adminAuditRouteClassifications[path]; ok {
		return class
	}
	section := strings.TrimSpace(targetType)
	if section == "" || section == "system_setting" {
		section = "general"
	}
	return adminAuditClassification{Section: section, Sensitive: false}
}

func sanitizeAuditJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	sanitized, ok := sanitizeAuditValue(input).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return sanitized
}

func marshalAuditJSON(input map[string]any) string {
	payload, err := json.Marshal(sanitizeAuditJSONMap(input))
	if err != nil {
		return `{}`
	}
	return string(payload)
}

func sanitizeAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if auditKeyIsSensitive(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = sanitizeAuditValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, sanitizeAuditValue(child))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, sanitizeAuditValue(child))
		}
		return out
	default:
		return value
	}
}

func auditKeyIsSensitive(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	sensitiveParts := []string{
		"secret",
		"token",
		"cookie",
		"password",
		"authorization",
		"api_key",
		"apikey",
		"paymentmethod",
		"payment_method",
		"yookassa_payment_method_id",
	}
	for _, part := range sensitiveParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func auditPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}
