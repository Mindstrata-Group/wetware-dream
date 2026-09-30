# Add a psychological mode (prompt)

A "mode" is a conversation scenario: its system prompt, welcome message,
completion criteria and AI settings. **Modes are data, not code**: you add
them in the admin panel of your installation, and every change is versioned
(admin → mode → History).

## Steps

1. `make dev`, sign in with the development admin.
2. Admin → **Modes** → create: name, prompt, welcome message, criteria.
3. Choose the AI gateway and model; try it in the chat.
4. Share the prompt with others as a text file in an issue (not in code),
   unless the task asks to ship it as a default for new installations.

## Only if the mode must ship with the code

Then it is a migration: a new file `apps/api/sql/<timestamp>_add_mode_x.sql`
inserting the row, the same insert reflected in
`apps/api/internal/testsupport/schema_base.sql` only if it is schema (it
usually is not), and an integration test that the mode exists and is visible.

## Prompt for an agent

> Read AGENTS.md. Add a migration in apps/api/sql that inserts a mode named
> "<name>" with this prompt: <prompt>. Do not change existing migrations.
> Add an integration test in apps/api/internal/httpapi that after the
> migration the mode is returned by the modes list endpoint (see
> apps/api/internal/httpapi/chat_modes.go). Run `make check`, paste the
> output, list what you did not verify.

---

## По-русски

Режим — сценарий разговора: системный промпт, приветствие, критерии и
настройки ИИ. **Режимы — это данные, а не код**: их заводят в админке своей
установки (Режимы → создать), каждая правка сохраняется в истории. Делиться
промптом — текстом в issue. Только если режим должен ехать вместе с кодом для
всех установок — это миграция и интеграционный тест (промпт для агента выше).
