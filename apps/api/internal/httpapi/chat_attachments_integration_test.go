//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestChatAttachments_UploadDedupeAndSendLinksOnce(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	dialog := f.CreateDialog(user.ID, mode.ID)
	ts.LoginAs(f.CreateSession(user.ID))

	first := uploadTestAttachment(t, ts, "brief.md", "text/markdown", []byte("# Бриф\nСегмент: собственники микробизнеса."))
	second := uploadTestAttachment(t, ts, "brief-copy.md", "text/markdown", []byte("# Бриф\nСегмент: собственники микробизнеса."))
	if first.ID == 0 {
		t.Fatalf("first attachment id is empty")
	}
	if second.ID != first.ID {
		t.Fatalf("dedupe id: got %d want %d", second.ID, first.ID)
	}
	if !second.Duplicate {
		t.Fatalf("second upload must be marked duplicate")
	}

	status, body := httpJSON(t, ts, http.MethodPost, "/api/chat/send", map[string]any{
		"dialogId":      dialog.ID,
		"text":          "Сделай вывод по приложенному брифу",
		"responseMode":  "test",
		"attachmentIds": []int64{first.ID, first.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("send status=%d body=%v", status, body)
	}
	userMsg, _ := body["user"].(map[string]any)
	if content, _ := userMsg["content"].(string); strings.Contains(content, "Сегмент:") {
		t.Fatalf("visible chat message leaked source file text: %q", content)
	}

	var fileCount, blobCount, linkCount int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from chat_files where owner_user_id=$1`, user.ID).Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if fileCount != 1 {
		t.Fatalf("chat_files count=%d want 1", fileCount)
	}
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from chat_file_blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count blobs: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("chat_file_blobs count=%d want 1", blobCount)
	}
	messageID := int64(userMsg["id"].(float64))
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from chat_message_attachments where message_id=$1`, messageID).Scan(&linkCount); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("attachment links=%d want 1", linkCount)
	}
	var source []byte
	var statusText string
	if err := env.Pool.QueryRow(context.Background(), `
		select b.source_bytes, b.annotation_status
		from chat_files f
		join chat_file_blobs b on b.id = f.blob_id
		where f.id=$1`, first.ID).Scan(&source, &statusText); err != nil {
		t.Fatalf("query source: %v", err)
	}
	if string(source) != "# Бриф\nСегмент: собственники микробизнеса." {
		t.Fatalf("source bytes were not preserved: %q", string(source))
	}
	if statusText != "direct" {
		t.Fatalf("annotation_status=%q want direct", statusText)
	}
}

