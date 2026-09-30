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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test service account: its own RSA key, so the signature can be VERIFIED,
// not just checked for being non-empty.
func testServiceAccount(t *testing.T, tokenURI string) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("не сгенерировать ключ: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("не сериализовать ключ: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sa := map[string]string{
		"type":         "service_account",
		"project_id":   "proj-from-key",
		"client_email": "bot@proj.iam.gserviceaccount.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenURI,
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("не собрать JSON: %v", err)
	}
	return string(raw), &key.PublicKey
}

// One explicit switch: without a service account key Vertex mode must stay
// silent; otherwise a Vertex enabled by oversight would start spending money
// (it has no free tier).
func TestVertexConfigured_ТолькоПоКлючу(t *testing.T) {
	if (Handler{}).vertexConfigured() {
		t.Fatal("без ключа Vertex не должен считаться включённым")
	}
	if (Handler{GeminiVertexSAJSON: "   "}).vertexConfigured() {
		t.Fatal("пробелы — это не ключ")
	}
	if !(Handler{GeminiVertexSAJSON: `{"a":1}`}).vertexConfigured() {
		t.Fatal("с ключом Vertex должен включаться")
	}
}

// The region must be in BOTH the host and the path: the API itself requires it.
// A mistake here gives a 404 out of nowhere.
func TestVertexBaseURL_РегионВХостеИВПути(t *testing.T) {
	h := Handler{GeminiVertexSAJSON: `{"project_id":"p1"}`, GeminiVertexProject: "my-proj", GeminiVertexLocation: "europe-west4"}
	got := h.vertexBaseURL()
	want := "https://europe-west4-aiplatform.googleapis.com/v1beta1/projects/my-proj/locations/europe-west4/endpoints/openapi"
	if got != want {
		t.Fatalf("URL собран неверно:\n получили %s\n ждали   %s", got, want)
	}
}

func TestVertexProject_БерётсяИзКлючаЕслиНеЗадан(t *testing.T) {
	raw, _ := testServiceAccount(t, "https://oauth2.googleapis.com/token")
	h := Handler{GeminiVertexSAJSON: raw}
	if got := h.vertexProject(); got != "proj-from-key" {
		t.Fatalf("проект должен браться из ключа, получили %q", got)
	}
	h.GeminiVertexProject = "явный"
	if got := h.vertexProject(); got != "явный" {
		t.Fatalf("явная настройка должна побеждать, получили %q", got)
	}
}

// Vertex addresses models with the publisher. Without the prefix you get 404,
// and it looks like "the model was removed" when it is really the name.
func TestVertexModelName_ПрефиксИздателя(t *testing.T) {
	cases := map[string]string{
		"gemini-3.1-flash-lite":        "google/gemini-3.1-flash-lite",
		"google/gemini-3.1-flash-lite": "google/gemini-3.1-flash-lite",
		"publishers/google/models/x":   "publishers/google/models/x",
		"":                             "",
	}
	for in, want := range cases {
		if got := vertexModelName(in); got != want {
			t.Errorf("vertexModelName(%q) = %q, ждали %q", in, got, want)
		}
	}
}

// A key from .env often arrives with escaped \n. Unless they are expanded, the
// PEM is not recognised and the error reads as "wrong key" although the key is
// right.
func TestParsePrivateKey_ЭкранированныеПереводыСтрок(t *testing.T) {
	raw, _ := testServiceAccount(t, "https://x/token")
	var sa serviceAccountKey
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		t.Fatalf("не разобрать: %v", err)
	}
	escaped := strings.ReplaceAll(sa.PrivateKey, "\n", "\\n")
	if _, err := parsePrivateKey(escaped); err != nil {
		t.Fatalf("ключ с экранированными \\n обязан читаться: %v", err)
	}
}

// The key is also accepted as base64: multi-line JSON does not survive in .env.
func TestVertexServiceAccount_ПринимаетBase64(t *testing.T) {
	raw, _ := testServiceAccount(t, "https://x/token")
	h := Handler{GeminiVertexSAJSON: base64.StdEncoding.EncodeToString([]byte(raw))}
	sa, err := h.vertexServiceAccount()
	if err != nil {
		t.Fatalf("base64-ключ должен приниматься: %v", err)
	}
	if sa.ClientEmail != "bot@proj.iam.gserviceaccount.com" {
		t.Fatalf("разобран не тот ключ: %q", sa.ClientEmail)
	}
}

