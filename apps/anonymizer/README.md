# Mindstrata anonymizer

Finds personal data in Russian and English text. Two modes, one code base:

| Mode | Process | Used by | What happens to the data |
|---|---|---|---|
| Reversible | HTTP service `python -m anonymizer.server` (port 8090, internal network only) | Go API, package `apps/api/internal/pseudonym`, before a text goes to an external LLM | Replaced with aliases (`ЛИЦО_1`, `PERSON_1`); the Go API keeps the originals, encrypted, and restores them in the answer |
| Irreversible | Temporal worker `python -m anonymizer.worker` (task queue `anonymizer-python`) | Monthly workflow in `apps/api/internal/anonymizer` | Old messages are rewritten with fixed tokens (`[ИМЯ]`, `[ТЕЛЕФОН]`); nothing is kept |

The same worker also runs the offsite DB backup activity (`anonymizer/backup.py`).

## Why Python here

The project is Go-first. This service is the exception: Natasha (slovnet NER)
and pymorphy3 have no Go equivalent of the same quality for Russian names in
oblique cases and diminutives, and Russian declension on restore needs the
same morphology. Everything stateful (vault, keys, retention) is in Go.

## How detection works

`detector.detect(text, lang, known)` runs every detector over the original
text, merges overlapping spans and hands them to the guard.

1. **Format rules** (`rules.py`): phones (RU, international, NANP), e-mails
   (also glued to Cyrillic), social and private-storage links, any link with a
   path on a non-reference domain, `@handles` and handles after "ник", "в тг";
   documents with checksums or cue words: SNILS, INN, passport, OMS policy,
   driver licence, SSN, cards (Luhn), bank accounts, IBAN.
2. **Spoken identifiers** (`spoken.py`): numbers written out in words (RU/EN),
   dictated e-mails ("иван точка петров собака мейл точка ру").
3. **People, Russian** (`people.py`): name gazetteer with diminutives
   (`names_ru.py`), pymorphy3 Name/Surn/Patr readings, relation cues ("мама",
   "муж", "начальница" + name, even lowercase), Natasha PER, unknown
   surname-shaped words next to names, initials. Canonical form is the
   nominative with gender kept ("с Петровой" -> "Петрова").
4. **People, Latin script** (`people_en.py`): English first names and Latin
   spellings of Russian names, titles ("Dr. Klein"), relation cues.
5. **Places, organisations, dates** (`places.py`): street addresses (RU/EN),
   institutions with numbers or names ("школа № 57", "ПНД" alone stays),
   workplaces and places of study after "работаю в", abbreviations (МГУ, УрФУ;
   diagnoses like ОКР, СДВГ stay), Natasha ORG/LOC, pymorphy geography, exact
   dates and any date of birth.
6. **The user's dictionary** (`known.py`): profile values and vault originals
   sent by the Go API, matched in every case form, lowercase, uppercase and
   Latin/Cyrillic spelling. This is what guarantees the user's own name,
   e-mail, phone and Telegram username are always caught.
7. **Guard** (`guard.py`): a blunt second look (long digit runs, anything
   with `@`, strict rules, known values) adds whatever is still uncovered,
   then scans the masked text. A finding there is a `residual`: the Go API
   refuses to send the message (fail closed). The guard reports counts and
   kinds only, never values.

## Policy

`anonymizer/policy.json` (or `ANONYMIZER_POLICY_FILE`). Default is strict:
every kind is masked. Kept on purpose: countries, years, months, doses,
diagnoses, generic institution types ("поликлиника", "ПНД") and holidays
("8 марта"). Rationale for dates: exact dates of events and any birth date are
masked; a year or month alone stays, the same trade-off as the HIPAA Safe
Harbor rule. Every kind must be listed; an unknown key fails loading.

## Restoration

`restore.restore(text, entries, context)` replaces aliases with stored values
and puts people and places into the case the model's sentence needs:
preposition, apposition ("с подругой ЛИЦО_2"), verbs with one government,
coordination ("с ЛИЦО_1 и ЛИЦО_3"), an ending the model glued on ("ЛИЦО_1у").
Unknown aliases (invented by the model) are marked `⟨ЛИЦО_9⟩` and counted,
never filled. Streaming is handled in Go (`pseudonym.StreamRestorer`), which
holds back a possible partial alias and passes the preceding text as context.

