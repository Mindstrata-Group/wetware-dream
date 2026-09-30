package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithRequestIDUsesTrustedIncomingID(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got != "trace-123" {
			t.Fatalf("request id in context = %q, want trace-123", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestIDHeader, "trace-123")
	w := httptest.NewRecorder()

	withRequestID(next).ServeHTTP(w, req)

	if got := w.Header().Get(requestIDHeader); got != "trace-123" {
		t.Fatalf("response request id = %q, want trace-123", got)
	}
}

func TestWithRequestIDGeneratesIDWhenIncomingIsUnsafe(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got == "" || got == "bad id\nnext" {
			t.Fatalf("generated request id = %q", got)
		}
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestIDHeader, "bad id\nnext")
	w := httptest.NewRecorder()

	withRequestID(next).ServeHTTP(w, req)

	got := w.Header().Get(requestIDHeader)
	if got == "" || got == "bad id\nnext" {
		t.Fatalf("response request id = %q", got)
	}
}
