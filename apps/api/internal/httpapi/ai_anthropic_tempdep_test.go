package httpapi

import "testing"

// Regression 2026-08-08: prod sent temperature to 5th-generation models and got
// 400 "temperature is deprecated for this model": every anthropic mode failed and
// the chat answered through the fallback provider in ~20 seconds instead of 3.
func TestAnthropicTemperatureDeprecated(t *testing.T) {
	deprecated := []string{"claude-sonnet-5", "claude-opus-5", "claude-fable-5", "Claude-Sonnet-5-20260101", "claude-haiku-5"}
	for _, m := range deprecated {
		if !anthropicTemperatureDeprecated(m) {
			t.Errorf("%s: ожидали deprecated=true (temperature слать нельзя)", m)
		}
	}
	allowed := []string{"claude-3-5-sonnet-20241022", "claude-haiku-4-5-20251001", "claude-3-opus-20240229", "gpt-4o", ""}
	for _, m := range allowed {
		if anthropicTemperatureDeprecated(m) {
			t.Errorf("%s: ожидали deprecated=false (temperature допустим)", m)
		}
	}
}
