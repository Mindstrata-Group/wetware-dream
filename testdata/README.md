# Test data

Only synthetic data lives here and in any fixture:

- e-mails on `example.com` / `.test`;
- phones from the `+7 900 000-00-00` block;
- SNILS `000-000-000 00`, passport `0000 000000`;
- invented names and invented chat text.

`make guard` (tools/prcheck hygiene) fails on values that look real. If a
format test truly needs a realistic-looking value, mark the line with
`hygiene:allow <reason>` so the exception is visible in review.

For a local database full of synthetic clients and chats run `make dev`
(tools/devseed).

---

Здесь и в любых фикстурах — только выдуманные данные: почты на `example.com`,
телефоны `+7 900 000-00-00`, СНИЛС `000-000-000 00`, выдуманные имена и
диалоги. `make guard` падает на похожем на настоящее. Локальная база с
синтетикой — `make dev`.
