package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const siteMediaMaxBytes = 5 << 20

var allowedSiteMediaTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
	"image/gif":  {},
}

// AdminSiteMediaUpload: POST /api/admin/site-media uploads and deletes images for the CMS.
func (h Handler) AdminSiteMediaUpload(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireRole(w, r, "owner", "admin", "content_admin")
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		h.adminSiteMediaDelete(w, r, actor.ID)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, siteMediaMaxBytes+1<<20)
	if err := r.ParseMultipartForm(siteMediaMaxBytes + 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid multipart payload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, siteMediaMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "cannot read file"})
		return
	}
	if len(data) == 0 || len(data) > siteMediaMaxBytes {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file must be 1 byte..5 MB"})
		return
	}

	contentType := http.DetectContentType(data)
	if _, allowed := allowedSiteMediaTypes[contentType]; !allowed {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "only jpeg, png, webp or gif images are allowed"})
		return
	}

	scope := sanitizeSiteMediaScope(r.FormValue("scope"))
	filename := sanitizeSiteMediaFilename(header.Filename, contentType)
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	id, createdAt, err := h.upsertSiteMedia(r.Context(), scope, filename, contentType, sha, data, actor.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	url := fmt.Sprintf("/api/public/site-media/%d/%s", id, filename)
	h.writeAdminAudit(r.Context(), r, actor.ID, "admin.site_media.upload", "site_media", &id, map[string]any{"scope": scope, "filename": filename, "size": len(data)})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"id":          id,
		"url":         url,
		"filename":    filename,
		"contentType": contentType,
		"sizeBytes":   len(data),
		"sha256":      sha,
		"createdAt":   createdAt,
	})
}

func (h Handler) adminSiteMediaDelete(w http.ResponseWriter, r *http.Request, actorID int64) {
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "valid id is required"})
		return
	}
	var filename, scope string
	err = h.DB.QueryRow(r.Context(), `
		delete from site_media
		where id = $1
		returning filename, scope`, id).Scan(&filename, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "media not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	h.writeAdminAudit(r.Context(), r, actorID, "admin.site_media.delete", "site_media", &id, map[string]any{"scope": scope, "filename": filename})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "deleted": true})
}

func (h Handler) upsertSiteMedia(ctx context.Context, scope, filename, contentType, sha string, data []byte, actorID int64) (int64, string, error) {
	var id int64
	var createdAt string
	err := h.DB.QueryRow(ctx, `
		insert into site_media (scope, filename, content_type, size_bytes, sha256, data, created_by)
		values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (scope, sha256) do update set
		  filename = excluded.filename,
		  content_type = excluded.content_type,
		  created_by = excluded.created_by
		returning id, created_at::text`,
		scope, filename, contentType, len(data), sha, data, actorID).Scan(&id, &createdAt)
	return id, createdAt, err
}

// PublicSiteMedia: GET /api/public/site-media/{id}/{filename} serves CMS media publicly.
func (h Handler) PublicSiteMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if h.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "database is not configured"})
		return
	}
	id, err := siteMediaIDFromPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var filename, contentType string
	var data []byte
	err = h.DB.QueryRow(r.Context(), `select filename, content_type, data from site_media where id=$1`, id).Scan(&filename, &contentType, &data)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDispositionInline(filename))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(data)
}

func siteMediaIDFromPath(path string) (int64, error) {
	rest := strings.TrimPrefix(path, "/api/public/site-media/")
	if rest == path || rest == "" {
		return 0, errors.New("missing id")
	}
	part := strings.SplitN(rest, "/", 2)[0]
	return strconv.ParseInt(part, 10, 64)
}

func sanitizeSiteMediaScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return "blog"
	}
	var b strings.Builder
	for _, r := range scope {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || b.Len() > 40 {
		return "blog"
	}
	return b.String()
}

func sanitizeSiteMediaFilename(name, contentType string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "/" || base == "" {
		base = "image"
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" {
		if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
			ext = exts[0]
		}
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, r := range strings.ToLower(stem) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		b.WriteString("image")
	}
	out := b.String()
	if len(out) > 80 {
		out = out[:80]
	}
	return out + ext
}

func contentDispositionInline(filename string) string {
	return `inline; filename="` + strings.ReplaceAll(filename, `"`, "") + `"`
}
