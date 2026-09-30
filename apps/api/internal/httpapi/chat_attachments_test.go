package httpapi

import (
	"archive/zip"
	"bytes"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestExtractChatAttachmentText_TextMarkdownDocxAndDoc(t *testing.T) {
	t.Parallel()

	txt, err := extractChatAttachmentText("txt", []byte("plain text"))
	if err != nil || txt != "plain text" {
		t.Fatalf("txt=%q err=%v", txt, err)
	}

	md, err := extractChatAttachmentText("md", []byte("# Title\n\n- item"))
	if err != nil || md != "# Title\n\n- item" {
		t.Fatalf("md=%q err=%v", md, err)
	}

	docxBytes := buildTestDocx(t, "Первый", "второй")
	docx, err := extractChatAttachmentText("docx", docxBytes)
	if err != nil {
		t.Fatalf("docx err=%v", err)
	}
	if docx != "Первый второй" {
		t.Fatalf("docx=%q", docx)
	}

	doc, err := extractChatAttachmentText("doc", []byte{0, 1, 'O', 'l', 'd', ' ', 'D', 'O', 'C', 0, 2})
	if err != nil {
		t.Fatalf("doc err=%v", err)
	}
	if doc != "Old DOC" {
		t.Fatalf("doc=%q", doc)
	}
}

func TestExtractChatAttachmentText_DecodesWindows1251Text(t *testing.T) {
	t.Parallel()

	source, err := charmap.Windows1251.NewEncoder().Bytes([]byte("Привет из Windows"))
	if err != nil {
		t.Fatalf("encode cp1251: %v", err)
	}
	txt, err := extractChatAttachmentText("txt", source)
	if err != nil {
		t.Fatalf("cp1251 txt err=%v", err)
	}
	if txt != "Привет из Windows" {
		t.Fatalf("txt=%q", txt)
	}
}

func TestExtractChatAttachmentText_DecodesUTF16LEWithBOM(t *testing.T) {
	t.Parallel()

	source := []byte{0xff, 0xfe, 'H', 0, 'i', 0, '\n', 0}
	txt, err := extractChatAttachmentText("md", source)
	if err != nil {
		t.Fatalf("utf16 txt err=%v", err)
	}
	if txt != "Hi\n" {
		t.Fatalf("txt=%q", txt)
	}
}

func TestExtractChatAttachmentText_RejectsBinaryText(t *testing.T) {
	t.Parallel()

	if _, err := extractChatAttachmentText("txt", []byte{0, 1, 2, 3, 4, 5, 6}); err == nil {
		t.Fatalf("binary txt accepted")
	}
}

func TestBuildAttachmentContextForAI_LimitsTextPerMessage(t *testing.T) {
	t.Parallel()

	context := buildAttachmentContextForAI([]ownedAttachmentContext{
		{ID: 1, FileName: "first.txt", Extension: "txt", SizeBytes: 100, PreparedText: "1234567890"},
		{ID: 2, FileName: "second.txt", Extension: "txt", SizeBytes: 100, PreparedText: "abcdef"},
	}, 12)

	if !bytes.Contains([]byte(context), []byte("1234567890")) {
		t.Fatalf("first file text missing: %q", context)
	}
	if !bytes.Contains([]byte(context), []byte("ab")) {
		t.Fatalf("second file prefix missing: %q", context)
	}
	if bytes.Contains([]byte(context), []byte("abcdef")) {
		t.Fatalf("second file was not trimmed: %q", context)
	}
	if !bytes.Contains([]byte(context), []byte("обрезано по лимиту")) {
		t.Fatalf("trim marker missing: %q", context)
	}
}

func buildTestDocx(t *testing.T, parts ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create document.xml: %v", err)
	}
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`))
	for _, part := range parts {
		_, _ = w.Write([]byte(`<w:p><w:r><w:t>` + part + `</w:t></w:r></w:p>`))
	}
	_, _ = w.Write([]byte(`</w:body></w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatalf("close docx: %v", err)
	}
	return buf.Bytes()
}
