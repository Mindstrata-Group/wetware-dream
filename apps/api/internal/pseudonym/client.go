package pseudonym

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Known is one value of the user's dictionary sent to the detector.
type Known struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Canon string `json:"canon,omitempty"`
	Key   string `json:"key,omitempty"`
}

// Span is a detected fragment. Start/End are Unicode code point offsets
// (Python string indices), not bytes.
type Span struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
	Canon string `json:"canon"`
	Key   string `json:"key"`
}

type DetectResult struct {
	Spans      []Span `json:"spans"`
	Residual   int    `json:"residual"`
	Lang       string `json:"lang"`
	GuardAdded int    `json:"guard_added"`
}

type restoreEntry struct {
	Kind   string `json:"kind"`
	Number int    `json:"number"`
	Canon  string `json:"canon"`
}

type RestoreResult struct {
	Text      string   `json:"text"`
	Restored  int      `json:"restored"`
	Inflected int      `json:"inflected"`
	Unknown   []string `json:"unknown"`
}

// Detector is the Python service seen from Go; an interface so tests can
// fake it without a network.
type Detector interface {
	Detect(ctx context.Context, text, lang string, known []Known) (DetectResult, error)
	Restore(ctx context.Context, text, context string, entries []Entry) (RestoreResult, error)
}

// ErrUnavailable wraps every failure to reach or understand the service.
var ErrUnavailable = errors.New("pseudonym: anonymizer service unavailable")

// HTTPClient talks to apps/anonymizer over the internal network.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (c *HTTPClient) post(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// The error of net/http may quote the URL; it carries no text.
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	return nil
}

func (c *HTTPClient) Detect(ctx context.Context, text, lang string, known []Known) (DetectResult, error) {
	var out DetectResult
	err := c.post(ctx, "/v1/detect", map[string]any{"text": text, "lang": lang, "known": known}, &out)
	return out, err
}

func (c *HTTPClient) Restore(ctx context.Context, text, contextText string, entries []Entry) (RestoreResult, error) {
	list := make([]restoreEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, restoreEntry{Kind: e.Kind, Number: e.Number, Canon: e.Canon})
	}
	var out RestoreResult
	err := c.post(ctx, "/v1/restore", map[string]any{"text": text, "context": contextText, "entries": list}, &out)
	return out, err
}
