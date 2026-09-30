//go:build integration

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Factory builds typed rows in the database for tests.
// Methods fail the test on any error — callers should not have to check.
type Factory struct {
	t    testing.TB
	pool *pgxpool.Pool
	seq  atomic.Int64
}

func NewFactory(t testing.TB, pool *pgxpool.Pool) *Factory {
	t.Helper()
	return &Factory{t: t, pool: pool}
}

// uniqueSuffix returns a string unique within this Factory instance + process.
// Useful for emails, mode names, promo codes — anything with a UNIQUE index.
func (f *Factory) uniqueSuffix() string {
	n := f.seq.Add(1)
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), n)
}

var testPasswordHashCache sync.Map

// cachedTestPasswordHash returns a verifier-compatible password hash without
// spending production PBKDF2 cost for every factory-created user.
//
// Two layers of speed-up:
//  1. The cache memoises by password — common defaults like "testpass123"
//     are derived exactly once per test binary, not 350+ times.
//  2. The hash is encoded with the current value of passwordPBKDF2Iter,
//     which TestMain lowers to a small constant under the integration build.
//     verifyPassword's lower bound (passwordPBKDF2IterMin) is matched, so
//     factory-issued hashes verify correctly through the login endpoint at
//     ~10 ms instead of ~1 s per call.
func cachedTestPasswordHash(password string) (string, error) {
	if len(password) < 10 {
		return "", errors.New("password must be at least 10 characters")
	}
	iter := passwordPBKDF2Iter
	cacheKey := fmt.Sprintf("%d:%s", iter, password)
	if cached, ok := testPasswordHashCache.Load(cacheKey); ok {
		return cached.(string), nil
	}
	saltSeed := sha256.Sum256([]byte("mindstrata integration password salt:" + password))
	salt := saltSeed[:passwordPBKDF2SaltSize]
	key := pbkdf2SHA256([]byte(password), salt, iter, passwordPBKDF2KeySize)
	encoded := fmt.Sprintf("pbkdf2_sha256$%d$%s$%s",
		iter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
	actual, _ := testPasswordHashCache.LoadOrStore(cacheKey, encoded)
	return actual.(string), nil
}

type TestUserOpts struct {
	Email          string
	Password       string // default: "testpass123"
	Role           string // default: "user"
	Status         string // default: "active"
	EmailVerified  bool
	TelegramHandle string
}

type TestUser struct {
	ID           int64
	Email        string
	Password     string // raw, useful for HTTP login tests
	PasswordHash string
	Role         string
	Status       string
}

// CreateUser inserts a user row with a properly hashed password.
func (f *Factory) CreateUser(opts TestUserOpts) *TestUser {
	f.t.Helper()
	if opts.Email == "" {
		opts.Email = "user_" + f.uniqueSuffix() + "@test.local"
	}
	if opts.Password == "" {
		opts.Password = "testpass123"
	}
	if opts.Role == "" {
		opts.Role = "user"
	}
	if opts.Status == "" {
		opts.Status = "active"
	}
	if opts.TelegramHandle == "" {
		opts.TelegramHandle = "webuser_" + f.uniqueSuffix()
	}
	hash, err := cachedTestPasswordHash(opts.Password)
	if err != nil {
		f.t.Fatalf("factory: hash password: %v", err)
	}
	var id int64
	var verifiedAt any
	if opts.EmailVerified {
		verifiedAt = time.Now()
	}
	err = f.pool.QueryRow(context.Background(), `
		insert into users (email, password_hash, role, status, telegram_username, email_verified_at)
		values ($1, $2, $3, $4, $5, $6)
		returning id`,
		strings.ToLower(opts.Email), hash, opts.Role, opts.Status, opts.TelegramHandle, verifiedAt,
	).Scan(&id)
	if err != nil {
		f.t.Fatalf("factory: insert user: %v", err)
	}
	return &TestUser{
		ID:           id,
		Email:        strings.ToLower(opts.Email),
		Password:     opts.Password,
		PasswordHash: hash,
		Role:         opts.Role,
		Status:       opts.Status,
	}
}

type TestModeOpts struct {
	Name           string
	Prompt         string
	WelcomeMessage string
	AIModel        string
	Temperature    float64
}

type TestMode struct {
	ID          int64
	Name        string
	Prompt      string
	AIModel     string
	Temperature float64
}

// CreateMode inserts a mode row.
func (f *Factory) CreateMode(opts TestModeOpts) *TestMode {
	f.t.Helper()
	if opts.Name == "" {
		opts.Name = "mode_" + f.uniqueSuffix()
	}
	if opts.Prompt == "" {
		opts.Prompt = "You are a test assistant for " + opts.Name
	}
	if opts.AIModel == "" {
		opts.AIModel = "openai/gpt-4o-mini"
	}
	if opts.Temperature == 0 {
		opts.Temperature = 0.7
	}
	var id int64
	err := f.pool.QueryRow(context.Background(), `
		insert into modes (name, prompt, welcome_message, ai_model, model_temperature)
		values ($1, $2, $3, $4, $5)
		returning id`,
		opts.Name, opts.Prompt, opts.WelcomeMessage, opts.AIModel, opts.Temperature,
	).Scan(&id)
	if err != nil {
		f.t.Fatalf("factory: insert mode: %v", err)
	}
	return &TestMode{
		ID:          id,
		Name:        opts.Name,
		Prompt:      opts.Prompt,
		AIModel:     opts.AIModel,
		Temperature: opts.Temperature,
	}
}

type GrantAccessOpts struct {
	UserID            int64
	ModeID            int64
	DailyMessageLimit int    // default: 50
	AccessType        string // default: "direct"
	ActiveFrom        *time.Time
	ActiveTo          *time.Time
	Priority          int
	SourceID          *int64
}

// GrantAccess inserts a user_mode_access row.
//
// Schema notes (verified against schema_base.sql):
//   - access_type must be one of: promocode, subscription, manual
//   - active_from / active_to are NOT NULL
//   - priority is NOT NULL (default 0 here for tests)
func (f *Factory) GrantAccess(opts GrantAccessOpts) {
	f.t.Helper()
	if opts.DailyMessageLimit == 0 {
		opts.DailyMessageLimit = 50
	}
	if opts.AccessType == "" {
		opts.AccessType = "manual"
	}
	now := time.Now()
	from := now.Add(-time.Hour)
	if opts.ActiveFrom != nil {
		from = *opts.ActiveFrom
	}
	to := now.Add(365 * 24 * time.Hour)
	if opts.ActiveTo != nil {
		to = *opts.ActiveTo
	}
	_, err := f.pool.Exec(context.Background(), `
		insert into user_mode_access
		  (user_id, mode_id, source_id, active_from, active_to, daily_message_limit, priority, access_type)
		values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		opts.UserID, opts.ModeID, opts.SourceID, from, to,
		opts.DailyMessageLimit, opts.Priority, opts.AccessType,
	)
	if err != nil {
		f.t.Fatalf("factory: grant access: %v", err)
	}
}

type TestDialog struct {
	ID     int64
	UserID int64
	ModeID int64
}

func (f *Factory) CreateDialog(userID, modeID int64) *TestDialog {
	f.t.Helper()
	var id int64
	err := f.pool.QueryRow(context.Background(), `
		insert into users_dialogs (user_id, mode_id) values ($1, $2)
		returning id`, userID, modeID,
	).Scan(&id)
	if err != nil {
		f.t.Fatalf("factory: insert dialog: %v", err)
	}
	return &TestDialog{ID: id, UserID: userID, ModeID: modeID}
}

func (f *Factory) AppendMessage(dialogID int64, role, content string) int64 {
	f.t.Helper()
	var id int64
	err := f.pool.QueryRow(context.Background(), `
		insert into dialogs_messages (dialog_id, role, content) values ($1, $2, $3)
		returning id`, dialogID, role, content,
	).Scan(&id)
	if err != nil {
		f.t.Fatalf("factory: append message: %v", err)
	}
	return id
}

type TestPromocodeOpts struct {
	Code         string
	MaxUses      int    // default: 0 (unlimited)
	GrantsType   string // mode | tariff | admin_role; default: mode
	TargetID     int64  // for grants_type=mode: mode ID
	DurationDays int    // default: 30
	ActiveFrom   *time.Time
	ActiveTo     *time.Time
}

type TestPromocode struct {
	ID   int64
	Code string
}

// CreatePromocode inserts a promocode row.
//
// Schema notes:
//   - max_uses, used_count, duration, access_priority, target_id, limit_type are NOT NULL.
//   - grants_type CHECK enforces one of: mode, tariff, admin_role.
func (f *Factory) CreatePromocode(opts TestPromocodeOpts) *TestPromocode {
	f.t.Helper()
	if opts.Code == "" {
		opts.Code = "TEST_" + f.uniqueSuffix()
	}
	if opts.GrantsType == "" {
		opts.GrantsType = "mode"
	}
	if opts.DurationDays == 0 {
		opts.DurationDays = 30
	}
	durationInterval := fmt.Sprintf("%d days", opts.DurationDays)
	var id int64
	err := f.pool.QueryRow(context.Background(), `
		insert into promocodes
		  (code, max_uses, used_count, active_from, active_to, duration,
		   access_priority, grants_type, target_id, limit_type, daily_message_limit)
		values
		  ($1, $2, 0, $3, $4, $5::interval, 0, $6, $7, 'fixed', 50)
		returning id`,
		opts.Code, opts.MaxUses, opts.ActiveFrom, opts.ActiveTo, durationInterval,
		opts.GrantsType, opts.TargetID,
	).Scan(&id)
	if err != nil {
		f.t.Fatalf("factory: insert promocode: %v", err)
	}
	return &TestPromocode{ID: id, Code: opts.Code}
}

// timeAgo returns a time.Time `hours` ago — useful for setting historic active_from/to in tests.
func timeAgo(hours int) time.Time {
	return time.Now().Add(-time.Duration(hours) * time.Hour)
}

// CreateSession creates an auth_session and returns the raw token for cookies.
func (f *Factory) CreateSession(userID int64) (rawToken string) {
	f.t.Helper()
	token, err := randomToken(32)
	if err != nil {
		f.t.Fatalf("factory: random token: %v", err)
	}
	_, err = f.pool.Exec(context.Background(), `
		insert into auth_sessions (user_id, token_hash, user_agent, ip_hash, expires_at)
		values ($1, $2, $3, $4, $5)`,
		userID, tokenHash(token), "test-agent", "test-ip-hash",
		time.Now().Add(authSessionTTL),
	)
	if err != nil {
		f.t.Fatalf("factory: insert session: %v", err)
	}
	return token
}

// TestServer wraps an httptest.Server with a cookie-aware HTTP client.
type TestServer struct {
	t       testing.TB
	server  *httptest.Server
	Client  *http.Client
	Handler Handler
}

// NewTestServer constructs a Handler with the given DB, starts an httptest.Server,
// and returns a client with cookie jar enabled.
//
// Each TestServer creates its own handlerCaches instance: rate limiters and
// admin caches are isolated between parallel tests. There are no global state
// variables any more, so tests safely run with t.Parallel().
func NewTestServer(t testing.TB, pool *pgxpool.Pool) *TestServer {
	t.Helper()
	// Rate ceilings are off: tests deliberately knock with wrong keys, and blocking
	// the address would break neighbouring checks running in parallel.
	// The limiter itself is checked by a separate test.
	return NewTestServerWithHandler(t, pool, Handler{DisableRateLimits: true})
}

func NewTestServerWithHandler(t testing.TB, pool *pgxpool.Pool, handler Handler) *TestServer {
	t.Helper()
	if handler.DB == nil {
		handler.DB = pool
	}
	if handler.HTTPClient == nil {
		handler.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if handler.PromoAdminSecret == "" {
		handler.PromoAdminSecret = "test-promo-admin-secret" // S-NEW-2: an explicit secret instead of the removed fallback
	}
	if handler.c == nil {
		handler.c = newHandlerCaches()
	}
	mux := NewRouter(handler, []string{"*"})
	srv := httptest.NewServer(mux)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("testserver: cookiejar: %v", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	t.Cleanup(srv.Close)
	return &TestServer{t: t, server: srv, Client: client, Handler: handler}
}

func (ts *TestServer) URL(path string) string {
	return ts.server.URL + path
}

// LoginAs sets the session cookie on the client jar so subsequent requests
// authenticate as the given user.
func (ts *TestServer) LoginAs(rawToken string) {
	ts.t.Helper()
	// Mimic the cookie set by createAuthSession.
	u, err := http.NewRequest("GET", ts.server.URL+"/", nil)
	if err != nil {
		ts.t.Fatalf("login: build request: %v", err)
	}
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
	}
	ts.Client.Jar.SetCookies(u.URL, []*http.Cookie{cookie})
}
