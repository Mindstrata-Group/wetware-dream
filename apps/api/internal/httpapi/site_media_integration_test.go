//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

func TestSiteMedia_AdminUpload_PublicRead_AndDedupe(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89,
	}

	status, body := uploadSiteMedia(t, ts, "cover.png", png, "blog")
	if status != http.StatusOK {
		t.Fatalf("upload status %d body=%v", status, body)
	}
	url, _ := body["url"].(string)
	if url == "" {
		t.Fatalf("expected public url, body=%v", body)
	}
	firstID := body["id"]

	status2, body2 := uploadSiteMedia(t, ts, "same.png", png, "blog")
	if status2 != http.StatusOK {
		t.Fatalf("second upload status %d body=%v", status2, body2)
	}
	if body2["id"] != firstID {
		t.Fatalf("dedupe must reuse id: first=%v second=%v", firstID, body2["id"])
	}

	resp, err := ts.Client.Get(ts.URL(url))
	if err != nil {
		t.Fatalf("public get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public get status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("content-type=%q want image/png", got)
	}

	var count int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from site_media where scope='blog'`).Scan(&count); err != nil {
		t.Fatalf("count site_media: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deduped media row, got %d", count)
	}
}

func TestSiteMedia_AdminUpload_RejectsNonImage(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))

	status, body := uploadSiteMedia(t, ts, "note.txt", []byte("hello"), "blog")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%v", status, body)
	}
}

func TestSiteMedia_ContentAdminCanUploadButSupportCannot(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89,
	}

	contentAdmin := f.CreateUser(TestUserOpts{Role: "content_admin"})
	ts.LoginAs(f.CreateSession(contentAdmin.ID))
	status, body := uploadSiteMedia(t, ts, "content-cover.png", png, "blog")
	if status != http.StatusOK {
		t.Fatalf("content_admin upload status=%d body=%v", status, body)
	}

	support := f.CreateUser(TestUserOpts{Role: "support"})
	ts.LoginAs(f.CreateSession(support.ID))
	status, _ = uploadSiteMedia(t, ts, "support-cover.png", png, "blog")
	if status != http.StatusForbidden {
		t.Fatalf("support upload status=%d, want 403", status)
	}
}

func TestSiteMedia_AdminCanDeleteUploadedMedia(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89,
	}

	admin := f.CreateUser(TestUserOpts{Role: "admin"})
	ts.LoginAs(f.CreateSession(admin.ID))
	status, body := uploadSiteMedia(t, ts, "delete-me.png", png, "blog")
	if status != http.StatusOK {
		t.Fatalf("upload status=%d body=%v", status, body)
	}
	id := int64(body["id"].(float64))

	req, err := http.NewRequest(http.MethodDelete, ts.URL(fmt.Sprintf("/api/admin/site-media?id=%d", id)), nil)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("delete media: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("delete status=%d body=%s", resp.StatusCode, string(raw))
	}

	var count int
	if err := env.Pool.QueryRow(context.Background(), `select count(*) from site_media where id=$1`, id).Scan(&count); err != nil {
		t.Fatalf("query site_media: %v", err)
	}
	if count != 0 {
		t.Fatalf("site_media row still exists after delete")
	}
}

// TestSiteMedia_DeleteInvalidID_400.
func TestSiteMedia_DeleteInvalidID_400(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	req, err := http.NewRequest(http.MethodDelete, ts.URL("/api/admin/site-media?id=not-a-number"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid id: %d, want 400", resp.StatusCode)
	}
}

// TestSiteMedia_DeleteNotFound_404.
func TestSiteMedia_DeleteNotFound_404(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	ts.LoginAs(f.CreateSession(f.CreateUser(TestUserOpts{Role: "admin"}).ID))

	req, err := http.NewRequest(http.MethodDelete, ts.URL("/api/admin/site-media?id=999999999"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing media: %d, want 404", resp.StatusCode)
	}
}

func uploadSiteMedia(t *testing.T, ts *TestServer, filename string, data []byte, scope string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("scope", scope); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL("/api/admin/site-media"), &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := ts.Client.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	return resp.StatusCode, parsed
}
