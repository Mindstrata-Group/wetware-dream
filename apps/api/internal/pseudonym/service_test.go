package pseudonym

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// A throwaway key built at run time, so no key-shaped literal sits in the repo.
var testKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

func newService(values map[string]string) (*Service, *fakeDetector, *memVault) {
	det := &fakeDetector{values: values}
	vault := newMemVault()
	return &Service{Detector: det, Vault: vault, Profiles: staticProfiles{}}, det, vault
}

func TestParseKeys(t *testing.T) {
	t.Parallel()
	if _, err := ParseKeys(""); !errors.Is(err, ErrNoKey) {
		t.Fatalf("empty key: %v", err)
	}
	if _, err := ParseKeys(base64.StdEncoding.EncodeToString([]byte("short"))); !errors.Is(err, ErrNoKey) {
		t.Fatalf("short key: %v", err)
	}
	if _, err := ParseKeys(testKey); err != nil {
		t.Fatalf("valid key: %v", err)
	}
}

func TestFingerprintIsPerUser(t *testing.T) {
	t.Parallel()
	k, _ := ParseKeys(testKey)
	a := k.Fingerprint(1, "PHONE", "9123456789")
	b := k.Fingerprint(2, "PHONE", "9123456789")
	if bytes.Equal(a, b) {
		t.Fatal("the same value must have unrelated fingerprints for two users")
	}
	if !bytes.Equal(a, k.Fingerprint(1, "PHONE", "9123456789")) {
		t.Fatal("fingerprint must be stable")
	}
}

func TestSealIsBoundToUserAndKind(t *testing.T) {
	t.Parallel()
	k, _ := ParseKeys(testKey)
	nonce, ct, err := k.Seal(7, "PERSON", []byte("Люда"))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := k.Open(7, "PERSON", nonce, ct); err != nil || string(plain) != "Люда" {
		t.Fatalf("open: %q %v", plain, err)
	}
	if _, err := k.Open(8, "PERSON", nonce, ct); err == nil {
		t.Fatal("a row copied to another user must not open")
	}
	if _, err := k.Open(7, "PLACE", nonce, ct); err == nil {
		t.Fatal("a row relabelled to another kind must not open")
	}
}

func TestTransformMasksWithStableAliases(t *testing.T) {
	t.Parallel()
	svc, _, _ := newService(map[string]string{"Люда": "PERSON", "89123456789": "PHONE"})
	ctx := context.Background()
	masked, ref, err := svc.Transform(ctx, 1, "ru", "Мама Люда звонила с 89123456789, Люда плакала")
	if err != nil {
		t.Fatal(err)
	}
	if masked != "Мама ЛИЦО_1 звонила с ТЕЛЕФОН_1, ЛИЦО_1 плакала" {
		t.Fatalf("masked: %q", masked)
	}
	if ref.lang() != "ru" {
		t.Fatalf("ref: %q", ref)
	}
	again, _, _ := svc.Transform(ctx, 1, "ru", "Люда опять")
	if again != "ЛИЦО_1 опять" {
		t.Fatalf("alias must be stable across messages: %q", again)
	}
}

func TestTransformNumbersArePerUser(t *testing.T) {
	t.Parallel()
	svc, _, _ := newService(map[string]string{"Люда": "PERSON", "Оля": "PERSON"})
	ctx := context.Background()
	a, _, _ := svc.Transform(ctx, 1, "ru", "Люда и Оля")
	b, _, _ := svc.Transform(ctx, 2, "ru", "Оля")
	if !strings.Contains(a, "ЛИЦО_1") || !strings.Contains(a, "ЛИЦО_2") || b != "ЛИЦО_1" {
		t.Fatalf("per-user numbering broken: %q / %q", a, b)
	}
}

func TestTransformEnglishLabels(t *testing.T) {
	t.Parallel()
	svc, _, _ := newService(map[string]string{"Sarah": "PERSON"})
	masked, ref, err := svc.Transform(context.Background(), 1, "en", "My wife Sarah")
	if err != nil || masked != "My wife PERSON_1" || ref.lang() != "en" {
		t.Fatalf("%q %q %v", masked, ref, err)
	}
}

