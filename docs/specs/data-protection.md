# Data protection: jurisdiction profiles, consent, subject rights

**Status: partially implemented.** Each section below carries its own status.
Legal background, sources and the date they were checked:
[`docs/legal/data-protection-reference.en.md`](../legal/data-protection-reference.en.md)
(Russian primary version: `data-protection-reference.ru.md`). This spec does
not restate the law; where it cites a number or a deadline, it is taken from
that reference.

> The code gives the operator tools. Legal compliance stays with the operator
> who deploys Mindstrata: notifications to regulators, contracts with
> processors, threat model, hosting location, policies.

## How to find what is not done yet

Every acceptance criterion (AC-N) has a Go test. Criteria that are specified
but not implemented are skipped tests with a fixed prefix:

```
grep -rn 'SPEC data-protection' apps/api/internal/httpapi
```

CI prints them in the "Spec backlog" step. Implementing a criterion means
removing its `t.Skip` and making the test pass.

## Configuration

| Variable | Values | Default | Meaning |
|---|---|---|---|
| `DATA_PROTECTION_PROFILE` | `none`, `ru`, `eu`, `us` | `none` | Legal regime of the installation. Unknown values fall back to `none`. Set in the environment on purpose: an admin click must not switch off a legal safeguard. |
| `DATA_STORAGE_COUNTRY` | ISO 3166-1 alpha-2 | empty | Where the main database is hosted, as declared by the operator. Used only for warnings: the service cannot measure it. |

Migration: `apps/api/sql/20260930_120000_data_protection.sql` (manual apply on
stage/prod, as for every migration). Until it is applied, profile `none` keeps
working unchanged; the privacy endpoints answer 503; profile `ru` fails closed
(foreign gateways are blocked, because consent cannot be read).

## A. Jurisdiction profile and cross-border transfer — implemented

Why: in the RU profile, chat content leaving Russia is a cross-border transfer
of what is conservatively treated as health data (152-FZ art. 10, 12; see
reference §1.3, §1.5). It needs a separate consent.

Rules:
- Each AI gateway has `processing_country` (ISO code; empty = unknown). A
  self-hosted model gets the country of the installation.
- Profile `ru`: a gateway is usable if its country is `RU`, or if the data
  subject holds an active `cross_border_transfer` consent. Unknown country
  counts as foreign.
- The data subject is carried in the request context (`withDataSubject`). Chat
  send, the answer, orchestration, dialog summaries and lead summaries carry it.
  A call without a subject is blocked from foreign gateways (fail closed).
- Profiles `eu` and `us`: no transfer block in code. GDPR transfers rely on
  SCC/DPF and HIPAA on BAAs, which are operator paperwork.
- Consent is read on every call without caching, so a withdrawal applies to
  the very next message.
- When the policy removes every gateway, the call returns
  `errCrossBorderConsentRequired`; the chat answers `403` with code
  `cross_border_consent_required` and a human message, gives back the quota
  slot and never contacts a provider.

| AC | Criterion | Test |
|---|---|---|
| AC-1 | Profile parsing is strict; `none` keeps today's behaviour | `TestDataProtectionProfile_Normalize`, `TestDataProtection_ProfileNone_NoChange` |
| AC-2 | RU: foreign gateway blocked without consent; no subject → blocked | `TestCrossBorderPermitted_DecisionTable`, `TestDataProtection_RUProfile_NoSubjectFailsClosed` |
| AC-3 | RU: gateway in `RU` needs no consent | `TestDataProtection_RUProfile_DomesticGatewayAllowed` |
| AC-4 | Subject travels in context; zero id is not a subject | `TestDataSubjectContext_RoundTrip` |
| AC-5 | Chat returns 403 `cross_border_consent_required`, no provider call, no quota spent | `TestCrossBorderError_IsDistinguishable`, `TestDataProtection_RUProfile_ChatSendReturnsConsentRequired` |
| AC-6 | Withdrawal blocks the next call immediately | `TestDataProtection_RUProfile_ConsentGatesForeignGateway` |

Known limits: Telegram/Max forwarding of user messages to the operator's
chats is a separate transfer path and is not covered here (see the messenger
channels spec). Reversible masking (section H) reduces risk but does not
replace consent (reference §1.9, §2.8).

