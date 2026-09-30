//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

// authedUserWithDialog: helper. Creates a logged-in user with mode access and a dialog.
func authedUserWithDialog(t *testing.T, env *testsupport.Env, ts *TestServer, dailyLimit int) (*TestUser, *TestMode, *TestDialog) {
	t.Helper()
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: dailyLimit})
	dialog := f.CreateDialog(user.ID, mode.ID)
	// Make sure users.current_dialog is set — SendMessage falls back to it when req.DialogID == 0
	_, err := env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3, accepted_tos = true where id = $1`,
		user.ID, mode.ID, dialog.ID)
	if err != nil {
		t.Fatalf("update users.current_dialog: %v", err)
	}
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)
	return user, mode, dialog
}

// TestChat_SendMessage_TestModeHappyPath: send → 200, user+assistant messages saved.
func TestChat_SendMessage_TestModeHappyPath(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	user, _, dialog := authedUserWithDialog(t, env, ts, 50)

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID,
		"text":     "Hello, world",
	})
	if code != http.StatusOK {
		t.Fatalf("send: got %d body=%v", code, body)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("response ok=false: %v", body)
	}

	// DB: 2 messages must exist (user + assistant)
	var count int64
	err := env.Pool.QueryRow(context.Background(),
		"select count(*) from dialogs_messages where dialog_id = $1", dialog.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 2 {
		t.Fatalf("messages: got %d want 2", count)
	}

	// Counter incremented
	var used int64
	_ = env.Pool.QueryRow(context.Background(),
		"select count from daily_message_counts where user_id = $1 and date = current_date",
		user.ID).Scan(&used)
	if used != 1 {
		t.Fatalf("daily count: got %d want 1", used)
	}

	// Quota in the response: Used=1, Remaining=49
	q, _ := body["quota"].(map[string]any)
	if q == nil {
		t.Fatalf("missing quota in response: %v", body)
	}
	if usedJSON, _ := q["used"].(float64); int(usedJSON) != 1 {
		t.Fatalf("quota.used: got %v want 1", q["used"])
	}
}

func TestChat_SendMessage_OrchestrationPersistsSwitchTrace(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode1 := f.CreateMode(TestModeOpts{Name: "Уточнить фразу"})
	mode2 := f.CreateMode(TestModeOpts{Name: "Переговоры"})
	_, _ = env.Pool.Exec(context.Background(), `update modes set orchestrator_check_interval = 1 where id = $1`, mode1.ID)
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode1.ID, DailyMessageLimit: 50})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode2.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, mode1.ID)
	_, _ = env.Pool.Exec(context.Background(),
		`update users set current_mode=$2, current_dialog=$3, accepted_tos=true where id=$1`,
		user.ID, mode1.ID, dialog.ID)

	fake := fakeVseGPT(t, http.StatusOK, `{"modeId":`+itoa(mode2.ID)+`,"reason":"switch"}`)
	ts := NewTestServerWithHandler(t, env.Pool, Handler{
		OpenAIAPIKey:  "k",
		OpenAIBaseURL: "https://api.vsegpt.ru/v1",
		HTTPClient:    &http.Client{Transport: vsegptRT{target: fake.URL}, Timeout: 5 * time.Second},
	})
	ts.LoginAs(f.CreateSession(user.ID))

	status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":         dialog.ID,
		"text":             "первое сообщение после промокода",
		"responseMode":     "live",
		"knowledgeModeIds": []int64{mode1.ID, mode2.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("send: %d body=%v", status, body)
	}
	if switched, _ := body["modeSwitched"].(bool); !switched {
		t.Fatalf("modeSwitched=false body=%v", body)
	}
	trace, ok := body["switchTrace"].(map[string]any)
	if !ok {
		t.Fatalf("switchTrace missing: %v", body)
	}
	if trace["role"] != "system" {
		t.Fatalf("switchTrace role=%v, want system", trace["role"])
	}
	content, _ := trace["content"].(string)
	if !strings.Contains(content, "Стратум переключил режим") || !strings.Contains(content, "Уточнить фразу") || !strings.Contains(content, "Переговоры") {
		t.Fatalf("unexpected switchTrace content: %q", content)
	}

	var systemCount int64
	if err := env.Pool.QueryRow(context.Background(), `
		select count(*)
		from dialogs_messages
		where dialog_id = $1
		  and role = 'system'
		  and content like $2`, dialog.ID, modeSwitchTracePrefix+"%").Scan(&systemCount); err != nil {
		t.Fatalf("count trace: %v", err)
	}
	if systemCount != 1 {
		t.Fatalf("system trace count=%d, want 1", systemCount)
	}

	var mode1AccessID, mode2AccessID int64
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 order by id desc limit 1`,
		user.ID, mode1.ID).Scan(&mode1AccessID); err != nil {
		t.Fatalf("query mode1 access: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 order by id desc limit 1`,
		user.ID, mode2.ID).Scan(&mode2AccessID); err != nil {
		t.Fatalf("query mode2 access: %v", err)
	}

	rows, err := env.Pool.Query(context.Background(), `
		select dm.role, dmau.mode_id, dmau.access_id
		from dialogs_messages dm
		join dialog_message_access_usage dmau on dmau.dialog_message_id = dm.id
		where dm.dialog_id = $1
		  and dm.role in ('user', 'assistant')
		order by dm.id asc`, dialog.ID)
	if err != nil {
		t.Fatalf("query message modes: %v", err)
	}
	defer rows.Close()
	gotModes := map[string]int64{}
	gotAccessIDs := map[string]int64{}
	for rows.Next() {
		var role string
		var attributedModeID int64
		var accessID int64
		if err := rows.Scan(&role, &attributedModeID, &accessID); err != nil {
			t.Fatalf("scan message modes: %v", err)
		}
		gotModes[role] = attributedModeID
		gotAccessIDs[role] = accessID
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("message mode rows: %v", err)
	}
	if gotModes["user"] != mode1.ID {
		t.Fatalf("user attributed mode=%d want original %d", gotModes["user"], mode1.ID)
	}
	if gotModes["assistant"] != mode2.ID {
		t.Fatalf("assistant attributed mode=%d want switched %d", gotModes["assistant"], mode2.ID)
	}
	if gotAccessIDs["user"] != mode1AccessID {
		t.Fatalf("user access_id=%d want original access %d", gotAccessIDs["user"], mode1AccessID)
	}
	if gotAccessIDs["assistant"] != mode2AccessID {
		t.Fatalf("assistant access_id=%d want switched access %d", gotAccessIDs["assistant"], mode2AccessID)
	}

	status, body = httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId":         dialog.ID,
		"text":             "продолжить после переключения",
		"responseMode":     "test",
		"knowledgeModeIds": []int64{mode1.ID, mode2.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("second send: %d body=%v", status, body)
	}
	if got, _ := body["modeId"].(float64); int64(got) != mode2.ID {
		t.Fatalf("second send modeId=%v want switched mode %d", body["modeId"], mode2.ID)
	}

	rows2, err := env.Pool.Query(context.Background(), `
		select dm.role, dmau.mode_id, dmau.access_id
		from dialogs_messages dm
		join dialog_message_access_usage dmau on dmau.dialog_message_id = dm.id
		where dm.dialog_id = $1
		  and dm.role in ('user', 'assistant')
		order by dm.id desc
		limit 2`, dialog.ID)
	if err != nil {
		t.Fatalf("query second message modes: %v", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var role string
		var attributedModeID int64
		var accessID int64
		if err := rows2.Scan(&role, &attributedModeID, &accessID); err != nil {
			t.Fatalf("scan second message modes: %v", err)
		}
		if attributedModeID != mode2.ID {
			t.Fatalf("second %s attributed mode=%d want switched %d", role, attributedModeID, mode2.ID)
		}
		if accessID != mode2AccessID {
			t.Fatalf("second %s access_id=%d want switched access %d", role, accessID, mode2AccessID)
		}
	}
	if err := rows2.Err(); err != nil {
		t.Fatalf("second message rows: %v", err)
	}
}

func TestChat_HistoryReturnsPerMessageModeAttribution(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	originalMode := f.CreateMode(TestModeOpts{Name: "Режим сообщения"})
	currentMode := f.CreateMode(TestModeOpts{Name: "Текущий режим диалога"})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: originalMode.ID, DailyMessageLimit: 50})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: currentMode.ID, DailyMessageLimit: 50})
	dialog := f.CreateDialog(user.ID, currentMode.ID)
	if _, err := env.Pool.Exec(context.Background(), `
		update users
		set current_mode = $2, current_dialog = $3, accepted_tos = true
		where id = $1`,
		user.ID, currentMode.ID, dialog.ID); err != nil {
		t.Fatalf("update user current dialog: %v", err)
	}
	messageID := f.AppendMessage(dialog.ID, "assistant", "Ответ с сохранённым режимом")
	ts.LoginAs(f.CreateSession(user.ID))

	var accessID int64
	if err := env.Pool.QueryRow(context.Background(), `
		select id
		from user_mode_access
		where user_id = $1 and mode_id = $2
		order by id desc
		limit 1`, user.ID, originalMode.ID).Scan(&accessID); err != nil {
		t.Fatalf("query access id: %v", err)
	}
	if _, err := env.Pool.Exec(context.Background(), `
		insert into dialog_message_access_usage (user_id, mode_id, dialog_message_id, access_id, usage_date, usage_kind)
		values ($1, $2, $3, $4, current_date, 'message')`,
		user.ID, originalMode.ID, messageID, accessID); err != nil {
		t.Fatalf("insert message attribution: %v", err)
	}

	status, body := httpJSON(t, ts, http.MethodGet, "/api/chat/history?dialogId="+itoa(dialog.ID)+"&limit=20", nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%v", status, body)
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages=%T %v", body["messages"], body["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("message type=%T", messages[0])
	}
	if got, _ := message["modeName"].(string); got != originalMode.Name {
		t.Fatalf("message modeName=%q want %q body=%v", got, originalMode.Name, body)
	}
	if got, _ := message["modeId"].(float64); int64(got) != originalMode.ID {
		t.Fatalf("message modeId=%v want %d body=%v", message["modeId"], originalMode.ID, body)
	}
}

