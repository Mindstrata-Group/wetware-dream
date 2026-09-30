# Contributing

[По-русски](CONTRIBUTING.ru.md)

Thank you for helping! Read [AGENTS.md](AGENTS.md) (the rules, for people and
AI agents), [docs/TESTING.md](docs/TESTING.md) (how to test) and
[AI_POLICY.md](AI_POLICY.md).

## The path of a contribution

1. **Pick or open an issue.** Say what changes for the user. Newcomers: look
   for `good first issue` and `recipe`; recipes in [docs/recipes/](docs/recipes/)
   come with the list of files and a ready prompt for an agent.
2. **Fork** the repository and create a branch.
3. **Optional: a runner on your computer**, so CI in your fork runs locally —
   three steps in [runner/README.md](runner/README.md).
4. **Make the change with a test.** A bug fix starts with a failing test.
5. **Run `make check`** until it is green (or get a green CI run in your fork).
6. **Open a pull request** from the template: what changes for the user, which
   tests, the `make check` output, whether AI was used.
7. **Sign the CLA** — the bot asks once, a one-line comment
   ([why](CLA.md): the project is dual-licensed).

## Run the tests on your computer

1. Install Go (version in `go.mod`), Node.js 22 and Docker.
2. `make web-install` once.
3. `make check` — the same checks as CI. `make help` lists smaller targets.

## Labels

- `good first issue` — small, well-described, a maintainer is ready to help.
- `help wanted` — maintainers would welcome a contribution here.
- `recipe` — a task written from a recipe template; fits one agent session.
- `no-tests-needed` — set **only by a maintainer**: waives the "code changed
  without tests" gate for a change that really needs no test.
- `over-limit` — too many open pull requests from a new contributor.

## Required checks (for maintainers)

In **Settings → Branches → main** enable "Require a pull request", "Require
approvals: 1" and these required status checks: `Repository gates`,
`Secret scan (changed commits)`, `Tests changed with code`,
`API vet + unit (race)`, `API integration (Postgres)`,
`Web typecheck + contract + vitest`, `Anonymizer (pytest)`,
`Comments in English`, `Fresh clone: make dev`, `Analyze (go)`,
`Analyze (javascript-typescript)`, `CLA`. Also enable "Require branches to be
up to date" and "Do not allow bypassing".
