package httpapi

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Vertex AI is the second way to reach Gemini, introduced on 2026-08-28.
//
// WHY. The regular generativelanguage.googleapis.com decides whether it may
// answer you by the CALLER's IP. Prod reaches it through a relay in a datacenter
// abroad, and Google classifies that address as a datacenter one:
// every request gets 400 FAILED_PRECONDITION "User location is not
// supported for the API use". Measured on 2026-08-28 over two different routes,
// through the relay and through a separate tunnel: the answer
// was the same. So ROUTING does not fix it: swapping one datacenter
// address for another datacenter address changes nothing.
//
// Vertex AI is addressed differently: the region is baked into the URL ITSELF
// ({LOCATION}-aiplatform.googleapis.com/.../projects/{PROJECT}/locations/{LOCATION}),
// not derived from the client IP. So the geo check goes away together with the
// reason for the refusal, rather than being bypassed.
//
// THE COST. Vertex is paid and has no free tier (unlike
// AI Studio with its 500 requests per day). For our profile (2-12 calls per
// day on flash-lite) this is cents, but it is NOT zero, and it must not be
// turned on blindly, hence the explicit toggle below.
//
// THE REQUEST FORMAT DOES NOT CHANGE: Vertex has an OpenAI-compatible endpoint, the
// same /chat/completions with the same body. Exactly two things change: the base URL and
// the authorisation method (an hourly service account OAuth token instead of a
// static key). So doGeminiChat is reused in full.

const (
	// The scope sufficient to call Vertex models.
	vertexScope = "https://www.googleapis.com/auth/cloud-platform"
	// Margin before the token expires: fetch a new one early so a long
	// request is not cut off midway with a 401.
	vertexTokenEarlyRenew = 5 * time.Minute
	vertexTokenLifetime   = time.Hour
)

// serviceAccountKey holds the fields of the service account JSON key that we need.
type serviceAccountKey struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// vertexConfigured reports whether Vertex mode is on. There is one explicit toggle: until
// a service account key is set, everything works the old way through AI Studio.
func (h Handler) vertexConfigured() bool {
	return strings.TrimSpace(h.GeminiVertexSAJSON) != ""
}

// vertexServiceAccount parses the key. It accepts both raw JSON and base64:
// multi-line JSON with newlines inside private_key does not survive well in .env,
// so base64 here is not a whim but the main way to pass it.
func (h Handler) vertexServiceAccount() (*serviceAccountKey, error) {
	raw := strings.TrimSpace(h.GeminiVertexSAJSON)
	if raw == "" {
		return nil, errors.New("vertex: ключ сервис-аккаунта не задан")
	}
	if !strings.HasPrefix(raw, "{") {
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("vertex: ключ не JSON и не base64: %w", err)
		}
		raw = string(decoded)
	}
	var sa serviceAccountKey
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		return nil, fmt.Errorf("vertex: не разобрать ключ сервис-аккаунта: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("vertex: в ключе нет client_email или private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &sa, nil
}

// vertexProject: the project comes from the explicit setting, otherwise from the key itself.
func (h Handler) vertexProject() string {
	if p := strings.TrimSpace(h.GeminiVertexProject); p != "" {
		return p
	}
	if sa, err := h.vertexServiceAccount(); err == nil {
		return sa.ProjectID
	}
	return ""
}

func (h Handler) vertexLocation() string {
	if l := strings.TrimSpace(h.GeminiVertexLocation); l != "" {
		return l
	}
	// us-central1 is the region where Gemini models appear first and live
	// the longest; for our load the latency does not matter.
	return "us-central1"
}

// vertexBaseURL builds the base of the Vertex OpenAI-compatible endpoint.
// The region appears both in the host and in the path, as the API requires.
//
// GeminiVertexBaseURL overrides it entirely and is needed not for tests but for
// prod: the server is in Russia, where *-aiplatform.googleapis.com is unreachable
// just like the rest of Google (measured 2026-08-28: a direct request returns 000).
// So we have to go through the relay, which has its own address and secret path.
func (h Handler) vertexBaseURL() string {
	if override := strings.TrimSpace(h.GeminiVertexBaseURL); override != "" {
		return strings.TrimRight(override, "/")
	}
	loc := h.vertexLocation()
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/endpoints/openapi",
		loc, h.vertexProject(), loc)
}

// vertexModelName converts the model name to the form Vertex understands:
// there models are addressed with the publisher, google/gemini-3.1-flash-lite.
// Already prefixed names and full paths (publishers/...) are left alone.
func vertexModelName(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return m
	}
	if strings.Contains(m, "/") {
		return m
	}
	return "google/" + m
}

// vertexAccessToken returns a live OAuth token, refreshing it no more often than needed.
// The cache is shared per Handler: the token lasts an hour and we make a handful of
// calls per day, so requesting one per chat would triple the calls to Google
// for nothing (the same mistake that made the watchdog eat the daily quota).
func (h Handler) vertexAccessToken(ctx context.Context) (string, error) {
	c := &h.c.vertexToken
	c.RLock()
	tok, exp := c.token, c.expiresAt
	c.RUnlock()
	if tok != "" && time.Now().Before(exp) {
		return tok, nil
	}

	sa, err := h.vertexServiceAccount()
	if err != nil {
		return "", err
	}
	assertion, err := signServiceAccountJWT(sa, time.Now())
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vertex: не получить токен: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("vertex: обмен JWT на токен вернул %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("vertex: не разобрать ответ токена: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("vertex: в ответе нет access_token")
	}
	lifetime := time.Duration(out.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = vertexTokenLifetime
	}
	if lifetime > vertexTokenEarlyRenew {
		lifetime -= vertexTokenEarlyRenew
	}

	c.Lock()
	c.token, c.expiresAt = out.AccessToken, time.Now().Add(lifetime)
	c.Unlock()
	return out.AccessToken, nil
}

// signServiceAccountJWT builds and signs an RS256 assertion to exchange for an
// access_token. Our own implementation instead of golang.org/x/oauth2 avoids pulling
// in a new dependency (and its transitive ones) for a hundred lines: everything
// needed is in the standard library.
func signServiceAccountJWT(sa *serviceAccountKey, now time.Time) (string, error) {
	key, err := parsePrivateKey(sa.PrivateKey)
	if err != nil {
		return "", err
	}
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": vertexScope,
		"aud":   sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(vertexTokenLifetime).Unix(),
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("vertex: не подписать JWT: %w", err)
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

// parsePrivateKey reads the PEM from the private_key field. In .env this key often
// ends up with escaped \n; unescape it, otherwise the PEM is not recognised
// and the error looks like "invalid key" although the key is valid.
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	s := strings.ReplaceAll(strings.TrimSpace(pemStr), "\\n", "\n")
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("vertex: private_key не является PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("vertex: ключ сервис-аккаунта не RSA")
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("vertex: не разобрать private_key: %w", err)
	}
	return key, nil
}
