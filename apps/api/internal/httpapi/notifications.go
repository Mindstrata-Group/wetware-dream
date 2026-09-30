package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

type notificationContact struct {
	Channel    string     `json:"channel"`
	Address    string     `json:"address"`
	Verified   bool       `json:"verified"`
	Source     string     `json:"source"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
}

type notificationConsent struct {
	Channel     string     `json:"channel"`
	ConsentType string     `json:"consentType"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	Reason      string     `json:"reason"`
	GrantedAt   *time.Time `json:"grantedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

type notificationInboxItem struct {
	ID          int64     `json:"id"`
	TemplateKey string    `json:"templateKey,omitempty"`
	ConsentType string    `json:"consentType"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type notificationReachability struct {
	EmailAvailable         bool       `json:"emailAvailable"`
	PhoneVerified          bool       `json:"phoneVerified"`
	PushEnabled            bool       `json:"pushEnabled"`
	ServiceChannels        []string   `json:"serviceChannels"`
	MarketingChannels      []string   `json:"marketingChannels"`
	BestChannel            string     `json:"bestChannel"`
	LastInteractionChannel string     `json:"lastInteractionChannel,omitempty"`
	LastInteractionAt      *time.Time `json:"lastInteractionAt,omitempty"`
}

type notificationTemplate struct {
	Key            string    `json:"key"`
	Name           string    `json:"name"`
	ConsentType    string    `json:"consentType"`
	DefaultChannel string    `json:"defaultChannel"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	Active         bool      `json:"active"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type notificationPreferencesPatch struct {
	Channel     string `json:"channel"`
	ConsentType string `json:"consentType"`
	Granted     *bool  `json:"granted"`
	Reason      string `json:"reason"`
}

type adminNotificationTemplateUpsert struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	ConsentType    string `json:"consentType"`
	DefaultChannel string `json:"defaultChannel"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	Active         *bool  `json:"active"`
}

type adminNotificationTestRequest struct {
	UserID       int64             `json:"userId"`
	UserIDs      []int64           `json:"userIds"`
	PromocodeIDs []int64           `json:"promocodeIds"`
	TemplateKey  string            `json:"templateKey"`
	Channel      string            `json:"channel"`
	Variables    map[string]string `json:"variables"`
}

var errNotificationRecipientsRequired = errors.New("notification recipients are required")

func (h Handler) NotificationsInbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.notificationInbox(r.Context(), user.ID, 20)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) NotificationPreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := h.requireRole(w, r, "user", "tester", "expert", "support", "content_admin", "billing_admin", "admin", "owner")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		payload, err := h.notificationPreferencesPayload(r.Context(), user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPatch:
		var req notificationPreferencesPatch
		if err := decodeJSONStrict(w, r, 8<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		if req.Granted == nil || !validReadyNotificationChannel(req.Channel) || !validConsentType(req.ConsentType) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid consent"})
			return
		}
		bonus, err := h.setNotificationConsentWithBonus(r.Context(), user.ID, req.Channel, req.ConsentType, *req.Granted, "profile", req.Reason)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		payload, err := h.notificationPreferencesPayload(r.Context(), user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		payload["grantedBonus"] = bonus
		writeJSON(w, http.StatusOK, payload)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) AdminNotificationTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.notificationTemplates(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "templates": items})
	case http.MethodPost:
		var req adminNotificationTemplateUpsert
		if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
			return
		}
		tpl, err := h.upsertNotificationTemplate(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.notification_template.upsert", "notification_template", nil, map[string]any{"key": tpl.Key})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": tpl})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (h Handler) AdminNotificationTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req adminNotificationTestRequest
	if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if strings.TrimSpace(req.TemplateKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "templateKey is required"})
		return
	}
	if strings.TrimSpace(req.Channel) == "" {
		req.Channel = "in_site"
	}
	result, err := h.createAdminNotifications(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.notification.test", "user", nil, map[string]any{"templateKey": req.TemplateKey, "channel": req.Channel, "recipientCount": result.RecipientCount, "statusCounts": result.StatusCounts})
	writeJSON(w, http.StatusOK, result.response())
}

// adminNotificationDirectRequest is a simplified direct broadcast without a template.
type adminNotificationDirectRequest struct {
	Audience string `json:"audience"` // "all" | "promocode" | "mode"
	Title    string `json:"title"`
	Body     string `json:"body"`
	// Delivery channels: "inbox" (site), "max", "telegram". Empty = ["inbox"].
	Channels []string `json:"channels"`
	// Promocodes
	PromocodeIDs []int64 `json:"promocodeIds"`
	// Promocode audience subgroups: "wrote" (received and wrote) and/or
	// "not_wrote" (received and did not write). Disjoint; empty/both = everyone.
	PromoFilters []string `json:"promoFilters"`
	// Modes
	ModeIDs    []int64 `json:"modeIds"`
	ModeFilter string  `json:"modeFilter"` // "wrote" | "has"
	PeriodFrom string  `json:"periodFrom"` // RFC3339 or ""
	PeriodTo   string  `json:"periodTo"`
	// Third filtering level: manually excluded recipients.
	ExcludeUserIDs []int64 `json:"excludeUserIds"`
	// Delivery delay to the second messenger (hours) when the recipient has
	// both Max and Telegram bound. 0/unset = 12.
	SecondChannelDelayHours *float64 `json:"secondChannelDelayHours"`
}

// Text length limits per channel (characters). Telegram sendMessage is 4096,
// MAX Bot API text is 4000, the inbox has its own soft ceiling.
const (
	inboxTextLimit    = 10000
	maxTextLimit      = 4000
	telegramTextLimit = 4096
)

func channelTextLimit(channels []string) int {
	limit := inboxTextLimit
	for _, ch := range channels {
		switch ch {
		case "max":
			if maxTextLimit < limit {
				limit = maxTextLimit
			}
		case "telegram":
			if telegramTextLimit < limit {
				limit = telegramTextLimit
			}
		}
	}
	return limit
}

// normalizeDirectChannels drops unknown channels; empty -> ["inbox"].
func normalizeDirectChannels(channels []string) []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, ch := range channels {
		ch = strings.ToLower(strings.TrimSpace(ch))
		if (ch == "inbox" || ch == "max" || ch == "telegram") && !seen[ch] {
			out = append(out, ch)
			seen[ch] = true
		}
	}
	if len(out) == 0 {
		out = append(out, "inbox")
	}
	return out
}

// adminNotificationRecipient holds recipient data for preview and delivery.
type adminNotificationRecipient struct {
	ID             int64  `json:"id"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	MaxLinked      bool   `json:"maxLinked"`
	TelegramLinked bool   `json:"telegramLinked"`
	maxChatID      *int64
	telegramID     *int64
}

// directNotificationRecipientDetails loads recipients' contacts in one
// query. Email/phone come both from users and from notification_contacts:
// the OAuth login writes them there (Yandex returns the phone in contacts, while
// users.phone stays empty).
func (h Handler) directNotificationRecipientDetails(ctx context.Context, ids []int64) ([]adminNotificationRecipient, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := h.DB.Query(ctx, `
		SELECT u.id,
		       COALESCE(NULLIF(u.email, ''), nc_email.address, ''),
		       COALESCE(NULLIF(u.phone, ''), nc_phone.address, ''),
		       u.max_chat_id, u.telegram_id
		FROM users u
		LEFT JOIN LATERAL (
			SELECT address FROM notification_contacts
			WHERE user_id = u.id AND channel = 'email'
			ORDER BY verified DESC, updated_at DESC LIMIT 1
		) nc_email ON true
		LEFT JOIN LATERAL (
			SELECT address FROM notification_contacts
			WHERE user_id = u.id AND channel = 'phone'
			ORDER BY verified DESC, updated_at DESC LIMIT 1
		) nc_phone ON true
		WHERE u.id = ANY($1::bigint[])
		ORDER BY u.id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]adminNotificationRecipient, 0, len(ids))
	for rows.Next() {
		var item adminNotificationRecipient
		if err := rows.Scan(&item.ID, &item.Email, &item.Phone, &item.maxChatID, &item.telegramID); err != nil {
			return nil, err
		}
		item.MaxLinked = item.maxChatID != nil
		item.TelegramLinked = item.telegramID != nil
		out = append(out, item)
	}
	return out, rows.Err()
}

// isMessengerBlockedError: the recipient blocked the bot or the chat is unavailable.
// Telegram: HTTP 403 "bot was blocked by the user" / "user is deactivated",
// 400 "chat not found". Max: HTTP 403 on send. Sending to such a recipient
// is pointless, so we remove the binding.
func isMessengerBlockedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 403") ||
		strings.Contains(msg, "bot was blocked") ||
		strings.Contains(msg, "user is deactivated") ||
		strings.Contains(msg, "chat not found")
}

// unlinkBlockedMessenger removes the binding of a recipient who blocked the bot: the checkbox in
// the profile goes off and we stop sending in vain. The context is a background one: it is called from
// send goroutines whose request context may already be cancelled.
func (h Handler) unlinkBlockedMessenger(userID int64, channel string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch channel {
	case "max":
		_, _ = h.DB.Exec(ctx, `UPDATE users SET max_chat_id = NULL, max_linked_at = NULL, updated_at = now() WHERE id = $1`, userID)
	case "telegram":
		_, _ = h.DB.Exec(ctx, `UPDATE users SET telegram_id = NULL, updated_at = now() WHERE id = $1`, userID)
	}
}

// excludeRecipients is the third filtering stage: recipients manually excluded by the admin.
func excludeRecipients(details []adminNotificationRecipient, exclude []int64) []adminNotificationRecipient {
	if len(exclude) == 0 {
		return details
	}
	skip := make(map[int64]bool, len(exclude))
	for _, id := range exclude {
		skip[id] = true
	}
	out := make([]adminNotificationRecipient, 0, len(details))
	for _, rec := range details {
		if !skip[rec.ID] {
			out = append(out, rec)
		}
	}
	return out
}

// filterRecipientsByChannels is the second filtering stage after the audience: if
// inbox is not among the channels (it is available to everyone), only recipients
// reachable by at least one chosen messenger remain.
func filterRecipientsByChannels(details []adminNotificationRecipient, channels []string) []adminNotificationRecipient {
	hasInbox := false
	wantMax := false
	wantTG := false
	for _, ch := range channels {
		switch ch {
		case "inbox":
			hasInbox = true
		case "max":
			wantMax = true
		case "telegram":
			wantTG = true
		}
	}
	if hasInbox {
		return details
	}
	out := make([]adminNotificationRecipient, 0, len(details))
	for _, rec := range details {
		if (wantMax && rec.MaxLinked) || (wantTG && rec.TelegramLinked) {
			out = append(out, rec)
		}
	}
	return out
}

var (
	mdLinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdBoldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// stripInlineMarkdown reduces CommonMark to plain text: a fallback for
// messengers when the API rejected a message with markup.
func stripInlineMarkdown(text string) string {
	text = mdLinkRE.ReplaceAllString(text, "$1 ($2)")
	text = mdBoldRE.ReplaceAllString(text, "$1")
	// An orphaned (unclosed) '**' is removed too: markers are not for the reader.
	return strings.ReplaceAll(text, "**", "")
}

// messengerPlainText prepares plain "title + body" text without markup.
func messengerPlainText(title, body string) string {
	return stripInlineMarkdown(strings.TrimSpace(title) + "\n\n" + strings.TrimSpace(body))
}

// messengerMarkdownText prepares markdown text for messengers: Max understands
// CommonMark natively (format=markdown), Telegram via conversion to HTML
// inside sendTelegramMessage.
func messengerMarkdownText(title, body string) string {
	return "**" + strings.TrimSpace(title) + "**\n\n" + strings.TrimSpace(body)
}

// AdminNotificationPreview handles POST /api/admin/notifications/preview.
// Returns who the broadcast will go to: count, contacts, channel reach.
func (h Handler) AdminNotificationPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := h.requireOwnerAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req adminNotificationDirectRequest
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	recipientIDs, err := h.resolveDirectNotificationRecipients(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	details, err := h.directNotificationRecipientDetails(r.Context(), recipientIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Filtering stages: audience -> channel reach -> manual exclusions.
	details = filterRecipientsByChannels(details, normalizeDirectChannels(req.Channels))
	details = excludeRecipients(details, req.ExcludeUserIDs)
	emailCount, phoneCount, maxCount, tgCount := 0, 0, 0, 0
	for _, item := range details {
		if item.Email != "" {
			emailCount++
		}
		if item.Phone != "" {
			phoneCount++
		}
		if item.MaxLinked {
			maxCount++
		}
		if item.TelegramLinked {
			tgCount++
		}
	}
	const previewLimit = 200
	preview := details
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"recipientCount":      len(details),
		"emailCount":          emailCount,
		"phoneCount":          phoneCount,
		"maxLinkedCount":      maxCount,
		"telegramLinkedCount": tgCount,
		"recipients":          preview,
		"previewTruncated":    len(details) > previewLimit,
	})
}

// AdminNotificationHistory handles GET /api/admin/notifications/history.
// Parameters: limit (default 20, max 100), offset, from/to
// (dates YYYY-MM-DD or RFC3339, by created_at).
func (h Handler) AdminNotificationHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := h.requireOwnerAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	limit := 20
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}
	parseDate := func(raw string, endOfDay bool) *time.Time {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return &t
		}
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			if endOfDay {
				t = t.Add(24*time.Hour - time.Nanosecond)
			}
			return &t
		}
		return nil
	}
	from := parseDate(q.Get("from"), false)
	to := parseDate(q.Get("to"), true)

	where := "WHERE 1=1"
	args := []any{}
	if from != nil {
		args = append(args, *from)
		where += fmt.Sprintf(" AND s.created_at >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += fmt.Sprintf(" AND s.created_at <= $%d", len(args))
	}

	var total int
	if err := h.DB.QueryRow(r.Context(),
		"SELECT count(*) FROM admin_notification_sends s "+where, args...).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	args = append(args, limit, offset)
	rows, err := h.DB.Query(r.Context(), fmt.Sprintf(`
		SELECT s.id, s.actor_id, COALESCE(u.email, ''), s.audience, s.params, s.channels,
		       s.title, s.body, s.recipient_count, s.delivered, s.created_at
		FROM admin_notification_sends s
		LEFT JOIN users u ON u.id = s.actor_id
		%s
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, actorID             int64
			actorEmail, audience    string
			paramsRaw, deliveredRaw []byte
			channels                []string
			title, body             string
			recipientCount          int
			createdAt               time.Time
		)
		if err := rows.Scan(&id, &actorID, &actorEmail, &audience, &paramsRaw, &channels, &title, &body, &recipientCount, &deliveredRaw, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		var params, delivered map[string]any
		_ = json.Unmarshal(paramsRaw, &params)
		_ = json.Unmarshal(deliveredRaw, &delivered)
		items = append(items, map[string]any{
			"id": id, "actorId": actorID, "actorEmail": actorEmail,
			"audience": audience, "params": params, "channels": channels,
			"title": title, "body": body,
			"recipientCount": recipientCount, "delivered": delivered,
			"createdAt": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "total": total, "limit": limit, "offset": offset})
}

// AdminNotificationSend handles POST /api/admin/notifications/send.
// A direct broadcast to the in-site inbox without a template.
func (h Handler) AdminNotificationSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := h.requireOwnerAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req adminNotificationDirectRequest
	if err := decodeJSONStrict(w, r, 64<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if title == "" || body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "title и body обязательны"})
		return
	}

	channels := normalizeDirectChannels(req.Channels)

	recipientIDs, err := h.resolveDirectNotificationRecipients(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if len(recipientIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "получателей не найдено"})
		return
	}
	details, err := h.directNotificationRecipientDetails(r.Context(), recipientIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// As in the preview: without inbox the broadcast goes only to recipients reachable
	// by the chosen messengers; then the admin's manual exclusions.
	details = filterRecipientsByChannels(details, channels)
	details = excludeRecipients(details, req.ExcludeUserIDs)
	if len(details) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "получателей не найдено (никто не привязал выбранные мессенджеры или все исключены)"})
		return
	}
	recipientIDs = recipientIDs[:0]
	for _, rec := range details {
		recipientIDs = append(recipientIDs, rec.ID)
	}

	// Length limit of the strictest chosen channel.
	if lim := channelTextLimit(channels); len([]rune(title))+len([]rune(body))+4 > lim {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": fmt.Sprintf("текст слишком длинный для выбранных каналов: лимит %d символов", lim),
		})
		return
	}

	hasChannel := func(name string) bool {
		for _, ch := range channels {
			if ch == name {
				return true
			}
		}
		return false
	}
	delivered := map[string]int{}

	if hasChannel("inbox") {
		sent := 0
		for _, userID := range recipientIDs {
			if _, err := h.DB.Exec(r.Context(), `
				INSERT INTO notification_inbox (user_id, consent_type, title, body, payload)
				VALUES ($1, 'service', $2, $3, '{"direct":true}'::jsonb)`,
				userID, title, body); err == nil {
				sent++
			}
		}
		delivered["inbox"] = sent
	}

	// Messenger delivery: a worker pool, so a broadcast to hundreds of
	// recipients does not stretch the HTTP request to minutes.
	sendToMessengers := func(send func(ctx context.Context, rec adminNotificationRecipient) error, filter func(adminNotificationRecipient) bool) int {
		var okCount int64
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		for _, rec := range details {
			if !filter(rec) {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(rec adminNotificationRecipient) {
				defer wg.Done()
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				if err := send(ctx, rec); err == nil {
					atomic.AddInt64(&okCount, 1)
				}
			}(rec)
		}
		wg.Wait()
		return int(okCount)
	}

	messengerText := messengerMarkdownText(title, body)

	// Deferred "second channel": a recipient with both bindings gets the message at once
	// only in the first chosen messenger, and in the second after a delay
	// (the notification_channel_queue), so the message is not duplicated.
	delaySecond := 12 * time.Hour
	if req.SecondChannelDelayHours != nil && *req.SecondChannelDelayHours >= 0 {
		delaySecond = time.Duration(*req.SecondChannelDelayHours * float64(time.Hour))
	}
	bothMessengers := hasChannel("max") && hasChannel("telegram")
	firstMessenger := ""
	for _, ch := range channels {
		if ch == "max" || ch == "telegram" {
			firstMessenger = ch
			break
		}
	}
	deferToQueue := func(rec adminNotificationRecipient, ch string) bool {
		if !bothMessengers || delaySecond <= 0 {
			return false
		}
		if rec.maxChatID == nil || rec.telegramID == nil {
			return false // one messenger bound: send at once
		}
		return ch != firstMessenger
	}
	queued := 0
	enqueueSecond := func(rec adminNotificationRecipient, ch string) {
		if _, err := h.DB.Exec(r.Context(), `
			INSERT INTO notification_channel_queue (user_id, channel, text, send_after)
			VALUES ($1, $2, $3, now() + $4::interval)`,
			rec.ID, ch, messengerText, fmt.Sprintf("%f seconds", delaySecond.Seconds())); err == nil {
			queued++
		}
	}

	if hasChannel("max") {
		delivered["max"] = sendToMessengers(
			func(ctx context.Context, rec adminNotificationRecipient) error {
				err := h.sendMaxMessage(ctx, *rec.maxChatID, messengerText)
				if isMessengerBlockedError(err) {
					h.unlinkBlockedMessenger(rec.ID, "max")
				}
				return err
			},
			func(rec adminNotificationRecipient) bool {
				if rec.maxChatID == nil {
					return false
				}
				if deferToQueue(rec, "max") {
					enqueueSecond(rec, "max")
					return false
				}
				return true
			},
		)
	}
	if hasChannel("telegram") {
		delivered["telegram"] = sendToMessengers(
			func(ctx context.Context, rec adminNotificationRecipient) error {
				err := h.sendTelegramMessage(ctx, strconv.FormatInt(*rec.telegramID, 10), messengerText)
				if isMessengerBlockedError(err) {
					h.unlinkBlockedMessenger(rec.ID, "telegram")
				}
				return err
			},
			func(rec adminNotificationRecipient) bool {
				if rec.telegramID == nil {
					return false
				}
				if deferToQueue(rec, "telegram") {
					enqueueSecond(rec, "telegram")
					return false
				}
				return true
			},
		)
	}
	if queued > 0 {
		delivered["queued"] = queued
	}

	// Send history.
	params, _ := json.Marshal(map[string]any{
		"promocodeIds": req.PromocodeIDs, "modeIds": req.ModeIDs,
		"modeFilter": req.ModeFilter, "periodFrom": req.PeriodFrom, "periodTo": req.PeriodTo,
	})
	deliveredJSON, _ := json.Marshal(delivered)
	if _, err := h.DB.Exec(r.Context(), `
		INSERT INTO admin_notification_sends (actor_id, audience, params, channels, title, body, recipient_count, delivered)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8::jsonb)`,
		actor.ID, req.Audience, string(params), channels, title, body, len(recipientIDs), string(deliveredJSON)); err != nil {
		// History must not break a broadcast that already went out: record it in the audit log.
		h.writeAdminAudit(r.Context(), r, actor.ID, "admin.notification.history_write_failed", "user", nil, map[string]any{"error": err.Error()})
	}

	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.notification.send", "user", nil, map[string]any{
		"audience": req.Audience, "channels": channels, "recipientCount": len(recipientIDs), "delivered": delivered,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"recipientCount": len(recipientIDs),
		"sentCount":      delivered["inbox"],
		"delivered":      delivered,
		"channels":       channels,
	})
}

