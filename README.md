# Wetware Dream

*More human than human. And now for something completely different.*

[Русская версия](README.ru.md)

Wetware Dream is the open-source core of **Mindstrata** (Стратум): a web
service for guided conversations with an AI assistant. Psychology-informed
chat "modes" written by experts, access plans and promo codes, payments,
notifications, and an admin panel to run it all.

- `apps/api` — Go API (PostgreSQL, auth, chat, AI gateways, billing, admin).
- `apps/web` — Next.js frontend (product UI in Russian and English).
- `apps/mcp-admin` — MCP server that lets AI agents run admin commands.
- `deploy/` — production template: one server, automatic HTTPS.
- `runner/` — optional test runner for contributors' forks.
- `tools/` — repository gates and helpers, written in Go.

## Run it locally in one command

You need Docker and `make` (nothing else: no Go, no Node). Then:

```bash
make dev
```

It creates `.env` with random secrets on the first run, starts the stack,
adds synthetic clients and chats, and prints a development-only login
(`admin@example.com`). Open http://localhost:3000. `make dev-down` stops it,
data is kept. The first build takes a few minutes.

Ports 3000, 18080 and 55433 busy? Set `WEB_PORT`, `API_PORT`, `DB_PORT` in
`.env` (and `PUBLIC_WEB_URL` / `PUBLIC_API_URL` to match).

Without `make`, or without test data:

```bash
cp .env.example .env        # change the three "change-me" values
docker compose up -d --build
```

This exact path is checked on every change by the `Fresh clone: make dev`
CI job on a clean machine.

## Become an administrator

The first administrator is created once, with the `BOOTSTRAP_ADMIN_TOKEN`
from `.env`:

```bash
curl -X POST http://localhost:18080/api/bootstrap/admin \
  -H "X-Bootstrap-Token: <BOOTSTRAP_ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"a-long-password"}'
```

After that the endpoint refuses: there is already an admin. Sign in on the
site with this e-mail and password.

## Connect an AI provider

AI keys are **not** environment variables. In the admin panel open
**Orchestration → AI** and add a gateway: an OpenAI-compatible endpoint
(OpenAI, OpenRouter and similar), Anthropic, or Google Gemini — its base URL
and key. Keys are stored in the database, so you can rotate one without a
redeploy. Then pick the gateway for each chat mode.

## Put it on a server

`deploy/` holds a production template with Caddy and automatic HTTPS, and
[docs/deploy.md](docs/deploy.md) walks through a first start on a VPS,
migrations, backups, updates and rollback.

## Contributing

Start with [CONTRIBUTING.md](CONTRIBUTING.md). In short: fork → optionally
[a runner on your computer](runner/README.md) → `make check` green → pull
request from the template. Working with an AI agent is welcome: it reads
[AGENTS.md](AGENTS.md), and [AI_POLICY.md](AI_POLICY.md) explains what we
expect from AI-assisted pull requests. Not a programmer? See
[docs/overview.md](docs/overview.md) and the [recipes](docs/recipes/).

## License

Mindstrata is dual-licensed:

- **[GNU AGPL-3.0](LICENSE)** — free for everyone. If you run a modified
  version as a service for others, you must publish your source code under
  the same license.
- **Commercial license** — for a closed product or service without
  publishing your changes. See [COMMERCIAL.md](COMMERCIAL.md).

Contributions are accepted under the [CLA](CLA.md), which keeps the dual
licensing possible. Security issues: [SECURITY.md](SECURITY.md).
