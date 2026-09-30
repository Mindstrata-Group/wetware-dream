package aireport

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Activities struct {
	pool     *pgxpool.Pool
	envLabel string
	// httpClient is overridden in tests.
	httpClient *http.Client
}

func NewActivities(pool *pgxpool.Pool, envLabel string) *Activities {
	return &Activities{pool: pool, envLabel: envLabel, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

type providerCount struct {
	Provider string
	Outcome  string
	N        int64
}

type ReportResult struct {
	Sent    bool   `json:"sent"`
	Message string `json:"message"`
}

// fetchYesterday pulls the counters for the previous (UTC) day: the job
// runs at 06:00 Yekaterinburg (01:00 UTC), i.e. it counts an already closed day.
func (a *Activities) fetchYesterday(ctx context.Context) ([]providerCount, error) {
	rows, err := a.pool.Query(ctx, `
		select provider, outcome, count(*)
		from ai_provider_events
		where event_date = (now() at time zone 'utc')::date - 1
		group by provider, outcome
		order by provider, outcome
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []providerCount
	for rows.Next() {
		var c providerCount
		if err := rows.Scan(&c.Provider, &c.Outcome, &c.N); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *Activities) kumaPushURL(ctx context.Context) (string, error) {
	var value string
	err := a.pool.QueryRow(ctx, `select value from system_settings where key = 'ai_daily_report_kuma_push_url'`).Scan(&value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// formatReport builds the message text: messages per provider, failed
// attempts, and how many times the whole chain was exhausted (the user got
// an error). An empty day ("no data") is also a valid, quiet result.
func formatReport(envLabel string, day time.Time, counts []providerCount) string {
	success := map[string]int64{}
	failed := map[string]int64{}
	var exhausted int64
	providers := map[string]bool{}
	for _, c := range counts {
		providers[c.Provider] = true
		switch c.Outcome {
		case "success":
			success[c.Provider] += c.N
		case "attempt_failed":
			failed[c.Provider] += c.N
		case "exhausted":
			exhausted += c.N
		}
	}
	names := make([]string, 0, len(providers))
	for p := range providers {
		names = append(names, p)
	}
	sort.Strings(names)

	b := new(strings.Builder)
	fmt.Fprintf(b, "AI-провайдеры [%s] за %s:", envLabel, day.Format("2006-01-02"))
	if len(names) == 0 {
		b.WriteString(" нет данных (не было ни одного вызова)")
		return b.String()
	}
	for _, p := range names {
		fmt.Fprintf(b, " %s=%d сообщ.", p, success[p])
		if failed[p] > 0 {
			fmt.Fprintf(b, "(%d сбоев попыток)", failed[p])
		}
		b.WriteString(";")
	}
	if exhausted > 0 {
		fmt.Fprintf(b, " ПОЛНЫЙ ОТКАЗ ЦЕПОЧКИ: %d раз(а) — пользователи получили ошибку.", exhausted)
	}
	return b.String()
}

// SendAIProviderDailyReport is the only activity: it counts yesterday's
// statistics and pushes them as a heartbeat to an Uptime Kuma push monitor.
// Best effort, like the other scheduled jobs: if the push URL is not
// configured in system_settings, it quietly finishes without an error (so
// Temporal's retry policy does not block the other schedules).
func (a *Activities) SendAIProviderDailyReport(ctx context.Context) (ReportResult, error) {
	pushURL, err := a.kumaPushURL(ctx)
	if err != nil {
		return ReportResult{}, fmt.Errorf("read kuma push url setting: %w", err)
	}
	if pushURL == "" {
		return ReportResult{Sent: false, Message: "ai_daily_report_kuma_push_url is not configured"}, nil
	}
	counts, err := a.fetchYesterday(ctx)
	if err != nil {
		return ReportResult{}, fmt.Errorf("fetch yesterday stats: %w", err)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	msg := formatReport(a.envLabel, yesterday, counts)

	q := url.Values{}
	q.Set("status", "up")
	q.Set("msg", msg)
	q.Set("ping", "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pushURL+"?"+q.Encode(), nil)
	if err != nil {
		return ReportResult{}, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ReportResult{}, fmt.Errorf("push to kuma: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ReportResult{}, fmt.Errorf("kuma push returned status %d", resp.StatusCode)
	}
	return ReportResult{Sent: true, Message: msg}, nil
}
