# Migrations

A fresh database is created from the full schema in
`deploy/initdb/01-schema.sql` (Postgres applies it on the first start).
`deploy/initdb/02-migrations.sql` marks the migrations already included in
that schema as applied.

Only new schema changes go here:

- one file per change, named `YYYYMMDD_HHMMSS_what_changes.sql` (UTC);
- create a stub with `make migration name=add_something`;
- never edit a published file — add a new one on top;
- update `apps/api/internal/testsupport/schema_base.sql` in the same pull
  request: tests build their database from it;
- apply to your database with `make migrate` (see `docs/deploy.md` for why
  production migrations are applied by hand).

---

**По-русски.** Новая база создаётся из полной схемы `deploy/initdb`. Сюда
кладутся только новые изменения: один файл с отметкой времени на изменение,
опубликованные файлы не правим, `schema_base.sql` обновляем в том же PR.
