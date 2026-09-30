package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Admin management of the AI gateway registry: create, edit, disable,
// move up the list, archive. The registry itself and how the fallback chain
// is built from it live in ai_gateways.go.
//
// Archiving instead of deleting is deliberate: modes.ai_provider and
// ai_provider_events reference a gateway. A deleted row would leave modes
// pointing at nothing and statistics without a label, both silently. So there
// is no DELETE here at all, rather than "there is one, but guarded".

// gatewayIDPattern mirrors the migration's check constraint. The duplication is
// deliberate: the DB protects the data, while this check gives the admin a clear
// error instead of a 500 from the driver.
var gatewayIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// aiGatewayAuditActions maps a request action to its audit record name.
//
// It is a map because concatenating "admin.ai_gateway."+action is invisible to
// the audit classification guard (it looks for a string literal in the
// writeAdminAudit call), and any new operation would silently slip past it. A
// test checks that this map matches the classifier.
var aiGatewayAuditActions = map[string]string{
	"update":  "admin.ai_gateway.update",
	"archive": "admin.ai_gateway.archive",
	"restore": "admin.ai_gateway.restore",
	"move":    "admin.ai_gateway.move",
}

// AdminAIGateways — GET/POST /api/admin/ai-gateways.
func (h Handler) AdminAIGateways(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", r.Method != http.MethodGet)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.adminListGateways(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"gateways":  items,
			"protocols": []string{gatewayProtocolOpenAI, gatewayProtocolAnthropic, gatewayProtocolGemini},
		})
	case http.MethodPost:
		var req struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Protocol     string `json:"protocol"`
			BaseURL      string `json:"baseUrl"`
			RelayURL     string `json:"relayUrl"`
			UseRelay     bool   `json:"useRelay"`
			APIKey       string `json:"apiKey"`
			DefaultModel string `json:"defaultModel"`
		}
		if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		id := strings.ToLower(strings.TrimSpace(req.ID))
		if !gatewayIDPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "id: только латиница в нижнем регистре, цифры, дефис и подчёркивание, 2–32 символа"})
			return
		}
		protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
		if !gatewayProtocols[protocol] {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "protocol: поддерживаются только openai, anthropic, gemini — для другого формата ответа нужен адаптер в коде"})
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = id
		}
		// A new gateway goes to the END of the chain, not the start: it is usually
		// added as a spare, and silently moving all traffic onto an untested gateway
		// would be the worst default. Moving it up is a separate, deliberate action.
		var priority int
		if err := h.DB.QueryRow(r.Context(), `select coalesce(max(priority), 0) + 10 from ai_gateways`).Scan(&priority); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_, err := h.DB.Exec(r.Context(), `
			insert into ai_gateways (id, title, protocol, base_url, relay_url, use_relay, api_key, default_model, priority, enabled)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)`,
			id, title, protocol,
			strings.TrimSpace(req.BaseURL), strings.TrimSpace(req.RelayURL), req.UseRelay,
			strings.TrimSpace(req.APIKey), strings.TrimSpace(req.DefaultModel),
			priority)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "шлюз с таким id уже есть (возможно, в архиве)"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.c.aiGateways.clear()
		h.c.adminAISettings.clear()
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.ai_gateway.create", "ai_gateway", nil, map[string]any{
			"id":       id,
			"protocol": protocol,
			"priority": priority,
			// The key never goes into the audit log, only the fact that it was set on creation.
			"apiKeySet": strings.TrimSpace(req.APIKey) != "",
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

