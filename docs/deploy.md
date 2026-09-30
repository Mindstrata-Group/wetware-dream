# Deploy to a server

[По-русски](deploy.ru.md)

One server, Docker, automatic HTTPS. Files: `deploy/compose.prod.yml`,
`deploy/Caddyfile.example`, `deploy/.env.prod.example`.

## Server requirements

Estimated from the resource settings in `deploy/compose.prod.yml`
(Postgres `shared_buffers=256MB`, up to 120 connections, API pool 30, worker
limited to 0.5 CPU / 128 MB) plus a Next.js server and Caddy:

| | Minimum | Recommended |
|---|---|---|
| CPU | 2 vCPU | 2–4 vCPU |
| RAM | 2 GB + 2 GB swap | 4 GB |
| Disk | 20 GB SSD | 40 GB SSD + backups elsewhere |

These numbers assume **prebuilt images** from the registry. Building the web
image needs about 3 GB of RAM more — build on your own computer or in CI, not
on a small server.

## First start in 7 steps

1. Point your domain's A (and AAAA) record at the server. Open ports 80 and 443.
2. Install Docker with the Compose plugin.
3. Get the deploy files: `git clone https://github.com/Mindstrata-Group/wetware-dream && cd mindstrata`
   and `git checkout vX.Y.Z` (the version you run).
4. `cp deploy/.env.prod.example deploy/.env.prod` and fill in every REQUIRED
   value; generate secrets with `openssl rand -hex 32`. `chmod 600 deploy/.env.prod`.
5. `cp deploy/Caddyfile.example deploy/Caddyfile`.
6. Start: `docker compose -f deploy/compose.prod.yml --env-file deploy/.env.prod up -d`.
   Postgres creates the schema on the first start. Caddy gets a certificate
   in a minute; open `https://your-domain`.
7. Create the first administrator (README → "Become an administrator", with
   `https://your-domain` instead of localhost), sign in and add an AI gateway
   in **Orchestration → AI**. AI keys live in the database, not in env.

Shortcut used below: `dc() { docker compose -f deploy/compose.prod.yml --env-file deploy/.env.prod "$@"; }`

## Migrations

New releases may add files in `apps/api/sql/`. They are **applied by hand**,
never automatically on start: a migration that fails halfway on production,
or runs before you took a backup, is much harder to undo than a minute of
manual work. Order:

1. Back up (below).
2. Read `CHANGELOG.md` for the version: it says whether there are migrations.
3. `dc --profile tools run --rm migrate` — applies only new files, in order,
   each in a transaction; already applied ones are skipped.
4. Then start the new version.

## Backup and restore

Back up every day and keep copies **off the server**:

```bash
dc exec -T postgres pg_dump -U mindstrata -Fc mindstrata > mindstrata-$(date +%F).dump
```

Restore into an empty database (stop api and web first):

```bash
dc stop api web
dc exec -T postgres dropdb -U mindstrata --if-exists mindstrata
dc exec -T postgres createdb -U mindstrata mindstrata
dc exec -T postgres pg_restore -U mindstrata -d mindstrata --no-owner < mindstrata-2026-01-01.dump
dc start api web
```

Test a restore at least once before you need it.

## Update to a new version

1. Back up.
2. `git fetch --tags && git checkout vNEW`, set `MINDSTRATA_VERSION=vNEW` in
   `deploy/.env.prod`, compare `deploy/.env.prod.example` for new variables.
3. `dc pull`.
4. Apply migrations if the changelog lists any (see above).
5. `dc up -d`.

## Roll back

Set the previous `MINDSTRATA_VERSION`, `git checkout vOLD`, `dc up -d`. If
the new version applied migrations that the old one cannot work with (the
changelog marks such releases as MAJOR), restore the backup taken before the
update.

## Background worker

Recurring billing and message anonymization run on Temporal. Without it the
site works; those jobs do not. To enable: run a Temporal server, set
`TEMPORAL_ADDRESS`, and start with `--profile worker`.

## Build images yourself

`dc build` builds from the checked-out source instead of pulling. Or build on
another machine, push to your registry and set `IMAGE_PREFIX` (images are
`<prefix>-api` and `<prefix>-web`).
