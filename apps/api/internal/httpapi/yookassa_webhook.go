package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	webhookapi "mindstrata-stage1/api/internal/httpapi/webhook"
)

// errYooKassaPaymentNotFound: YooKassa returned 404 on GET /payments/{id}:
// the payment physically does not exist on its side (stale, or never
// created where our invoice points) and will never answer otherwise
// in the future. Distinguished from other errors (5xx/timeout/network), which
// may be temporary and deserve a retry the next day.
var errYooKassaPaymentNotFound = errors.New("yookassa payment not found")

// extractRemoteIP extracts the client's real IP. S-NEW-4: XFF/X-Real-IP
// are taken into account only if the request came from a trusted proxy. Otherwise any
// external client could fake a YooKassa IP via headers.
// The trusted proxy list comes from Handler.TrustedProxyIPs (empty = legacy).
func (h Handler) extractRemoteIP(r *http.Request) string {
	return clientIPWithTrust(r, parseTrustedProxyCIDRs(h.TrustedProxyIPs))
}

// isYooKassaIP checks that the IP falls into one of the trusted ranges.
func isYooKassaIP(ipStr string) bool {
	return webhookapi.IsYooKassaIP(ipStr)
}

// S-1: a typed payload instead of map[string]any
// prevents bugs from unchecked type assertions.
type yooKassaWebhookPayload struct {
	Event  string             `json:"event"`
	Object yooKassaObjectData `json:"object"`
}

type yooKassaObjectData struct {
	ID                  string                    `json:"id"`
	Status              string                    `json:"status,omitempty"`
	PaymentID           string                    `json:"payment_id,omitempty"`
	Metadata            map[string]string         `json:"metadata,omitempty"`
	Amount              *yooKassaAmount           `json:"amount,omitempty"`
	PaymentMethod       yooKassaPaymentMethodData `json:"payment_method,omitempty"`
	CancellationDetails *yooKassaCancellationData `json:"cancellation_details,omitempty"`
}

type yooKassaAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type yooKassaPaymentMethodData struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Saved bool   `json:"saved"`
}

type yooKassaCancellationData struct {
	Party  string `json:"party"`
	Reason string `json:"reason"`
}

type yooKassaWebhookOutcome string

const (
	yooKassaOutcomeAccepted   yooKassaWebhookOutcome = "accepted"
	yooKassaOutcomeDuplicate  yooKassaWebhookOutcome = "duplicate"
	yooKassaOutcomeIgnored    yooKassaWebhookOutcome = "ignored"
	yooKassaOutcomeRetryable  yooKassaWebhookOutcome = "retryable"
	yooKassaOutcomeMalformed  yooKassaWebhookOutcome = "malformed"
	yooKassaOutcomeSuspicious yooKassaWebhookOutcome = "suspicious"
)

type yooKassaWebhookResult struct {
	Outcome    yooKassaWebhookOutcome
	StatusCode int
	OK         bool
	Error      string
}

func yooKassaWebhookResponse(w http.ResponseWriter, result yooKassaWebhookResult) {
	body := map[string]any{
		"ok":      result.OK,
		"outcome": string(result.Outcome),
	}
	if result.Error != "" {
		body["error"] = result.Error
	}
	writeJSON(w, result.StatusCode, body)
}

func (h Handler) fetchYooKassaPayment(ctx context.Context, paymentID string) (yooKassaObjectData, bool, error) {
	var out yooKassaObjectData
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return out, false, fmt.Errorf("payment id is required")
	}
	creds, err := h.activeYooKassaCredentials(ctx)
	if err != nil {
		return out, false, err
	}
	if !creds.configured() {
		return out, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.yookassa.ru/v3/payments/"+url.PathEscape(paymentID), nil)
	if err != nil {
		return out, true, err
	}
	req.Header.Set("Authorization", creds.basicAuth())
	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, true, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return out, true, fmt.Errorf("decode yookassa payment: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return out, true, fmt.Errorf("%w: %s", errYooKassaPaymentNotFound, paymentID)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, true, fmt.Errorf("yookassa payment status %d", resp.StatusCode)
	}
	if strings.TrimSpace(out.ID) == "" {
		return out, true, fmt.Errorf("yookassa payment response missing id")
	}
	if out.ID != paymentID {
		return out, true, fmt.Errorf("yookassa payment id mismatch: got %q", out.ID)
	}
	return out, true, nil
}

