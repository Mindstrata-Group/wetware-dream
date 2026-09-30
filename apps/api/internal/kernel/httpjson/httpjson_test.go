package httpjson

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	Write(rec, http.StatusTeapot, map[string]any{"ok": true})
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
}

func TestDecodeStrict(t *testing.T) {
	t.Parallel()
	type in struct {
		A string `json:"a"`
	}
	cases := []struct {
		name, body string
		max        int64
		wantErr    bool
	}{
		{"ok", `{"a":"x"}`, 64, false},
		{"unknown field rejected", `{"a":"x","b":1}`, 64, true},
		{"too large rejected", `{"a":"` + strings.Repeat("x", 100) + `"}`, 16, true},
		{"not json", `nope`, 64, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var v in
			err := DecodeStrict(httptest.NewRecorder(), req, tc.max, &v)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