func TestChat_SendMessage_AttributesQuotaToLatestPromocodeThenFallsBack(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{Name: "Promo Attribution Mode"})
	oldPromo := f.CreatePromocode(TestPromocodeOpts{Code: "CHATOLD_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	newPromo := f.CreatePromocode(TestPromocodeOpts{Code: "CHATNEW_" + itoa(int64(env.Pool.Stat().AcquireCount())), TargetID: mode.ID})
	if _, err := env.Pool.Exec(context.Background(),
		`insert into promocode_usages (user_id, promocode_id, used_at) values ($1, $2, now() - interval '2 hours'), ($1, $3, now() - interval '1 hour')`,
		user.ID, oldPromo.ID, newPromo.ID); err != nil {
		t.Fatalf("seed promo usages: %v", err)
	}
	oldFrom := time.Now().Add(-2 * time.Hour)
	newFrom := time.Now().Add(-1 * time.Hour)
	oldSource, newSource := oldPromo.ID, newPromo.ID
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 10, AccessType: "promocode", SourceID: &oldSource, ActiveFrom: &oldFrom})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 1, AccessType: "promocode", SourceID: &newSource, ActiveFrom: &newFrom})
	dialog := f.CreateDialog(user.ID, mode.ID)
	if _, err := env.Pool.Exec(context.Background(),
		`update users set current_mode=$2, current_dialog=$3, accepted_tos=true where id=$1`,
		user.ID, mode.ID, dialog.ID); err != nil {
		t.Fatalf("set current dialog: %v", err)
	}
	ts.LoginAs(f.CreateSession(user.ID))

	for i, text := range []string{"first message should spend newest promo", "second message falls back to old promo"} {
		status, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
			"dialogId": dialog.ID,
			"text":     text,
		})
		if status != http.StatusOK {
			t.Fatalf("send #%d: status=%d body=%v", i+1, status, body)
		}
	}

	var oldAccessID, newAccessID int64
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 and access_type='promocode' and source_id=$3`,
		user.ID, mode.ID, oldPromo.ID).Scan(&oldAccessID); err != nil {
		t.Fatalf("query old access: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select id from user_mode_access where user_id=$1 and mode_id=$2 and access_type='promocode' and source_id=$3`,
		user.ID, mode.ID, newPromo.ID).Scan(&newAccessID); err != nil {
		t.Fatalf("query new access: %v", err)
	}

	rows, err := env.Pool.Query(context.Background(), `
		select dm.role, dmau.access_id
		from dialogs_messages dm
		join dialog_message_access_usage dmau on dmau.dialog_message_id = dm.id
		where dm.dialog_id = $1
		order by dm.id asc`, dialog.ID)
	if err != nil {
		t.Fatalf("query message attribution: %v", err)
	}
	defer rows.Close()
	accessIDs := []int64{}
	for rows.Next() {
		var role string
		var accessID int64
		if err := rows.Scan(&role, &accessID); err != nil {
			t.Fatalf("scan attribution: %v", err)
		}
		accessIDs = append(accessIDs, accessID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	want := []int64{newAccessID, newAccessID, oldAccessID, oldAccessID}
	if len(accessIDs) != len(want) {
		t.Fatalf("attribution rows=%v want %v", accessIDs, want)
	}
	for i := range want {
		if accessIDs[i] != want[i] {
			t.Fatalf("attribution[%d]=%d want %d; all=%v", i, accessIDs[i], want[i], accessIDs)
		}
	}

	var oldUsed, newUsed int64
	if err := env.Pool.QueryRow(context.Background(),
		`select messages_used from daily_mode_usage where user_id=$1 and mode_id=$2 and access_id=$3 and usage_date=current_date`,
		user.ID, mode.ID, oldAccessID).Scan(&oldUsed); err != nil {
		t.Fatalf("old daily usage: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(),
		`select messages_used from daily_mode_usage where user_id=$1 and mode_id=$2 and access_id=$3 and usage_date=current_date`,
		user.ID, mode.ID, newAccessID).Scan(&newUsed); err != nil {
		t.Fatalf("new daily usage: %v", err)
	}
	if oldUsed != 1 || newUsed != 1 {
		t.Fatalf("daily_mode_usage old=%d new=%d want 1/1", oldUsed, newUsed)
	}
}

// TestChat_SendMessage_EmptyText: empty text → 400.
func TestChat_SendMessage_EmptyText(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)
	_, _, dialog := authedUserWithDialog(t, env, ts, 50)

	code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID,
		"text":     "   ",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty text, got %d", code)
	}
}