func (h Handler) verifyYooKassaPaymentStatus(ctx context.Context, event, paymentID, expectedStatus string) (yooKassaObjectData, bool) {
	remote, checked, err := h.fetchYooKassaPayment(ctx, paymentID)
	if err != nil {
		log.Printf("yookassa webhook: remote verify %s %q failed: %v", event, paymentID, err)
		h.markWebhookOutcome(ctx, event, paymentID, yooKassaOutcomeRetryable)
		return remote, false
	}
	if !checked {
		log.Printf("yookassa webhook: remote verify %s %q skipped because YooKassa credentials are not configured", event, paymentID)
		return remote, true
	}
	if strings.TrimSpace(remote.Status) != expectedStatus {
		log.Printf("yookassa webhook: remote verify %s %q status mismatch: got %q want %q", event, paymentID, remote.Status, expectedStatus)
		h.markWebhookOutcome(ctx, event, paymentID, yooKassaOutcomeSuspicious)
		return remote, false
	}
	return remote, true
}

// YooKassaWebhook receives notifications with built-in protection:
//  1. HTTPS + TLS 1.2+ (a YooKassa requirement, fronted by Caddy)
//  2. IP allowlist: the official YooKassa CIDRs from the docs
//     https://yookassa.ru/developers/using-api/webhooks
//  3. Idempotency: a repeated delivery of the same event -> 200, without reprocessing
//
// An HMAC / Signing Secret for HTTP Basic Auth does NOT exist in YooKassa;
// verified against the official documentation (the "Authenticity check" section).
// An additional officially recommended step (TODO/future) is checking the object's
// status via GET /v3/{type}/{id} to make sure the status from the notification
// is current. Enable it after the first real payment processing.
//
// YooKassa requires a fast 200 OK (within 30 s), otherwise it retries for 24 h.
func (h Handler) YooKassaWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	// 1. IP allowlist (the official list from the docs).
	// S-NEW-4: if TrustedProxyIPs are configured, check that XFF/XRI comes
	// from Caddy. Otherwise XFF is ignored and r.RemoteAddr is used.
	remoteIP := h.extractRemoteIP(r)
	if !isYooKassaIP(remoteIP) {
		log.Printf("yookassa webhook: outcome=%s rejected non-YooKassa IP %q", yooKassaOutcomeSuspicious, remoteIP)
		yooKassaWebhookResponse(w, yooKassaWebhookResult{
			Outcome:    yooKassaOutcomeSuspicious,
			StatusCode: http.StatusForbidden,
			OK:         false,
			Error:      "forbidden",
		})
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		log.Printf("yookassa webhook: outcome=%s body read failed ip=%s: %v", yooKassaOutcomeMalformed, remoteIP, err)
		yooKassaWebhookResponse(w, yooKassaWebhookResult{
			Outcome:    yooKassaOutcomeMalformed,
			StatusCode: http.StatusBadRequest,
			OK:         false,
			Error:      "body read failed",
		})
		return
	}

	// S-1: a typed struct, so an unchecked type assertion is no longer needed.
	var payload yooKassaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("yookassa webhook: outcome=%s invalid payload ip=%s: %v", yooKassaOutcomeMalformed, remoteIP, err)
		yooKassaWebhookResponse(w, yooKassaWebhookResult{
			Outcome:    yooKassaOutcomeMalformed,
			StatusCode: http.StatusBadRequest,
			OK:         false,
			Error:      "invalid payload",
		})
		return
	}
	event := payload.Event
	objectID := payload.Object.ID
	if strings.TrimSpace(event) == "" || strings.TrimSpace(objectID) == "" {
		log.Printf("yookassa webhook: outcome=%s event=%q object.id=%q ip=%s", yooKassaOutcomeMalformed, event, objectID, remoteIP)
		yooKassaWebhookResponse(w, yooKassaWebhookResult{
			Outcome:    yooKassaOutcomeMalformed,
			StatusCode: http.StatusBadRequest,
			OK:         false,
			Error:      "missing event or object.id",
		})
		return
	}
	log.Printf("yookassa webhook: event=%q object.id=%q ip=%s", event, objectID, remoteIP)

	result := yooKassaWebhookResult{
		Outcome:    yooKassaOutcomeAccepted,
		StatusCode: http.StatusOK,
		OK:         true,
	}
	if h.DB != nil && event != "" && objectID != "" {
		// Idempotency: ON CONFLICT (event, object_id) DO NOTHING.
		// If already seen, 200 OK without action.
		// If new, store it and run the event-specific logic.
		ctx, cancel := contextWithTimeout(r, 3*time.Second)
		defer cancel()
		ct, execErr := h.DB.Exec(ctx, `
			insert into yookassa_webhook_events (event, object_id, payload, remote_ip)
			values ($1, $2, $3::jsonb, $4)
			on conflict (event, object_id) do nothing`,
			event, objectID, string(body), remoteIP)
		if execErr != nil {
			log.Printf("yookassa webhook: outcome=%s persist failed event=%q object.id=%q: %v", yooKassaOutcomeRetryable, event, objectID, execErr)
			// K-3: return 500 so YooKassa redelivers:
			// the event was not saved and must not be lost.
			yooKassaWebhookResponse(w, yooKassaWebhookResult{
				Outcome:    yooKassaOutcomeRetryable,
				StatusCode: http.StatusInternalServerError,
				OK:         false,
				Error:      "internal error",
			})
			return
		}
		// K-NEW-3: on a duplicate (RowsAffected=0) check process_status.
		// If the handler's previous attempt failed, retry it.
		needsHandling := ct.RowsAffected() != 0
		if !needsHandling {
			var procStatus string
			_ = h.DB.QueryRow(ctx, `select coalesce(process_status, 'pending') from yookassa_webhook_events where event=$1 and object_id=$2`, event, objectID).Scan(&procStatus)
			if procStatus == "pending" || procStatus == "failed" || procStatus == string(yooKassaOutcomeRetryable) {
				needsHandling = true
				log.Printf("yookassa webhook: retry event=%q object.id=%q status=%q", event, objectID, procStatus)
			} else {
				result.Outcome = yooKassaOutcomeDuplicate
				log.Printf("yookassa webhook: outcome=%s event=%q object.id=%q already processed status=%q", yooKassaOutcomeDuplicate, event, objectID, procStatus)
			}
		}
		if needsHandling {
			switch event {
			case "payment.succeeded":
				if payload.Object.Status != "" && payload.Object.Status != "succeeded" {
					log.Printf("yookassa webhook: outcome=%s payment.succeeded %q has object.status=%q", yooKassaOutcomeSuspicious, objectID, payload.Object.Status)
					h.markWebhookOutcome(ctx, "payment.succeeded", objectID, yooKassaOutcomeSuspicious)
					result.Outcome = yooKassaOutcomeSuspicious
					break
				}
				if remote, ok := h.verifyYooKassaPaymentStatus(ctx, "payment.succeeded", objectID, "succeeded"); ok {
					if remote.ID != "" {
						payload.Object = remote
					}
					result = h.handlePaymentSucceeded(ctx, objectID, payload.Object)
				} else {
					result = h.webhookResultFromStoredOutcome(ctx, "payment.succeeded", objectID)
				}
			case "payment.canceled":
				if remote, ok := h.verifyYooKassaPaymentStatus(ctx, "payment.canceled", objectID, "canceled"); ok {
					if remote.ID != "" {
						payload.Object = remote
					}
					result = h.handlePaymentCanceled(ctx, objectID, payload.Object)
				} else {
					result = h.webhookResultFromStoredOutcome(ctx, "payment.canceled", objectID)
				}
			case "refund.succeeded":
				result = h.handleRefundSucceeded(ctx, objectID, payload.Object)
			case "payment.waiting_for_capture", "payment_method.active":
				h.markWebhookOutcome(ctx, event, objectID, yooKassaOutcomeAccepted)
			default:
				log.Printf("yookassa webhook: outcome=%s unknown event=%q object.id=%q", yooKassaOutcomeSuspicious, event, objectID)
				h.markWebhookOutcome(ctx, event, objectID, yooKassaOutcomeSuspicious)
				result.Outcome = yooKassaOutcomeSuspicious
			}
		}
	}

	log.Printf("yookassa webhook: outcome=%s event=%q object.id=%q status=%d", result.Outcome, event, objectID, result.StatusCode)
	yooKassaWebhookResponse(w, result)
}

