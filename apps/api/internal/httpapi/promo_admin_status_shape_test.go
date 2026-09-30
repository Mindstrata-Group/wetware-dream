//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestPromoAdminStatus_ContractFields: checks that the response of
// /api/promo-admin/status contains ALL fields the frontend depends on
// (usePromoStatusLoader, SummarizeForm). This is a contract test: if someone
// "cleans up" the backend, the messageCount/history/etc field disappears, the
// frontend breaks, and the test catches it.
//
// It also guards against the "summaryRemaining null vs 0" regression: the types
// differ, the frontend expects number|null.
func TestPromoAdminStatus_ContractFields(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{
		Code:     "CONTRACT_" + itoa(int64(env.Pool.Stat().AcquireCount())),
		TargetID: mode.ID,
	})
	user := f.CreateUser(TestUserOpts{})
	_, _ = env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now())`,
		user.ID, promo.ID)
	dialog := f.CreateDialog(user.ID, mode.ID)
	f.AppendMessage(dialog.ID, "user", "test message")

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET",
		"/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}

	// Top-level fields the frontend needs.
	requiredTop := []string{"ok", "promo", "prompts", "modes", "history"}
	for _, k := range requiredTop {
		if _, ok := body[k]; !ok {
			t.Errorf("response отсутствует поле %q", k)
		}
	}

	// The promo sub-object: fields usePromoStatusLoader and SummarizeForm depend on.
	promoMap, _ := body["promo"].(map[string]any)
	if promoMap == nil {
		t.Fatal("body.promo не объект")
	}
	requiredPromo := []string{
		"id", "code",
		"summaryUsed", "summaryRemaining",
		"messageCount", "activationsWithoutMessages",
	}
	for _, k := range requiredPromo {
		if _, ok := promoMap[k]; !ok {
			t.Errorf("promo отсутствует поле %q", k)
		}
	}

	// messageCount must be a number (not a string)
	if _, ok := promoMap["messageCount"].(float64); !ok {
		t.Errorf("messageCount=%v типа %T, want number", promoMap["messageCount"], promoMap["messageCount"])
	}

	// history must be an array (even if empty)
	if _, ok := body["history"].([]any); !ok {
		t.Errorf("history=%v типа %T, want []any", body["history"], body["history"])
	}
}

// TestPromoAdminStatus_EmptyState: a new promo code without activations and
// messages: all counters = 0, history empty. Guards against a nil panic in the
// counting formulas.
func TestPromoAdminStatus_EmptyState(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	mode := f.CreateMode(TestModeOpts{})
	promo := f.CreatePromocode(TestPromocodeOpts{
		Code:     "EMPTY_" + itoa(int64(env.Pool.Stat().AcquireCount())),
		TargetID: mode.ID,
	})

	key := promoAdminTokenForCode(ts.Handler, promo.Code)
	status, body := httpJSON(t, ts, "GET",
		"/api/promo-admin/status?promo="+promo.Code+"&key="+key, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%v", status, body)
	}

	promoMap, _ := body["promo"].(map[string]any)
	if mc, _ := promoMap["messageCount"].(float64); mc != 0 {
		t.Errorf("messageCount=%v, want 0", mc)
	}
	if act, _ := promoMap["activationsWithoutMessages"].(float64); act != 0 {
		t.Errorf("activationsWithoutMessages=%v, want 0", act)
	}
	history, _ := body["history"].([]any)
	if len(history) != 0 {
		t.Errorf("history len=%d, want 0", len(history))
	}
}