func TestChat_SendMessage_MessageTooLong_DefaultLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	_, _, dialog := authedUserWithDialog(t, env, ts, 50)

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID,
		"text":     strings.Repeat("т", defaultChatMessageMaxChars+1),
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too long text, got %d body=%v", code, body)
	}
	if codeVal, _ := body["code"].(string); codeVal != "message_too_long" {
		t.Fatalf("expected code=message_too_long, got=%v", body["code"])
	}
	if maxChars, _ := body["maxChars"].(float64); int(maxChars) != defaultChatMessageMaxChars {
		t.Fatalf("maxChars=%v want %d", maxChars, defaultChatMessageMaxChars)
	}
}

func TestChat_SendMessage_RespectsConfiguredMessageLimit(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	_, _, dialog := authedUserWithDialog(t, env, ts, 50)
	if _, err := env.Pool.Exec(context.Background(), `
		insert into system_settings (key, value) values ($1, $2)
		on conflict (key) do update set value=excluded.value`,
		"chat_message_max_chars", "8"); err != nil {
		t.Fatalf("set chat_message_max_chars: %v", err)
	}

	code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID,
		"text":     "12345678",
	})
	if code != http.StatusOK {
		t.Fatalf("expected configured 8-char message to pass, got %d", code)
	}

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID,
		"text":     "123456789",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for configured limit exceed, got %d body=%v", code, body)
	}
	if codeVal, _ := body["code"].(string); codeVal != "message_too_long" {
		t.Fatalf("expected code=message_too_long, got=%v", body["code"])
	}
}