## B. Consent journal — implemented (API); UI — spec only

Documents: `personal_data`, `health_data`, `cross_border_transfer`,
`terms_of_service`. `health_data` is separate on purpose: special-category
data need their own written consent (152-FZ art. 10(2)(1), GDPR art. 9(2)(a)).

API (logged-in user or guest with a valid guest cookie; an invalid session
cookie gives 401 and clears the cookie):
- `GET /api/privacy/consents` — own records;
- `POST /api/privacy/consents` `{document, version, textSha256}` — a new row
  per act of consent, never overwriting;
- `POST /api/privacy/consents/withdraw` `{document}` — sets `withdrawn_at`.

Stored: document, version, SHA-256 of the displayed text, time granted,
time withdrawn, salted IP hash. No user agent, no raw IP.

| AC | Criterion | Test |
|---|---|---|
| AC-7 | Payload validation; anonymous → 401; invalid cookie → 401 + cleared | `TestValidateConsentPayload`, `TestDataProtection_ConsentJournal_Rejections` |
| AC-8 | Own records only; version and hash kept; withdrawal recorded; no raw IP | `TestDataProtection_ConsentJournal_GrantListWithdraw` |
| AC-19 | UI: consent screens (incl. cross-border and health data) send the version and hash of the exact text; server verifies the hash against the canonical text | spec only |
| AC-25 | In psychological modes under profiles `ru`/`eu`, the chat requires an active `health_data` consent | spec only |

## C. Data subject rights — implemented

- `GET /api/profile/export` — one JSON file (`mindstrata.user-export.v1`):
  profile, all dialogs with messages (including ones the user deleted), consent
  journal, cookie consents, notification contacts, subscriptions, invoices,
  uploaded file metadata. Payment method tokens are excluded. The target is
  always the session owner; any `userId` parameter is ignored.
- `DELETE /api/profile` now also erases: message texts of all own dialogs
  (rows stay, because usage and billing reference them), orchestration logs,
  uploaded files and their blobs (unless shared by another owner),
  notification contacts, channel consents, reachability, inbox, messenger ids.
  Consent records stay as proof but are withdrawn. Billing records stay for the
  accounting retention period.
- Backups: erased data disappear from backups when those expire. The shipped
  script keeps 14 days (`scripts/backup-db.sh`, `KEEP_DAYS=14`); the privacy
  policy must state this period.

| AC | Criterion | Test |
|---|---|---|
| AC-9 | Export contains own data and nothing of another user; offered as a file | `TestDataProtection_Export_OnlyOwnData` |
| AC-10 | Deletion erases own message texts, files, contacts; withdraws consents | `TestDataProtection_DeleteAccount_ErasesOwnContentOnly` |
| AC-11 | Deletion cannot target another account | same test |
| AC-12 | Deletion record (RKN Order No. 179) and backup expiry tracked per request | spec only |

## D. Retention — spec only

Configurable retention per data type (messages, files, logs, consent records,
audit log) with automatic deletion or anonymisation. Built on the existing
Temporal scheduler and the anonymizer (`apps/api/internal/anonymizer`, monthly,
`ANON_MESSAGE_AGE_DAYS`, default 30), not a new tool.

| AC | Criterion | Test |
|---|---|---|
| AC-18 | Per-type retention settings; scheduled job deletes/anonymises past the limit; each run is logged | spec only |

## E. Encryption

In transit: TLS at the reverse proxy (Caddy) — exists.

At rest — operator guide (no code):
1. Encrypt the database volume (LUKS/dm-crypt or the hosting provider's
   encrypted disks).
2. Encrypt backups before they leave the server (`age` or `gpg` with a key
   kept off the server); test a restore at least monthly.
3. Keep `.env` files readable by the service user only (`chmod 600`).

| AC | Criterion | Test |
|---|---|---|
| AC-21 | Field-level encryption for the most sensitive columns (message text, attachments) with key rotation | spec only |

## F. Admin access log — implemented (API); screen — spec only