func TestTransformCodePointOffsets(t *testing.T) {
	t.Parallel()
	// emoji and Cyrillic before the value: offsets are code points, not bytes
	svc, _, _ := newService(map[string]string{"Серёжа": "PERSON"})
	masked, _, err := svc.Transform(context.Background(), 1, "ru", "🙂 ёжик, муж Серёжа 🙂")
	if err != nil || masked != "🙂 ёжик, муж ЛИЦО_1 🙂" {
		t.Fatalf("%q %v", masked, err)
	}
}

func TestTransformFailsClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var nilSvc *Service
	if _, _, err := nilSvc.Transform(ctx, 1, "ru", "x"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("unconfigured: %v", err)
	}

	svc, det, _ := newService(nil)
	det.detectErr = ErrUnavailable
	if _, _, err := svc.Transform(ctx, 1, "ru", "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("service down: %v", err)
	}

	svc, det, _ = newService(nil)
	det.residual = 1
	if out, _, err := svc.Transform(ctx, 1, "ru", "секрет"); !errors.Is(err, ErrResidual) || out != "" {
		t.Fatalf("residual: %q %v", out, err)
	}

	svc, _, vault := newService(map[string]string{"Люда": "PERSON"})
	vault.err = errBoom
	if _, _, err := svc.Transform(ctx, 1, "ru", "Люда"); err == nil {
		t.Fatal("vault failure must fail the transform")
	}
}

func TestTransformGoSideProfileCheck(t *testing.T) {
	t.Parallel()
	// A detector that misses the user's own e-mail and phone: the Go check
	// still refuses to let them out.
	for _, text := range []string{"пишите anna.k@mail.ru", "звоните 8 (912) 345-67-89", "тг @anna_k"} {
		svc, _, _ := newService(nil)
		svc.Profiles = staticProfiles{1: {Email: "Anna.K@mail.ru", Phone: "+79123456789", Telegram: "anna_k"}}
		if _, _, err := svc.Transform(context.Background(), 1, "ru", text); !errors.Is(err, ErrResidual) {
			t.Fatalf("%q: %v", text, err)
		}
	}
}

func TestTransformSendsProfileAndVaultAsKnown(t *testing.T) {
	t.Parallel()
	svc, det, _ := newService(map[string]string{"Люда": "PERSON"})
	svc.Profiles = staticProfiles{1: {DisplayName: "Анна Кузнецова", Email: "a@b.ru"}}
	ctx := context.Background()
	if _, _, err := svc.Transform(ctx, 1, "ru", "Люда"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Transform(ctx, 1, "ru", "привет"); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, k := range det.lastKnown {
		kinds[k.Kind+":"+k.Value] = true
	}
	for _, want := range []string{"PERSON:Анна Кузнецова", "EMAIL:a@b.ru", "PERSON:Люда"} {
		if !kinds[want] {
			t.Fatalf("known values sent to the detector miss %q: %v", want, det.lastKnown)
		}
	}
}

func TestRestoreFallsBackToNominative(t *testing.T) {
	t.Parallel()
	svc, det, _ := newService(map[string]string{"Люда": "PERSON"})
	ctx := context.Background()
	if _, _, err := svc.Transform(ctx, 1, "ru", "Люда"); err != nil {
		t.Fatal(err)
	}
	det.restoreFn = func(string, string, []Entry) (RestoreResult, error) { return RestoreResult{}, ErrUnavailable }
	got := svc.Restore(ctx, 1, "v1:ru", "Поговорите с ЛИЦО_1 и ЛИЦО_9.")
	if got != "Поговорите с Люда и ⟨ЛИЦО_9⟩." {
		t.Fatalf("fallback: %q", got)
	}
}

func TestRestoreWithoutAliasesSkipsTheService(t *testing.T) {
	t.Parallel()
	svc, det, _ := newService(nil)
	called := false
	det.restoreFn = func(string, string, []Entry) (RestoreResult, error) {
		called = true
		return RestoreResult{}, nil
	}
	if got := svc.Restore(context.Background(), 1, "v1:ru", "Просто текст"); got != "Просто текст" || called {
		t.Fatalf("%q called=%v", got, called)
	}
}
