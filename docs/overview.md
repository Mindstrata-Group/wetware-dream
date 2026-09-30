# How the project works (for non-programmers)

```
 Browser ──► web (Next.js) ──► api (Go) ──► Postgres (database)
                                   │
                                   └──► AI provider (OpenAI-compatible,
                                        Anthropic, Gemini) — keys in the database
```

- **web** draws the pages: landing, chat, profile, admin panel.
- **api** does the work: sign-in, chat, limits, payments, admin actions. It
  sends the conversation to the AI provider chosen for the mode.
- **Postgres** keeps users, chats, modes, settings.
- **deploy/** puts it all on one server with HTTPS; **runner/** runs tests on
  a contributor's computer.

## Glossary

- **API** — the set of addresses the web app (or another program) calls to
  get and change data, e.g. "give me this user's chats".
- **Endpoint** — one such address, e.g. `POST /api/chat/send`.
- **Migration** — a small file that changes the database structure (adds a
  table or a column). They are applied in order and never edited afterwards.
- **Fixture** — prepared data for a test. Ours are always invented.
- **Test** — code that checks other code: "with these answers the score is 23".
  A red test means something broke.
- **CI** — the robot that runs all tests for every change. Red CI = the change
  is not ready.
- **Pull request (PR)** — a proposal to change the project; reviewed before
  it is accepted.
- **Mode** — a conversation scenario: system prompt, welcome message,
  criteria, AI settings. Edited in the admin panel.
- **Agent** — an AI tool that writes code for you (Claude Code, Cursor, …).
  It follows AGENTS.md.

---

## Как устроен проект (для непрограммистов)

Браузер → **web** (страницы) → **api** (вся работа: вход, чат, лимиты, оплата,
админка) → **Postgres** (база). Для ответа api отправляет разговор провайдеру
ИИ, выбранному для режима; ключи хранятся в базе.

**Словарь.** *API* — адреса, по которым программа получает и меняет данные.
*Эндпоинт* — один такой адрес. *Миграция* — файл, меняющий структуру базы;
применяются по порядку, после публикации не правятся. *Фикстура* — заготовка
данных для теста, у нас всегда выдуманная. *Тест* — код, проверяющий код.
*CI* — робот, который гоняет тесты на каждое изменение. *PR* — предложение
изменения. *Режим* — сценарий разговора. *Агент* — ИИ-инструмент, пишущий код.