func (h Handler) webhookResultFromStoredOutcome(ctx context.Context, event, objectID string) yooKassaWebhookResult {
	var status string
	_ = h.DB.QueryRow(ctx, `
		select coalesce(process_status, 'retryable')
		from yookassa_webhook_events
		where event = $1 and object_id = $2`, event, objectID).Scan(&status)
	switch status {
	case string(yooKassaOutcomeSuspicious):
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeSuspicious, StatusCode: http.StatusOK, OK: true}
	case string(yooKassaOutcomeRetryable), "failed", "pending":
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	case string(yooKassaOutcomeIgnored):
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeIgnored, StatusCode: http.StatusOK, OK: true}
	default:
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeAccepted, StatusCode: http.StatusOK, OK: true}
	}
}

// handlePaymentSucceeded (N-1) processes a successful payment.
// Finds the invoice by yookassa_payment_id, updates the status, grants access.
func (h Handler) handlePaymentSucceeded(ctx context.Context, paymentID string, obj yooKassaObjectData) yooKassaWebhookResult {
	var invoiceID, userID, tariffID int64
	var subscriptionMonths int
	var amount string
	var autorenewRequested bool
	err := h.DB.QueryRow(ctx, `
		select id, user_id, tariff_id, subscription_months, amount::text, autorenew_requested from invoices
	where yookassa_payment_id = $1 and status = 'pending'`, paymentID).Scan(&invoiceID, &userID, &tariffID, &subscriptionMonths, &amount, &autorenewRequested)
	if err != nil {
		log.Printf("yookassa webhook: payment.succeeded %q — invoice not found or not pending: %v", paymentID, err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "invoice not ready"}
	}
	if subscriptionMonths <= 0 {
		subscriptionMonths = 1
	}

	// Grant access by tariff via user_mode_access.
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		log.Printf("yookassa webhook: payment.succeeded tx begin: %v", err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}
	defer tx.Rollback(ctx)

	modeIDs, err := queryModeIDs(ctx, tx, `
		select distinct mode_id from tariff_mode where tariff_id = $1 order by mode_id asc`, tariffID)
	if err != nil || len(modeIDs) == 0 {
		log.Printf("yookassa webhook: payment.succeeded — no modes for tariff %d: %v", tariffID, err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "tariff access not ready"}
	}

	activeTo := time.Now().AddDate(0, subscriptionMonths, 0)
	for _, modeID := range modeIDs {
		// access_type='subscription' is paid access, source_id=invoice for uniqueness
		if _, err := tx.Exec(ctx, `
			insert into user_mode_access
				(user_id, mode_id, active_from, active_to, access_type, source_id, priority, created_at, updated_at)
			values ($1, $2, now(), $3, 'subscription', $4, 0, now(), now())
			on conflict (user_id, mode_id, access_type, source_id) do nothing`,
			userID, modeID, activeTo, invoiceID); err != nil {
			log.Printf("yookassa webhook: grant access user=%d mode=%d: %v", userID, modeID, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		update invoices set status = 'paid', paid_at = now(), updated_at = now()
	where id = $1`, invoiceID); err != nil {
		log.Printf("yookassa webhook: invoice update: %v", err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}
	if autorenewRequested {
		if err := h.enableAutoRenewFromPaidInvoice(ctx, tx, invoiceID, userID, tariffID, paymentID, amount, subscriptionMonths, activeTo, obj.PaymentMethod); err != nil {
			log.Printf("yookassa webhook: enable autorenew payment=%s invoice=%d: %v", paymentID, invoiceID, err)
			h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
			return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
		}
	}
	if err := h.handleRecurringPaymentSucceeded(ctx, tx, paymentID, activeTo); err != nil {
		log.Printf("yookassa webhook: recurring success payment=%s: %v", paymentID, err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("yookassa webhook: payment.succeeded commit: %v", err)
		h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}

	h.markWebhookOutcome(ctx, "payment.succeeded", paymentID, yooKassaOutcomeAccepted)
	log.Printf("yookassa webhook: payment.succeeded user=%d tariff=%d modes=%v — доступ выдан", userID, tariffID, modeIDs)
	return yooKassaWebhookResult{Outcome: yooKassaOutcomeAccepted, StatusCode: http.StatusOK, OK: true}
}

func (h Handler) handlePaymentCanceled(ctx context.Context, paymentID string, obj yooKassaObjectData) yooKassaWebhookResult {
	reason := ""
	if obj.CancellationDetails != nil {
		reason = strings.TrimSpace(obj.CancellationDetails.Reason)
	}
	ct, err := h.DB.Exec(ctx, `
		update invoices
		set status = 'canceled', cancellation_reason = $2, updated_at = now()
	where yookassa_payment_id = $1 and status = 'pending'`, paymentID, reason)
	if err != nil {
		log.Printf("yookassa webhook: payment.canceled invoice update: %v", err)
		h.markWebhookOutcome(ctx, "payment.canceled", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}
	if err := h.handleRecurringPaymentCanceled(ctx, paymentID, reason); err != nil {
		log.Printf("yookassa webhook: recurring cancel payment=%s: %v", paymentID, err)
		h.markWebhookOutcome(ctx, "payment.canceled", paymentID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}
	if ct.RowsAffected() == 0 {
		log.Printf("yookassa webhook: payment.canceled %q — invoice not found or not pending", paymentID)
		h.markWebhookOutcome(ctx, "payment.canceled", paymentID, yooKassaOutcomeIgnored)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeIgnored, StatusCode: http.StatusOK, OK: true}
	}
	h.markWebhookOutcome(ctx, "payment.canceled", paymentID, yooKassaOutcomeAccepted)
	return yooKassaWebhookResult{Outcome: yooKassaOutcomeAccepted, StatusCode: http.StatusOK, OK: true}
}

// handleRefundSucceeded (L-5) processes a refund and revokes access.
func (h Handler) handleRefundSucceeded(ctx context.Context, refundID string, obj yooKassaObjectData) yooKassaWebhookResult {
	// YooKassa: refund.object.payment_id references the original payment.
	paymentID := strings.TrimSpace(obj.PaymentID)
	if paymentID == "" && obj.Metadata != nil {
		paymentID = strings.TrimSpace(obj.Metadata["payment_id"])
	}
	if paymentID == "" && obj.Metadata != nil {
		paymentID = strings.TrimSpace(obj.Metadata["yookassa_payment_id"])
	}
	if paymentID == "" {
		log.Printf("yookassa webhook: refund.succeeded %q — payment_id missing", refundID)
		h.markWebhookOutcome(ctx, "refund.succeeded", refundID, yooKassaOutcomeIgnored)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeIgnored, StatusCode: http.StatusOK, OK: true}
	}

	var invoiceID int64
	err := h.DB.QueryRow(ctx, `
		update invoices
		set status = 'refunded', updated_at = now()
		where yookassa_payment_id = $1 and status = 'paid'
		returning id`, paymentID).Scan(&invoiceID)
	if err != nil {
		log.Printf("yookassa webhook: refund.succeeded %q — paid invoice not found for payment %q: %v", refundID, paymentID, err)
		h.markWebhookOutcome(ctx, "refund.succeeded", refundID, yooKassaOutcomeAccepted)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeAccepted, StatusCode: http.StatusOK, OK: true}
	}
	if _, err := h.DB.Exec(ctx, `
		update user_mode_access
		set active_to = now(), updated_at = now()
		where access_type = 'subscription'
			  and source_id = $1
			  and active_to > now()`, invoiceID); err != nil {
		log.Printf("yookassa webhook: refund.succeeded %q — access revoke failed: %v", refundID, err)
		h.markWebhookOutcome(ctx, "refund.succeeded", refundID, yooKassaOutcomeRetryable)
		return yooKassaWebhookResult{Outcome: yooKassaOutcomeRetryable, StatusCode: http.StatusInternalServerError, OK: false, Error: "internal error"}
	}
	h.markWebhookOutcome(ctx, "refund.succeeded", refundID, yooKassaOutcomeAccepted)
	return yooKassaWebhookResult{Outcome: yooKassaOutcomeAccepted, StatusCode: http.StatusOK, OK: true}
}

func (h Handler) markWebhookOutcome(ctx context.Context, event, objectID string, outcome yooKassaWebhookOutcome) {
	_, _ = h.DB.Exec(ctx, `
		update yookassa_webhook_events
		set process_status = $3, processed_at = now()
		where event = $1 and object_id = $2`, event, objectID, string(outcome))
}

func (h Handler) ReconcilePendingYooKassaInvoices(ctx context.Context, limit int) (map[string]int, error) {
	result := map[string]int{"checked": 0, "paid": 0, "canceled": 0, "failed": 0, "skipped": 0, "not_found": 0}
	if h.DB == nil {
		return result, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := h.DB.Query(ctx, `
		select yookassa_payment_id
		from invoices
		where status = 'pending'
		  and nullif(yookassa_payment_id, '') is not null
		order by created_at asc, id asc
		limit $1`, limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return result, err
		}
		ids = append(ids, strings.TrimSpace(id))
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	for _, paymentID := range ids {
		if paymentID == "" {
			continue
		}
		result["checked"]++
		remote, checked, err := h.fetchYooKassaPayment(ctx, paymentID)
		if err != nil {
			if errors.Is(err, errYooKassaPaymentNotFound) {
				// The payment does not exist on the YooKassa side and never
				// will; leaving the invoice in 'pending' means it will
				// be selected in this same batch on every subsequent iteration,
				// and the reconcile loop in DailyYooKassaRenewalsWorkflow will never
				// see checked=0 (see workflow.go), burning through the full
				// ScheduleToCloseTimeout without ever reaching the renewals themselves.
				if _, updErr := h.DB.Exec(ctx, `
					update invoices
					set status = 'canceled', cancellation_reason = 'yookassa_payment_not_found', updated_at = now()
					where yookassa_payment_id = $1 and status = 'pending'`, paymentID); updErr != nil {
					log.Printf("yookassa reconcile: payment %q not_found, mark canceled failed: %v", paymentID, updErr)
					result["failed"]++
					continue
				}
				result["not_found"]++
				continue
			}
			log.Printf("yookassa reconcile: payment %q remote fetch failed: %v", paymentID, err)
			result["failed"]++
			continue
		}
		if !checked {
			result["skipped"]++
			continue
		}
		switch remote.Status {
		case "succeeded":
			h.handlePaymentSucceeded(ctx, paymentID, remote)
			result["paid"]++
		case "canceled":
			h.handlePaymentCanceled(ctx, paymentID, remote)
			result["canceled"]++
		default:
			result["skipped"]++
		}
	}
	return result, nil
}

// contextWithTimeout: the request context dies when the client disconnects; we
// want to finish writing to the DB even if the YooKassa side has already hung up.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, func()) {
	return context.WithTimeout(r.Context(), d)
}
