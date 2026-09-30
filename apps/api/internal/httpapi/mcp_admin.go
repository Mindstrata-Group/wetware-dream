package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Command-based management of Mindstrata: the entry point for the MCP server
// (apps/mcp-admin) that Claude agents work through.
//
// Why one route with an action field rather than two dozen endpoints: each
// route has to be added to two route registries, to OpenAPI and to the audit
// classifier. The command set here will grow, and the cost of "forgot to add it" is
// a red CI for nothing. One route is added once, and the command list
// lives in the map below and is checked by a test.
//
// Access uses a service key from system_settings (mcp_admin_key), compared in
// constant time. The agent deliberately has no service account: that way the
// key has no session, no cookie and no way into the web admin UI.
//
// Everything that changes data is written to admin_audit_log with actor_user_id = 0 and
// the source marked in meta; otherwise changes made by an agent would be
// indistinguishable from manual ones.

const mcpAdminKeySetting = "mcp_admin_key"

// mcpActions maps a command to its audit record. An empty string means "read
// only, do not write to the audit log". The map also serves as an allowlist: an unknown
// command never reaches the database.
var mcpActions = map[string]string{
	"ping":          "",
	"stats.summary": "",
	"users.find":    "",
	"access.list":   "",
	"tariffs.list":  "",
	"promo.list":    "",
	"tir.results":   "",
	"access.grant":  "admin.mcp.access_grant",
	"access.revoke": "admin.mcp.access_revoke",
	"promo.create":  "admin.mcp.promo_create",
	"promo.update":  "admin.mcp.promo_update",
	// Machines with a reverse SSH tunnel. sync is called by the synchroniser on the host once
	// a minute; it is not written to the audit log, otherwise the log would drown in service rows.
	"machine.list":      "",
	"machine.sync":      "",
	"machine.sshconfig": "",
	"security.events":   "",
	"machine.register":  "admin.mcp.machine_register",
	"machine.forget":    "admin.mcp.machine_forget",
}

// mcpRequest is the command body. The fields are shared by all actions: what exactly
// is required is checked by the handler itself, which says so in Russian.
type mcpRequest struct {
	Action   string   `json:"action"`
	Query    string   `json:"query"`
	UserID   int64    `json:"userId"`
	ModeIDs  []int64  `json:"modeIds"`
	TariffID int64    `json:"tariffId"`
	Days     int      `json:"days"`
	Limit    int      `json:"limit"`
	Code     string   `json:"code"`
	MaxUses  int      `json:"maxUses"`
	Comment  string   `json:"comment"`
	Group    string   `json:"group"`
	Reason   string   `json:"reason"`
	Tasks    []string `json:"tasks"`
	// Machines with a reverse SSH tunnel (mcp_machines.go).
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	OS        string `json:"os"`
	Port      int    `json:"port"`
	Ports     []int  `json:"ports"`
	All       bool   `json:"all"`
}

