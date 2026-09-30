package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Data protection policy of an installation. Spec: docs/specs/data-protection.md.
//
// The profile is an operator decision about which legal regime the
// installation follows. It is set through the environment
// (DATA_PROTECTION_PROFILE), not from the admin panel: an admin click must not
// be able to switch off a legal safeguard silently.
//
// What the code enforces today:
//   - "ru" (Federal Law 152-FZ): chat content may leave Russia only for a user
//     who holds an active, separate cross-border consent. Gateways are marked
//     with the country where they process data; an unmarked gateway counts as
//     foreign. Calls without a known data subject fail closed.
//   - "eu", "us": no transfer block in code. GDPR transfers rely on contracts
//     (SCC/DPF) and HIPAA on BAAs, which are operator paperwork, not a switch.
//   - "none" (default): behaviour is unchanged.
const (
	dataProtectionProfileNone = "none"
	dataProtectionProfileRU   = "ru"
	dataProtectionProfileEU   = "eu"
	dataProtectionProfileUS   = "us"
)

// Consent documents known to the journal. Keep in sync with the check
// constraint in apps/api/sql/20260930_120000_data_protection.sql.
//
// health_data stands apart from personal_data: chats in psychological modes
// are treated as special-category data, which need a separate written consent
// (152-FZ art. 10(2)(1), GDPR art. 9(2)(a)).
const (
	consentDocPersonalData = "personal_data"
	consentDocHealthData   = "health_data"
	consentDocCrossBorder  = "cross_border_transfer"
	consentDocTerms        = "terms_of_service"
)

var consentDocuments = map[string]bool{
	consentDocPersonalData: true,
	consentDocHealthData:   true,
	consentDocCrossBorder:  true,
	consentDocTerms:        true,
}

// countryRussia is the only country the RU profile treats as domestic.
const countryRussia = "RU"

// errCrossBorderConsentRequired means the call was stopped by policy before
// any request left the server. It is not a provider failure: callers must not
// retry it or report it as an outage.
var errCrossBorderConsentRequired = errors.New("cross-border transfer of personal data requires the user's consent")

func normalizeDataProtectionProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dataProtectionProfileRU:
		return dataProtectionProfileRU
	case dataProtectionProfileEU:
		return dataProtectionProfileEU
	case dataProtectionProfileUS:
		return dataProtectionProfileUS
	default:
		return dataProtectionProfileNone
	}
}

func (h Handler) dataProtectionProfile() string {
	return normalizeDataProtectionProfile(h.DataProtectionProfile)
}

// dataProtectionWarnings lists operator-side problems the code can see but
// cannot fix. The storage country is declared by the operator
// (DATA_STORAGE_COUNTRY): the service has no way to measure where its
// database physically lives.
func dataProtectionWarnings(profile, storageCountry string) []string {
	warnings := []string{}
	storage := strings.ToUpper(strings.TrimSpace(storageCountry))
	if normalizeDataProtectionProfile(profile) == dataProtectionProfileRU && storage != countryRussia {
		if storage == "" {
			warnings = append(warnings, "storage_country_unknown")
		} else {
			warnings = append(warnings, "storage_outside_russia")
		}
	}
	return warnings
}

type dataSubjectContextKey struct{}

// withDataSubject marks the context with the user whose personal data the
// following AI calls carry. Nested mechanics (orchestration, summaries)
// inherit it, so the policy check needs no extra parameters.
func withDataSubject(ctx context.Context, userID int64) context.Context {
	if userID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, dataSubjectContextKey{}, userID)
}

func dataSubjectFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(dataSubjectContextKey{}).(int64)
	return id, ok && id > 0
}

// crossBorderPermitted is the whole decision table in one pure function.
func crossBorderPermitted(profile, gatewayCountry string, hasSubject, consentGranted bool) bool {
	if normalizeDataProtectionProfile(profile) != dataProtectionProfileRU {
		return true
	}
	if strings.ToUpper(strings.TrimSpace(gatewayCountry)) == countryRussia {
		return true
	}
	return hasSubject && consentGranted
}

// normalizeProcessingCountry accepts an ISO 3166-1 alpha-2 code or an empty
// string ("unknown").
func normalizeProcessingCountry(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return "", nil
	}
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return "", fmt.Errorf("country must be a two-letter ISO 3166-1 code, got %q", raw)
	}
	return code, nil
}

// gatewayCountries reads processing countries in a separate query on purpose:
// the main registry query must keep working on a database where the
// migration is not applied yet. A failure here means "unknown", which the RU
// profile treats as foreign (fail closed).
func (h Handler) gatewayCountries(ctx context.Context) map[string]string {
	out := map[string]string{}
	if h.DB == nil {
		return out
	}
	rows, err := h.DB.Query(ctx, `select id, processing_country from ai_gateways`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, country string
		if rows.Scan(&id, &country) == nil {
			out[id] = country
		}
	}
	return out
}

// hasActiveConsent is read on every call without a cache: a withdrawal must
// take effect on the very next message. Any error counts as "no consent".
func (h Handler) hasActiveConsent(ctx context.Context, userID int64, document string) bool {
	if h.DB == nil || userID <= 0 {
		return false
	}
	var ok bool
	if err := h.DB.QueryRow(ctx, `
		select exists (
			select 1 from consent_records
			where user_id = $1 and document = $2 and withdrawn_at is null
		)`, userID, document).Scan(&ok); err != nil {
		return false
	}
	return ok
}

// filterGatewaysByJurisdiction drops gateways the policy forbids for this
// call and reports whether anything was dropped.
func (h Handler) filterGatewaysByJurisdiction(ctx context.Context, gateways []aiGateway) ([]aiGateway, bool) {
	profile := h.dataProtectionProfile()
	if profile != dataProtectionProfileRU || len(gateways) == 0 {
		return gateways, false
	}
	countries := h.gatewayCountries(ctx)
	subject, hasSubject := dataSubjectFromContext(ctx)
	consentKnown, consent := false, false
	allowed := make([]aiGateway, 0, len(gateways))
	blocked := false
	for _, g := range gateways {
		country := countries[g.ID]
		if !crossBorderPermitted(profile, country, false, false) {
			if !consentKnown && hasSubject {
				consent = h.hasActiveConsent(ctx, subject, consentDocCrossBorder)
			}
			consentKnown = true
			if !crossBorderPermitted(profile, country, hasSubject, consent) {
				blocked = true
				continue
			}
		}
		allowed = append(allowed, g)
	}
	return allowed, blocked
}

func validateConsentPayload(document, version, textSHA256 string) error {
	if !consentDocuments[document] {
		return fmt.Errorf("unknown consent document %q", document)
	}
	if len(version) == 0 || len(version) > 64 {
		return errors.New("version must be 1..64 characters")
	}
	for _, r := range version {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return errors.New("version may contain only letters, digits, dot, dash and underscore")
		}
	}
	if len(textSHA256) != 64 {
		return errors.New("textSha256 must be 64 lowercase hex characters")
	}
	for _, r := range textSHA256 {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return errors.New("textSha256 must be 64 lowercase hex characters")
		}
	}
	return nil
}
