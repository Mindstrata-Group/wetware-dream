# Verifying scoring and analysis code

Psychometric code can be wrong while every test is green — if the tests were
written from the same misunderstanding as the code. A **reference** breaks
that loop: numbers that come from outside the code.

## Reference test

1. **Source.** The instrument's manual or validation paper: title, edition,
   page or table. Put it in the test comment and in `SOURCE.md`.
2. **Worked examples.** Answers and the expected scores exactly as the source
   gives them. If the source has none, compute by hand once and have a second
   person check.
3. **Edge cases.** All minimum, all maximum, every reverse-keyed item flipped,
   missing answers (the manual's rule, e.g. "prorate if at most 1 missing"),
   out-of-range values.
4. **Norms.** If raw scores are converted (T-scores, percentiles), test the
   conversion table at its boundaries.

## Comparing with R or SPSS

- Keep the script next to the test (`testdata/<name>/reference.R`), with the
  input as CSV of synthetic answers and the output numbers.
- Record versions: R and package versions (`sessionInfo()`), or the SPSS
  version and syntax.
- Anything random (bootstrap, simulation) uses a fixed **seed**, written in
  both the script and the test.
- Compare with an explicit tolerance and say why it is that large
  (e.g. `1e-9` for sums, `1e-6` for iterative estimates).

## In the pull request

Link the source, paste the comparison table (ours vs reference), and state
what was **not** verified (for example, norms for a different population).

---

## Проверка подсчёта и анализа

Психометрический код может ошибаться при зелёных тестах, если тесты написаны
из того же непонимания. Спасает **эталон** — числа не из нашего кода: примеры
из руководства или статьи (источник и страница в тесте), крайние случаи,
обратные пункты, пропуски, нормы на границах таблиц. Сверка с R/SPSS — скрипт
рядом с тестом, версии пакетов, фиксированный seed, явный допуск. В PR —
ссылка на источник, таблица «у нас / эталон» и что не проверено.
