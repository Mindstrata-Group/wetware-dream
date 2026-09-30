package httpapi

import (
	"context"
	"errors"
	"testing"
)

// Unit tests for the data-protection policy (docs/specs/data-protection.md).
// They cover the pure decision logic; the DB-backed parts live in
// data_protection_integration_test.go.

// AC-1: the installation profile is parsed strictly; anything unknown falls
// back to "none" so a typo never silently enables or disables a legal regime.
func TestDataProtectionProfile_Normalize(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":        dataProtectionProfileNone,
		"none":    dataProtectionProfileNone,
		"RU":      dataProtectionProfileRU,
		" ru ":    dataProtectionProfileRU,
		"eu":      dataProtectionProfileEU,
		"US":      dataProtectionProfileUS,
		"russia":  dataProtectionProfileNone,
		"ru,eu":   dataProtectionProfileNone,
		"unknown": dataProtectionProfileNone,
	}
	for in, want := range cases {
		in, want := in, want
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDataProtectionProfile(in); got != want {
				t.Fatalf("normalizeDataProtectionProfile(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// AC-2 / AC-3: cross-border decision table.
func TestCrossBorderPermitted_DecisionTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		profile        string
		gatewayCountry string
		hasSubject     bool
		consentGranted bool
		wantPermitted  bool
	}{
		{"profile none allows anything", dataProtectionProfileNone, "US", false, false, true},
		{"profile eu does not block in code", dataProtectionProfileEU, "US", false, false, true},
		{"profile us does not block in code", dataProtectionProfileUS, "", false, false, true},
		{"ru: domestic gateway needs no consent", dataProtectionProfileRU, "RU", false, false, true},
		{"ru: foreign gateway without subject is blocked", dataProtectionProfileRU, "US", false, false, false},
		{"ru: foreign gateway without consent is blocked", dataProtectionProfileRU, "US", true, false, false},
		{"ru: foreign gateway with consent is allowed", dataProtectionProfileRU, "US", true, true, true},
		{"ru: unknown country counts as foreign", dataProtectionProfileRU, "", true, false, false},
		{"ru: unknown country with consent is allowed", dataProtectionProfileRU, "", true, true, true},
		{"ru: lowercase country is normalized", dataProtectionProfileRU, "ru", false, false, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := crossBorderPermitted(tc.profile, tc.gatewayCountry, tc.hasSubject, tc.consentGranted)
			if got != tc.wantPermitted {
				t.Fatalf("crossBorderPermitted(%q, %q, subject=%v, consent=%v) = %v, want %v",
					tc.profile, tc.gatewayCountry, tc.hasSubject, tc.consentGranted, got, tc.wantPermitted)
			}
		})
	}
}

// AC-4: the data subject travels in the context so that nested AI mechanics
// (orchestration, summaries) inherit it without new function parameters.
func TestDataSubjectContext_RoundTrip(t *testing.T) {
	t.Parallel()
	if _, ok := dataSubjectFromContext(context.Background()); ok {
		t.Fatal("empty context must not carry a subject")
	}
	ctx := withDataSubject(context.Background(), 42)
	id, ok := dataSubjectFromContext(ctx)
	if !ok || id != 42 {
		t.Fatalf("subject = %d, %v; want 42, true", id, ok)
	}
	if _, ok := dataSubjectFromContext(withDataSubject(context.Background(), 0)); ok {
		t.Fatal("zero user id must not count as a subject")
	}
}

// AC-5: the chat layer recognises the policy error and never mistakes it for
// a provider outage (which would trigger retries and a 502).
func TestCrossBorderError_IsDistinguishable(t *testing.T) {
	t.Parallel()
	wrapped := errors.Join(errors.New("context"), errCrossBorderConsentRequired)
	if !errors.Is(wrapped, errCrossBorderConsentRequired) {
		t.Fatal("wrapped policy error must be detectable with errors.Is")
	}
	if liveAIErrorStatus(errCrossBorderConsentRequired) != 0 {
		t.Fatal("policy error must not look like an HTTP status from a provider")
	}
}

// AC-7: consent payload validation.
func TestValidateConsentPayload(t *testing.T) {
	t.Parallel()
	validHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		name    string
		doc     string
		version string
		hash    string
		wantErr bool
	}{
		{"valid cross-border", consentDocCrossBorder, "2026-09-30", validHash, false},
		{"valid personal data", consentDocPersonalData, "v1.2", validHash, false},
		{"health data is its own document", consentDocHealthData, "v1", validHash, false},
		{"unknown document", "marketing_spam", "v1", validHash, true},
		{"empty version", consentDocCrossBorder, "", validHash, true},
		{"version with spaces", consentDocCrossBorder, "v 1", validHash, true},
		{"short hash", consentDocCrossBorder, "v1", "abc", true},
		{"uppercase hash", consentDocCrossBorder, "v1", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateConsentPayload(tc.doc, tc.version, tc.hash)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateConsentPayload(%q, %q, %q) err = %v, wantErr %v", tc.doc, tc.version, tc.hash, err, tc.wantErr)
			}
		})
	}
}

// AC-17: country codes accepted by the admin endpoint.
func TestNormalizeProcessingCountry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"RU", "RU", false},
		{" us ", "US", false},
		{"", "", false},
		{"RUS", "", true},
		{"R1", "", true},
		{"Россия", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeProcessingCountry(tc.in)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("normalizeProcessingCountry(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// AC-24: the RU profile warns when the declared database location is not
// Russia or not declared at all (152-FZ art. 18(5)).
func TestDataProtectionWarnings_StorageCountry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		profile, storage string
		want             []string
	}{
		{dataProtectionProfileRU, "RU", []string{}},
		{dataProtectionProfileRU, "ru", []string{}},
		{dataProtectionProfileRU, "", []string{"storage_country_unknown"}},
		{dataProtectionProfileRU, "DE", []string{"storage_outside_russia"}},
		{dataProtectionProfileEU, "", []string{}},
		{dataProtectionProfileNone, "US", []string{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.profile+"/"+tc.storage, func(t *testing.T) {
			t.Parallel()
			got := dataProtectionWarnings(tc.profile, tc.storage)
			if len(got) != len(tc.want) {
				t.Fatalf("warnings = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("warnings = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// Spec-only acceptance criteria that are covered by the specification but not
// implemented yet. They are skipped on purpose and listed by the CI
// "Spec backlog" step.

func TestSpecDataProtection_AC20_PreSendTransformHook(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-20: not implemented")
}

func TestSpecDataProtection_AC21_FieldLevelEncryption(t *testing.T) {
	t.Parallel()
	t.Skip("SPEC data-protection AC-21: not implemented")
}
