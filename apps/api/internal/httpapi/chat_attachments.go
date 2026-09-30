package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/encoding/charmap"
)

const (
	defaultAttachmentDirectMaxBytes  = int64(24 << 10)
	defaultAttachmentMaxUploadBytes  = int64(5 << 20)
	defaultAttachmentContextMaxChars = 24000
	defaultAttachmentPrompt          = "Сожми вложенный файл для ответа в чате: сохрани факты, числа, термины, требования и явные вопросы. Не добавляй новых фактов."
)

type ChatAttachment struct {
	ID               int64  `json:"id"`
	FileName         string `json:"fileName"`
	Extension        string `json:"extension"`
	SizeBytes        int64  `json:"sizeBytes"`
	SHA256           string `json:"sha256"`
	AnnotationStatus string `json:"annotationStatus"`
	Duplicate        bool   `json:"duplicate,omitempty"`
}

type chatAttachmentSettings struct {
	DirectMaxBytes  int64
	MaxUploadBytes  int64
	ContextMaxChars int
	Model           string
	Prompt          string
}

type preparedChatFileBlob struct {
	sha256Hex        string
	sizeBytes        int64
	sourceBytes      []byte
	extractedText    string
	preparedText     string
	annotationStatus string
	annotationModel  string
	annotationPrompt string
}

type existingChatFileBlob struct {
	ID               int64
	SHA256           string
	SizeBytes        int64
	AnnotationStatus string
}

type ownedAttachmentContext struct {
	ID           int64
	PreparedText string
	FileName     string
	Extension    string
	SizeBytes    int64
}

func (h Handler) ChatAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	ctx := r.Context()
	userID, _, _, err := h.ensureGuestUser(ctx, w, r)
	if err != nil {
		writeGuestAuthError(w, r, err)
		return
	}
	if ok, err := h.userHasActiveMode(ctx, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	} else if !ok {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "access required", "code": "access_required"})
		return
	}

	settings := h.chatAttachmentSettings(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, settings.MaxUploadBytes+64<<10)
	if err := r.ParseMultipartForm(settings.MaxUploadBytes + 64<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid multipart payload"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file is required"})
		return
	}
	defer file.Close()

	attachment, err := h.storeChatAttachment(ctx, userID, file, header, settings)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errAttachmentAnnotationFailed) {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "attachment": attachment})
}

var errAttachmentAnnotationFailed = errors.New("attachment annotation failed")

