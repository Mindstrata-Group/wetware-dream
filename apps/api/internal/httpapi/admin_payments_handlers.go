package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type adminYooKassaPaymentRequest struct {
	AmountRub         float64 `json:"amountRub"`
	SavePaymentMethod bool    `json:"savePaymentMethod"`
	EnableAutoRenew   bool    `json:"enableAutoRenew"`
	Description       string  `json:"description"`
	ReturnURL         string  `json:"returnUrl"`
	// K-NEW-2: optional link to a user and tariff for subscription payments.
	// If both are set, an invoice row is created and the payment.succeeded webhook grants access.
	// If not, it is an ad-hoc charge with no automatic access grant.
	UserID             int64 `json:"userId,omitempty"`
	TariffID           int64 `json:"tariffId,omitempty"`
	SubscriptionMonths int   `json:"subscriptionMonths,omitempty"`
}

// AdminYooKassaCreatePayment creates a YooKassa payment and returns the confirmation URL.
// Only admin/billing_admin/owner.
//
// Flow: the admin sets an arbitrary amount -> the API creates a YooKassa payment ->
// the client goes to confirmationUrl -> after payment YooKassa calls the webhook.
// If savePaymentMethod=true, the card is saved for future auto-charges.
func (h Handler) AdminYooKassaCreatePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	actor, ok := h.requireRole(w, r, "owner", "admin", "billing_admin")
	if !ok {
		return
	}
	creds, err := h.activeYooKassaCredentials(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !creds.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"error": "ЮKassa не настроена: задайте live env или тестовые реквизиты в админке",
			"code":  "yookassa_not_configured",
		})
		return
	}
	var req adminYooKassaPaymentRequest
	if err := decodeJSONStrict(w, r, 32<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if req.AmountRub <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount must be positive"})
		return
	}
	if req.ReturnURL == "" {
		req.ReturnURL = "https://mindstrata.ru/admin"
	}
	if strings.TrimSpace(req.Description) == "" {
		req.Description = "Админ-платеж Mindstrata"
	}
	if req.EnableAutoRenew {
		if req.UserID <= 0 || req.TariffID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "userId and tariffId are required for autorenew"})
			return
		}
		req.SavePaymentMethod = true
	}
	payload := map[string]any{
		"amount":              map[string]string{"value": fmt.Sprintf("%.2f", req.AmountRub), "currency": "RUB"},
		"capture":             true,
		"confirmation":        map[string]string{"type": "redirect", "return_url": req.ReturnURL},
		"description":         req.Description,
		"save_payment_method": req.SavePaymentMethod,
	}
	payment, err := h.createYooKassaPayment(r.Context(), payload, fmt.Sprintf("admin-%d", time.Now().UnixNano()))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "code": "yookassa_create_failed"})
		return
	}
	confirmationURL := ""
	if payment.Confirmation != nil {
		confirmationURL = payment.Confirmation.ConfirmationURL
	}
	// K-NEW-2: if a user+tariff pair is set, write an invoice row.
	// The handlePaymentSucceeded webhook finds it by yookassa_payment_id and grants access.
	if req.UserID > 0 && req.TariffID > 0 && h.DB != nil {
		paymentID := payment.ID
		months := req.SubscriptionMonths
		if months <= 0 {
			months = 1
		}
		if paymentID != "" {
			_, err := h.DB.Exec(r.Context(), `
				insert into invoices (
					user_id, tariff_id, yookassa_payment_id, amount, currency, status,
					subscription_months, expires_at, is_recurring, autorenew_requested, confirmation_url
				)
				values ($1, $2, $3, $4, 'RUB', 'pending', $5, now() + interval '1 hour' * 24, $6, $6, $7)
				on conflict do nothing`,
				req.UserID, req.TariffID, paymentID, req.AmountRub, months, req.EnableAutoRenew, confirmationURL)
			if err != nil {
				// Do not block the response: the YooKassa payment is already created. Log it
				// so the admin sees that the invoice was not written and can reconcile by hand.
				fmt.Printf("admin yookassa: invoice insert failed for payment=%s user=%d: %v\n", paymentID, req.UserID, err)
			}
		}
	}
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.billing.yookassa.create_payment", "payment", nil, map[string]any{
		"amountRub":          req.AmountRub,
		"savePaymentMethod":  req.SavePaymentMethod,
		"enableAutoRenew":    req.EnableAutoRenew,
		"userId":             req.UserID,
		"tariffId":           req.TariffID,
		"subscriptionMonths": req.SubscriptionMonths,
		"paymentId":          payment.ID,
		"mode":               creds.Mode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "payment": payment, "confirmationUrl": confirmationURL, "mode": creds.Mode})
}
