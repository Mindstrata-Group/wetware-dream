# Change interface text

Two kinds of text:

- **Editable in the admin panel** (landing blocks, access page, legal texts):
  admin → **Content**. No code change; defaults for a fresh install live next
  to the page (`getString(content, 'key', 'default')`).
- **In code**: dictionaries `apps/web/src/app/**/translations.ts` (Russian and
  English) and texts inside components.

## Files

- `apps/web/src/app/<page>/translations.ts` or the component itself;
- the page test `apps/web/src/app/<page>/*.test.tsx` if it asserts the text.

## Rules

Short, plain words, about what changes for the person. No technical terms.
Change both languages. Legal pages take operator details from
`apps/web/src/lib/operator.ts`.

## Prompt for an agent

> Read AGENTS.md. On the page <path> replace the text "<old>" with "<new>" in
> Russian and "<old en>" with "<new en>" in English. Find where the text
> lives (translations.ts or the component). Update the tests that assert it;
> if none does, add a vitest assertion that the new text is rendered. Run
> `make web-check`, then `make check`, paste the output and list what you did
> not verify.

---

## По-русски

Тексты бывают двух видов: редактируемые в админке (раздел «Контент») — правка
без кода; и в коде — словари `translations.ts` и компоненты. Пишем коротко,
простыми словами, меняем оба языка, обновляем или добавляем тест, который
проверяет текст. Промпт для агента — выше.
