package pseudonym

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrResidual means personal data was still found after masking. The text
// must not be sent anywhere; the detector never says which values.
var ErrResidual = errors.New("pseudonym: personal data remains after masking")

// Service is the pre-send transform / post-receive restore pair.
type Service struct {
	Detector Detector
	Vault    Vault
	Profiles ProfileSource
}

// Config of the production wiring.
type Config struct {
	MasterKey  string        // PII_MASTER_KEY, base64 of 32 bytes
	ServiceURL string        // ANONYMIZER_URL, e.g. http://anonymizer-http:8090
	VaultTTL   time.Duration // PII_VAULT_TTL_DAYS, default 180 days
}

func ConfigFromEnv() Config {
	ttl := 180 * 24 * time.Hour
	if days, err := strconv.Atoi(os.Getenv("PII_VAULT_TTL_DAYS")); err == nil && days > 0 {
		ttl = time.Duration(days) * 24 * time.Hour
	}
	url := os.Getenv("ANONYMIZER_URL")
	if url == "" {
		url = "http://anonymizer-http:8090"
	}
	return Config{MasterKey: os.Getenv("PII_MASTER_KEY"), ServiceURL: url, VaultTTL: ttl}
}

// New wires the Postgres vault, the users table and the HTTP detector.
// Without a master key it returns ErrNoKey: callers treat that as "external
// models are off", never as "send in clear text".
func New(cfg Config, pool *pgxpool.Pool) (*Service, error) {
	keys, err := ParseKeys(cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	return &Service{
		Detector: NewHTTPClient(cfg.ServiceURL),
		Vault:    &PGVault{Pool: pool, Keys: keys},
		Profiles: PGProfiles{Pool: pool},
	}, nil
}

// SessionRef ties a restore to the transform that produced the aliases. It
// carries the alias language; aliases themselves are per user, not per
// session, so an alias the model remembers from an earlier message restores
// too.
type SessionRef string

func (r SessionRef) lang() string {
	if strings.HasSuffix(string(r), ":en") {
		return "en"
	}
	return "ru"
}

func profileKnown(p Profile) []Known {
	var out []Known
	if strings.TrimSpace(p.DisplayName) != "" {
		out = append(out, Known{Kind: "PERSON", Value: p.DisplayName})
	}
	if strings.TrimSpace(p.Email) != "" {
		out = append(out, Known{Kind: "EMAIL", Value: p.Email})
	}
	if strings.TrimSpace(p.Phone) != "" {
		out = append(out, Known{Kind: "PHONE", Value: p.Phone})
	}
	if strings.TrimSpace(p.Telegram) != "" {
		out = append(out, Known{Kind: "HANDLE", Value: p.Telegram})
	}
	return out
}

// Transform masks text for userID. lang is "ru", "en" or "auto".
func (s *Service) Transform(ctx context.Context, userID int64, lang, text string) (string, SessionRef, error) {
	if s == nil || s.Detector == nil || s.Vault == nil {
		return "", "", ErrNoKey
	}
	entries, err := s.Vault.Load(ctx, userID)
	if err != nil {
		return "", "", fmt.Errorf("pseudonym: load vault: %w", err)
	}
	var profile Profile
	if s.Profiles != nil {
		if profile, err = s.Profiles.Profile(ctx, userID); err != nil {
			return "", "", fmt.Errorf("pseudonym: load profile: %w", err)
		}
	}
	known := profileKnown(profile)
	byKey := make(map[string]int, len(entries))
	for _, e := range entries {
		known = append(known, Known{Kind: e.Kind, Value: e.Canon, Canon: e.Canon, Key: e.Key})
		byKey[e.Kind+"\x00"+e.Key] = e.Number
	}
	res, err := s.Detector.Detect(ctx, text, lang, known)
	if err != nil {
		return "", "", err
	}
	if res.Residual > 0 {
		return "", "", ErrResidual
	}
	if res.Lang == "" {
		res.Lang = "ru"
	}
	masked, err := s.mask(ctx, userID, res, text, byKey)
	if err != nil {
		return "", "", err
	}
	if leaksProfile(masked, profile) {
		return "", "", ErrResidual
	}
	return masked, SessionRef("v1:" + res.Lang), nil
}

func (s *Service) mask(ctx context.Context, userID int64, res DetectResult, text string, byKey map[string]int) (string, error) {
	// Span offsets are code points; walk the string once to map them to bytes.
	runeToByte := make([]int, 0, utf8.RuneCountInString(text)+1)
	for i := range text {
		runeToByte = append(runeToByte, i)
	}
	runeToByte = append(runeToByte, len(text))
	spans := append([]Span(nil), res.Spans...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if !ValidKind(sp.Kind) || sp.Start < 0 || sp.End > len(runeToByte)-1 || sp.Start >= sp.End {
			return "", fmt.Errorf("%w: bad span", ErrUnavailable)
		}
		start, end := runeToByte[sp.Start], runeToByte[sp.End]
		if start < cursor {
			return "", fmt.Errorf("%w: overlapping spans", ErrUnavailable)
		}
		number, ok := byKey[sp.Kind+"\x00"+sp.Key]
		if !ok {
			var err error
			number, err = s.Vault.Assign(ctx, userID, sp.Kind, sp.Key, sp.Canon)
			if err != nil {
				return "", fmt.Errorf("pseudonym: assign alias: %w", err)
			}
			byKey[sp.Kind+"\x00"+sp.Key] = number
		}
		b.WriteString(text[cursor:start])
		b.WriteString(Alias(sp.Kind, res.Lang, number))
		cursor = end
	}
	b.WriteString(text[cursor:])
	return b.String(), nil
}

var nonDigit = regexp.MustCompile(`\D`)

// leaksProfile is a last exact-match check in Go, independent of the
// detector: the profile's e-mail, phone digits and Telegram username must
// not appear in the masked text.
func leaksProfile(masked string, p Profile) bool {
	low := strings.ToLower(masked)
	if e := strings.ToLower(strings.TrimSpace(p.Email)); e != "" && strings.Contains(low, e) {
		return true
	}
	if t := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(p.Telegram), "@")); len(t) >= 3 && strings.Contains(low, "@"+t) {
		return true
	}
	if d := nonDigit.ReplaceAllString(p.Phone, ""); len(d) >= 10 {
		if strings.Contains(nonDigit.ReplaceAllString(masked, ""), d[len(d)-10:]) {
			return true
		}
	}
	return false
}

// Restore puts the user's values back into a complete piece of model text.
// It never fails: if the service is down, aliases get the stored value in
// the nominative case, and unknown aliases are marked.
func (s *Service) Restore(ctx context.Context, userID int64, ref SessionRef, chunk string) string {
	entries, err := s.Vault.Load(ctx, userID)
	if err != nil {
		return restoreNominative(chunk, nil)
	}
	return s.restoreWith(ctx, entries, "", chunk)
}

func (s *Service) restoreWith(ctx context.Context, entries []Entry, contextText, chunk string) string {
	if len(findAliases(chunk)) == 0 {
		return chunk
	}
	res, err := s.Detector.Restore(ctx, chunk, contextText, entries)
	if err != nil {
		table := make(map[entryKey]Entry, len(entries))
		for _, e := range entries {
			table[entryKey{e.Kind, e.Number}] = e
		}
		return restoreNominative(chunk, table)
	}
	return res.Text
}
