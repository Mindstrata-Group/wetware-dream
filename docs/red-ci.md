# CI is red: what now

A red check is not a verdict on you. It's just a flesh wound: read the first
error, fix one thing, push again.

1. Open the pull request → **Checks** → the red job → the red step.
2. Scroll to the **first** error, not the last line. Usually it is a line with
   `FAIL`, `Error:` or `prcheck:`.
3. Find the job in this table:

| Job | Typical cause | What to do |
|---|---|---|
| Repository gates | gofmt; a real-looking phone/e-mail in a fixture; an unpinned action | run `make fmt` or `make guard` locally; the message names the file and line |
| Secret scan | a key or token in a commit | **rotate the key** (it is already public), then remove it from the branch |
| Tests changed with code | logic changed without a test | add a test; or explain in the PR why none is needed |
| API vet + unit | compile error, failing unit test, data race | `make api-vet api-unit` |
| API integration | failing API test against Postgres | `make api-integration`; look for the test name after `--- FAIL` |
| Web | TypeScript error, outdated API types, failing vitest | `make web-check`; outdated types: `npm run openapi:generate` in apps/web |
| Comments in English | a comment with Cyrillic | translate the comment; UI text in strings is fine |

## What to show your AI agent

Give it **the failing step's output from the first error down** (not a
screenshot, not the whole log) and one sentence of context:

> CI job "<name>" fails on my branch. Here is the output from the first error:
> <paste>. Find the cause, fix it with a test if it is a bug, run the same
> make target locally until it is green, and tell me what you changed and
> what you did not verify.

Do not paste `.env` values or real data into the conversation.

---

## CI красный: что делать

Откройте PR → **Checks** → красная джоба → красный шаг, найдите **первую**
ошибку. Сверьтесь с таблицей выше (там же — какая `make`-цель повторяет шаг
локально). Агенту отдайте вывод шага от первой ошибки и одно предложение
контекста; секреты и реальные данные в разговор не вставляйте. Если сработал
поиск секретов — ключ уже засвечен: сначала замените его, потом чистите ветку.