func (h Handler) storeChatAttachment(ctx context.Context, userID int64, file multipart.File, header *multipart.FileHeader, settings chatAttachmentSettings) (ChatAttachment, error) {
	limited := io.LimitReader(file, settings.MaxUploadBytes+1)
	source, err := io.ReadAll(limited)
	if err != nil {
		return ChatAttachment{}, err
	}
	if int64(len(source)) > settings.MaxUploadBytes {
		return ChatAttachment{}, fmt.Errorf("file is too large")
	}
	if len(bytes.TrimSpace(source)) == 0 {
		return ChatAttachment{}, fmt.Errorf("file is empty")
	}

	filename := sanitizeAttachmentFilename(header.Filename)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if !allowedAttachmentExtension(ext) {
		return ChatAttachment{}, fmt.Errorf("unsupported file extension")
	}
	sum := sha256.Sum256(source)
	shaHex := hex.EncodeToString(sum[:])
	if existing, ok, err := h.findExistingChatAttachment(ctx, userID, shaHex, int64(len(source))); err != nil {
		return ChatAttachment{}, err
	} else if ok {
		existing.Duplicate = true
		return existing, nil
	}
	if blob, ok, err := h.findExistingChatFileBlob(ctx, shaHex, int64(len(source))); err != nil {
		return ChatAttachment{}, err
	} else if ok {
		created, err := h.insertChatFileForBlob(ctx, userID, blob, filename, strings.TrimSpace(header.Header.Get("Content-Type")), ext)
		if err != nil {
			return ChatAttachment{}, err
		}
		return created, nil
	}

	extracted, err := extractChatAttachmentText(ext, source)
	if err != nil {
		return ChatAttachment{}, err
	}
	extracted = strings.TrimSpace(sanitizeUserText(extracted))
	if extracted == "" {
		return ChatAttachment{}, fmt.Errorf("file has no readable text")
	}

	prepared := extracted
	status := "direct"
	model := ""
	prompt := ""
	if int64(len(source)) > settings.DirectMaxBytes {
		model = settings.Model
		prompt = settings.Prompt
		annotated, err := h.annotateChatAttachment(ctx, model, prompt, extracted)
		if err != nil {
			return ChatAttachment{}, fmt.Errorf("%w: %v", errAttachmentAnnotationFailed, err)
		}
		prepared = strings.TrimSpace(sanitizeUserText(annotated))
		if prepared == "" {
			return ChatAttachment{}, fmt.Errorf("%w: empty annotation", errAttachmentAnnotationFailed)
		}
		status = "annotated"
	}

	preparedFile := preparedChatFileBlob{
		sha256Hex:        shaHex,
		sizeBytes:        int64(len(source)),
		sourceBytes:      source,
		extractedText:    extracted,
		preparedText:     prepared,
		annotationStatus: status,
		annotationModel:  model,
		annotationPrompt: prompt,
	}
	created, err := h.insertChatAttachment(ctx, userID, filename, strings.TrimSpace(header.Header.Get("Content-Type")), ext, preparedFile)
	if err != nil {
		return ChatAttachment{}, err
	}
	return created, nil
}

func (h Handler) findExistingChatAttachment(ctx context.Context, userID int64, shaHex string, sizeBytes int64) (ChatAttachment, bool, error) {
	var out ChatAttachment
	err := h.DB.QueryRow(ctx, `
		select f.id, f.original_filename, f.extension, b.size_bytes, b.sha256_hex, b.annotation_status
		from chat_files f
		join chat_file_blobs b on b.id = f.blob_id
		where f.owner_user_id=$1 and b.sha256_hex=$2 and b.size_bytes=$3`,
		userID, shaHex, sizeBytes,
	).Scan(&out.ID, &out.FileName, &out.Extension, &out.SizeBytes, &out.SHA256, &out.AnnotationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatAttachment{}, false, nil
	}
	if err != nil {
		return ChatAttachment{}, false, err
	}
	return out, true, nil
}

func (h Handler) insertChatAttachment(ctx context.Context, userID int64, filename, mimeType, extension string, file preparedChatFileBlob) (ChatAttachment, error) {
	var annotatedAt any
	if file.annotationStatus == "annotated" {
		annotatedAt = "now"
	}
	var blobID int64
	err := h.DB.QueryRow(ctx, `
		insert into chat_file_blobs
			(sha256_hex, size_bytes, source_bytes, extracted_text, prepared_text,
			 annotation_status, annotation_model, annotation_prompt, annotated_at, created_at, updated_at)
		values
			($1,$2,$3,$4,$5,$6,$7,$8,case when $9::text = 'now' then now() else null end, now(), now())
		on conflict (sha256_hex, size_bytes) do update set sha256_hex = excluded.sha256_hex
		returning id`,
		file.sha256Hex, file.sizeBytes, file.sourceBytes, file.extractedText, file.preparedText,
		file.annotationStatus, file.annotationModel, file.annotationPrompt, annotatedAt,
	).Scan(&blobID)
	if err != nil {
		return ChatAttachment{}, err
	}
	return h.insertChatFileForBlob(ctx, userID, existingChatFileBlob{
		ID:               blobID,
		SHA256:           file.sha256Hex,
		SizeBytes:        file.sizeBytes,
		AnnotationStatus: file.annotationStatus,
	}, filename, mimeType, extension)
}