// MCPAdmin — POST /api/mcp/call.
func (h Handler) MCPAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	ctx := r.Context()
	want, err := h.systemSetting(ctx, mcpAdminKeySetting)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(want) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "ключ управления не задан на сервере"})
		return
	}
	if locked, wait := h.keyAttemptsExhausted(r); locked {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "error": "слишком много неудачных попыток, подождите " + wait.Truncate(time.Second).String(),
		})
		return
	}
	got := strings.TrimSpace(r.Header.Get("X-MCP-Key"))
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		h.guardKeyAttempt(ctx, r, "mcp")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "неверный ключ управления"})
		return
	}

	var req mcpRequest
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	action := strings.TrimSpace(req.Action)
	auditAction, known := mcpActions[action]
	if !known {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестная команда: " + action})
		return
	}

	data, code, err := h.mcpDispatch(ctx, action, req)
	if err != nil {
		writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if auditAction != "" {
		meta := map[string]any{"source": "mcp", "action": action, "reason": strings.TrimSpace(req.Reason)}
		if req.UserID > 0 {
			meta["userId"] = req.UserID
		}
		h.writeAdminAudit(ctx, r, 0, auditAction, "mcp", nil, meta)
	}
	out := map[string]any{"ok": true, "action": action}
	for k, v := range data {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

func (h Handler) mcpDispatch(ctx context.Context, action string, req mcpRequest) (map[string]any, int, error) {
	switch action {
	case "ping":
		return map[string]any{"time": time.Now().UTC().Format(time.RFC3339), "commands": mcpCommandList()}, http.StatusOK, nil
	case "stats.summary":
		return h.mcpStats(ctx)
	case "users.find":
		return h.mcpUsersFind(ctx, req)
	case "access.list":
		return h.mcpAccessList(ctx, req)
	case "tariffs.list":
		return h.mcpTariffs(ctx)
	case "promo.list":
		return h.mcpPromoList(ctx, req)
	case "tir.results":
		return h.mcpTirResults(ctx, req)
	case "access.grant":
		return h.mcpAccessGrant(ctx, req)
	case "access.revoke":
		return h.mcpAccessRevoke(ctx, req)
	case "promo.create":
		return h.mcpPromoCreate(ctx, req)
	case "promo.update":
		return h.mcpPromoUpdate(ctx, req)
	case "machine.register":
		return h.mcpMachineRegister(ctx, req)
	case "machine.list":
		return h.mcpMachineList(ctx, req)
	case "machine.forget":
		return h.mcpMachineForget(ctx, req)
	case "machine.sync":
		return h.mcpMachineSync(ctx, req)
	case "machine.sshconfig":
		return h.mcpMachineSSHConfig(ctx, req)
	case "security.events":
		return h.mcpSecurityEvents(ctx, req)
	}
	return nil, http.StatusBadRequest, fmt.Errorf("команда не реализована: %s", action)
}

func mcpCommandList() []string {
	out := make([]string, 0, len(mcpActions))
	for k := range mcpActions {
		out = append(out, k)
	}
	return out
}

func (h Handler) mcpStats(ctx context.Context) (map[string]any, int, error) {
	var users, withAccess, activeSubs, tirReports int
	err := h.DB.QueryRow(ctx, `
		select (select count(*) from users where deleted_at is null),
		       (select count(distinct user_id) from user_mode_access where active_to > now() and active_from <= now()),
		       (select count(*) from subscriptions where status = 'active' and active_to > now()),
		       (select count(*) from game_results)`).Scan(&users, &withAccess, &activeSubs, &tirReports)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{
		"users": users, "usersWithAccess": withAccess,
		"activeSubscriptions": activeSubs, "tirReports": tirReports,
	}, http.StatusOK, nil
}

func (h Handler) mcpUsersFind(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	q := strings.TrimSpace(req.Query)
	if q == "" && req.UserID == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен query или userId")
	}
	rows, err := h.DB.Query(ctx, `
		select u.id, coalesce(u.display_name, ''), coalesce(u.telegram_username, ''), coalesce(u.email, ''),
		       u.role, u.created_at::date::text, coalesce(u.last_login_at::date::text, ''),
		       (select count(*) from user_mode_access a where a.user_id = u.id and a.active_to > now()) as live_access
		from users u
		where u.deleted_at is null
		  and ($1 = 0 or u.id = $1)
		  and ($2 = '' or u.display_name ilike '%'||$2||'%' or u.telegram_username ilike '%'||$2||'%' or u.email ilike '%'||$2||'%')
		order by u.id desc
		limit $3`, req.UserID, q, mcpLimit(req.Limit, 50))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, tg, email, role, created, lastLogin string
		var live int
		if err := rows.Scan(&id, &name, &tg, &email, &role, &created, &lastLogin, &live); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{
			"id": id, "displayName": name, "telegram": tg, "email": email, "role": role,
			"registered": created, "lastLogin": lastLogin, "liveAccessRows": live,
		})
	}
	return map[string]any{"users": out}, http.StatusOK, rows.Err()
}

func (h Handler) mcpAccessList(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	rows, err := h.DB.Query(ctx, `
		select u.id, coalesce(nullif(u.display_name, ''), coalesce(nullif(u.telegram_username, ''), coalesce(u.email, ''))),
		       a.access_type, count(distinct a.mode_id), min(a.active_from)::date::text, max(a.active_to)::date::text
		from user_mode_access a
		join users u on u.id = a.user_id
		where a.active_to > now() and a.active_from <= now()
		  and ($1 = 0 or a.user_id = $1)
		group by u.id, u.display_name, u.telegram_username, u.email, a.access_type
		order by max(a.active_to) desc
		limit $2`, req.UserID, mcpLimit(req.Limit, 100))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var who, kind, from, to string
		var modes int
		if err := rows.Scan(&id, &who, &kind, &modes, &from, &to); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{"userId": id, "who": who, "type": kind, "modes": modes, "from": from, "to": to})
	}
	return map[string]any{"access": out}, http.StatusOK, rows.Err()
}

