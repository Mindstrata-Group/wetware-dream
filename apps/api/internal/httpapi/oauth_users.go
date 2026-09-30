package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (h Handler) findOrCreateOAuthUser(ctx context.Context, p OAuthProfile) (AuthenticatedUser, error) {
	return h.findOrCreateOAuthUserForIntent(ctx, p, true)
}

func (h Handler) findOAuthUserForLogin(ctx context.Context, p OAuthProfile) (AuthenticatedUser, error) {
	return h.findOrCreateOAuthUserForIntent(ctx, p, false)
}

func (h Handler) findOrCreateOAuthUserForIntent(ctx context.Context, p OAuthProfile, allowCreate bool) (AuthenticatedUser, error) {
	if p.Provider == "" || p.ProviderUserID == "" {
		return AuthenticatedUser{}, errors.New("invalid oauth profile")
	}
	email := normalizeEmail(p.Email)
	raw, _ := json.Marshal(p.Raw)

	tx, err := beginTxTimeout(ctx, h.DB, dbAcquireTimeout)
	if err != nil {
		return AuthenticatedUser{}, err
	}
	defer tx.Rollback(ctx)

	var user AuthenticatedUser
	err = tx.QueryRow(ctx, `
		select u.id, coalesce(u.email, ''), u.role, u.status
		from user_identities i
		join users u on u.id = i.user_id
		where i.provider = $1
		  and i.provider_user_id = $2
		  and u.deleted_at is null
		for update`, p.Provider, p.ProviderUserID).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
	if err == nil {
		if user.Status != "active" {
			return AuthenticatedUser{}, errors.New("user is not active")
		}
		_, _ = tx.Exec(ctx, `
			update user_identities
			set provider_email = $3,
			    email_verified = $4,
			    display_name = $5,
			    avatar_url = $6,
			    raw_profile = $7::jsonb,
			    updated_at = now()
			where provider = $1 and provider_user_id = $2`,
			p.Provider,
			p.ProviderUserID,
			email,
			p.EmailVerified,
			p.DisplayName,
			p.AvatarURL,
			string(raw),
		)
		if email != "" && user.Email == "" {
			_, _ = tx.Exec(ctx, `update users set email = $2, email_verified_at = case when $3 then coalesce(email_verified_at, now()) else email_verified_at end, updated_at = now() where id = $1 and email is null`, user.ID, email, p.EmailVerified)
			user.Email = email
		}
		if err = tx.Commit(ctx); err != nil {
			return AuthenticatedUser{}, err
		}
		h.upsertYandexNotificationContacts(ctx, user.ID, p)
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AuthenticatedUser{}, err
	}

	// K-NEW4-1: account hijack protection: link an OAuth identity to an existing
	// user by email ONLY if the OAuth provider verified the email.
	// Otherwise an attacker creates a Yandex account with victim@example.com (unverified),
	// logs in and gets access to the victim. EmailVerified=true guarantees
	// that the email really belongs to the OAuth login.
	if email != "" && p.EmailVerified {
		err = tx.QueryRow(ctx, `
			select id, coalesce(email, ''), role, status
			from users
			where lower(email) = lower($1)
			  and deleted_at is null
			for update`, email).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
		if err == nil {
			if user.Status != "active" || user.Role != "user" {
				return AuthenticatedUser{}, errors.New("social login cannot link privileged account")
			}
			if _, err = tx.Exec(ctx, `
				insert into user_identities (user_id, provider, provider_user_id, provider_email, email_verified, display_name, avatar_url, raw_profile)
				values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
				user.ID,
				p.Provider,
				p.ProviderUserID,
				email,
				p.EmailVerified,
				p.DisplayName,
				p.AvatarURL,
				string(raw),
			); err != nil {
				return AuthenticatedUser{}, err
			}
			_, _ = tx.Exec(ctx, `update users set email_verified_at = coalesce(email_verified_at, now()), updated_at = now() where id = $1`, user.ID)
			if err = tx.Commit(ctx); err != nil {
				return AuthenticatedUser{}, err
			}
			h.upsertYandexNotificationContacts(ctx, user.ID, p)
			return user, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return AuthenticatedUser{}, err
		}
	}

	if !allowCreate {
		return AuthenticatedUser{}, errors.New("oauth account not found")
	}

	username := fmt.Sprintf("%s_%s_%d", p.Provider, strings.ReplaceAll(p.ProviderUserID, "-", "_"), time.Now().Unix())
	err = tx.QueryRow(ctx, `
		insert into users (
			email,
			role,
			status,
			accepted_tos,
			telegram_username,
			display_name,
			avatar_url,
			email_verified_at,
			updated_at
		)
		values ($1, 'user', 'active', true, $2, $3, $4, case when $5 then now() else null end, now())
		returning id, coalesce(email, ''), role, status`,
		nullString(email),
		username,
		nullString(p.DisplayName),
		nullString(p.AvatarURL),
		p.EmailVerified,
	).Scan(&user.ID, &user.Email, &user.Role, &user.Status)
	if err != nil {
		return AuthenticatedUser{}, err
	}
	if _, err = tx.Exec(ctx, `
		insert into user_identities (user_id, provider, provider_user_id, provider_email, email_verified, display_name, avatar_url, raw_profile)
		values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		user.ID,
		p.Provider,
		p.ProviderUserID,
		email,
		p.EmailVerified,
		p.DisplayName,
		p.AvatarURL,
		string(raw),
	); err != nil {
		return AuthenticatedUser{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AuthenticatedUser{}, err
	}
	h.upsertYandexNotificationContacts(ctx, user.ID, p)
	return user, nil
}

func nullString(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func oauthHTTPError(provider string, status int, body string) error {
	if len(body) > 400 {
		body = body[:400]
	}
	return fmt.Errorf("%s oauth status %d: %s", provider, status, strings.TrimSpace(body))
}