func (h Handler) insertChatFileForBlob(ctx context.Context, userID int64, blob existingChatFileBlob, filename, mimeType, extension string) (ChatAttachment, error) {
	var out ChatAttachment
	err := h.DB.QueryRow(ctx, `
		insert into chat_files
			(owner_user_id, blob_id, original_filename, mime_type, extension, created_at, updated_at)
		values ($1,$2,$3,$4,$5,now(),now())
		on conflict (owner_user_id, blob_id) do nothing
		returning id, original_filename, extension`,
		userID, blob.ID, filename, mimeType, extension,
	).Scan(&out.ID, &out.FileName, &out.Extension)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, ok, lookupErr := h.findExistingChatAttachment(ctx, userID, blob.SHA256, blob.SizeBytes)
		if lookupErr != nil {
			return ChatAttachment{}, lookupErr
		}
		if ok {
			existing.Duplicate = true
			return existing, nil
		}
	}
	if err != nil {
		return ChatAttachment{}, err
	}
	out.SizeBytes = blob.SizeBytes
	out.SHA256 = blob.SHA256
	out.AnnotationStatus = blob.AnnotationStatus
	return out, nil
}

func (h Handler) findExistingChatFileBlob(ctx context.Context, shaHex string, sizeBytes int64) (existingChatFileBlob, bool, error) {
	var out existingChatFileBlob
	err := h.DB.QueryRow(ctx, `
		select id, sha256_hex, size_bytes, annotation_status
		from chat_file_blobs
		where sha256_hex=$1 and size_bytes=$2`,
		shaHex, sizeBytes,
	).Scan(&out.ID, &out.SHA256, &out.SizeBytes, &out.AnnotationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return existingChatFileBlob{}, false, nil
	}
	if err != nil {
		return existingChatFileBlob{}, false, err
	}
	return out, true, nil
}

func (h Handler) annotateChatAttachment(ctx context.Context, model, prompt, text string) (string, error) {
	if strings.TrimSpace(model) == "" {
		return "", errors.New("attachment annotation model is empty")
	}
	messages := []map[string]string{
		{"role": "system", "content": prompt},
		{"role": "user", "content": text},
	}
	content, _, _, _, err := h.doMechanicAIChat(ctx, "chat_attachment_annotation_provider", model, 0.2, messages, "CHAT_ATTACHMENT_ANNOTATION", openAIChatOptions{})
	return content, err
}

func (h Handler) chatAttachmentSettings(ctx context.Context) chatAttachmentSettings {
	directMax := h.systemSettingInt64(ctx, "chat_attachment_direct_max_bytes", defaultAttachmentDirectMaxBytes, 0, 10<<20)
	maxUpload := h.systemSettingInt64(ctx, "chat_attachment_max_upload_bytes", defaultAttachmentMaxUploadBytes, 1, 25<<20)
	if maxUpload < directMax {
		maxUpload = directMax
	}
	model, _ := h.systemSetting(ctx, "chat_attachment_annotation_model")
	if model == "" {
		model, _ = h.systemSetting(ctx, "ai_summary_model")
	}
	if model == "" {
		model = defaultAIFallbackModel
	}
	prompt, _ := h.systemSetting(ctx, "chat_attachment_annotation_prompt")
	if prompt == "" {
		prompt = defaultAttachmentPrompt
	}
	contextMax := int(h.systemSettingInt64(ctx, "chat_attachment_context_max_chars", defaultAttachmentContextMaxChars, 1000, 200000))
	return chatAttachmentSettings{DirectMaxBytes: directMax, MaxUploadBytes: maxUpload, ContextMaxChars: contextMax, Model: model, Prompt: prompt}
}

