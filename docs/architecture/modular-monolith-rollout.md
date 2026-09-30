# Modular monolith: rollout plan

Decision: [ADR-0001](adr-0001-modular-monolith.md). Current state and debt:
[modules.md](modules.md).

## Principles

- **One module per PR.** Each PR is mechanical and behaviour-preserving.
- **Rollback is one `git revert`.** No PR mixes a move with a feature, a
  migration or a behaviour change.
- **The guard only tightens.** `internal/archtest` baseline may lose lines in
  a move PR, never gain them.

## Definition of done for every move PR

1. `go test ./...`, `go vet -tags=integration ./...` and the full integration
   suite are green locally and in CI.
2. `docs/openapi/openapi.json` is unchanged, and the OpenAPI contract,
   route-inventory and admin-audit tests pass without edits to their
   expectations. Same routes, same JSON, same status codes.
3. Staging deploy and E2E (Playwright → stage) are green.
4. Migrations: none, or strictly additive (new table or column, no
   renames/drops). Applied manually on stage/prod as usual.
5. `archtest` baseline has only deletions; ownership map updated for moved
   files.
6. The module's caches live in its own struct; `handlerCaches` loses the
   matching fields.
7. `docs/architecture/modules.md` numbers re-generated.

## Prerequisites (before the first real module)

| Step | What | Why | Size |
|---|---|---|---|
| P0 | **Done:** ADR, module map, `archtest` guard, pilot `argumentclinic`, `kernel/httpjson`, `kernel/identity` | proves the pattern end to end | done |
| P1 | Move `NewTestServer` and the test factory out of `httpapi` test files into a shared test package (`internal/testsupport`) | module tests cannot use helpers that live in `httpapi` `_test.go` files | 1 day |
| P2 | Kernel settings API for `system_settings` (typed getters, one cache) | removes 10 refs from 5 modules; every later move needs it | 1 day |
| P3 | `auth` read API: `User(id)`, `Contacts(ids)`, `EnsureGuest`, `LogSecurityEvent` | `users` is the most-read foreign table (76 refs); unblocks notifications, chat, admin, games | 2 days |

## Order of module moves

Ordered by (a) foreign references the move must resolve, (b) coupling with
branches in flight, (c) size.

| # | Module | Why here | Must resolve | Size (est.) |
|---|---|---|---|---|
| 1 | `sitecontent` | 476 lines, no outgoing debt | nothing; 2 incoming refs keep working until billing/notifications move | 0.5 day |
| 2 | `games` | 2 142 lines, 1 outgoing ref (`security_events` → P3) | `auth.LogSecurityEvent` | 1–1.5 days |
| 3 | `aigateway` | 3 outgoing refs; clean provider interface already exists | settings API (P2); wait until the data-protection branch (processing country) is merged | 1.5–2 days |
| 4 | `notifications` | biggest *reader of users* (27); after P3 it is self-contained | `auth.Contacts`, `modes` summary prompts, `access` promo usage (read APIs) | 3–4 days |
| 5 | `billing` | payments must call access inside the payment transaction | `access.GrantPaidAccessTx`, `access.Tariff` | 3 days |
| 6 | `modes` + `access` | tightly coupled (`access→modes` 10, `modes→access` 11). Move `modes` first with `modes.Exists/Active`; then `access` with tariff/grant/usage APIs | the two APIs | 4–5 days |
| 7 | `chat` | depends on everything above; split `handlers.go` | all read APIs; anonymizer hook from data-protection | 4–5 days |
| 8 | `admin` | 107 outgoing refs: reports across all modules | read-only query APIs or owner-provided SQL views (`<module>_admin_*`) | 4–5 days |
| 9 | Route registry (ADR phase 2) | switch the three route tests to a runtime registry; modules register routes | — | 1 day |

Total: about **25–30 working days**, 12–14 PRs. Each PR is independently
deployable; the work can pause after any step.

## How this fits with the other work in flight

1. **Merge the in-flight branches first** (data protection, reversible
   anonymizer, open-source export). They touch `ai_gateways`, profile,
   consent and chat code; moving those files now would make every merge a
   conflict.
2. **Translate comments to English next**, as one mechanical pass over the
   whole tree. Moves and translation both touch every line of a file;
   doing them in parallel doubles the conflicts.
3. **Publish the open repository, then do the module moves there.**
   Recommendation: moves happen *after* publication, in the open repo that is
   the source of truth. Reasons:
   - publication is blocked on the translation pass, not on modules; the moves
     are 5–6 weeks of work and should not delay it;
   - once the open repo is the source of truth, a move lands there once;
     moves started in the private repo around the export date would have to
     be redone or ported by hand;
   - small, well-specified move PRs with a guard that says exactly what is
     wrong are good first issues for outside contributors.
   The pilot and the guard land now: they are small, touch no in-flight
   files, and every change from today on is checked by the guard.

## Rollback

- A move PR: `git revert <sha>`; no data changes to undo.
- A read API introduced for a move: stays harmless if the move is reverted.
- If the guard itself blocks urgent work: fix forward by adding the entry to
  the baseline with a reason in the PR (`ARCHTEST_UPDATE_BASELINE=1`); never
  delete the test.
