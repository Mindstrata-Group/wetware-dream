package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (h Handler) userRole(ctx context.Context, userID int64) (string, error) {
	role := ""
	err := h.DB.QueryRow(ctx, `select role from users where id = $1 and deleted_at is null`, userID).Scan(&role)
	return role, err
}

// signGuestCookie signs userID with HMAC-SHA256.
// Format: "{id}.{hmac_hex}"
func signGuestCookie(id int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d", id)
	return strconv.FormatInt(id, 10) + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyGuestCookie checks the HMAC and returns userID; 0 on an invalid signature.
func verifyGuestCookie(value, secret string) int64 {
	idx := strings.LastIndexByte(value, '.')
	if idx < 1 {
		return 0
	}
	idStr := value[:idx]
	sig := value[idx+1:]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d", id)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return 0
	}
	return id
}

// guestUserIDFromRequest reads and verifies the guest cookie.
// If the secret is empty it works in dev mode without a signature.
func guestUserIDFromRequest(r interface {
	Cookie(string) (*http.Cookie, error)
}, secret string) int64 {
	cookie, err := r.Cookie(guestCookieName)
	if err != nil || cookie.Value == "" {
		return 0
	}
	if secret != "" {
		return verifyGuestCookie(cookie.Value, secret)
	}
	// Dev mode: no signature required.
	id, err := strconv.ParseInt(cookie.Value, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// guestCookieValue builds the cookie value: signed, or plain in dev.
func (h Handler) guestCookieValue(userID int64) string {
	if h.GuestCookieSecret != "" {
		return signGuestCookie(userID, h.GuestCookieSecret)
	}
	return strconv.FormatInt(userID, 10)
}

func (h Handler) transferGuestAccess(ctx context.Context, guestUserID, targetUserID int64) error {
	if guestUserID <= 0 || targetUserID <= 0 || guestUserID == targetUserID {
		return nil
	}
	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// N-4: lock the guest user's row to prevent a double transfer
	// on parallel OAuth callbacks from the same browser.
	var lockID int64
	_ = tx.QueryRow(ctx, `select id from users where id = $1 for update`, guestUserID).Scan(&lockID)

	// ON CONFLICT DO NOTHING: a repeated call (a parallel OAuth callback)
	// does not duplicate access for a non-NULL source_id. A NULL source_id is safe:
	// active access is checked by active_to, not by the number of rows.
	if _, err = tx.Exec(ctx, `
		insert into user_mode_access
			(user_id, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id, created_at, updated_at)
		select $2, mode_id, active_from, active_to, daily_message_limit, priority, access_type, source_id, now(), now()
		from user_mode_access
		where user_id = $1
		  and (active_from is null or active_from <= now())
		  and (active_to is null or active_to >= now())
		on conflict (user_id, mode_id, access_type, source_id) do nothing`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		with access_map as (
			select guest_access.id as guest_access_id,
			       target_access.id as target_access_id
			from user_mode_access guest_access
			join user_mode_access target_access
			  on target_access.user_id = $2
			 and target_access.mode_id = guest_access.mode_id
			 and target_access.access_type = guest_access.access_type
			 and target_access.source_id is not distinct from guest_access.source_id
			where guest_access.user_id = $1
			  and (guest_access.active_from is null or guest_access.active_from <= now())
			  and (guest_access.active_to is null or guest_access.active_to >= now())
		)
		insert into daily_mode_usage
			(user_id, mode_id, access_id, usage_date, messages_used, text_messages_used, audio_messages_used, created_at, updated_at)
		select $2, usage.mode_id, access_map.target_access_id, usage.usage_date,
		       usage.messages_used,
		       coalesce(usage.text_messages_used, 0),
		       coalesce(usage.audio_messages_used, 0),
		       now(), now()
		from daily_mode_usage usage
		join access_map on access_map.guest_access_id = usage.access_id
		where usage.user_id = $1
		on conflict (user_id, mode_id, access_id, usage_date) do update
		set messages_used = daily_mode_usage.messages_used + excluded.messages_used,
		    text_messages_used = coalesce(daily_mode_usage.text_messages_used, 0) + coalesce(excluded.text_messages_used, 0),
		    audio_messages_used = coalesce(daily_mode_usage.audio_messages_used, 0) + coalesce(excluded.audio_messages_used, 0),
		    updated_at = now()`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		delete from daily_mode_usage usage
		using user_mode_access guest_access
		where usage.user_id = $1
		  and usage.access_id = guest_access.id
		  and guest_access.user_id = $1
		  and (guest_access.active_from is null or guest_access.active_from <= now())
		  and (guest_access.active_to is null or guest_access.active_to >= now())`, guestUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		with access_map as (
			select guest_access.id as guest_access_id,
			       target_access.id as target_access_id
			from user_mode_access guest_access
			join user_mode_access target_access
			  on target_access.user_id = $2
			 and target_access.mode_id = guest_access.mode_id
			 and target_access.access_type = guest_access.access_type
			 and target_access.source_id is not distinct from guest_access.source_id
			where guest_access.user_id = $1
			  and (guest_access.active_from is null or guest_access.active_from <= now())
			  and (guest_access.active_to is null or guest_access.active_to >= now())
		)
		insert into dialog_message_access_usage
			(user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind, created_at)
		select $2, usage.mode_id, usage.dialog_message_id, access_map.target_access_id,
		       usage.usage_date, usage.usage_kind, usage.created_at
		from dialog_message_access_usage usage
		join access_map on access_map.guest_access_id = usage.access_id
		where usage.user_id = $1
		on conflict (dialog_message_id, access_id) do nothing`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		delete from dialog_message_access_usage usage
		using user_mode_access guest_access
		where usage.user_id = $1
		  and usage.access_id = guest_access.id
		  and guest_access.user_id = $1
		  and (guest_access.active_from is null or guest_access.active_from <= now())
		  and (guest_access.active_to is null or guest_access.active_to >= now())`, guestUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		with access_map as (
			select guest_access.id as guest_access_id,
			       target_access.id as target_access_id
			from user_mode_access guest_access
			join user_mode_access target_access
			  on target_access.user_id = $2
			 and target_access.mode_id = guest_access.mode_id
			 and target_access.access_type = guest_access.access_type
			 and target_access.source_id is not distinct from guest_access.source_id
			where guest_access.user_id = $1
			  and (guest_access.active_from is null or guest_access.active_from <= now())
			  and (guest_access.active_to is null or guest_access.active_to >= now())
		)
		update message_usage usage
		set user_id = $2,
		    access_id = access_map.target_access_id
		from access_map
		where usage.user_id = $1
		  and usage.access_id = access_map.guest_access_id`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update message_usage
		set user_id = $2
		where user_id = $1`, guestUserID, targetUserID); err != nil {
		return err
	}
	// Transferred (active) rows are deleted from the guest; otherwise a repeat visit
	// with the same guest cookie clones paid access into an unlimited number of
	// new accounts (the cookie is not single-use, clearGuestCookie is only a hint to the client).
	if _, err = tx.Exec(ctx, `
		delete from user_mode_access
		where user_id = $1
		  and (active_from is null or active_from <= now())
		  and (active_to is null or active_to >= now())`, guestUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		insert into promocode_usages (user_id, promocode_id, used_at)
		select $2, promocode_id, used_at
		from promocode_usages pu
		where pu.user_id = $1
		  and not exists (
		    select 1 from promocode_usages existing
		    where existing.user_id = $2 and existing.promocode_id = pu.promocode_id
		  )`, guestUserID, targetUserID); err != nil {
		return err
	}
	// Likewise the guest's usage is deleted, so replaying the cookie does not bypass max_uses/the protection against reapplying a promocode.
	if _, err = tx.Exec(ctx, `delete from promocode_usages where user_id = $1`, guestUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update invoices
		set user_id = $2,
		    updated_at = now()
		where user_id = $1`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update payment_methods
		set user_id = $2,
		    updated_at = now()
		where user_id = $1`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update subscriptions
		set user_id = $2,
		    updated_at = now()
		where user_id = $1`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update users_dialogs
		set user_id = $2
		where user_id = $1
		  and deleted_at is null`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update users target
		set current_mode = coalesce(target.current_mode, guest.current_mode),
		    current_dialog = coalesce(target.current_dialog, guest.current_dialog),
		    updated_at = now()
		from users guest
		where target.id = $2
		  and guest.id = $1
		  and (
		    (target.current_mode is null and guest.current_mode is not null)
		    or (target.current_dialog is null and guest.current_dialog is not null)
		  )`, guestUserID, targetUserID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		update users
		set current_mode = null,
		    current_dialog = null,
		    updated_at = now()
		where id = $1
		  and (current_mode is not null or current_dialog is not null)`, guestUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// userCanUseLiveAI checks the user's role.
// N-3: an explicit role allowlist (role != "" is not enough, an explicit check is needed).
func (h Handler) userCanUseLiveAI(ctx context.Context, userID int64) bool {
	role, err := h.userRole(ctx, userID)
	if err != nil {
		return false
	}
	return validRole(role)
}