func TestChatAttachments_AnnotationRunsOnceForDuplicateLargeFile(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeOpenAISuccessForTest(t, w, "аннотация: ключевые выводы")
	}))
	t.Cleanup(fake.Close)

	handler := Handler{
		DB:            env.Pool,
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: fake.URL,
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	}
	upsertSetting(t, handler, "ai_queue_enabled", "0")
	upsertSetting(t, handler, "chat_attachment_direct_max_bytes", "4")
	upsertSetting(t, handler, "chat_attachment_annotation_model", "openai/gpt-4o-mini")
	upsertSetting(t, handler, "chat_attachment_annotation_prompt", "Сожми файл для теста")
	ts := NewTestServerWithHandler(t, env.Pool, handler)
	ts.LoginAs(f.CreateSession(user.ID))

	payload := []byte("длинный текст для аннотации")
	first := uploadTestAttachment(t, ts, "long.txt", "text/plain", payload)
	second := uploadTestAttachment(t, ts, "long-again.txt", "text/plain", payload)
	if first.ID != second.ID || !second.Duplicate {
		t.Fatalf("duplicate upload mismatch: first=%+v second=%+v", first, second)
	}
	other := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: other.ID, ModeID: mode.ID})
	ts.LoginAs(f.CreateSession(other.ID))
	third := uploadTestAttachment(t, ts, "long-other-user.txt", "text/plain", payload)
	if third.ID == first.ID || third.Duplicate {
		t.Fatalf("cross-user upload must create a new user attachment on existing blob: first=%+v third=%+v", first, third)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("annotation calls=%d want 1", got)
	}
	var blobCount, fileCount int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from chat_file_blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count blobs: %v", err)
	}
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from chat_files`).Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if blobCount != 1 || fileCount != 2 {
		t.Fatalf("blobCount=%d fileCount=%d want 1/2", blobCount, fileCount)
	}
	var prepared, model, prompt, status string
	if err := env.Pool.QueryRow(context.Background(), `
		select b.prepared_text, b.annotation_model, b.annotation_prompt, b.annotation_status
		from chat_files f
		join chat_file_blobs b on b.id = f.blob_id
		where f.id=$1`, first.ID).Scan(&prepared, &model, &prompt, &status); err != nil {
		t.Fatalf("query annotation: %v", err)
	}
	if prepared != "аннотация: ключевые выводы" || status != "annotated" {
		t.Fatalf("prepared=%q status=%q", prepared, status)
	}
	if model != "openai/gpt-4o-mini" || prompt != "Сожми файл для теста" {
		t.Fatalf("annotation settings not persisted on file: model=%q prompt=%q", model, prompt)
	}
}

func TestChatAttachments_UploadRequiresActiveModeAccess(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	ts.LoginAs(f.CreateSession(user.ID))

	status, parsed := uploadTestAttachmentStatus(t, ts, "notes.txt", "text/plain", []byte("проверка доступа"))
	if status != http.StatusForbidden {
		t.Fatalf("upload without access status=%d body=%+v, want 403", status, parsed)
	}
	if parsed.Error != "access required" {
		t.Fatalf("upload without access error=%q, want access required", parsed.Error)
	}
}

func TestChatAttachments_AdminSettingsPersist(t *testing.T) {
	t.Parallel()

	env := testsupport.NewEnv(t)
	ts, _ := loginAdminForAISettings(t, env)

	status, body := httpJSON(t, ts, http.MethodPost, "/api/admin/ai-settings", aiSettingsPayload(map[string]any{
		"attachmentDirectMaxBytes":   "8192",
		"attachmentMaxUploadBytes":   "1048576",
		"attachmentContextMaxChars":  "12000",
		"attachmentAnnotationModel":  "openai/gpt-4o-mini",
		"attachmentAnnotationPrompt": "Сделай краткую аннотацию",
	}))
	if status != http.StatusOK {
		t.Fatalf("POST ai-settings status=%d body=%v", status, body)
	}
	status, body = httpJSON(t, ts, http.MethodGet, "/api/admin/ai-settings", nil)
	if status != http.StatusOK {
		t.Fatalf("GET ai-settings status=%d body=%v", status, body)
	}
	for key, want := range map[string]string{
		"attachmentDirectMaxBytes":   "8192",
		"attachmentMaxUploadBytes":   "1048576",
		"attachmentContextMaxChars":  "12000",
		"attachmentAnnotationModel":  "openai/gpt-4o-mini",
		"attachmentAnnotationPrompt": "Сделай краткую аннотацию",
	} {
		if got, _ := body[key].(string); got != want {
			t.Fatalf("%s=%q want %q body=%v", key, got, want, body)
		}
	}
}

func uploadTestAttachment(t *testing.T, ts *TestServer, filename, contentType string, payload []byte) ChatAttachment {
	t.Helper()
	status, parsed := uploadTestAttachmentStatus(t, ts, filename, contentType, payload)
	if status != http.StatusOK || !parsed.OK {
		t.Fatalf("upload status=%d ok=%v error=%q", status, parsed.OK, parsed.Error)
	}
	return parsed.Attachment
}

func uploadTestAttachmentStatus(t *testing.T, ts *TestServer, filename, contentType string, payload []byte) (int, struct {
	OK         bool           `json:"ok"`
	Attachment ChatAttachment `json:"attachment"`
	Error      string         `json:"error"`
	Code       string         `json:"code"`
}) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL("/api/chat/attachments"), &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		req.Header.Set("X-Test-Content-Type", contentType)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}
	defer resp.Body.Close()
	var parsed struct {
		OK         bool           `json:"ok"`
		Attachment ChatAttachment `json:"attachment"`
		Error      string         `json:"error"`
		Code       string         `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return resp.StatusCode, parsed
}