func (h Handler) systemSettingInt64(ctx context.Context, key string, fallback, minValue, maxValue int64) int64 {
	value, err := h.systemSetting(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return fallback
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func sanitizeAttachmentFilename(v string) string {
	v = strings.TrimSpace(filepath.Base(v))
	v = strings.Map(func(r rune) rune {
		if r == 0 || unicode.IsControl(r) {
			return -1
		}
		return r
	}, v)
	if v == "" || v == "." || v == "/" {
		return "attachment.txt"
	}
	if len(v) > 180 {
		ext := filepath.Ext(v)
		base := strings.TrimSuffix(v, ext)
		if len(base) > 160 {
			base = base[:160]
		}
		v = base + ext
	}
	return v
}

func allowedAttachmentExtension(ext string) bool {
	switch ext {
	case "txt", "md", "doc", "docx":
		return true
	default:
		return false
	}
}

func extractChatAttachmentText(ext string, source []byte) (string, error) {
	switch ext {
	case "txt", "md":
		return decodeTextAttachment(source)
	case "docx":
		return extractDocxText(source)
	case "doc":
		return extractBinaryDocText(source), nil
	default:
		return "", fmt.Errorf("unsupported file extension")
	}
}

func decodeTextAttachment(source []byte) (string, error) {
	if len(source) == 0 {
		return "", nil
	}
	if utf8.Valid(source) {
		decoded := string(dropUTF8BOM(source))
		if !looksLikePlainTextString(decoded) {
			return "", fmt.Errorf("file is not a supported text encoding")
		}
		return decoded, nil
	}
	if hasPrefixBytes(source, 0xff, 0xfe) {
		return decodeUTF16Text(source[2:], true)
	}
	if hasPrefixBytes(source, 0xfe, 0xff) {
		return decodeUTF16Text(source[2:], false)
	}
	if !looksLikePlainTextBytes(source) {
		return "", fmt.Errorf("file is not a supported text encoding")
	}
	decoded, err := charmap.Windows1251.NewDecoder().String(string(source))
	if err != nil {
		return "", fmt.Errorf("decode windows-1251 text: %w", err)
	}
	if !looksLikePlainTextString(decoded) {
		return "", fmt.Errorf("file is not a supported text encoding")
	}
	return decoded, nil
}

func decodeUTF16Text(source []byte, littleEndian bool) (string, error) {
	if len(source) == 0 || len(source)%2 != 0 {
		return "", fmt.Errorf("invalid utf-16 text")
	}
	values := make([]uint16, 0, len(source)/2)
	for i := 0; i < len(source); i += 2 {
		if littleEndian {
			values = append(values, uint16(source[i])|uint16(source[i+1])<<8)
		} else {
			values = append(values, uint16(source[i])<<8|uint16(source[i+1]))
		}
	}
	decoded := string(utf16.Decode(values))
	if !looksLikePlainTextString(decoded) {
		return "", fmt.Errorf("file is not a supported text encoding")
	}
	return decoded, nil
}

func dropUTF8BOM(source []byte) []byte {
	if hasPrefixBytes(source, 0xef, 0xbb, 0xbf) {
		return source[3:]
	}
	return source
}

func hasPrefixBytes(source []byte, prefix ...byte) bool {
	if len(source) < len(prefix) {
		return false
	}
	for i, b := range prefix {
		if source[i] != b {
			return false
		}
	}
	return true
}

func looksLikePlainTextBytes(source []byte) bool {
	if len(source) == 0 {
		return true
	}
	control := 0
	for _, b := range source {
		if b == 0 || (b < 0x20 && b != '\n' && b != '\r' && b != '\t') {
			control++
		}
	}
	return control == 0 || control*100/len(source) <= 2
}

func looksLikePlainTextString(text string) bool {
	if text == "" {
		return true
	}
	control := 0
	total := 0
	for _, r := range text {
		total++
		if r == utf8.RuneError || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			control++
		}
	}
	return total > 0 && control*100/total <= 2
}

func extractDocxText(source []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		return "", fmt.Errorf("invalid docx archive")
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		decoder := xml.NewDecoder(io.LimitReader(rc, 8<<20))
		var parts []string
		for {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", err
			}
			switch typed := token.(type) {
			case xml.StartElement:
				if typed.Name.Local == "t" {
					var text string
					if err := decoder.DecodeElement(&text, &typed); err != nil {
						return "", err
					}
					if strings.TrimSpace(text) != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, " "), nil
	}
	return "", fmt.Errorf("docx document.xml not found")
}

func extractBinaryDocText(source []byte) string {
	var parts []string
	var current []rune
	flush := func() {
		if len(current) >= 4 {
			parts = append(parts, string(current))
		}
		current = current[:0]
	}
	for len(source) > 0 {
		r, size := utf8.DecodeRune(source)
		if r == utf8.RuneError && size == 1 {
			flush()
			source = source[1:]
			continue
		}
		if r == '\n' || r == '\r' || r == '\t' || (unicode.IsPrint(r) && !unicode.IsControl(r)) {
			current = append(current, r)
		} else {
			flush()
		}
		source = source[size:]
	}
	flush()
	return strings.Join(parts, "\n")
}

func (h Handler) loadOwnedAttachmentContexts(ctx context.Context, userID int64, ids []int64) ([]ownedAttachmentContext, error) {
	ids = uniquePositiveInt64s(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := h.DB.Query(ctx, `
		select f.id, b.prepared_text, f.original_filename, f.extension, b.size_bytes
		from chat_files f
		join chat_file_blobs b on b.id = f.blob_id
		where f.owner_user_id=$1 and f.id=any($2)`,
		userID, ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[int64]ownedAttachmentContext, len(ids))
	for rows.Next() {
		var item ownedAttachmentContext
		if err := rows.Scan(&item.ID, &item.PreparedText, &item.FileName, &item.Extension, &item.SizeBytes); err != nil {
			return nil, err
		}
		byID[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(byID) != len(ids) {
		return nil, fmt.Errorf("attachment not found")
	}
	out := make([]ownedAttachmentContext, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}

func linkMessageAttachments(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, messageID int64, attachmentIDs []int64) error {
	attachmentIDs = uniquePositiveInt64s(attachmentIDs)
	for i, id := range attachmentIDs {
		if _, err := q.Exec(ctx, `
			insert into chat_message_attachments (message_id, file_id, position)
			values ($1,$2,$3)
			on conflict (message_id, file_id) do nothing`,
			messageID, id, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func buildAttachmentContextForAI(items []ownedAttachmentContext, maxChars int) string {
	if len(items) == 0 {
		return ""
	}
	if maxChars <= 0 {
		maxChars = defaultAttachmentContextMaxChars
	}
	var b strings.Builder
	b.WriteString("\n\nКонтекст из файловых вложений к текущему сообщению. Используй только если это помогает ответить:\n")
	remaining := maxChars
	for i, item := range items {
		text := strings.TrimSpace(item.PreparedText)
		if text == "" {
			continue
		}
		if remaining <= 0 {
			_, _ = fmt.Fprintf(&b, "\n[Файл %d: %s, .%s, %d bytes]\n[текст не добавлен: общий лимит вложений исчерпан]\n", i+1, item.FileName, item.Extension, item.SizeBytes)
			continue
		}
		part := trimRunes(text, remaining)
		remaining -= utf8.RuneCountInString(part)
		_, _ = fmt.Fprintf(&b, "\n[Файл %d: %s, .%s, %d bytes]\n%s", i+1, item.FileName, item.Extension, item.SizeBytes, part)
		if part != text {
			b.WriteString("\n[обрезано по лимиту текста вложений]")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func trimRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	var b strings.Builder
	b.Grow(max)
	count := 0
	for _, r := range text {
		if count >= max {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

func uniquePositiveInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	set := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return indexOfInt64(values, out[i]) < indexOfInt64(values, out[j])
	})
	return out
}

func indexOfInt64(values []int64, target int64) int {
	for i, v := range values {
		if v == target {
			return i
		}
	}
	return len(values)
}