// resolveDirectNotificationRecipients returns the list of userIDs based on the audience.
func (h Handler) resolveDirectNotificationRecipients(ctx context.Context, req adminNotificationDirectRequest) ([]int64, error) {
	var rows []int64
	switch req.Audience {
	case "admins":
		// Test audience: admins/owners only, a safe "on ourselves" check of the
		// channels before a real broadcast.
		rr, err := h.DB.Query(ctx, `
			SELECT id FROM users
			WHERE deleted_at IS NULL AND status = 'active' AND role IN ('admin', 'owner')
			ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for rr.Next() {
			var id int64
			if err := rr.Scan(&id); err != nil {
				return nil, err
			}
			rows = append(rows, id)
		}
		return rows, rr.Err()

	case "all":
		rr, err := h.DB.Query(ctx, `SELECT id FROM users WHERE deleted_at IS NULL AND status = 'active' ORDER BY id`)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for rr.Next() {
			var id int64
			if err := rr.Scan(&id); err != nil {
				return nil, err
			}
			rows = append(rows, id)
		}
		return rows, rr.Err()

	case "promocode":
		ids := uniquePositiveIDs(req.PromocodeIDs)
		if len(ids) == 0 {
			return nil, nil
		}
		// Subgroups: "received and wrote" / "received and did not write". If one is chosen,
		// filter; if both or neither, take everyone with the promocode.
		wantWrote, wantNotWrote := false, false
		for _, pf := range req.PromoFilters {
			switch strings.TrimSpace(pf) {
			case "wrote":
				wantWrote = true
			case "not_wrote":
				wantNotWrote = true
			}
		}
		query := `
			SELECT DISTINCT pu.user_id FROM promocode_usages pu
			JOIN users u ON u.id = pu.user_id
			WHERE pu.promocode_id = ANY($1::bigint[]) AND u.deleted_at IS NULL AND u.status = 'active'`
		args := []any{ids}
		// Access period: by promocode activation time.
		if req.PeriodFrom != "" {
			if t, err := time.Parse(time.RFC3339, req.PeriodFrom); err == nil {
				args = append(args, t)
				query += fmt.Sprintf(" AND pu.used_at >= $%d", len(args))
			}
		}
		if req.PeriodTo != "" {
			if t, err := time.Parse(time.RFC3339, req.PeriodTo); err == nil {
				args = append(args, t)
				query += fmt.Sprintf(" AND pu.used_at <= $%d", len(args))
			}
		}
		const wroteExists = ` EXISTS (
			SELECT 1 FROM dialogs_messages dm
			JOIN users_dialogs ud ON ud.id = dm.dialog_id
			WHERE ud.user_id = pu.user_id AND dm.role = 'user')`
		if wantWrote != wantNotWrote {
			if wantWrote {
				query += " AND" + wroteExists
			} else {
				query += " AND NOT" + wroteExists
			}
		}
		query += " ORDER BY pu.user_id"
		rr, err := h.DB.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for rr.Next() {
			var id int64
			if err := rr.Scan(&id); err != nil {
				return nil, err
			}
			rows = append(rows, id)
		}
		return rows, rr.Err()

	case "mode":
		ids := uniquePositiveIDs(req.ModeIDs)
		if len(ids) == 0 {
			return nil, nil
		}
		if req.ModeFilter == "wrote" {
			// Actually wrote: there is a message with role='user' in a dialog of this mode
			var periodFrom, periodTo *time.Time
			if req.PeriodFrom != "" {
				t, err := time.Parse(time.RFC3339, req.PeriodFrom)
				if err == nil {
					periodFrom = &t
				}
			}
			if req.PeriodTo != "" {
				t, err := time.Parse(time.RFC3339, req.PeriodTo)
				if err == nil {
					periodTo = &t
				}
			}
			query := `
				SELECT DISTINCT ud.user_id FROM dialogs_messages dm
				JOIN users_dialogs ud ON ud.id = dm.dialog_id
				JOIN users u ON u.id = ud.user_id
				WHERE ud.mode_id = ANY($1::bigint[]) AND dm.role = 'user'
				  AND u.deleted_at IS NULL AND u.status = 'active'`
			args := []any{ids}
			if periodFrom != nil {
				args = append(args, periodFrom)
				query += fmt.Sprintf(" AND dm.created_at >= $%d", len(args))
			}
			if periodTo != nil {
				args = append(args, periodTo)
				query += fmt.Sprintf(" AND dm.created_at <= $%d", len(args))
			}
			query += " ORDER BY ud.user_id"
			rr, err := h.DB.Query(ctx, query, args...)
			if err != nil {
				return nil, err
			}
			defer rr.Close()
			for rr.Next() {
				var id int64
				if err := rr.Scan(&id); err != nil {
					return nil, err
				}
				rows = append(rows, id)
			}
			return rows, rr.Err()
		}
		// "has": there is active access to the mode (or there was during the period)
		query := `
			SELECT DISTINCT uma.user_id FROM user_mode_access uma
			JOIN users u ON u.id = uma.user_id
			WHERE uma.mode_id = ANY($1::bigint[]) AND u.deleted_at IS NULL AND u.status = 'active'`
		args := []any{ids}
		if req.PeriodFrom != "" {
			t, err := time.Parse(time.RFC3339, req.PeriodFrom)
			if err == nil {
				args = append(args, t)
				query += fmt.Sprintf(" AND uma.active_from <= $%d", len(args))
			}
		}
		if req.PeriodTo != "" {
			t, err := time.Parse(time.RFC3339, req.PeriodTo)
			if err == nil {
				args = append(args, t)
				query += fmt.Sprintf(" AND (uma.active_to IS NULL OR uma.active_to >= $%d)", len(args))
			}
		}
		query += " ORDER BY uma.user_id"
		rr, err := h.DB.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rr.Close()
		for rr.Next() {
			var id int64
			if err := rr.Scan(&id); err != nil {
				return nil, err
			}
			rows = append(rows, id)
		}
		return rows, rr.Err()

	default:
		return nil, nil
	}
}

type adminNotificationSendResult struct {
	InboxIDs            []int64        `json:"inboxIds"`
	RecipientIDs        []int64        `json:"recipientIds"`
	RecipientCount      int            `json:"recipientCount"`
	DedupedRecipientHit int            `json:"dedupedRecipientHit"`
	StatusCounts        map[string]int `json:"statusCounts"`
	AttemptStatus       string         `json:"attemptStatus"`
}

func (r adminNotificationSendResult) response() map[string]any {
	payload := map[string]any{
		"ok":                  true,
		"inboxIds":            r.InboxIDs,
		"recipientIds":        r.RecipientIDs,
		"recipientCount":      r.RecipientCount,
		"dedupedRecipientHit": r.DedupedRecipientHit,
		"statusCounts":        r.StatusCounts,
		"attemptStatus":       r.AttemptStatus,
	}
	if len(r.InboxIDs) > 0 {
		payload["inboxId"] = r.InboxIDs[0]
	}
	return payload
}

func (h Handler) notificationPreferencesPayload(ctx context.Context, userID int64) (map[string]any, error) {
	if err := h.ensureNotificationDefaults(ctx, userID); err != nil {
		return nil, err
	}
	contacts, err := h.notificationContacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	consents, err := h.notificationConsents(ctx, userID)
	if err != nil {
		return nil, err
	}
	reachability, err := h.notificationReachability(ctx, userID)
	if err != nil {
		return nil, err
	}
	inbox, err := h.notificationInbox(ctx, userID, 5)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "contacts": contacts, "consents": consents, "reachability": reachability, "inbox": inbox}, nil
}

func (h Handler) ensureNotificationDefaults(ctx context.Context, userID int64) error {
	if _, err := h.DB.Exec(ctx, `
		insert into notification_channel_consents (user_id, channel, consent_type, status, source, reason, granted_at)
		values ($1, 'in_site', 'service', 'granted', 'migration', 'Служебные уведомления внутри сервиса', now())
		on conflict (user_id, channel, consent_type) do nothing`, userID); err != nil {
		return err
	}
	return h.refreshNotificationReachability(ctx, userID)
}

// setNotificationConsentWithBonus stores the consent and on the FIRST manual
// enabling of the subscription (source=profile) grants bonus messages to the daily
// limit, as for binding Max. The notification_consent_bonuses ledger
// guarantees it happens once: turning it off and on again does not repeat the bonus.
func (h Handler) setNotificationConsentWithBonus(ctx context.Context, userID int64, channel, consentType string, granted bool, source, reason string) (int64, error) {
	status := "denied"
	var grantedAt, revokedAt any
	if granted {
		status = "granted"
		grantedAt = time.Now()
	} else {
		revokedAt = time.Now()
	}
	if _, err := h.DB.Exec(ctx, `
		insert into notification_channel_consents (user_id, channel, consent_type, status, source, reason, granted_at, revoked_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, now())
		on conflict (user_id, channel, consent_type) do update
		set status = excluded.status,
		    source = excluded.source,
		    reason = excluded.reason,
		    granted_at = excluded.granted_at,
		    revoked_at = excluded.revoked_at,
		    updated_at = now()`,
		userID, channel, consentType, status, source, strings.TrimSpace(reason), grantedAt, revokedAt); err != nil {
		return 0, err
	}

	var bonus int64
	if granted && source == "profile" {
		// INSERT ... ON CONFLICT DO NOTHING RETURNING returns a row only
		// on the first insert; that is exactly "the first time the subscription is enabled".
		var inserted bool
		err := h.DB.QueryRow(ctx, `
			insert into notification_consent_bonuses (user_id, channel, consent_type)
			values ($1, $2, $3)
			on conflict (user_id, channel, consent_type) do nothing
			returning true`, userID, channel, consentType).Scan(&inserted)
		if err == nil && inserted {
			granted, gerr := h.grantDailyLimitBonus(ctx, userID)
			if gerr == nil {
				bonus = granted
				_, _ = h.DB.Exec(ctx, `
					update notification_consent_bonuses
					set bonus_messages = $4
					where user_id = $1 and channel = $2 and consent_type = $3`,
					userID, channel, consentType, bonus)
			}
		}
	}
	return bonus, h.refreshNotificationReachability(ctx, userID)
}

func (h Handler) notificationContacts(ctx context.Context, userID int64) ([]notificationContact, error) {
	rows, err := h.DB.Query(ctx, `
		select channel, address, verified, source, last_seen_at
		from notification_contacts
		where user_id = $1
		order by channel, verified desc, updated_at desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notificationContact{}
	for rows.Next() {
		var item notificationContact
		if err := rows.Scan(&item.Channel, &item.Address, &item.Verified, &item.Source, &item.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) notificationConsents(ctx context.Context, userID int64) ([]notificationConsent, error) {
	rows, err := h.DB.Query(ctx, `
		select channel, consent_type, status, source, reason, granted_at, revoked_at
		from notification_channel_consents
		where user_id = $1
		order by consent_type, channel`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notificationConsent{}
	for rows.Next() {
		var item notificationConsent
		if err := rows.Scan(&item.Channel, &item.ConsentType, &item.Status, &item.Source, &item.Reason, &item.GrantedAt, &item.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) notificationInbox(ctx context.Context, userID int64, limit int) ([]notificationInboxItem, error) {
	rows, err := h.DB.Query(ctx, `
		select id, coalesce(template_key, ''), consent_type, title, body, status, created_at
		from notification_inbox
		where user_id = $1
		order by created_at desc, id desc
		limit $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notificationInboxItem{}
	for rows.Next() {
		var item notificationInboxItem
		if err := rows.Scan(&item.ID, &item.TemplateKey, &item.ConsentType, &item.Title, &item.Body, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) notificationReachability(ctx context.Context, userID int64) (notificationReachability, error) {
	if err := h.refreshNotificationReachability(ctx, userID); err != nil {
		return notificationReachability{}, err
	}
	var out notificationReachability
	var serviceRaw, marketingRaw []byte
	err := h.DB.QueryRow(ctx, `
		select email_available, phone_verified, push_enabled, service_channels, marketing_channels, best_channel, coalesce(last_interaction_channel, ''), last_interaction_at
		from notification_reachability
		where user_id = $1`, userID).Scan(&out.EmailAvailable, &out.PhoneVerified, &out.PushEnabled, &serviceRaw, &marketingRaw, &out.BestChannel, &out.LastInteractionChannel, &out.LastInteractionAt)
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal(serviceRaw, &out.ServiceChannels)
	_ = json.Unmarshal(marketingRaw, &out.MarketingChannels)
	return out, nil
}

func (h Handler) refreshNotificationReachability(ctx context.Context, userID int64) error {
	_, err := h.DB.Exec(ctx, `
		with
		  contacts as (
		    select
		      exists(select 1 from notification_contacts where user_id = $1 and channel = 'email') as email_available,
		      exists(select 1 from notification_contacts where user_id = $1 and channel = 'phone' and verified) as phone_verified,
		      exists(select 1 from notification_contacts where user_id = $1 and channel = 'push' and verified) as push_enabled
		  ),
		  service as (
		    select coalesce(jsonb_agg(channel order by channel), '[]'::jsonb) as channels
		    from notification_channel_consents
		    where user_id = $1 and consent_type = 'service' and status = 'granted'
		  ),
		  marketing as (
		    select coalesce(jsonb_agg(channel order by channel), '[]'::jsonb) as channels
		    from notification_channel_consents
		    where user_id = $1 and consent_type = 'marketing' and status = 'granted'
		  )
		insert into notification_reachability (user_id, email_available, phone_verified, push_enabled, service_channels, marketing_channels, best_channel, updated_at)
		select
		  $1,
		  contacts.email_available,
		  contacts.phone_verified,
		  contacts.push_enabled,
		  service.channels,
		  marketing.channels,
		  case
		    when service.channels ? 'email' and contacts.email_available then 'email'
		    else 'in_site'
		  end,
		  now()
		from contacts, service, marketing
		on conflict (user_id) do update
		set email_available = excluded.email_available,
		    phone_verified = excluded.phone_verified,
		    push_enabled = excluded.push_enabled,
		    service_channels = excluded.service_channels,
		    marketing_channels = excluded.marketing_channels,
		    best_channel = excluded.best_channel,
		    updated_at = now()`, userID)
	return err
}

func (h Handler) notificationTemplates(ctx context.Context) ([]notificationTemplate, error) {
	rows, err := h.DB.Query(ctx, `
		select key, name, consent_type, default_channel, title, body, active, updated_at
		from notification_templates
		order by key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []notificationTemplate{}
	for rows.Next() {
		var item notificationTemplate
		if err := rows.Scan(&item.Key, &item.Name, &item.ConsentType, &item.DefaultChannel, &item.Title, &item.Body, &item.Active, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h Handler) upsertNotificationTemplate(ctx context.Context, req adminNotificationTemplateUpsert) (notificationTemplate, error) {
	key := strings.TrimSpace(req.Key)
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	if !validNotificationTemplateKey(key) || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" || !validReadyNotificationChannel(req.DefaultChannel) || !validConsentType(req.ConsentType) {
		return notificationTemplate{}, pgx.ErrNoRows
	}
	var tpl notificationTemplate
	err := h.DB.QueryRow(ctx, `
		insert into notification_templates (key, name, consent_type, default_channel, title, body, active, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, now())
		on conflict (key) do update
		set name = excluded.name,
		    consent_type = excluded.consent_type,
		    default_channel = excluded.default_channel,
		    title = excluded.title,
		    body = excluded.body,
		    active = excluded.active,
		    updated_at = now()
		returning key, name, consent_type, default_channel, title, body, active, updated_at`,
		key, strings.TrimSpace(req.Name), req.ConsentType, req.DefaultChannel, strings.TrimSpace(req.Title), strings.TrimSpace(req.Body), active,
	).Scan(&tpl.Key, &tpl.Name, &tpl.ConsentType, &tpl.DefaultChannel, &tpl.Title, &tpl.Body, &tpl.Active, &tpl.UpdatedAt)
	return tpl, err
}

func (h Handler) createTestNotification(ctx context.Context, req adminNotificationTestRequest) (int64, string, error) {
	if !validReadyNotificationChannel(req.Channel) {
		return 0, "", pgx.ErrNoRows
	}
	var tpl notificationTemplate
	err := h.DB.QueryRow(ctx, `
		select key, name, consent_type, default_channel, title, body, active, updated_at
		from notification_templates
		where key = $1 and active`, strings.TrimSpace(req.TemplateKey)).
		Scan(&tpl.Key, &tpl.Name, &tpl.ConsentType, &tpl.DefaultChannel, &tpl.Title, &tpl.Body, &tpl.Active, &tpl.UpdatedAt)
	if err != nil {
		return 0, "", err
	}
	if req.Channel == "" {
		req.Channel = tpl.DefaultChannel
	}
	title := renderNotificationTemplate(tpl.Title, req.Variables)
	body := renderNotificationTemplate(tpl.Body, req.Variables)
	payload, _ := json.Marshal(map[string]any{"test": true, "channel": req.Channel, "variables": req.Variables})

	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx)
	var inboxID int64
	if err := tx.QueryRow(ctx, `
		insert into notification_inbox (user_id, template_key, consent_type, title, body, payload)
		values ($1, $2, $3, $4, $5, $6::jsonb)
		returning id`, req.UserID, tpl.Key, tpl.ConsentType, title, body, string(payload)).Scan(&inboxID); err != nil {
		return 0, "", err
	}
	attemptStatus := "sent"
	if req.Channel != "in_site" {
		var granted bool
		_ = tx.QueryRow(ctx, `
			select exists(
			  select 1 from notification_channel_consents
			  where user_id = $1 and channel = $2 and consent_type = $3 and status = 'granted'
			)`, req.UserID, req.Channel, tpl.ConsentType).Scan(&granted)
		if !granted {
			attemptStatus = "skipped_no_consent"
		} else {
			attemptStatus = "queued"
		}
	}
	if _, err := tx.Exec(ctx, `
		insert into notification_delivery_attempts (inbox_id, user_id, channel, status)
		values ($1, $2, $3, $4)`, inboxID, req.UserID, req.Channel, attemptStatus); err != nil {
		return 0, "", err
	}
	if _, err := tx.Exec(ctx, `
		update notification_reachability
		set last_interaction_channel = $2, last_interaction_at = now(), updated_at = now()
		where user_id = $1`, req.UserID, req.Channel); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return inboxID, attemptStatus, nil
}

func (h Handler) createAdminNotifications(ctx context.Context, req adminNotificationTestRequest) (adminNotificationSendResult, error) {
	recipientIDs, deduped, err := h.resolveAdminNotificationRecipients(ctx, req)
	if err != nil {
		return adminNotificationSendResult{}, err
	}
	if len(recipientIDs) == 0 {
		return adminNotificationSendResult{}, errNotificationRecipientsRequired
	}
	result := adminNotificationSendResult{
		InboxIDs:            make([]int64, 0, len(recipientIDs)),
		RecipientIDs:        recipientIDs,
		RecipientCount:      len(recipientIDs),
		DedupedRecipientHit: deduped,
		StatusCounts:        map[string]int{},
	}
	for _, userID := range recipientIDs {
		itemReq := req
		itemReq.UserID = userID
		inboxID, status, err := h.createTestNotification(ctx, itemReq)
		if err != nil {
			return adminNotificationSendResult{}, err
		}
		result.InboxIDs = append(result.InboxIDs, inboxID)
		result.StatusCounts[status]++
		if result.AttemptStatus == "" {
			result.AttemptStatus = status
		} else if result.AttemptStatus != status {
			result.AttemptStatus = "mixed"
		}
	}
	return result, nil
}

func (h Handler) resolveAdminNotificationRecipients(ctx context.Context, req adminNotificationTestRequest) ([]int64, int, error) {
	explicitIDs := append([]int64{}, req.UserIDs...)
	if req.UserID > 0 {
		explicitIDs = append(explicitIDs, req.UserID)
	}
	explicitIDs = uniquePositiveIDs(explicitIDs)
	promoIDs := uniquePositiveIDs(req.PromocodeIDs)

	rawHits := []int64{}
	if len(explicitIDs) > 0 {
		rows, err := h.DB.Query(ctx, `
			select id
			from users
			where id = any($1::bigint[])
			  and deleted_at is null
			  and status = 'active'
			order by id`, explicitIDs)
		if err != nil {
			return nil, 0, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, 0, err
			}
			rawHits = append(rawHits, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, 0, err
		}
		rows.Close()
	}
	if len(promoIDs) > 0 {
		rows, err := h.DB.Query(ctx, `
			select pu.user_id
			from promocode_usages pu
			join users u on u.id = pu.user_id
			where pu.promocode_id = any($1::bigint[])
			  and u.deleted_at is null
			  and u.status = 'active'
			order by pu.user_id`, promoIDs)
		if err != nil {
			return nil, 0, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, 0, err
			}
			rawHits = append(rawHits, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, 0, err
		}
		rows.Close()
	}
	unique := uniquePositiveIDs(rawHits)
	return unique, len(rawHits) - len(unique), nil
}

func (h Handler) upsertNotificationContact(ctx context.Context, userID int64, channel, address string, verified bool, source string) error {
	address = strings.TrimSpace(address)
	if userID <= 0 || address == "" || !validNotificationContactChannel(channel) {
		return nil
	}
	if _, err := h.DB.Exec(ctx, `
		insert into notification_contacts (user_id, channel, address, verified, source, last_seen_at, updated_at)
		values ($1, $2, $3, $4, $5, now(), now())
		on conflict (user_id, channel, address) do update
		set verified = notification_contacts.verified or excluded.verified,
		    source = excluded.source,
		    last_seen_at = now(),
		    updated_at = now()`,
		userID, channel, address, verified, source); err != nil {
		return err
	}
	return h.refreshNotificationReachability(ctx, userID)
}

func (h Handler) upsertYandexNotificationContacts(ctx context.Context, userID int64, p OAuthProfile) {
	if normalizeEmail(p.Email) != "" {
		_ = h.upsertNotificationContact(ctx, userID, "email", normalizeEmail(p.Email), p.EmailVerified, "yandex_id")
	}
	if phone := normalizePhone(p.Phone); phone != "" {
		_ = h.upsertNotificationContact(ctx, userID, "phone", phone, p.PhoneVerified, "yandex_id")
	}
}

func validNotificationContactChannel(channel string) bool {
	switch channel {
	case "email", "phone", "push":
		return true
	default:
		return false
	}
}

func validReadyNotificationChannel(channel string) bool {
	return channel == "in_site" || channel == "email"
}

func validConsentType(value string) bool {
	return value == "service" || value == "marketing"
}

var notificationTemplateKeyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,120}$`)

func validNotificationTemplateKey(value string) bool {
	return notificationTemplateKeyRE.MatchString(value)
}

func renderNotificationTemplate(text string, vars map[string]string) string {
	out := text
	for key, value := range vars {
		if key == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
		out = strings.ReplaceAll(out, "{{ "+key+" }}", value)
	}
	return out
}

func normalizePhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	return replacer.Replace(value)
}