// The signature is checked with real cryptography: the assembled JWT must
// verify with the public key, and the claims must contain what Google
// requires. A "non-empty string" check would let a broken signature through.
func TestSignServiceAccountJWT_ПодписьПроверяется(t *testing.T) {
	raw, pub := testServiceAccount(t, "https://oauth2.googleapis.com/token")
	var sa serviceAccountKey
	if err := json.Unmarshal([]byte(raw), &sa); err != nil {
		t.Fatalf("не разобрать: %v", err)
	}
	now := time.Unix(1787900000, 0)
	tok, err := signServiceAccountJWT(&sa, now)
	if err != nil {
		t.Fatalf("не подписать: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT должен состоять из трёх частей, получили %d", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("подпись не base64url: %v", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("подпись не проходит проверку: %v", err)
	}
	var claims map[string]any
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatalf("claims не разобрать: %v", err)
	}
	if claims["iss"] != sa.ClientEmail {
		t.Errorf("iss = %v, ждали %v", claims["iss"], sa.ClientEmail)
	}
	if claims["aud"] != sa.TokenURI {
		t.Errorf("aud = %v, ждали %v", claims["aud"], sa.TokenURI)
	}
	if claims["scope"] != vertexScope {
		t.Errorf("scope = %v, ждали %v", claims["scope"], vertexScope)
	}
	if exp, ok := claims["exp"].(float64); !ok || int64(exp) <= now.Unix() {
		t.Errorf("exp должен быть в будущем, получили %v", claims["exp"])
	}
}

// The token lasts an hour and we have a handful of chats per day. Fetching a new
// one on every call would triple the calls to Google for nothing: exactly the
// mistake that made the watchdog eat the daily quota. We count CALLS, not text.
func TestVertexAccessToken_КэшируетсяИНеДёргаетGoogleКаждыйРаз(t *testing.T) {
	var calls int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"ya29.test","expires_in":3600}`)
	}))
	defer ts.Close()

	raw, _ := testServiceAccount(t, ts.URL)
	h := Handler{GeminiVertexSAJSON: raw, c: newHandlerCaches(), HTTPClient: ts.Client()}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tok, err := h.vertexAccessToken(ctx)
		if err != nil {
			t.Fatalf("вызов %d: %v", i, err)
		}
		if tok != "ya29.test" {
			t.Fatalf("вызов %d: получили токен %q", i, tok)
		}
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("за 5 вызовов к Google должно уйти РОВНО 1 обращение, ушло %d", got)
	}
}

// An expired token must be refreshed; otherwise in an hour everything stops with 401.
func TestVertexAccessToken_ОбновляетсяКогдаИстёк(t *testing.T) {
	var calls int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, n)
	}))
	defer ts.Close()

	raw, _ := testServiceAccount(t, ts.URL)
	h := Handler{GeminiVertexSAJSON: raw, c: newHandlerCaches(), HTTPClient: ts.Client()}
	if _, err := h.vertexAccessToken(context.Background()); err != nil {
		t.Fatalf("первый вызов: %v", err)
	}
	// Expiry instead of waiting an hour.
	h.c.vertexToken.Lock()
	h.c.vertexToken.expiresAt = time.Now().Add(-time.Second)
	h.c.vertexToken.Unlock()

	tok, err := h.vertexAccessToken(context.Background())
	if err != nil {
		t.Fatalf("второй вызов: %v", err)
	}
	if tok != "tok-2" {
		t.Fatalf("после истечения должен прийти НОВЫЙ токен, получили %q", tok)
	}
}

// An exchange error must not be swallowed: a silent empty token turns into a
// 401 from Vertex, and people will debug the wrong thing.
func TestVertexAccessToken_ОшибкаОбменаВидна(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer ts.Close()
	raw, _ := testServiceAccount(t, ts.URL)
	h := Handler{GeminiVertexSAJSON: raw, c: newHandlerCaches(), HTTPClient: ts.Client()}
	_, err := h.vertexAccessToken(context.Background())
	if err == nil {
		t.Fatal("ошибка обмена обязана возвращаться, а не глотаться")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("текст ошибки должен нести ответ Google, получили: %v", err)
	}
}

// The main check: in Vertex mode the request must go to the vertex URL, with a
// Bearer token and a model with the publisher prefix. The body stays
// OpenAI-compatible: that is why the mode reuses doGeminiChat instead of a
// separate branch.
func TestDoGeminiChat_РежимVertex(t *testing.T) {
	var tokenSrv *httptest.Server
	tokenSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"ya29.vertex","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	var gotPath, gotAuth, gotModel string
	vertexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ок"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer vertexSrv.Close()

	raw, _ := testServiceAccount(t, tokenSrv.URL)
	h := Handler{
		GeminiVertexSAJSON:  raw,
		GeminiVertexProject: "proj-x",
		c:                   newHandlerCaches(),
		HTTPClient:          vertexSrv.Client(),
	}
	// First make sure that WITHOUT an override the base is built correctly.
	h.GeminiVertexLocation = "us-central1"
	if base := h.vertexBaseURL(); !strings.Contains(base, "/projects/proj-x/locations/us-central1/endpoints/openapi") {
		t.Fatalf("база собрана неверно: %s", base)
	}
	// Now point it at the test server the same way prod redirects the request to
	// the relay: by overriding the base.
	h.GeminiVertexBaseURL = vertexSrv.URL + "/v1beta1/projects/proj-x/locations/us-central1/endpoints/openapi"

	out, _, _, _, err := h.doGeminiChat(context.Background(), "gemini-3.1-flash-lite", 0.5, "default", []map[string]string{{"role": "user", "content": "привет"}}, openAIChatOptions{})
	if err != nil {
		t.Fatalf("вызов упал: %v", err)
	}
	if out != "ок" {
		t.Fatalf("ответ = %q", out)
	}
	if gotAuth != "Bearer ya29.vertex" {
		t.Errorf("авторизация должна быть Bearer-токеном сервис-аккаунта, получили %q", gotAuth)
	}
	if gotModel != "google/gemini-3.1-flash-lite" {
		t.Errorf("модель должна уйти с префиксом издателя, получили %q", gotModel)
	}
	if !strings.Contains(gotPath, "/endpoints/openapi/chat/completions") {
		t.Errorf("путь должен быть OpenAI-совместимым эндпоинтом Vertex, получили %q", gotPath)
	}
}