## Quality

Synthetic gold set (`tests/gold`, generated, every text invented):
RU 370 texts, EN 125 texts, including tricky cases (lowercase names,
diminutives, oblique cases, dictation, Latin names in Russian text) and
texts without personal data.

| Kind | RU | EN |
|---|---|---|
| PERSON | 221/221 | 57/57 |
| PHONE | 35/35 | 11/11 |
| EMAIL | 18/18 | 6/6 |
| ADDRESS | 18/18 | 6/6 |
| DOCUMENT | 17/17 | 5/5 |
| CARD | 6/6 | 1/1 |
| HANDLE | 7/7 | 5/5 |
| URL | 16/16 | 5/5 |
| ORG | 55/55 | 16/16 |
| PLACE | 28/28 | 23/23 |
| DATE | 38/38 | 11/11 |
| False positives on texts without personal data | 0 | 0 |

The gold set was written together with the detectors, so 100 % on it is a
regression floor, not a promise for arbitrary text. The honest number comes
from real conversations (below).

Real conversations (staging copy of prod, 30.09.2026, read-only, aggregates
only; oracle = the user's own profile values found in the text):

| Set | Messages | Profile values found | Missed without profile | Missed with profile |
|---|---|---|---|---|
| Recent, not yet anonymized (all roles, 10 users) | 740 | names 124, nick words 22 | names 0, nick words 2 | 0 |
| Sample of messages the old monthly pass already processed (271 users) | 6000 | Telegram usernames 13 | 0 | 0 |

"Nick words" are display-name parts that are not names (nicknames, ordinary
words); without the profile nothing can tell them from ordinary text, which
is exactly why the Go API always sends the profile. Strict independent
patterns (phones, e-mails, Luhn cards) found 0 leftovers in all runs.

Known limits:

- Rare identifying details ("единственный детский психиатр в посёлке") are
  not detected; nothing reads meaning.
- Lowercase surnames without a cue ("петрова сказала") are caught only if the
  value is in the user's dictionary.
- Places are detected by Natasha, pymorphy geography and a short list of big
  foreign cities; small foreign towns without a preposition are missed.
- Masking removes about 2 % of characters in real conversations.

## Performance

Measured in a container limited to 2 CPUs / 4 GB (the smallest VPS we target):

- p50 92 ms, p95 398 ms per message (real messages, ~1 000 characters on
  average, both roles); very short messages take under 1 ms.
- Natasha is ~75 % of the time; the model loads in ~0.5 s at start.
- Peak RSS 227 MB.

## Running

    docker compose up -d --build        # worker + HTTP service (see docker-compose.yml)
    python -m pytest                    # tests, 4 processes (pytest.ini)
    python -m tests.gold.generate       # regenerate the gold files after changing templates
    python -m tests.gold.score          # recall/false positives table with the misses listed

Quality run on real data (aggregates only, the tool cannot print text):

    psql ... -c "\copy (select user_id, id, content, display_name, email, phone,
      telegram_username from ...) to stdout with (format csv)" \
      | docker run -i --rm --cpus=2 --memory=4g IMAGE python -m anonymizer.evaluate

Use the staging database read-only (`set default_transaction_read_only = on`)
and never pipe the export anywhere but into the evaluator.

## Environment

| Variable | Where | Meaning |
|---|---|---|
| `ANONYMIZER_PORT` | HTTP service | listen port, default 8090 |
| `ANONYMIZER_NO_NER` | both | `1` disables Natasha (tests of single layers) |
| `ANONYMIZER_POLICY_FILE` | both | custom policy JSON |
| `ANONYMIZER_MAX_BODY` | HTTP service | request size limit, default 512 KB |
| `TEMPORAL_ADDRESS`, `TEMPORAL_NAMESPACE`, `TASK_QUEUE` | worker | Temporal connection |
| `BACKUP_*` | worker | offsite backup, see `.env.example` |

Go side (API env): `PII_MASTER_KEY` (base64 of 32 random bytes; without it
the reversible mode is off), `ANONYMIZER_URL` (default
`http://anonymizer-http:8090`), `PII_VAULT_TTL_DAYS` (default 180).
