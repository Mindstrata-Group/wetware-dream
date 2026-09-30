# Privacy: what never goes into an AI tool or the repository

This is practical guidance for contributors, not legal advice. Operators of an
installation are responsible for their own compliance (Russian 152-FZ, GDPR).

## Never send to an AI tool, an issue or a fixture

- chats and messages of real users, even "just one example";
- names, phones, e-mails, addresses, birth dates, SNILS, passport data;
- anything about health, emotional state or psychological test results of a
  real person — this is a **special category** of personal data under both
  152-FZ and GDPR;
- production database dumps or exports, logs with request bodies;
- `.env` files and any keys.

## Why "I anonymized it" is not enough

- Removing the name leaves the story: a unique life situation plus a city and
  an age identifies a person.
- Free text carries identifiers in unexpected places: a nickname, a
  colleague's name, a workplace.
- Once pasted into a third-party AI service, the data is processed abroad and
  possibly stored; you cannot take it back, and cross-border transfer of
  personal data has its own rules.
- In a public repository, git history keeps everything forever.

## What to do instead

- Use synthetic data: `make dev` generates clients and chats; write fixtures
  by hand following `testdata/README.md`.
- To reproduce a bug from production, describe the **shape** of the data
  ("a message of 5000 characters with an emoji at the end"), not the content.
- The gates (`make guard`, the pre-commit hook from `make hooks`) catch
  obvious slips, but they do not replace judgment.

---

## Конфиденциальность

Практические правила, не юридическая консультация. В ИИ, issue и фикстуры
**никогда** не отправляем: переписки реальных пользователей, ФИО, телефоны,
почты, адреса, СНИЛС, паспорта, данные о здоровье и психологическом состоянии
(это **специальная категория** ПДн по 152-ФЗ и GDPR), дампы и выгрузки базы,
`.env`. Обезличивания мало: уникальная история плюс город и возраст узнают
человека; в свободном тексте прячутся имена коллег и места работы; отправленное
в сторонний ИИ обрабатывается за рубежом и не возвращается; а git помнит всё.
Вместо этого — синтетика (`make dev`, `testdata/`) и описание формы данных, а
не их содержания.
