# Testing

[По-русски](TESTING.ru.md)

**Rule:** open a pull request only after `make check` is green on your
computer, or CI is green in your fork (with or without [your own
runner](../runner/README.md)). Paste the output into the pull request.

## One command

```bash
make web-install   # once
make check
```

`make check` runs exactly what CI runs:

| Step | Target | What it checks |
|---|---|---|
| Repository gates | `make guard` | gofmt; Go tests of the gates; self-hosted runner only in forks; runner container is safe; actions pinned to SHAs; no public IPs, real e-mails or real-looking personal data; the deploy template is generic |
| Secrets | `make secrets` | gitleaks over the commits of your change (`RANGE=origin/main..HEAD` by default) |
| API vet | `make api-vet` | `go vet`, including integration-tagged files |
| API unit | `make api-unit` | `go test -race ./...` in `apps/api` |
| API integration | `make api-integration` | real Postgres: starts a throwaway container on port 55439, or uses `TEST_DATABASE_URL` |
| Web | `make web-check` | TypeScript typecheck, API contract (OpenAPI types are up to date), vitest |
| Anonymizer | `make anonymizer-test` | personal-data detector (Python, pytest in a local venv): rules, gold set, property tests. The one Python exception to "tests in Go": the detector is built on Natasha and pymorphy3 |
| Comments | `make comments` | code comments are in English |
| Tests changed | `make tests-touched` | pull requests only: changed logic comes with changed tests |

## Test layers

- **Go unit tests** (`*_test.go` without build tags) — pure logic, fast, run
  with the race detector.
- **Go integration tests** (`//go:build integration`) — the HTTP API against a
  real Postgres. Each test gets its own database cloned from a template built
  from `apps/api/internal/testsupport/schema_base.sql`, so tests run in
  parallel without sharing state. Use handler-local caches, not package
  globals.
- **Frontend tests** (vitest, `*.test.ts(x)`) — components and helpers.
- **End-to-end** (Playwright, `apps/web/e2e`) — against a running site:
  `PLAYWRIGHT_BASE_URL=http://localhost:3000 npx playwright test` in
  `apps/web` after `make dev`. Not part of `make check`.
- **Repository tools** (`tools/`) — Go with table tests.
- **Fresh clone** (CI job `Fresh clone: make dev`) — on a clean GitHub
  machine, with `go`/`node`/`npm` replaced by shims that fail, `make dev` must bring up the
  site, the API and the development admin, and a second `make dev` must work
  too. It keeps the README's "one command" promise honest.

## Writing tests

- New tests and tools are written in Go; the frontend is tested with vitest.
- `t.Parallel()` by default; if a test cannot be parallel, say why in a
  comment next to it.
- A bug fix starts with a test that fails without the fix.
- Endpoints that touch a user's object: add "another user → 403/404".
- Fixtures are synthetic only (`testdata/README.md`).
- Time: take "now" from the database in integration tests, not from the
  test process, and allow a couple of seconds of drift.

## CI in pull requests

A job fails if logic changed (`apps/api/**/*.go`, `apps/web/src/**`,
`tools/**/*.go`) and no test file changed. If a test really makes no sense,
explain why in the pull request; a maintainer may add the `no-tests-needed`
label.
