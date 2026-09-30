package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()
	if !reflect.DeepEqual(Generate(7, 20), Generate(7, 20)) {
		t.Fatal("the same seed must give the same data")
	}
	if reflect.DeepEqual(Generate(7, 20), Generate(8, 20)) {
		t.Fatal("different seeds should give different data")
	}
}

// Generated data must be obviously synthetic so it can never be mistaken for
// a real person in a screenshot or a bug report.
func TestGenerateIsSynthetic(t *testing.T) {
	t.Parallel()
	phone := regexp.MustCompile(`^\+7 900 000-\d{2}-\d{2}$`)
	seen := map[string]bool{}
	for _, c := range Generate(1, 150) {
		if !strings.HasSuffix(c.Email, "@example.com") {
			t.Fatalf("e-mail on a real domain: %s", c.Email)
		}
		if seen[c.Email] {
			t.Fatalf("duplicate e-mail %s", c.Email)
		}
		seen[c.Email] = true
		if !phone.MatchString(c.Phone) {
			t.Fatalf("phone outside the synthetic block: %s", c.Phone)
		}
		if len(c.Messages) == 0 || c.Messages[0].Role != "user" {
			t.Fatalf("chat must start with the user: %+v", c.Messages)
		}
	}
}

// The dev login is printed only when it was really created.
func TestBootstrapAdminReportsCreation(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		status  int
		created bool
		fails   bool
	}{{201, true, false}, {409, false, false}, {403, false, true}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Bootstrap-Token") != "tok" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(c.status)
		}))
		created, err := bootstrapAdmin(context.Background(), srv.URL, "tok")
		srv.Close()
		if created != c.created || (err != nil) != c.fails {
			t.Fatalf("status %d: created=%v err=%v", c.status, created, err)
		}
	}
}

func TestAllowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flag, env string
		ok        bool
	}{
		{"1", "local", true},
		{"1", "", true},
		{"", "local", false},
		{"yes", "local", false},
		{"1", "production", false},
		{"1", "PRODUCTION", false},
	}
	for _, c := range cases {
		if err := allowed(c.flag, c.env); (err == nil) != c.ok {
			t.Fatalf("allowed(%q, %q) = %v, want ok=%v", c.flag, c.env, err, c.ok)
		}
	}
}

// `make dev` runs the seeder right after `docker compose up`; the API may
// still be starting. Connection errors and 502/503 are retried until the
// context ends instead of failing the whole setup on the first request.
func TestBootstrapAdminWaitsForAPI(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	created, err := bootstrapAdminWithRetry(ctx, srv.URL, "tok", 10*time.Millisecond)
	if err != nil || !created || calls.Load() != 3 {
		t.Fatalf("created=%v err=%v calls=%d, want created after 3 calls", created, err, calls.Load())
	}
}

// A closed port (API not listening yet) is retried; once the deadline passes
// the error is returned, not swallowed.
func TestBootstrapAdminGivesUpAtDeadline(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens there any more
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := bootstrapAdminWithRetry(ctx, url, "tok", 20*time.Millisecond); err == nil {
		t.Fatal("an API that never comes up must end with an error")
	}
}

// A wrong token is a real answer, not a startup hiccup: no retries.
func TestBootstrapAdminDoesNotRetryRefusal(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := bootstrapAdminWithRetry(context.Background(), srv.URL, "bad", time.Millisecond); err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want one call and an error", err, calls.Load())
	}
}