// TestChat_SendMessage_NoAccess: user without a grant → 403 access_required.
func TestChat_SendMessage_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": 999, "text": "test",
	})
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%v", code, body)
	}
	if codeStr, _ := body["code"].(string); codeStr != "access_required" {
		t.Fatalf("expected code=access_required, got %v", body["code"])
	}
}

// TestChat_SendMessage_QuotaExhausted: Remaining=0 → 429.
// The message must NOT be inserted.
func TestChat_SendMessage_QuotaExhausted(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	_, _, dialog := authedUserWithDialog(t, env, ts, 1)

	// First message: ok
	code1, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "first",
	})
	if code1 != http.StatusOK {
		t.Fatalf("first send: %d", code1)
	}

	// Second: 429
	code2, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": dialog.ID, "text": "second",
	})
	if code2 != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on quota exhaustion, got %d body=%v", code2, body)
	}
	if codeStr, _ := body["code"].(string); codeStr != "daily_quota_exhausted" {
		t.Fatalf("expected code=daily_quota_exhausted, got %v", body["code"])
	}

	// DB check: still only 2 messages (from the first send), no third user msg
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1 and content = 'second'`,
		dialog.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("rejected message was inserted anyway")
	}
}

// TestChat_SendMessage_ForeignDialog: trying to post into someone else's dialog → 403.
func TestChat_SendMessage_ForeignDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	// Victim: has access + a dialog
	victim := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: victim.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	victimDialog := f.CreateDialog(victim.ID, mode.ID)
	_ = victim

	// Attacker: also has access to THE SAME mode, but tries to write into the victim's dialog
	attacker := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: attacker.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	attackerDialog := f.CreateDialog(attacker.ID, mode.ID)
	_, _ = env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3 where id = $1`,
		attacker.ID, mode.ID, attackerDialog.ID)
	token := f.CreateSession(attacker.ID)
	ts.LoginAs(token)

	code, body := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
		"dialogId": victimDialog.ID, "text": "stealing",
	})
	if code != http.StatusForbidden && code != http.StatusBadRequest {
		t.Fatalf("expected 403 or 400 on foreign dialog, got %d body=%v", code, body)
	}

	// DB: the victim's dialog is not modified
	var msgCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1`,
		victimDialog.ID).Scan(&msgCount)
	if msgCount != 0 {
		t.Fatalf("foreign dialog was modified: %d messages", msgCount)
	}
}

// TestChat_SendMessage_RaceConditionOnLastSlot verifies the fix for the
// quota check-then-increment race. SendMessage now uses
// tryIncrementDailyMessageCount which does atomic UPDATE ... WHERE
// count+1<=limit RETURNING. 10 concurrent requests on a limit of 3 must
// produce exactly 3 successes and 7 rejections.
func TestChat_SendMessage_RaceConditionOnLastSlot(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 3})
	dialog := f.CreateDialog(user.ID, mode.ID)
	_, _ = env.Pool.Exec(context.Background(),
		`update users set current_mode = $2, current_dialog = $3 where id = $1`,
		user.ID, mode.ID, dialog.ID)

	const N = 10
	const limit = 3

	var ok, denied atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts := NewTestServer(t, env.Pool)
			token := f.CreateSession(user.ID)
			ts.LoginAs(token)
			code, _ := httpJSON(t, ts, "POST", "/api/chat/send", map[string]any{
				"dialogId": dialog.ID,
				"text":     "race attempt",
			})
			switch code {
			case http.StatusOK:
				ok.Add(1)
			case http.StatusTooManyRequests:
				denied.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := ok.Load(); got != limit {
		t.Fatalf("quota race: got %d successes, want exactly %d (denied=%d)",
			got, limit, denied.Load())
	}
	if got := denied.Load(); got != N-limit {
		t.Fatalf("quota race: got %d denied, want %d", got, N-limit)
	}

	// DB invariant: counter equals limit exactly.
	var count int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&count)
	if count != limit {
		t.Fatalf("daily counter: got %d want %d", count, limit)
	}

	// DB invariant: only `limit` user messages stored — no orphans.
	var msgCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1 and role = 'user'`,
		dialog.ID).Scan(&msgCount)
	if msgCount != limit {
		t.Fatalf("user messages in DB: got %d want %d (orphans from failed sends)",
			msgCount, limit)
	}
}

