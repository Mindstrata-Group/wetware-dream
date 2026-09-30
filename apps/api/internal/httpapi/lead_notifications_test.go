package httpapi

import (
	"strings"
	"testing"
)

func TestParseLeadChatIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []int64
	}{
		{"", nil},
		{"111222333", []int64{111222333}},
		{"111222333, 123456789", []int64{111222333, 123456789}},
		{"111222333;42\n7", []int64{111222333, 42, 7}},
		{"abc, -5, 0, 99", []int64{99}},
		{" , ,, ", nil},
	}
	for _, tc := range cases {
		got := parseLeadChatIDs(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("parseLeadChatIDs(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseLeadChatIDs(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestParseLeadTelegramIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"123456789", []string{"123456789"}},
		{"123456789, @channel", []string{"123456789", "@channel"}},
		{"123;@a\n@b", []string{"123", "@a", "@b"}},
		{" , ,, ", nil},
	}
	for _, tc := range cases {
		got := parseLeadTelegramIDs(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("parseLeadTelegramIDs(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseLeadTelegramIDs(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestBuildLeadTranscript_SkipsServiceRolesAndTruncatesFromStart(t *testing.T) {
	t.Parallel()
	messages := []ChatMessage{
		{Role: "system", Content: "служебное"},
		{Role: "user", Content: "старое сообщение"},
		{Role: "summary", Content: "саммари"},
		{Role: "assistant", Content: "ответ"},
		{Role: "user", Content: "свежее сообщение"},
	}
	full := buildLeadTranscript(messages, 100000)
	if strings.Contains(full, "служебное") || strings.Contains(full, "саммари") {
		t.Fatalf("транскрипт содержит служебные роли: %q", full)
	}
	if !strings.Contains(full, "Пользователь: старое сообщение") || !strings.Contains(full, "Ассистент: ответ") {
		t.Fatalf("транскрипт потерял реплики: %q", full)
	}
	// Truncation keeps the fresh part, not the beginning.
	short := buildLeadTranscript(messages, 60)
	if strings.Contains(short, "старое сообщение") {
		t.Fatalf("обрезка должна убирать начало, got %q", short)
	}
	if !strings.Contains(short, "свежее сообщение") {
		t.Fatalf("обрезка потеряла свежую часть: %q", short)
	}
}

func TestLastUserReplies_ChronologicalTail(t *testing.T) {
	t.Parallel()
	messages := []ChatMessage{
		{Role: "user", Content: "один"},
		{Role: "assistant", Content: "ответ"},
		{Role: "user", Content: "два"},
		{Role: "user", Content: "три"},
	}
	got := lastUserReplies(messages, 2)
	want := "два\n---\nтри"
	if got != want {
		t.Fatalf("lastUserReplies = %q, want %q", got, want)
	}
}
