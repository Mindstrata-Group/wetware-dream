# Add a field to the admin panel

Example: a new setting of a chat mode. It touches the database, the API and
the UI, so it is the largest recipe — still one pull request.

## Files

1. `apps/api/sql/<timestamp>_add_<field>.sql` (`make migration name=add_<field>`)
   — `alter table ... add column ...` with a safe default.
2. `apps/api/internal/testsupport/schema_base.sql` — the same column, so tests
   see it.
3. API handler, e.g. `apps/api/internal/httpapi/admin_mode_detail_handlers.go`
   — read and write the field; validate input.
4. `docs/openapi/openapi.json` — the field in the schema; then
   `npm run openapi:generate` in `apps/web`.
5. UI, e.g. `apps/web/src/app/admin/components/_modes/…` — the input, with a
   short label in plain words.
6. Tests: an integration test (admin saves → reads back; a non-admin gets
   403), and a vitest test that the input sends the value.

## Prompt for an agent

> Read AGENTS.md. Add a field <field> (<type>, default <value>) to <entity>
> editable in the admin panel. Steps: migration via `make migration`, the
> same column in schema_base.sql, API read/write with validation in <handler>,
> openapi.json + `npm run openapi:generate`, the input in <component> with the
> label "<label>". Tests: integration (save and read back; non-admin → 403)
> and vitest for the input. Do not edit existing migrations. Run `make check`,
> paste the output, list what you did not verify.

---

## По-русски

Новое поле затрагивает базу, API и интерфейс: миграция (новым файлом),
та же колонка в `schema_base.sql`, чтение/запись с проверкой в обработчике
API, `openapi.json` и генерация типов, поле ввода с понятной подписью, тесты —
интеграционный (сохранить и прочитать; не админ → 403) и vitest. Промпт для
агента — выше.