// AdminAIGatewayDetail handles POST /api/admin/ai-gateways/<id>.
// The action field is one of: update | archive | restore | move.
func (h Handler) AdminAIGatewayDetail(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "orchestration", true)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	id := strings.ToLower(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/ai-gateways/"), "/"))
	if !gatewayIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "некорректный id шлюза"})
		return
	}
	var req struct {
		Action       string  `json:"action"`
		Title        *string `json:"title"`
		BaseURL      *string `json:"baseUrl"`
		RelayURL     *string `json:"relayUrl"`
		UseRelay     *bool   `json:"useRelay"`
		APIKey       *string `json:"apiKey"`
		DefaultModel *string `json:"defaultModel"`
		Enabled      *bool   `json:"enabled"`
		Direction    string  `json:"direction"`
	}
	if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}

	var err error
	action := strings.TrimSpace(req.Action)
	switch action {
	case "", "update":
		action = "update"
		err = h.updateGateway(r.Context(), id, req.Title, req.BaseURL, req.RelayURL, req.UseRelay, req.APIKey, req.DefaultModel, req.Enabled)
	case "archive":
		err = h.setGatewayArchived(r.Context(), id, true)
	case "restore":
		err = h.setGatewayArchived(r.Context(), id, false)
	case "move":
		err = h.moveGateway(r.Context(), id, req.Direction)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "неизвестное действие: " + action})
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "шлюз не найден"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.c.aiGateways.clear()
	h.c.adminAISettings.clear()
	h.writeAdminAudit(r.Context(), r, actor.ID, aiGatewayAuditActions[action], "ai_gateway", nil, map[string]any{
		"id":        id,
		"direction": strings.TrimSpace(req.Direction),
		"apiKeySet": req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "",
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminGatewayView is a registry row for the admin UI. The key is only sent
// masked: nobody needs to see it in full, and leaking it through admin JSON is
// a common way to lose a secret.
type adminGatewayView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"baseUrl"`
	RelayURL     string `json:"relayUrl"`
	UseRelay     bool   `json:"useRelay"`
	EffectiveURL string `json:"effectiveUrl"`
	APIKeyMasked string `json:"apiKeyMasked"`
	APIKeySet    string `json:"apiKeySource"`
	DefaultModel string `json:"defaultModel"`
	Priority     int    `json:"priority"`
	Enabled      bool   `json:"enabled"`
	Archived     bool   `json:"archived"`
	Configured   bool   `json:"configured"`
}

func (h Handler) adminListGateways(ctx context.Context) ([]adminGatewayView, error) {
	rows, err := h.DB.Query(ctx, `
		select id, title, protocol, coalesce(base_url, ''), coalesce(relay_url, ''),
		       coalesce(use_relay, false), coalesce(api_key, ''),
		       coalesce(default_model, ''), priority, enabled, archived_at is not null
		from ai_gateways
		order by archived_at is not null, priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []adminGatewayView{}
	for rows.Next() {
		var g aiGateway
		var archived bool
		if err := rows.Scan(&g.ID, &g.Title, &g.Protocol, &g.BaseURL, &g.RelayURL, &g.UseRelay, &g.APIKey, &g.DefaultModel, &g.Priority, &g.Enabled, &archived); err != nil {
			return nil, err
		}
		effective := h.gatewayAPIKey(ctx, g)
		// apiKeySource shows where the key actually came from: the registry row
		// or a legacy source. Without it the admin cannot tell "the key is set
		// here" from "it runs on a key inherited from env", and is surprised when
		// env goes away.
		source := "none"
		switch {
		case strings.TrimSpace(g.APIKey) != "":
			source = "gateway"
		case effective != "":
			source = "legacy"
		}
		out = append(out, adminGatewayView{
			ID:       g.ID,
			Title:    g.Title,
			Protocol: g.Protocol,
			BaseURL:  g.BaseURL,
			RelayURL: g.RelayURL,
			UseRelay: g.UseRelay,
			// The effective address is a separate field: a checkbox and two fields
			// side by side are three places to make a mistake, while the admin has one
			// question: "where does the request actually go right now".
			EffectiveURL: g.effectiveBaseURL(),
			APIKeyMasked: maskSecret(effective),
			APIKeySet:    source,
			DefaultModel: g.DefaultModel,
			Priority:     g.Priority,
			Enabled:      g.Enabled,
			Archived:     archived,
			Configured:   effective != "",
		})
	}
	return out, rows.Err()
}

func (h Handler) updateGateway(ctx context.Context, id string, title, baseURL, relayURL *string, useRelay *bool, apiKey, defaultModel *string, enabled *bool) error {
	// The key is written ONLY when non-empty: an empty string from the form means
	// "field untouched, it shows a mask", not "erase the key". Erasing a key on
	// purpose goes through archive or a DB edit; the form cannot silently cut
	// power to a gateway.
	var keyArg any
	if apiKey != nil && strings.TrimSpace(*apiKey) != "" {
		keyArg = strings.TrimSpace(*apiKey)
	}
	tag, err := h.DB.Exec(ctx, `
		update ai_gateways set
			title         = coalesce($2, title),
			base_url      = coalesce($3, base_url),
			relay_url     = coalesce($4, relay_url),
			use_relay     = coalesce($5, use_relay),
			api_key       = coalesce($6, api_key),
			default_model = coalesce($7, default_model),
			enabled       = coalesce($8, enabled),
			updated_at    = now()
		where id = $1`,
		id,
		trimmedPtr(title),
		trimmedPtr(baseURL),
		trimmedPtr(relayURL),
		useRelay,
		keyArg,
		trimmedPtr(defaultModel),
		enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (h Handler) setGatewayArchived(ctx context.Context, id string, archived bool) error {
	if archived {
		// Do not allow archiving the last working gateway: that instantly turns
		// every chat into "live AI is not configured", and the user notices it,
		// not the admin.
		var alive int
		if err := h.DB.QueryRow(ctx, `
			select count(*) from ai_gateways
			where archived_at is null and enabled and id <> $1`, id).Scan(&alive); err != nil {
			return err
		}
		if alive == 0 {
			return errors.New("это последний включённый шлюз — архивировать его значит остановить чат")
		}
	}
	var tag string
	if archived {
		tag = `update ai_gateways set archived_at = now(), updated_at = now() where id = $1 and archived_at is null`
	} else {
		tag = `update ai_gateways set archived_at = null, updated_at = now() where id = $1 and archived_at is not null`
	}
	res, err := h.DB.Exec(ctx, tag, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// moveGateway swaps a gateway's priority with its neighbour in the list.
//
// We swap the neighbours' priority VALUES rather than assigning
// "priority-1": priorities need not be contiguous, and arithmetic on the
// neighbour would eventually hit equal values and stop moving anything.
func (h Handler) moveGateway(ctx context.Context, id, direction string) error {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "up" && direction != "down" {
		return errors.New("direction: ожидается up или down")
	}
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var priority int
	if err := tx.QueryRow(ctx, `select priority from ai_gateways where id = $1 and archived_at is null`, id).Scan(&priority); err != nil {
		return err
	}
	neighbourQuery := `
		select id, priority from ai_gateways
		where archived_at is null and (priority, id) < ($2, $1)
		order by priority desc, id desc limit 1`
	if direction == "down" {
		neighbourQuery = `
		select id, priority from ai_gateways
		where archived_at is null and (priority, id) > ($2, $1)
		order by priority, id limit 1`
	}
	var neighbourID string
	var neighbourPriority int
	if err := tx.QueryRow(ctx, neighbourQuery, id, priority).Scan(&neighbourID, &neighbourPriority); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Already at the edge: not an error, there is just nowhere to move.
			return tx.Commit(ctx)
		}
		return err
	}
	if neighbourPriority == priority {
		// Equal priorities: the order between them is held by id, so there is
		// nothing to swap; spread them apart so the move becomes visible.
		if direction == "up" {
			neighbourPriority = priority + 1
		} else {
			neighbourPriority = priority - 1
		}
	}
	if _, err := tx.Exec(ctx, `update ai_gateways set priority = $2, updated_at = now() where id = $1`, id, neighbourPriority); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update ai_gateways set priority = $2, updated_at = now() where id = $1`, neighbourID, priority); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// trimmedPtr: nil stays nil (field not sent, leave it alone); a non-empty
// pointer is returned trimmed.
func trimmedPtr(v *string) any {
	if v == nil {
		return nil
	}
	return strings.TrimSpace(*v)
}
