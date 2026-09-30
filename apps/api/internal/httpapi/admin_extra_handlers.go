package httpapi

import (
	"sync"
	"time"
)

const adminExportOptionsCacheTTL = 60 * time.Second

var adminExportOptionsCache = struct {
	sync.RWMutex
	expiresAt  time.Time
	users      []map[string]any
	modes      []map[string]any
	promocodes []map[string]any
}{}

func cloneMapRows(src []map[string]any) []map[string]any {
	out := make([]map[string]any, len(src))
	for i, row := range src {
		copyRow := make(map[string]any, len(row))
		for key, value := range row {
			copyRow[key] = value
		}
		out[i] = copyRow
	}
	return out
}

func clearAdminExportOptionsCache() {
	adminExportOptionsCache.Lock()
	adminExportOptionsCache.expiresAt = time.Time{}
	adminExportOptionsCache.users = nil
	adminExportOptionsCache.modes = nil
	adminExportOptionsCache.promocodes = nil
	adminExportOptionsCache.Unlock()
}

type adminTariffRequest struct {
	Name                     string  `json:"name"`
	Description              string  `json:"description"`
	TariffType               string  `json:"tariffType"`
	MonthlyPrice             float64 `json:"monthlyPrice"`
	DailyMessageLimit        *int64  `json:"dailyMessageLimit"`
	LimitType                string  `json:"limitType"`
	GroupID                  *int64  `json:"groupId"`
	AvailableForSubscription bool    `json:"availableForSubscription"`
	ModeIDs                  []int64 `json:"modeIds"`
	FirstModeID              *int64  `json:"firstModeId"`
}

type adminTariffGroupRequest struct {
	Name                     string `json:"name"`
	Description              string `json:"description"`
	AvailableForSubscription bool   `json:"availableForSubscription"`
}

type adminPromocodeRequest struct {
	Code              string   `json:"code"`
	AutoGenerate      bool     `json:"autoGenerate"`
	BulkCount         int      `json:"bulkCount"`
	GrantsType        string   `json:"grantsType"`
	TargetIDs         []int64  `json:"targetIds"`
	FirstModeID       *int64   `json:"firstModeId"`
	ActiveFrom        string   `json:"activeFrom"`
	ActiveTo          string   `json:"activeTo"`
	DurationDays      int      `json:"durationDays"`
	MaxUses           int      `json:"maxUses"`
	DailyMessageLimit *int64   `json:"dailyMessageLimit"`
	SummaryLimit      *int64   `json:"summaryLimit"`
	AccessPriority    int      `json:"accessPriority"`
	LimitType         string   `json:"limitType"`
	Price             *float64 `json:"price"`
	Comment           string   `json:"comment"`
	Purpose           string   `json:"purpose"`
	TemporaryAdmin    bool     `json:"temporaryAdmin"`
}

type adminSummaryPromptRequest struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	IsDefault bool   `json:"isDefault"`
}

type adminExportSummaryRequest struct {
	ModeIDs      []int64 `json:"modeIds"`
	UserIDs      []int64 `json:"userIds"`
	PromocodeIDs []int64 `json:"promocodeIds"`
	PromptID     int64   `json:"promptId"`
	Prompt       string  `json:"prompt"`
	RoleFilter   string  `json:"roleFilter"`
	Limit        int64   `json:"limit"`
	DateFrom     string  `json:"dateFrom"`
	DateTo       string  `json:"dateTo"`
}

type adminGuardrailRequest struct {
	Text          string `json:"text"`
	ApplyToAll    bool   `json:"applyToAll"`
	ReplaceAll    bool   `json:"replaceAll"`
	Find          string `json:"find"`
	Replacement   string `json:"replacement"`
	ProtectFuture bool   `json:"protectFuture"`
}
