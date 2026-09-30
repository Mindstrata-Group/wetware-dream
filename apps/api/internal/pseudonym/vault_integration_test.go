//go:build integration

package pseudonym

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func newUser(t *testing.T, env *testsupport.Env, email string) int64 {
	t.Helper()
	var id int64
	err := env.Pool.QueryRow(context.Background(), `
		insert into users (email, display_name, phone, telegram_username)
		values ($1, 'Злата Крапивина', '+79120001122', 'zlata_k') returning id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestPGVaultAssignLoadAndEncryption(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	keys, _ := ParseKeys(testKey)
	v := &PGVault{Pool: env.Pool, Keys: keys}
	ctx := context.Background()
	u := newUser(t, env, fmt.Sprintf("vault-%d@example.test", time.Now().UnixNano()))

	n1, err := v.Assign(ctx, u, "PERSON", "люда", "Люда")
	if err != nil || n1 != 1 {
		t.Fatalf("first: %d %v", n1, err)
	}
	n2, _ := v.Assign(ctx, u, "PERSON", "оля", "Оля")
	again, _ := v.Assign(ctx, u, "PERSON", "люда", "Люда")
	phone, _ := v.Assign(ctx, u, "PHONE", "9120001122", "89120001122")
	if n2 != 2 || again != 1 || phone != 1 {
		t.Fatalf("numbering: %d %d %d", n2, again, phone)
	}
	entries, err := v.Load(ctx, u)
	if err != nil || len(entries) != 3 {
		t.Fatalf("load: %+v %v", entries, err)
	}
	// originals are not stored in clear text
	var raw []byte
	if err := env.Pool.QueryRow(ctx, `select ciphertext from pii_vault where user_id = $1 and kind = 'PERSON' and alias_no = 1`, u).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("Люда")) || bytes.Contains(raw, []byte("люда")) {
		t.Fatal("ciphertext contains the original value")
	}
}

func TestPGVaultConcurrentAssignKeepsNumbersDense(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	keys, _ := ParseKeys(testKey)
	v := &PGVault{Pool: env.Pool, Keys: keys}
	ctx := context.Background()
	u := newUser(t, env, fmt.Sprintf("race-%d@example.test", time.Now().UnixNano()))

	var wg sync.WaitGroup
	numbers := make([]int, 10)
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			numbers[i], errs[i] = v.Assign(ctx, u, "PERSON", fmt.Sprintf("человек %d", i%5), "X")
		}(i)
	}
	wg.Wait()
	seen := map[int]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("assign %d: %v", i, err)
		}
		seen[numbers[i]] = true
	}
	if len(seen) != 5 {
		t.Fatalf("5 distinct values must get 5 distinct numbers, got %v", numbers)
	}
	for n := 1; n <= 5; n++ {
		if !seen[n] {
			t.Fatalf("numbers must be dense 1..5, got %v", numbers)
		}
	}
}

func TestVaultIsDeletedWithTheUserAndPurged(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	keys, _ := ParseKeys(testKey)
	v := &PGVault{Pool: env.Pool, Keys: keys}
	ctx := context.Background()
	gone := newUser(t, env, fmt.Sprintf("gone-%d@example.test", time.Now().UnixNano()))
	soft := newUser(t, env, fmt.Sprintf("soft-%d@example.test", time.Now().UnixNano()))
	old := newUser(t, env, fmt.Sprintf("old-%d@example.test", time.Now().UnixNano()))
	for _, u := range []int64{gone, soft, old} {
		if _, err := v.Assign(ctx, u, "PERSON", "люда", "Люда"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Pool.Exec(ctx, `delete from users where id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Pool.Exec(ctx, `update users set deleted_at = now() where id = $1`, soft); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Pool.Exec(ctx, `update pii_vault set last_used_at = now() - interval '200 days' where user_id = $1`, old); err != nil {
		t.Fatal(err)
	}
	n, err := Purge(ctx, env.Pool, 180*24*time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("purge: %d %v", n, err)
	}
	var left int
	_ = env.Pool.QueryRow(ctx, `select count(*) from pii_vault where user_id = any($1)`, []int64{gone, soft, old}).Scan(&left)
	if left != 0 {
		t.Fatalf("%d rows left", left)
	}
}

func TestServiceEndToEndWithPostgres(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	keys, _ := ParseKeys(testKey)
	det := &fakeDetector{values: map[string]string{"Люда": "PERSON"}}
	svc := &Service{Detector: det, Vault: &PGVault{Pool: env.Pool, Keys: keys}, Profiles: PGProfiles{Pool: env.Pool}}
	ctx := context.Background()
	u := newUser(t, env, fmt.Sprintf("e2e-%d@example.test", time.Now().UnixNano()))
	masked, ref, err := svc.Transform(ctx, u, "ru", "Мама Люда")
	if err != nil || masked != "Мама ЛИЦО_1" {
		t.Fatalf("%q %v", masked, err)
	}
	// the profile went to the detector as known values
	var gotProfile bool
	for _, k := range det.lastKnown {
		if k.Kind == "HANDLE" && k.Value == "zlata_k" {
			gotProfile = true
		}
	}
	if !gotProfile {
		t.Fatalf("profile not sent: %+v", det.lastKnown)
	}
	if got := svc.Restore(ctx, u, ref, "Как ЛИЦО_1?"); got != "Как Люда?" {
		t.Fatalf("restore: %q", got)
	}
	// the user's own phone typed by the user is refused if the detector missed it
	if _, _, err := svc.Transform(ctx, u, "ru", "мой номер 8 912 000 11 22"); err != ErrResidual {
		t.Fatalf("own phone must not leave: %v", err)
	}
}
