//go:build integration

package httpapi

import (
	"context"
	"net/http"
	"testing"

	"mindstrata-stage1/api/internal/testsupport"
)

// TestChatAttachments_RejectsUnsupportedExtension (Standards: only
// txt/md/doc/docx are declared as supported; .exe must be rejected with a
// clear error, not silently accepted or crash).
func TestChatAttachments_RejectsUnsupportedExtension(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	ts.LoginAs(f.CreateSession(user.ID))

	status, parsed := uploadTestAttachmentStatus(t, ts, "virus.exe", "application/octet-stream", []byte("MZ\x00\x00fake binary"))
	if status != http.StatusBadRequest || parsed.OK {
		t.Fatalf("upload .exe: status=%d ok=%v, want 400/false", status, parsed.OK)
	}
}

// TestChatAttachments_RejectsEmptyFile: an empty file gets a clear error.
func TestChatAttachments_RejectsEmptyFile(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	ts.LoginAs(f.CreateSession(user.ID))

	status, parsed := uploadTestAttachmentStatus(t, ts, "empty.txt", "text/plain", []byte("   \n\t  "))
	if status != http.StatusBadRequest || parsed.OK {
		t.Fatalf("upload whitespace-only file: status=%d ok=%v, want 400/false", status, parsed.OK)
	}
}

// TestChatAttachments_PathTraversalFilename_SanitizedNotRejected (Image: the
// Content-Disposition filename may carry "../../etc/passwd"; that must NOT reach
// disk/DB as a path; sanitizeAttachmentFilename already calls filepath.Base,
// here we check the effect end-to-end through a real HTTP request).
func TestChatAttachments_PathTraversalFilename_SanitizedNotRejected(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	user := f.CreateUser(TestUserOpts{})
	mode := f.CreateMode(TestModeOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: user.ID, ModeID: mode.ID})
	ts.LoginAs(f.CreateSession(user.ID))

	att := uploadTestAttachment(t, ts, "../../../etc/passwd.txt", "text/plain", []byte("содержимое файла"))
	if att.FileName != "passwd.txt" {
		t.Fatalf("filename = %q, want sanitized basename %q (path components must be stripped)", att.FileName, "passwd.txt")
	}
}

// TestChatAttachments_SharedBlobDedupeAcrossUsers (World: two DIFFERENT users
// upload a byte-for-byte identical file: the blob is reused
// (insertChatFileForBlob) rather than a duplicate content copy being created;
// chat_files is still a separate row per owner).
func TestChatAttachments_SharedBlobDedupeAcrossUsers(t *testing.T) {
	t.Parallel()
	env := testsupport.NewEnv(t)
	f := NewFactory(t, env.Pool)
	ts := NewTestServer(t, env.Pool)
	mode := f.CreateMode(TestModeOpts{})

	userA := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: userA.ID, ModeID: mode.ID})
	userB := f.CreateUser(TestUserOpts{})
	f.GrantAccess(GrantAccessOpts{UserID: userB.ID, ModeID: mode.ID})

	content := []byte("Общий для двух пользователей контент файла")

	ts.LoginAs(f.CreateSession(userA.ID))
	attA := uploadTestAttachment(t, ts, "shared.txt", "text/plain", content)

	ts.LoginAs(f.CreateSession(userB.ID))
	attB := uploadTestAttachment(t, ts, "shared-b.txt", "text/plain", content)

	if attA.ID == attB.ID {
		t.Fatalf("different owners must get distinct chat_files rows, got same id %d", attA.ID)
	}
	if attB.Duplicate {
		t.Fatalf("userB's first-ever upload of this content must not be marked duplicate (dedupe is per-owner)")
	}

	var blobCount int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from chat_file_blobs where sha256_hex=$1`, attA.SHA256).Scan(&blobCount); err != nil {
		t.Fatalf("count blobs: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("blob count = %d, want 1 (content-addressed blob must be shared across owners)", blobCount)
	}

	var fileCount int
	if err := env.Pool.QueryRow(context.Background(),
		`select count(*) from chat_files where blob_id = (select id from chat_file_blobs where sha256_hex=$1)`, attA.SHA256).Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if fileCount != 2 {
		t.Fatalf("file count = %d, want 2 (one chat_files row per owner)", fileCount)
	}
}
