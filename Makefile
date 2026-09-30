# One entry point for people, agents and CI. `make check` runs exactly what
# CI runs; open a pull request only after it is green (docs/TESTING.md).

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO ?= go
BASE ?= origin/main
RANGE ?= $(BASE)..HEAD
TEST_PG_NAME ?= mindstrata-test-pg
TEST_PG_PORT ?= 55439

PYTHON ?= python3
ANON_VENV ?= .cache/anonymizer-venv

.PHONY: help check guard comments secrets tests-touched fmt-check api-vet api-unit api-integration \
        web-install web-check anonymizer-test dev dev-down hooks migration migrate

help: ## list targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

check: guard secrets api-vet api-unit api-integration web-check anonymizer-test comments ## everything CI runs
	@echo "make check: green"

guard: fmt-check ## repository gates: runner safety, pinned actions, hygiene, deploy template
	$(GO) test ./tools/...
	$(GO) run ./tools/prcheck runner-safety
	$(GO) run ./tools/prcheck runner-compose
	$(GO) run ./tools/prcheck pinned-actions
	$(GO) run ./tools/prcheck hygiene
	$(GO) run ./tools/prcheck deploy
	$(GO) run ./tools/prcheck fresh-clone

comments: ## code comments are in English
	$(GO) run ./tools/prcheck comments

secrets: ## gitleaks over the commits of this change (RANGE=base..head)
	bash scripts/scan-secrets.sh "$(RANGE)"

tests-touched: ## logic changed => tests changed (or the no-tests-needed label)
	$(GO) run ./tools/prcheck tests-touched -base "$(BASE)"

fmt-check:
	@out="$$(gofmt -l apps/api tools)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

api-vet: ## go vet, including integration tests
	cd apps/api && $(GO) vet ./... && $(GO) vet -tags=integration ./...

api-unit: ## API unit tests with the race detector
	cd apps/api && $(GO) test -race ./...

# Integration tests need Postgres. The contributor runner provides one
# (TEST_DATABASE_URL); otherwise a throwaway container is started here.
api-integration: ## API integration tests against Postgres
	@if [ -n "$${TEST_DATABASE_URL:-}" ]; then \
	  cd apps/api && $(GO) test -tags=integration -timeout=90m ./internal/httpapi/...; \
	else \
	  docker rm -f $(TEST_PG_NAME) >/dev/null 2>&1 || true; \
	  docker run -d --name $(TEST_PG_NAME) -e POSTGRES_USER=mindstrata -e POSTGRES_PASSWORD=mind \
	    -p 127.0.0.1:$(TEST_PG_PORT):5432 postgres:17-alpine -c max_connections=300 >/dev/null; \
	  trap 'docker rm -f $(TEST_PG_NAME) >/dev/null' EXIT; \
	  for i in $$(seq 1 60); do docker exec $(TEST_PG_NAME) pg_isready -U mindstrata >/dev/null 2>&1 && break; sleep 1; done; \
	  cd apps/api && TEST_DATABASE_URL="postgres://mindstrata:mind@127.0.0.1:$(TEST_PG_PORT)/postgres?sslmode=disable" \
	    $(GO) test -tags=integration -timeout=90m ./internal/httpapi/...; \
	fi

web-install:
	cd apps/web && npm ci --no-audit --fund=false

web-check: ## typecheck, API contract, vitest
	cd apps/web && npm run typecheck && npm run openapi:check && npx vitest run

anonymizer-test: ## personal-data anonymizer: unit, gold set, property tests (Python)
	test -x $(abspath $(ANON_VENV))/bin/python || $(PYTHON) -m venv $(abspath $(ANON_VENV))
	$(abspath $(ANON_VENV))/bin/pip install -q -r apps/anonymizer/requirements-dev.txt
	cd apps/anonymizer && $(abspath $(ANON_VENV))/bin/python -m pytest

dev: .env ## local stack with synthetic data and a development admin (needs only Docker and make)
	@docker compose up -d --build --wait || { \
	  echo; \
	  echo "The stack did not start. If a port is busy, pick free ones in .env:"; \
	  echo "  WEB_PORT (3000), API_PORT (18080), DB_PORT (55433)"; \
	  echo "and keep PUBLIC_WEB_URL / PUBLIC_API_URL in line with them."; \
	  exit 1; }
	docker compose --profile dev run --rm devseed
	@. ./.env; echo "open http://localhost:$${WEB_PORT:-3000}"

# First run: .env from the example, every "change-me" replaced with a fresh
# random value, so `make dev` works without editing anything by hand.
.env:
	@while IFS= read -r line; do \
	  case "$$line" in \
	    *=change-me) printf '%s=%s\n' "$${line%=change-me}" "$$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" ;; \
	    *) printf '%s\n' "$$line" ;; \
	  esac; \
	done < .env.example > .env
	@echo "created .env with random secrets"

dev-down: ## stop the local stack (data is kept)
	@if [ -f .env ]; then docker compose down; else echo "no .env: nothing was started"; fi

hooks: ## optional pre-commit hook: secrets and personal data before every commit
	git config core.hooksPath .githooks
	@echo "pre-commit hook enabled (.githooks/pre-commit)"

migration: ## new migration stub: make migration name=add_something
	@test -n "$(name)" || (echo "usage: make migration name=foo" >&2; exit 1)
	@slug=$$(printf '%s' "$(name)" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/_/g; s/^_+|_+$$//g'); \
	file="apps/api/sql/$$(date -u +%Y%m%d_%H%M%S)_$${slug}.sql"; \
	printf -- "-- %s\n\n" "$(name)" > "$$file"; echo "created $$file"

migrate: ## apply new migrations to the database in DATABASE_URL
	$(GO) run ./tools/migrate.go -dir apps/api/sql
