package aireport

import (
	"strings"
	"testing"
	"time"
)

func TestFormatReportEmpty(t *testing.T) {
	got := formatReport("prod", time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), nil)
	if !strings.Contains(got, "нет данных") {
		t.Fatalf("expected empty-data message, got %q", got)
	}
}

func TestFormatReportCountsByProvider(t *testing.T) {
	counts := []providerCount{
		{Provider: "gemini", Outcome: "success", N: 120},
		{Provider: "gemini", Outcome: "attempt_failed", N: 3},
		{Provider: "vsegpt", Outcome: "success", N: 5},
		{Provider: "anthropic", Outcome: "exhausted", N: 2},
	}
	got := formatReport("prod", time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), counts)

	for _, want := range []string{
		"[prod]",
		"2026-07-13",
		"gemini=120 сообщ.(3 сбоев попыток)",
		"vsegpt=5 сообщ.",
		"ПОЛНЫЙ ОТКАЗ ЦЕПОЧКИ: 2 раз(а)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected report to contain %q, got %q", want, got)
		}
	}
}

func TestFormatReportNoFailuresOmitsParens(t *testing.T) {
	counts := []providerCount{{Provider: "gemini", Outcome: "success", N: 10}}
	got := formatReport("staging", time.Now(), counts)
	if strings.Contains(got, "сбоев попыток") {
		t.Fatalf("did not expect failure count when there were none: %q", got)
	}
	if strings.Contains(got, "ПОЛНЫЙ ОТКАЗ") {
		t.Fatalf("did not expect exhausted line when there were none: %q", got)
	}
}