func (h Handler) mcpTariffs(ctx context.Context) (map[string]any, int, error) {
	rows, err := h.DB.Query(ctx, `
		select t.id, t.name, t.tariff_type, t.monthly_price, coalesce(t.yearly_price, 0),
		       t.available_for_subscription, t.archived_at is not null, t.daily_message_limit
		from tariffs t order by t.archived_at is not null, t.id`)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, kind string
		var monthly, yearly float64
		var sellable, archived bool
		var limit int
		if err := rows.Scan(&id, &name, &kind, &monthly, &yearly, &sellable, &archived, &limit); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "type": kind, "monthlyPrice": monthly, "yearlyPrice": yearly,
			"sellable": sellable, "archived": archived, "dailyMessageLimit": limit,
		})
	}
	return map[string]any{"tariffs": out}, http.StatusOK, rows.Err()
}

func (h Handler) mcpPromoList(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	rows, err := h.DB.Query(ctx, `
		select p.id, p.code, p.duration::text, p.grants_type, p.target_id, p.max_uses, p.used_count,
		       p.active_from::date::text, p.active_to::date::text, coalesce(p.comment, ''), p.temporary_admin_enabled
		from promocodes p
		where ($1 = '' or p.code ilike '%'||$1||'%' or coalesce(p.comment,'') ilike '%'||$1||'%')
		order by p.id desc
		limit $2`, strings.TrimSpace(req.Query), mcpLimit(req.Limit, 30))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, target int64
		var code, duration, grants, from, to, comment string
		var maxUses, used int
		var tempAdmin bool
		if err := rows.Scan(&id, &code, &duration, &grants, &target, &maxUses, &used, &from, &to, &comment, &tempAdmin); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{
			"id": id, "code": code, "duration": duration, "grants": grants, "targetId": target,
			"maxUses": maxUses, "usedCount": used, "activeFrom": from, "activeTo": to,
			"comment": comment, "temporaryAdmin": tempAdmin,
		})
	}
	return map[string]any{"promocodes": out}, http.StatusOK, rows.Err()
}

func (h Handler) mcpTirResults(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	results, err := h.gameBoard(ctx, gamePraktika1)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	group := clipName(req.Group)
	if group != "" {
		filtered := make([]map[string]any, 0, len(results))
		for _, row := range results {
			if g, _ := row["group"].(string); strings.EqualFold(strings.TrimSpace(g), group) {
				filtered = append(filtered, row)
			}
		}
		results = filtered
	}
	return map[string]any{"results": results}, http.StatusOK, nil
}

