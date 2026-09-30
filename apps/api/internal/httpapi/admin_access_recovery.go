package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type adminAccessRecoveryTransferRequest struct {
	InvoiceID    int64  `json:"invoiceId"`
	PaymentID    string `json:"paymentId"`
	TargetUserID int64  `json:"targetUserId"`
	TargetEmail  string `json:"targetEmail"`
	Reason       string `json:"reason"`
}

func (h Handler) AdminAccessRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.AdminAccessRecoverySearch(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.AdminAccessRecoveryTransfer(w, r)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
}

func (h Handler) AdminAccessRecoverySearch(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "billing", false)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": []map[string]any{}})
		return
	}
	like := "%" + strings.ToLower(q) + "%"
	numericID, _ := strconv.ParseInt(q, 10, 64)
	rows, err := h.DB.Query(r.Context(), `
		with invoice_access as (
		  select
		    i.id as invoice_id,
		    count(uma.id) as access_count,
		    coalesce(sum(dmu.messages_used), 0) + count(mu.id) as usage_count,
		    max(uma.active_to) as access_active_to
		  from invoices i
		  left join user_mode_access uma on uma.source_id = i.id and uma.access_type = 'subscription'
		  left join daily_mode_usage dmu on dmu.access_id = uma.id
		  left join message_usage mu on mu.access_id = uma.id
		  group by i.id
		)
		select i.id, i.user_id, coalesce(u.email, ''), coalesce(u.phone, ''),
		       i.tariff_id, coalesce(t.name, ''), i.yookassa_payment_id,
		       i.amount::text, i.currency, i.status, i.subscription_months,
		       coalesce(i.autorenew_requested, false), coalesce(i.yookassa_payment_method_id, ''),
		       i.created_at::text, coalesce(i.paid_at::text, ''),
		       coalesce(ia.access_count, 0), coalesce(ia.usage_count, 0),
		       coalesce(ia.access_active_to::text, '')
		from invoices i
		join users u on u.id = i.user_id
		left join tariffs t on t.id = i.tariff_id
		left join invoice_access ia on ia.invoice_id = i.id
		where i.id = $1
		   or i.user_id = $1
		   or lower(i.yookassa_payment_id) like $2
		   or lower(coalesce(u.email, '')) like $2
		   or lower(coalesce(u.phone, '')) like $2
		order by i.created_at desc
		limit 25`, numericID, like)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var invoiceID, userID, tariffID int64
		var email, phone, tariffName, paymentID, amount, currency, status, methodID, createdAt, paidAt, accessActiveTo string
		var months int
		var autorenew bool
		var accessCount, usageCount int64
		if err := rows.Scan(&invoiceID, &userID, &email, &phone, &tariffID, &tariffName, &paymentID, &amount, &currency, &status, &months, &autorenew, &methodID, &createdAt, &paidAt, &accessCount, &usageCount, &accessActiveTo); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		items = append(items, map[string]any{
			"invoiceId":               invoiceID,
			"userId":                  userID,
			"email":                   email,
			"phone":                   phone,
			"tariffId":                tariffID,
			"tariffName":              tariffName,
			"paymentId":               paymentID,
			"amount":                  amount,
			"currency":                currency,
			"status":                  status,
			"subscriptionMonths":      months,
			"autoRenewRequested":      autorenew,
			"hasSavedPaymentMethod":   strings.TrimSpace(methodID) != "",
			"createdAt":               createdAt,
			"paidAt":                  paidAt,
			"accessCount":             accessCount,
			"usageCount":              usageCount,
			"accessActiveTo":          accessActiveTo,
			"transferableWithoutRisk": status == "paid" && accessCount > 0 && usageCount == 0,
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.billing.access_recovery.search", "invoice", nil, map[string]any{
		"queryLength": len(q),
		"numeric":     numericID > 0,
		"resultCount": len(items),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

func (h Handler) AdminAccessRecoveryTransfer(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireAdminSection(w, r, "billing", true)
	if !ok {
		return
	}
	var req adminAccessRecoveryTransferRequest
	if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	result, err := h.transferUnusedPaidAccess(r.Context(), r, actor.ID, req)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "already used") {
			status = http.StatusConflict
		}
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	result["ok"] = true
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) transferUnusedPaidAccess(ctx context.Context, r *http.Request, actorID int64, req adminAccessRecoveryTransferRequest) (map[string]any, error) {
	if req.InvoiceID <= 0 && strings.TrimSpace(req.PaymentID) == "" {
		return nil, fmt.Errorf("invoiceId or paymentId is required")
	}
	if req.TargetUserID <= 0 && strings.TrimSpace(req.TargetEmail) == "" {
		return nil, fmt.Errorf("targetUserId or targetEmail is required")
	}
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	targetUserID := req.TargetUserID
	if targetUserID <= 0 {
		err := tx.QueryRow(ctx, `
			select id from users
			where lower(email) = lower($1) and deleted_at is null and status = 'active'
			order by id desc limit 1`, strings.TrimSpace(req.TargetEmail)).Scan(&targetUserID)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("target user not found")
		}
		if err != nil {
			return nil, err
		}
	}

	var invoiceID, sourceUserID, tariffID int64
	var paymentID, status, methodID string
	err = tx.QueryRow(ctx, `
		select id, user_id, tariff_id, yookassa_payment_id, status, coalesce(yookassa_payment_method_id, '')
		from invoices
		where ($1::bigint = 0 or id = $1)
		  and ($2 = '' or yookassa_payment_id = $2)
		order by id desc
		limit 1
		for update`, req.InvoiceID, strings.TrimSpace(req.PaymentID)).Scan(&invoiceID, &sourceUserID, &tariffID, &paymentID, &status, &methodID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invoice not found")
	}
	if err != nil {
		return nil, err
	}
	if status != "paid" {
		return nil, fmt.Errorf("paid invoice not found")
	}

	var usageCount int64
	if err := tx.QueryRow(ctx, `
		select coalesce(sum(dmu.messages_used), 0) + count(mu.id)
		from user_mode_access uma
		left join daily_mode_usage dmu on dmu.access_id = uma.id
		left join message_usage mu on mu.access_id = uma.id
		where uma.source_id = $1 and uma.access_type = 'subscription'`, invoiceID).Scan(&usageCount); err != nil {
		return nil, err
	}
	if usageCount > 0 {
		return nil, fmt.Errorf("paid access already used")
	}

	ct, err := tx.Exec(ctx, `
		insert into user_mode_access (
			user_id, mode_id, active_from, active_to, daily_message_limit,
			priority, access_type, source_id, created_at, updated_at
		)
		select $2, mode_id, greatest(active_from, now()), active_to, daily_message_limit,
		       priority, access_type, source_id, now(), now()
		from user_mode_access
		where source_id = $1
		  and access_type = 'subscription'
		  and active_to > now()
		on conflict on constraint uq_user_mode_access_source do update
		set active_to = greatest(user_mode_access.active_to, excluded.active_to),
		    daily_message_limit = greatest(user_mode_access.daily_message_limit, excluded.daily_message_limit),
		    updated_at = now()`, invoiceID, targetUserID)
	if err != nil {
		return nil, err
	}
	transferredAccess := ct.RowsAffected()
	if transferredAccess == 0 {
		return nil, fmt.Errorf("active access not found")
	}
	if targetUserID != sourceUserID {
		if _, err := tx.Exec(ctx, `
			update user_mode_access
			set active_to = least(active_to, now()), updated_at = now()
			where source_id = $1 and access_type = 'subscription' and user_id = $2`, invoiceID, sourceUserID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `update invoices set user_id = $2, updated_at = now() where id = $1`, invoiceID, targetUserID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		update subscriptions
		set user_id = $1, updated_at = now()
		where user_id = $2 and tariff_id = $3 and payment_reference = $4`,
		targetUserID, sourceUserID, tariffID, paymentID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(methodID) != "" {
		if _, err := tx.Exec(ctx, `
			update payment_methods set user_id = $2, updated_at = now()
			where yookassa_payment_method_id = $1 and user_id = $3`, methodID, targetUserID, sourceUserID); err != nil {
			return nil, err
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "lost_cookie_recovery"
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	h.writeAdminAudit(ctx, r, actorID, "admin.billing.access_recovery.transfer", "invoice", &invoiceID, map[string]any{
		"sourceUserId": sourceUserID,
		"targetUserId": targetUserID,
		"paymentId":    paymentID,
		"reason":       reason,
	})
	return map[string]any{
		"invoiceId":         invoiceID,
		"sourceUserId":      sourceUserID,
		"targetUserId":      targetUserID,
		"transferredAccess": transferredAccess,
		"usageCount":        usageCount,
	}, nil
}
