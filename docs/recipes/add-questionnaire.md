# Add a questionnaire with scoring

Scoring is the part that must be exactly right, so it is built first, alone,
as a pure Go function with a **reference test**. Showing the questionnaire to
users is a separate pull request.

## Files

- `apps/api/internal/psychometrics/<name>/score.go` — items, reverse-keyed
  items, subscales, `Score(answers) (Result, error)`;
- `apps/api/internal/psychometrics/<name>/score_test.go` — the reference test;
- `apps/api/internal/psychometrics/<name>/SOURCE.md` — citation, version of
  the instrument, license of the items (many questionnaires are copyrighted:
  check before adding item texts).

## The reference test

- Take worked examples from the manual or the validation paper: answers →
  expected raw scores (and norms, if used). Write the source and page in the
  test.
- Cover: all minimum answers, all maximum answers, reverse-keyed items,
  missing answers (what does the manual say?), invalid values.
- If you compared with R or SPSS, put the script and the numbers next to the
  test. See [docs/verify-analysis.md](../verify-analysis.md).
- `t.Parallel()`, table tests.

## Prompt for an agent

> Read AGENTS.md and docs/verify-analysis.md. Create package
> apps/api/internal/psychometrics/<name> with a pure Score function for <name>
> (<n> items, answer scale <a>–<b>, reverse-keyed items: <list>, subscales:
> <list>). Write SOURCE.md with the citation <citation>. Write a table test
> with these reference cases from <source, page>: <answers → expected>. Do
> not add an HTTP endpoint. Run `make check`, paste the output, and list what
> you did not verify.

---

## По-русски

Подсчёт делается первым и отдельно: чистая функция на Go и **эталонный тест**
с примерами из руководства или статьи (источник и страница в тесте):
минимальные и максимальные ответы, обратные пункты, пропуски, неверные
значения. Если сверяли с R/SPSS — скрипт и числа рядом с тестом. Показ
опросника пользователям — отдельный PR. Проверьте лицензию на тексты пунктов.
