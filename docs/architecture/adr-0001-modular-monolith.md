# ADR-0001: Modular monolith for the API

- Status: accepted (2026-09-30)
- Scope: `apps/api` (Go). The web app, the anonymizer service and the static
  sites are outside this decision.

## Context

The API is one Go binary. Almost all of it lives in one package,
`internal/httpapi`: 106 source files, about 25 700 lines, plus 135 test files
(about 40 000 lines). One `Handler` struct carries 35 fields (every secret and
client of every feature) and 24 caches. 108 routes are registered in one
`router.go`. The database has 71 tables, all in one Postgres.

Feature boundaries exist only in file names. Nothing stops chat code from
writing to the billing tables, and it happens: a scan of the SQL strings finds
**71 module pairs and 275 SQL literals** where one feature queries another
feature's tables (see [modules.md](modules.md)).

We considered microservices and rejected them:

- **A small VPS.** Production runs on a 2–4 GB server. Every service adds a
  process, a connection pool, health checks and memory; there is no headroom
  for that.
- **One database, real transactions.** Users, access, promo codes, payments
  and messages are updated together (applying a promo code grants access and
  counts a usage in one transaction). Splitting them across services turns
  these into distributed consistency problems. The current flaky
  promo-transfer test shows how fragile this area already is inside one
  process.
- **Contributors are often not programmers.** The open repository targets
  psychologists who code with AI agents. One app started with one command is
  something they can run and debug; a set of services is not.
- **The bottleneck is not the architecture.** Load tests put the ceiling at
  about 200–300 concurrent users, limited by database queries per request.
  Services would add network hops, not capacity.

## Decision

Keep one binary and one database. Split the code into modules with enforced
boundaries.

### Structure

```
apps/api/internal/
  kernel/            shared, business-free building blocks
    httpjson/        JSON request/response helpers
    identity/        identity primitives (email normalization, later session lookup)
    ...              db, config, errors, observability as they move
  modules/
    <name>/          one directory per module; the root package is its public API
      internal/...   implementation details, invisible to other modules
  httpapi/           legacy package; shrinks as modules move out
```

Target modules (details and order in [modules.md](modules.md)): `auth`,
`access`, `billing`, `chat`, `modes`, `aigateway`, `notifications`,
`sitecontent`, `games`, `admin`, plus `argumentclinic` (the pilot).

### Rules

1. **A module owns its tables.** Only the owning module's SQL touches them.
   Other modules call the owner's Go API. Ownership is declared in
   `internal/archtest/ownership_map_test.go`.
2. **Modules talk through the root package only.** `modules/games` may import
   `modules/auth`, never `modules/auth/store`. Go's `internal/` directories
   make the rest invisible anyway.
3. **Direction of dependencies:** `httpapi` (legacy) → modules → kernel.
   The kernel never imports modules; modules never import `httpapi`.
4. **State is module-local.** A module keeps its caches in its own struct,
   created per router instance, so tests stay isolated (the existing
   handler-local cache rule, applied per module). No package-level mutable
   state.
5. **Config is passed in, not read.** A module gets exactly the settings and
   clients it needs through its constructor, not the whole `Handler`.
6. **The kernel stays minimal:** HTTP/JSON helpers, identity and session
   lookup, DB access helpers, config loading, error and observability
   plumbing. No business rules. If two modules need the same business rule,
   one of them owns it.
7. **Migrations stay in `apps/api/sql`**, one timeline for the whole database.
   A migration touches only tables of one module unless it is a documented
   data move.

### Route registration (deliberate deviation)

Modules export their HTTP handlers, but **the route table stays in
`internal/httpapi/router.go`** for now. Three tests read that file as text and
act as security controls: the OpenAPI contract check, the admin audit
classification and the privileged-route inventory. If a module registered its
own routes, those routes would silently drop out of all three checks.

Phase 2 (after the first few modules): replace text parsing with a runtime
route registry (`Routes() []Route` per module), switch the three tests to it,
then let modules register their own routes.

### Enforcement

`internal/archtest` runs in `go test ./...` (CI job `api-unit`):

- import rules above, on every non-test file;
- every table created by a migration has an owner; every source file maps to
  a module;
- cross-module table access is frozen in
  `internal/archtest/testdata/cross_module_table_access.txt`. New entries fail
  the build; entries that disappeared must be deleted from the file. The file
  can only shrink without a visible, reviewed diff.

## When a module may become a separate process

All of these must hold, measured, not assumed:

1. It owns its tables completely: zero baseline entries in either direction.
2. There is a concrete reason the shared process hurts: a different runtime
   (like the Python anonymizer), a resource profile that endangers the rest
   (measured: sustained >50 % of CPU or memory of the API), a different
   security boundary (for example an internet-facing webhook receiver that
   must not hold other secrets), or a deploy cadence that keeps breaking
   unrelated features.
3. Its API is already called through an interface, so the in-process
   implementation can be swapped for a client without touching callers.
4. The VPS budget covers the extra process.

Today only the anonymizer meets these criteria, and it is already separate.

## Consequences

- Moving a module is a mechanical, behaviour-preserving PR: same routes, same
  JSON, same SQL; the OpenAPI contract and all tests must pass unchanged.
- The biggest cost is not moving files but replacing cross-module SQL with
  calls (users 76 references, access tables 110). `admin` is the heaviest
  reader (107 references) because it builds reports across everything; it gets
  read-only query APIs or owner-provided views rather than raw access.
- New code goes into modules from day one; `httpapi` only shrinks.

## Pilot

`internal/modules/argumentclinic` (the public vote widget): one table, one
route, no dependencies besides the kernel. It moved without behaviour change;
the existing HTTP integration tests pass unchanged through the real router.
`writeJSON`, `decodeJSONStrict` and `normalizeEmail` in `httpapi` now delegate
to `kernel/httpjson` and `kernel/identity`, so legacy code and modules share
one implementation.
