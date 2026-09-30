package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type publicYooKassaPaymentRequest struct {
	TariffID           int64  `json:"tariffId"`
	SubscriptionMonths int    `json:"subscriptionMonths,omitempty"`
	EnableAutoRenew    *bool  `json:"enableAutoRenew,omitempty"`
	ReturnURL          string `json:"returnUrl,omitempty"`
}

func (h Handler) PublicYooKassaCreatePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
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
			"error": "ЮKassa не настроена",
			"code":  "yookassa_not_configured",
		})
		return
	}
	userID, _, _, err := h.ensureGuestUser(r.Context(), w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	var req publicYooKassaPaymentRequest
	if err := decodeJSONStrict(w, r, 16<<10, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid payload"})
		return
	}
	if req.TariffID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tariffId is required"})
		return
	}
	months := req.SubscriptionMonths
	if months <= 0 {
		months = 1
	}
	if months > 12 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "subscriptionMonths must be <= 12"})
		return
	}
	enableAutoRenew := true
	if req.EnableAutoRenew != nil {
		enableAutoRenew = *req.EnableAutoRenew
	}

	var tariffName, description string
	var amount float64
	err = h.DB.QueryRow(r.Context(), `
		select t.name, coalesce(t.description, ''), coalesce(t.monthly_price, 0)
		from tariffs t
		left join tariff_groups tg on tg.id = t.group_id
		where t.id = $1
		  and t.archived_at is null
		  and t.available_for_subscription = true
		  and t.tariff_type = 'regular'
		  and t.monthly_price > 0
		  and (tg.id is null or tg.available_for_subscription = true)`,
		req.TariffID,
	).Scan(&tariffName, &description, &amount)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "tariff not found"})
		return
	}
	discountPercent := h.shopPeriodDiscountPercent(r.Context(), months)
	totalAmount := applyShopPeriodDiscount(amount*float64(months), discountPercent)
	returnURL := safePublicReturnURL(r, req.ReturnURL, "/profile")
	payment, err := h.createYooKassaPayment(r.Context(), map[string]any{
		"amount":              map[string]string{"value": fmt.Sprintf("%.2f", totalAmount), "currency": "RUB"},
		"capture":             true,
		"confirmation":        map[string]string{"type": "redirect", "return_url": returnURL},
		"description":         fmt.Sprintf("Mindstrata: %s", tariffName),
		"save_payment_method": enableAutoRenew,
		"metadata": map[string]string{
			"user_id":     fmt.Sprint(userID),
			"tariff_id":   fmt.Sprint(req.TariffID),
			"months":      fmt.Sprint(months),
			"kind":        "tariff_purchase",
			"autorenew":   fmt.Sprint(enableAutoRenew),
			"discount":    fmt.Sprintf("%.2f", discountPercent),
			"description": description,
			"source":      "public_pricing",
		},
	}, fmt.Sprintf("public-%d-%d-%d", userID, req.TariffID, time.Now().UnixNano()))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error(), "code": "yookassa_create_failed"})
		return
	}
	confirmationURL := ""
	if payment.Confirmation != nil {
		confirmationURL = payment.Confirmation.ConfirmationURL
	}
	if _, err := h.DB.Exec(r.Context(), `
		insert into invoices (
			user_id, tariff_id, yookassa_payment_id, amount, currency, status,
			subscription_months, expires_at, is_recurring, autorenew_requested, confirmation_url
		)
		values ($1, $2, $3, $4, 'RUB', 'pending', $5, now() + interval '24 hours', $6, $6, $7)
		on conflict (yookassa_payment_id) do nothing`,
		userID, req.TariffID, payment.ID, totalAmount, months, enableAutoRenew, confirmationURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"paymentId":       payment.ID,
		"confirmationUrl": confirmationURL,
		"autoRenew":       enableAutoRenew,
		"discountPercent": discountPercent,
		"amount":          totalAmount,
	})
}

func (h Handler) shopPeriodDiscountPercent(ctx context.Context, months int) float64 {
	if h.DB == nil || months <= 1 {
		return 0
	}
	var raw []byte
	if err := h.DB.QueryRow(ctx, `select value from site_content where key = 'shop.periods'`).Scan(&raw); err != nil || len(raw) == 0 {
		return 0
	}
	var periods []struct {
		Months          any `json:"months"`
		DiscountPercent any `json:"discountPercent"`
		Enabled         any `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &periods); err != nil {
		return 0
	}
	for _, period := range periods {
		if !siteContentItemEnabled(period.Enabled) {
			continue
		}
		if intFromJSONLike(period.Months) != months {
			continue
		}
		discount := floatFromJSONLike(period.DiscountPercent)
		if discount < 0 {
			return 0
		}
		if discount > 95 {
			return 95
		}
		return discount
	}
	return 0
}

func applyShopPeriodDiscount(amount float64, discountPercent float64) float64 {
	if discountPercent <= 0 {
		return amount
	}
	if discountPercent > 95 {
		discountPercent = 95
	}
	return math.Round(amount*(1-discountPercent/100)*100) / 100
}

func siteContentItemEnabled(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return normalized != "false" && normalized != "0" && normalized != "no" && normalized != "off" && normalized != "выкл" && normalized != "draft" && normalized != "disabled"
	default:
		return true
	}
}

func intFromJSONLike(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var out int
		_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &out)
		return out
	default:
		return 0
	}
}

func floatFromJSONLike(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		var out float64
		_, _ = fmt.Sscanf(strings.ReplaceAll(strings.TrimSpace(v), ",", "."), "%f", &out)
		return out
	default:
		return 0
	}
}

func publicReturnURL(r *http.Request, path string) string {
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if !publicReturnHostAllowed(host) {
		host = strings.TrimSpace(r.Host)
	}
	if !publicReturnHostAllowed(host) {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return proto + "://" + host + path
}

func safePublicReturnURL(r *http.Request, raw string, fallbackPath string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return publicReturnURL(r, fallbackPath)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return publicReturnURL(r, fallbackPath)
	}
	if parsed.IsAbs() {
		if (parsed.Scheme == "https" || parsed.Scheme == "http") && publicReturnHostAllowed(parsed.Host) {
			return parsed.String()
		}
		return publicReturnURL(r, fallbackPath)
	}
	if strings.HasPrefix(raw, "//") || !strings.HasPrefix(raw, "/") {
		return publicReturnURL(r, fallbackPath)
	}
	return publicReturnURL(r, raw)
}

func publicReturnHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return false
	}
	if strings.Contains(host, ":") {
		withoutPort, _, err := net.SplitHostPort(host)
		if err != nil {
			return false
		}
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(withoutPort)), ".")
	}
	return host == "mindstrata.ru" || host == "stage.mindstrata.ru"
}