Opening a dialog (`admin.dialog.read`) or a user card (`admin.user.read`)
writes an audit record with the subject id, never the text. Existing tester
and export reads are included. `GET /api/admin/data-protection/access-log`
(filters `subjectUserId`, `actorUserId`, `limit`) is available to owner/admin
only: the log itself reveals whose data was looked at.

| AC | Criterion | Test |
|---|---|---|
| AC-13 | Dialog and user-card reads are logged with subject id and without text | `TestDataProtection_AdminAccessLog` |
| AC-14 | Only owner/admin read the access log | same test |

## Admin overview — implemented

`GET /api/admin/data-protection`: profile, declared storage country, warnings,
gateway countries. `POST /api/admin/data-protection/gateway-country`
`{gatewayId, country}` sets a gateway's country (audited).

| AC | Criterion | Test |
|---|---|---|
| AC-15 | Overview shows the profile; plain users get 403 | `TestDataProtection_AdminGatewayCountry` |
| AC-16 | Country is validated, stored, audited; unknown gateway → 404 | same test |
| AC-17 | Country codes: two Latin letters or empty | `TestNormalizeProcessingCountry` |
| AC-24 | RU profile warns when storage country is not declared or not `RU` (152-FZ art. 18(5), localisation since 01.07.2025) | `TestDataProtectionWarnings_StorageCountry`, `TestDataProtection_AdminGatewayCountry` |

## G. Breach notification — spec only (procedure)

Deadlines from the reference: Russia — RKN within 24 hours (initial) and
72 hours (investigation results); EU — supervisory authority within 72 hours,
data subjects when the risk is high (typical for health data); US — FTC Health
Breach Notification Rule and HIPAA, 60 days.

Procedure for the operator: contain → record what, when, whose data → notify
within the deadlines above → inform affected users in plain language → post-
mortem. Templates for each notice live next to this spec when implemented.

| AC | Criterion | Test |
|---|---|---|
| AC-22 | Breach register in the admin panel with deadline timers per jurisdiction and notice templates | spec only |

## H. Pre-send transform hook — spec only (interface)

An external reversible anonymiser plugs in between the chat and the AI
provider. Interface (Go):

```go
// PreSendTransformer masks personal data before an AI call and restores it
// in the answer. Implementations must keep the mapping only in memory for
// the duration of one call.
type PreSendTransformer interface {
	Mask(ctx context.Context, messages []map[string]string) (masked []map[string]string, restore func(answer string) string, err error)
}
```

It runs after the jurisdiction check (masking does not replace consent) and
must work with streamed answers.

| AC | Criterion | Test |
|---|---|---|
| AC-20 | A configured transformer masks outgoing messages and restores the answer; a failure blocks the call instead of sending unmasked text | spec only |

## I. "You are talking to AI" notice — spec only

EU AI Act art. 50(1) requires telling users they interact with AI, in force
since 02.08.2026. Mandatory in profile `eu`, on by default everywhere; also
required by some US state laws (reference §3.3, unconfirmed texts).

| AC | Criterion | Test |
|---|---|---|
| AC-23 | Chat shows a clear AI notice before the first message and in each dialog; it cannot be turned off in profile `eu` | spec only |

## J. Crisis protocol — spec only

Messages about suicide, self-harm or violence get an answer with helplines
for the installation's country, without refusing to continue the
conversation. Helpline numbers are operator content (admin panel / site
content), never hard-coded: they differ by country and change over time.

| AC | Criterion | Test |
|---|---|---|
| AC-26 | Crisis messages are detected (multilingual) and the answer includes the configured helplines; the dialog continues | spec only |
| AC-27 | Helplines are configured per country by the operator; a missing entry for the profile's country is shown as a warning in the admin overview | spec only |

## K. Minors — spec only

152-FZ sets no age of consent; under 14 a guardian consents; 14–18 is
contested for special categories. Conservative rule: under 18 only with
guardian consent, or no access to psychological modes. EU: 16 (13–16 by member
state). US: under 13 — COPPA.

| AC | Criterion | Test |
|---|---|---|
| AC-28 | Age gate with a threshold set by the profile (default 18 for `ru`, 16 for `eu`, 13 for `us`) | spec only |
| AC-29 | Below the threshold, psychological modes are closed unless a guardian consent is recorded | spec only |
