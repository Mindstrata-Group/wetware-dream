# Rules for contributors and their AI agents

[По-русски](AGENTS.ru.md). These rules apply to people and to coding agents
alike. An agent reads this file first; a person reads it once.

## Workflow

- One task — one pull request — one fresh agent session. Small pull requests.
- Do not say "done" until `make check` is green. Paste its output into the
  pull request, and always add a section **"Not verified"**: what you did not
  run or could not check.
- Every feature or fix comes with a test. A bug fix starts with a test that
  fails before the fix.
- Tests run in parallel by default: `t.Parallel()` in Go wherever state is
  isolated. If a test cannot be parallel, say why in a comment next to it.
- Any new endpoint that returns or changes a user's object gets a test
  "another user → 403/404".
- Follow the patterns already in the code. No new framework, library or
  service without a concrete reason written in the pull request.
- New tests and repository tools are written in **Go**. The frontend is
  tested with vitest because it is TypeScript. Do not add bash/python/node
  scripts without a reason; bash is fine only as a thin wrapper (installers,
  calling Docker).

## Code

- **Code comments are in English.** Identifiers are English. Product UI text
  stays in the product's languages (Russian and English): it is not a comment.
  `make comments` checks this. (Older code still has Russian comments and is
  being translated; do not add new ones.)
- Auth is strict: an auth cookie that is invalid, expired, revoked or points
  to a deleted user gives `401` and clears the cookie. Guest access is allowed
  only when there is no auth cookie at all.
- Migrations: only new files in `apps/api/sql/`, never edit a published one;
  update `apps/api/internal/testsupport/schema_base.sql` in the same pull
  request.
- UI text: short, plain words, about what changes for the person. No
  technical or process terms in user-facing copy.
- Psychometrics (scoring, norms, reverse-keyed items) changes only together
  with a reference test: the source (paper or manual, with page) and the
  expected numbers from it. See docs/verify-analysis.md.

## Data and secrets

- No real people's data anywhere: not in prompts to an AI, not in fixtures,
  not in issues or screenshots. Use synthetic data (`make dev`, `testdata/`).
  `make guard` flags real-looking phones, e-mails, SNILS and passports.
- Agents must not read or print `.env`, `deploy/.env.prod`, `runner/.env` or
  any other secrets file.
- Never commit secrets, generated artifacts (`node_modules`, `.next`,
  coverage, test results), database dumps or exports.

## Boundaries for agents

- Do not touch deployment, production migrations or CI workflows unless the
  task says so explicitly.
- Do not push to `main`, do not force-push, do not merge.
- If the task is unclear, stop and ask instead of guessing a large change.
