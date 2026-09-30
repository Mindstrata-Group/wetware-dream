package httpapi

import "testing"

// parseOrchestratorModeID is a pure parser; easy to unit test.

func TestParseOrchestratorModeID_JSONResponse(t *testing.T) {
	t.Parallel()

	got := parseOrchestratorModeID(`{"modeId":42,"reason":"best fit"}`, []int64{1, 42, 99})
	if got != 42 {
		t.Errorf("got %d want 42", got)
	}
}

func TestParseOrchestratorModeID_JSONWithText(t *testing.T) {
	t.Parallel()

	// AI often wraps JSON in prose: "Sure! {...}"
	got := parseOrchestratorModeID("Sure! Here is my answer: {\"modeId\":99,\"reason\":\"x\"}\nThanks.", []int64{1, 42, 99})
	if got != 99 {
		t.Errorf("got %d want 99", got)
	}
}

func TestParseOrchestratorModeID_NotInAllowed(t *testing.T) {
	t.Parallel()

	got := parseOrchestratorModeID(`{"modeId":777}`, []int64{1, 2, 3})
	if got != 0 {
		t.Errorf("modeId not in allowed list must return 0, got %d", got)
	}
}

func TestParseOrchestratorModeID_PlainNumberFallback(t *testing.T) {
	t.Parallel()

	// AI says "use mode 42", without JSON
	got := parseOrchestratorModeID("I think mode 42 is best", []int64{1, 42, 99})
	if got != 42 {
		t.Errorf("plain-number fallback: got %d want 42", got)
	}
}

func TestParseOrchestratorModeID_EmptyAnswer(t *testing.T) {
	t.Parallel()

	got := parseOrchestratorModeID("", []int64{1, 2})
	if got != 0 {
		t.Errorf("empty answer: got %d want 0", got)
	}
}

func TestParseOrchestratorModeID_MalformedJSON(t *testing.T) {
	t.Parallel()

	got := parseOrchestratorModeID(`{not json`, []int64{1, 2})
	if got != 0 {
		t.Errorf("malformed JSON without number: got %d want 0", got)
	}
}

func TestIdInList(t *testing.T) {
	t.Parallel()

	if !idInList(5, []int64{1, 5, 10}) {
		t.Errorf("5 should be in [1,5,10]")
	}
	if idInList(7, []int64{1, 5, 10}) {
		t.Errorf("7 should NOT be in [1,5,10]")
	}
	if idInList(0, []int64{}) {
		t.Errorf("anything in empty list should be false")
	}
}
