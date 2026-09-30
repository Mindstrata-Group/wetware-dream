# AI policy

**AI tools are welcome.** Many of our contributors are psychologists and
researchers who build with coding agents. What matters is the result and who
stands behind it.

1. **A person is responsible for every pull request.** You must understand
   each change and be able to explain it in review. "The agent wrote it" is
   not an answer to a review question.
2. **Say which tool you used** in the pull request template (tool and model).
   This is not a penalty; it helps reviewers know what to look at.
3. **Start from an issue.** A pull request without a linked issue, or without
   tests, is closed with a template reply (below). Open or pick an issue
   first; for newcomers the `recipe` and `good first issue` labels are the
   best start.
4. **No real data in prompts.** Never paste real clients' chats, personal data
   or production exports into an AI tool. See docs/privacy.md.
5. **Green `make check` before review**, with the output in the pull request.

## Limits for new contributors

Until your first pull request is merged, keep **at most two** pull requests
open. Maintainers apply this by hand: extra pull requests get the
`over-limit` label and a template reply, and are closed after 7 days if
still over the limit. After the first merge the limit is five.

## Template replies (for maintainers)

> Thanks for the contribution! We close pull requests that have no linked
> issue or no tests: see AI_POLICY.md. Please open an issue describing what
> changes for the user, add a test, and reopen.

> Thanks! You already have the maximum number of open pull requests for a new
> contributor (AI_POLICY.md). Let's finish the open ones first.

---

## По-русски

ИИ разрешён. За каждый PR отвечает человек: он понимает каждое изменение и
объясняет его на ревью. В шаблоне PR указываете инструмент и модель. PR без
задачи или без тестов закрываются шаблонным ответом. Реальные переписки и
персональные данные в ИИ не отправляем. До первого принятого PR — не больше
двух открытых PR одновременно (сопровождающие ставят метку `over-limit` и
закрывают лишние через 7 дней), после — не больше пяти.