// mcpAccessGrant grants access to modes for N days. Exactly the same way as
// a promocode (access_type = 'manual', default priority and limit), so that
// access granted by an agent is no different from access granted by hand.
func (h Handler) mcpAccessGrant(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	if req.UserID <= 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен userId")
	}
	if req.Days <= 0 || req.Days > 366 {
		return nil, http.StatusBadRequest, fmt.Errorf("срок от 1 до 366 дней")
	}
	modeIDs := req.ModeIDs
	if len(modeIDs) == 0 && req.TariffID > 0 {
		rows, err := h.DB.Query(ctx, `select mode_id from tariff_mode where tariff_id = $1`, req.TariffID)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, http.StatusInternalServerError, err
			}
			modeIDs = append(modeIDs, id)
		}
		if err := rows.Err(); err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}
	if len(modeIDs) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужны modeIds или tariffId с режимами")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	tag, err := h.DB.Exec(ctx, `
		insert into user_mode_access
			(user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, created_at, updated_at)
		select $1, unnest($2::bigint[]), now(), now() + make_interval(days => $3), $4, 100, 'manual', now(), now()`,
		req.UserID, modeIDs, req.Days, limit)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{"granted": tag.RowsAffected(), "days": req.Days, "modes": len(modeIDs)}, http.StatusOK, nil
}

// mcpAccessRevoke ends access by setting the end date rather than deleting: one can see what was
// granted and when, and it can be restored with one command.
func (h Handler) mcpAccessRevoke(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	if req.UserID <= 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен userId")
	}
	tag, err := h.DB.Exec(ctx, `
		update user_mode_access set active_to = now(), updated_at = now()
		where user_id = $1 and active_to > now()`, req.UserID)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{"revoked": tag.RowsAffected()}, http.StatusOK, nil
}

func (h Handler) mcpPromoCreate(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	if req.TariffID <= 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен tariffId")
	}
	if req.Days <= 0 || req.Days > 366 {
		return nil, http.StatusBadRequest, fmt.Errorf("срок от 1 до 366 дней")
	}
	maxUses := req.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		code = fmt.Sprintf("MS-MCP-%d", time.Now().UnixNano())
	}
	var id int64
	err := h.DB.QueryRow(ctx, `
		insert into promocodes
			(code, max_uses, used_count, active_from, active_to, duration, daily_message_limit,
			 access_priority, grants_type, target_id, limit_type, comment, temporary_admin_enabled, created_at, updated_at)
		values ($1, $2, 0, now(), now() + interval '30 days', make_interval(days => $3), $4,
		        100, 'tariff', $5, 'shared', $6, false, now(), now())
		returning id`, code, maxUses, req.Days, limit, req.TariffID, strings.TrimSpace(req.Comment)).Scan(&id)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]any{"id": id, "code": code, "days": req.Days, "maxUses": maxUses}, http.StatusOK, nil
}

// mcpPromoUpdate changes the validity period and activation count of a not yet used
// promocode. A used one is left alone: people already have access under the old
// terms, and changing them retroactively is a way to get a dispute without a trace.
func (h Handler) mcpPromoUpdate(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("нужен code")
	}
	if req.Days < 0 || req.Days > 366 {
		return nil, http.StatusBadRequest, fmt.Errorf("срок от 1 до 366 дней")
	}
	tag, err := h.DB.Exec(ctx, `
		update promocodes
		set duration = case when $2 > 0 then make_interval(days => $2) else duration end,
		    max_uses = case when $3 > 0 then $3 else max_uses end,
		    comment  = case when $4 <> '' then $4 else comment end,
		    updated_at = now()
		where upper(code) = $1 and used_count = 0`, code, req.Days, req.MaxUses, strings.TrimSpace(req.Comment))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if tag.RowsAffected() == 0 {
		return nil, http.StatusNotFound, fmt.Errorf("промокод не найден или уже активирован")
	}
	return map[string]any{"updated": tag.RowsAffected(), "code": code}, http.StatusOK, nil
}

func mcpLimit(v, def int) int {
	if v <= 0 {
		return def
	}
	if v > 500 {
		return 500
	}
	return v
}

// mcpSecurityEvents is the log of other people's attempts: key guessing, foreign sessions,
// impossible timings, score tampering. It must be viewed in full and in one
// place, otherwise the traces get lost among regular records.
func (h Handler) mcpSecurityEvents(ctx context.Context, req mcpRequest) (map[string]any, int, error) {
	rows, err := h.DB.Query(ctx, `
		select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD HH24:MI'), kind, severity, path,
		       left(detail::text, 300), left(ip_hash, 12)
		from security_events
		where ($1 = '' or kind = $1)
		order by created_at desc
		limit $2`, strings.TrimSpace(req.Query), mcpLimit(req.Limit, 50))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var when, kind, severity, path, detail, ip string
		if err := rows.Scan(&when, &kind, &severity, &path, &detail, &ip); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		out = append(out, map[string]any{
			"when": when, "kind": kind, "severity": severity, "path": path, "detail": detail, "ip": ip,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	// A summary by kind: it shows at once whether guessing is going on right now.
	sum, err := h.DB.Query(ctx, `
		select kind, count(*), max(to_char(created_at at time zone 'UTC', 'YYYY-MM-DD HH24:MI'))
		from security_events where created_at > now() - interval '7 days'
		group by kind order by count(*) desc`)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer sum.Close()
	summary := []map[string]any{}
	for sum.Next() {
		var kind, last string
		var n int
		if err := sum.Scan(&kind, &n, &last); err != nil {
			return nil, http.StatusInternalServerError, err
		}
		summary = append(summary, map[string]any{"kind": kind, "count": n, "last": last})
	}
	return map[string]any{"events": out, "summaryLast7Days": summary}, http.StatusOK, sum.Err()
}
