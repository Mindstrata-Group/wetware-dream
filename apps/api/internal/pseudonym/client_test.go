package pseudonym

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPClientDetect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/detect" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Text  string  `json:"text"`
			Lang  string  `json:"lang"`
			Known []Known `json:"known"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Lang != "ru" || len(body.Known) != 1 {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(DetectResult{
			Spans: []Span{{Start: 5, End: 9, Kind: "PERSON", Canon: "Люда", Key: "люда"}}, Lang: "ru",
		})
	}))
	defer srv.Close()
	c := NewHTTPClient(srv.URL)
	res, err := c.Detect(context.Background(), "Мама Люда", "ru", []Known{{Kind: "PERSON", Value: "Люда"}})
	if err != nil || len(res.Spans) != 1 || res.Spans[0].Canon != "Люда" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHTTPClientRestoreSendsContext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["context"] != "Поговорите с " {
			http.Error(w, "no context", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(RestoreResult{Text: "Людой", Restored: 1})
	}))
	defer srv.Close()
	res, err := NewHTTPClient(srv.URL).Restore(context.Background(), "ЛИЦО_1", "Поговорите с ",
		[]Entry{{Kind: "PERSON", Number: 1, Canon: "Люда"}})
	if err != nil || res.Text != "Людой" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHTTPClientErrorsAreUnavailable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := NewHTTPClient(srv.URL).Detect(context.Background(), "секрет Шмыгло", "ru", nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "Шмыгло") {
		t.Fatal("errors must not carry the text")
	}
	_, err = NewHTTPClient("http://127.0.0.1:1").Detect(context.Background(), "x", "ru", nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed port: %v", err)
	}
}