// TestChat_GetHistory_ReturnsInChronologicalOrder: history in chronological order.
func TestChat_GetHistory_ReturnsInChronologicalOrder(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	_, _, dialog := authedUserWithDialog(t, env, ts, 50)

	f.AppendMessage(dialog.ID, "user", "msg-1")
	f.AppendMessage(dialog.ID, "assistant", "msg-2")
	f.AppendMessage(dialog.ID, "user", "msg-3")

	code, body := httpJSON(t, ts, "GET", "/api/chat/history?dialogId="+itoa(dialog.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("history: %d body=%v", code, body)
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("history len: got %d want 3 body=%v", len(msgs), body)
	}
	// Content in insertion order
	for i, expected := range []string{"msg-1", "msg-2", "msg-3"} {
		m, _ := msgs[i].(map[string]any)
		if content, _ := m["content"].(string); content != expected {
			t.Fatalf("msgs[%d].content: got %q want %q", i, content, expected)
		}
	}
}

// TestChat_GetHistory_LimitWorks: limit=2 → the last 2 messages.
func TestChat_GetHistory_LimitWorks(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	_, _, dialog := authedUserWithDialog(t, env, ts, 50)

	for i := 1; i <= 5; i++ {
		f.AppendMessage(dialog.ID, "user", "msg-"+itoa(int64(i)))
	}

	code, body := httpJSON(t, ts, "GET", "/api/chat/history?dialogId="+itoa(dialog.ID)+"&limit=2", nil)
	if code != http.StatusOK {
		t.Fatalf("history: %d", code)
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("limit=2: got %d messages", len(msgs))
	}
}

// TestChat_DeleteHistory_SoftDeletesDialog: DELETE → dialog with deleted_at, current_dialog=null.
func TestChat_DeleteHistory_SoftDeletesDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	ts := NewTestServer(t, env.Pool)

	user, _, dialog := authedUserWithDialog(t, env, ts, 50)

	code, _ := httpJSON(t, ts, "DELETE", "/api/chat/history?dialogId="+itoa(dialog.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}

	// DB: dialog marked deleted_at
	var deleted bool
	_ = env.Pool.QueryRow(context.Background(),
		`select deleted_at is not null from users_dialogs where id = $1`,
		dialog.ID).Scan(&deleted)
	if !deleted {
		t.Fatalf("dialog not soft-deleted")
	}

	// DB: the user's current_dialog is cleared
	var currentDialog *int64
	_ = env.Pool.QueryRow(context.Background(),
		`select current_dialog from users where id = $1`,
		user.ID).Scan(&currentDialog)
	if currentDialog != nil {
		t.Fatalf("user.current_dialog not cleared after delete: %d", *currentDialog)
	}
}

// TestChat_CompleteDialog_TestMode: summary inserted, counter incremented.
func TestChat_CompleteDialog_TestMode(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user, _, dialog := authedUserWithDialog(t, env, ts, 50)

	// Add a few messages to the dialog so the summary makes sense
	f.AppendMessage(dialog.ID, "user", "context-1")
	f.AppendMessage(dialog.ID, "assistant", "context-2")

	code, body := httpJSON(t, ts, "POST", "/api/chat/complete", map[string]any{
		"dialogId": dialog.ID,
	})
	if code != http.StatusOK {
		t.Fatalf("complete: %d body=%v", code, body)
	}

	// DB: a row with role='summary' appeared
	var summaryCount int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count(*) from dialogs_messages where dialog_id = $1 and role = 'summary'`,
		dialog.ID).Scan(&summaryCount)
	if summaryCount != 1 {
		t.Fatalf("summary not created (got count=%d)", summaryCount)
	}

	// Counter incremented (the summary also counts towards the quota)
	var used int64
	_ = env.Pool.QueryRow(context.Background(),
		`select count from daily_message_counts where user_id = $1 and date = current_date`,
		user.ID).Scan(&used)
	if used != 1 {
		t.Fatalf("summary did not increment counter: got %d", used)
	}
}

// TestChat_SelectMode_NoAccess: SelectMode without a grant → 403 access_required.
func TestChat_SelectMode_NoAccess(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	code, body := httpJSON(t, ts, "POST", "/api/chat/select-mode", map[string]any{
		"modeId": 1, "newDialog": true,
	})
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%v", code, body)
	}
}

// TestChat_SelectMode_CreatesDialog: with a grant → dialog created, current_dialog updated.
func TestChat_SelectMode_CreatesDialog(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID, DailyMessageLimit: 50})
	token := f.CreateSession(user.ID)
	ts.LoginAs(token)

	code, body := httpJSON(t, ts, "POST", "/api/chat/select-mode", map[string]any{
		"modeId": mode.ID, "newDialog": true,
	})
	if code != http.StatusOK {
		t.Fatalf("select-mode: %d body=%v", code, body)
	}

	// DB: dialog created, the user's current_dialog updated
	var currentDialog *int64
	var currentMode *int64
	_ = env.Pool.QueryRow(context.Background(),
		`select current_mode, current_dialog from users where id = $1`,
		user.ID).Scan(&currentMode, &currentDialog)
	if currentDialog == nil || currentMode == nil || *currentMode != mode.ID {
		t.Fatalf("user state: mode=%v dialog=%v want mode=%d", currentMode, currentDialog, mode.ID)
	}
}

// itoa: a fast int64→string without strconv on the hot path.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
