"""Format detectors: regular expressions plus checksums and context words.

Everything that has a shape lives here: phones, e-mails, links and social
handles, identity documents, cards and bank accounts. Where the shape alone
matches too much (a 12-digit number is not always a tax id) the rule demands
a valid checksum or a keyword nearby. Missing a real phone is worse than
masking an order number, so the phone and e-mail rules are deliberately
greedy; the document rules are the strict ones.
"""

from __future__ import annotations

import re
from typing import Iterator

from .kinds import Kind
from .spans import Span

NBSP = " "
_SEP = r"[\s  \-\.–—()]{0,3}"

# --------------------------------------------------------------------- phones

# Russian and international numbers: a country code and 10 digits for +7/8,
# 7..13 digits after any other +code. Separators of any kind, up to three in
# a row, because people type "8 (912) 345 - 67 - 89".
RE_PHONE_RU = re.compile(
    r"(?<![\w\d+])(?:\+\s?7|8|7)" r"(?:" + _SEP + r"\d){10}(?![\d])"
)
RE_PHONE_INTL = re.compile(
    r"(?<![\w\d])\+\s?(?:[1-9]\d{0,2})(?:" + _SEP + r"\d){6,13}(?![\d])"
)
# A bare ten-digit mobile starting with 9: "912 345 67 89".
RE_PHONE_BARE = re.compile(
    r"(?<![\w\d\-=/#№])9\d{2}" + _SEP + r"\d{3}" + _SEP + r"\d{2}" + _SEP + r"\d{2}(?![\d\w])"
)
# North American shapes: "(555) 123-4567", "555-123-4567", "555.123.4567".
RE_PHONE_NANP = re.compile(
    r"(?<![\w\d])(?:\(\d{3}\)\s?|\d{3}[\-.\s])\d{3}[\-.\s]\d{4}(?![\d\w])"
)
# Short local number only next to a phone word: "тел. 45-67-89".
RE_PHONE_LOCAL = re.compile(
    r"(?i)(?:тел(?:ефон)?|т\.|моб|сот|звони\w*|phone|tel|call)[\s:. ]{0,4}"
    r"(\d{2,3}[\-\s]\d{2}[\-\s]\d{2})(?![\d])"
)
_YEAR = re.compile(r"(?:19|20)\d{2}")


def _phone_digits(text: str) -> str:
    return re.sub(r"\D", "", text)


def _ru_code_ok(digits: str) -> bool:
    # There are no Russian area/operator codes starting with 0, 1 or 2, and
    # 800-numbers are not personal.
    code = digits[1:4]
    return code[:1] in "3456789" and not code.startswith("80")


def phones(text: str) -> Iterator[Span]:
    seen: set[tuple[int, int]] = set()

    def emit(start: int, end: int) -> Iterator[Span]:
        if (start, end) in seen:
            return
        seen.add((start, end))
        raw = text[start:end]
        digits = _phone_digits(raw)
        key = digits[-10:] if len(digits) >= 10 else digits
        yield Span(start, end, Kind.PHONE, digits, key)

    for m in RE_PHONE_RU.finditer(text):
        digits = _phone_digits(m.group(0))
        if len(digits) != 11 or not _ru_code_ok(digits):
            continue
        if _YEAR.search(m.group(0)) and m.group(0).count(".") >= 2:
            continue  # "8.03.2024 15:30" style timestamps
        yield from emit(m.start(), m.end())
    for m in RE_PHONE_INTL.finditer(text):
        yield from emit(m.start(), m.end())
    for m in RE_PHONE_BARE.finditer(text):
        yield from emit(m.start(), m.end())
    for m in RE_PHONE_NANP.finditer(text):
        if _YEAR.fullmatch(_phone_digits(m.group(0))[:4]) and "-" not in m.group(0):
            continue
        yield from emit(m.start(), m.end())
    for m in RE_PHONE_LOCAL.finditer(text):
        yield from emit(m.start(1), m.end(1))


# ------------------------------------------------------------------ e-mails

RE_EMAIL = re.compile(
    r"(?<![A-Za-z0-9._%+\-])[A-Za-z0-9](?:[A-Za-z0-9._%+\-]{0,63})"
    r"\s?@\s?(?:[A-Za-z0-9](?:[A-Za-z0-9\-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,24}(?![A-Za-z])"
)
# Cyrillic domains: "почта@яндекс.рф".
RE_EMAIL_CYR = re.compile(
    r"(?<![\w.%+\-])[\wА-Яа-яЁё][\wА-Яа-яЁё.%+\-]{0,63}@(?:[\wА-Яа-яЁё\-]+\.)+(?:рф|рус|бел|укр|срб|[A-Za-z]{2,24})(?![\w])"
)


def emails(text: str) -> Iterator[Span]:
    for rx in (RE_EMAIL, RE_EMAIL_CYR):
        for m in rx.finditer(text):
            value = re.sub(r"\s", "", m.group(0)).lower()
            yield Span(m.start(), m.end(), Kind.EMAIL, value, value)


# ------------------------------------------------------- links and handles

_SOCIAL = (
    r"t\.me|telegram\.me|vk\.com|vk\.ru|m\.vk\.com|ok\.ru|instagram\.com|instagr\.am|"
    r"facebook\.com|fb\.com|m\.facebook\.com|twitter\.com|x\.com|tiktok\.com|youtube\.com/@|"
    r"linkedin\.com|github\.com|gitlab\.com|habr\.com/(?:ru/)?users|dzen\.ru|zen\.yandex\.ru|"
    r"livejournal\.com|pinterest\.com|snapchat\.com|threads\.net|reddit\.com/u(?:ser)?|"
    r"wa\.me|max\.ru|hh\.ru/resume|superjob\.ru/resume|onlyfans\.com|boosty\.to|patreon\.com"
)
_PRIVATE_STORAGE = (
    r"docs\.google\.com|drive\.google\.com|disk\.yandex\.\w+|yadi\.sk|dropbox\.com|"
    r"cloud\.mail\.ru|onedrive\.live\.com|1drv\.ms|icloud\.com|mega\.nz|"
    r"calendly\.com|zoom\.us/j|meet\.google\.com|telemost\.yandex\.ru"
)
RE_SOCIAL_URL = re.compile(
    r"(?i)(?:https?://)?(?:www\.)?(?:" + _SOCIAL + r")/?[^\s,;\"'<>()\]]*"
)
RE_PRIVATE_URL = re.compile(
    r"(?i)(?:https?://)?(?:www\.)?(?:" + _PRIVATE_STORAGE + r")/[^\s,;\"'<>()\]]*"
)
RE_TOKEN_URL = re.compile(
    r"(?i)https?://[^\s\"'<>]*?[?&;#](?:token|access_token|auth|api_key|apikey|key|sig|"
    r"signature|secret|session|sessionid|sessid|ticket|code|invite|ref|uid|user|id)="
    r"[^\s\"'<>]+"
)
# Any other link. Real conversations showed Telegram usernames and nicknames
# inside links to personal pages on arbitrary domains, so the default is to
# mask every link with a path or a personal subdomain, except well-known
# reference sites a therapist may point to.
RE_ANY_URL = re.compile(
    r"(?i)(?:https?://|www\.)[^\s\"'<>()\]]+"
    r"|(?<![\w@.\-])(?:[a-z0-9](?:[a-z0-9\-]{0,61}[a-z0-9])?\.)+"
    r"(?:ru|com|net|org|рф|io|me|app|site|online|info|su|by|kz|ua|tv|ws|pro|link|page|dev|ai|co|xyz|club|space)"
    r"(?:/[^\s\"'<>()\]]*)?(?![\w@])"
)
SAFE_HOSTS = (
    "wikipedia.org", "wikimedia.org", "who.int", "apa.org", "nih.gov", "cochrane.org", "gov.ru",
    "gosuslugi.ru", "consultant.ru", "garant.ru", "mindstrata.ru", "psychologytoday.com", "nhs.uk",
    "cdc.gov", "ya.ru",
)
SAFE_PREFIXES = ("google.com/search", "yandex.ru/search")


def _personal_link(value: str) -> bool:
    bare = re.sub(r"^(?:https?://)?(?:www\.)?", "", value.lower())
    host, _, path = bare.partition("/")
    if any(host == h or host.endswith("." + h) for h in SAFE_HOSTS) or bare.startswith(SAFE_PREFIXES):
        return False
    if path.strip("/"):
        return True
    return host.count(".") >= 2  # personal subdomain: "nick.tilda.ws"


RE_HANDLE = re.compile(r"(?<![\w@.])@([A-Za-z0-9_](?:[A-Za-z0-9_.]{2,31}))(?![\w@])")
# "мой ник ivan_2000", "логин: kotik", "my username is sam_k"
RE_HANDLE_CUE = re.compile(
    r"(?i)(?:ник(?:нейм)?|логин|юзернейм|username|user name|nickname|handle|"
    r"в\s+(?:телеграме?|тг|инсте|инстаграме?|вк|контакте|дискорде?|скайпе?)|"
    r"(?:telegram|tg|instagram|insta|discord|skype|snapchat|vk)\s*[:\-]?)"
    r"\s*(?:[:\-—]|это|is)?\s*@?([A-Za-z][A-Za-z0-9_.\-]{2,31})(?![\w@])"
)


def links(text: str) -> Iterator[Span]:
    for rx, kind in ((RE_SOCIAL_URL, Kind.URL), (RE_PRIVATE_URL, Kind.URL), (RE_TOKEN_URL, Kind.URL)):
        for m in rx.finditer(text):
            value = m.group(0).rstrip(".,!?:")
            end = m.start() + len(value)
            if end - m.start() < 6:
                continue
            key = re.sub(r"^(?:https?://)?(?:www\.)?", "", value.lower()).rstrip("/")
            yield Span(m.start(), end, kind, value, key)
    for m in RE_ANY_URL.finditer(text):
        value = m.group(0).rstrip(".,!?:;")
        if not _personal_link(value):
            continue
        key = re.sub(r"^(?:https?://)?(?:www\.)?", "", value.lower()).rstrip("/")
        yield Span(m.start(), m.start() + len(value), Kind.URL, value, key)
    for m in RE_HANDLE.finditer(text):
        # "@" inside an e-mail is excluded by the lookbehind on the word.
        value = m.group(1).rstrip(".")
        end = m.start(1) + len(value)
        yield Span(m.start(), end, Kind.HANDLE, "@" + value, value.lower())
    for m in RE_HANDLE_CUE.finditer(text):
        value = m.group(1).rstrip(".-")
        if value.lower() in _HANDLE_STOP:
            continue
        end = m.start(1) + len(value)
        yield Span(m.start(1), end, Kind.HANDLE, value, value.lower())


_HANDLE_STOP = frozenset({"the", "and", "not", "was", "has", "have", "that", "this", "with", "you", "your", "for"})

# ---------------------------------------------------------------- documents


def _inn_ok(d: str) -> bool:
    def check(digits: str, coeffs: list[int]) -> int:
        return sum(int(x) * c for x, c in zip(digits, coeffs)) % 11 % 10

    if len(d) == 10:
        return check(d, [2, 4, 10, 3, 5, 9, 4, 6, 8]) == int(d[9])
    if len(d) == 12:
        c11 = check(d, [7, 2, 4, 10, 3, 5, 9, 4, 6, 8])
        c12 = check(d, [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8])
        return c11 == int(d[10]) and c12 == int(d[11])
    return False


def _snils_ok(d: str) -> bool:
    if len(d) != 11:
        return False
    body, ctrl = d[:9], int(d[9:])
    if int(body) <= 1001998:
        return True  # numbers below this range were issued without a checksum
    s = sum(int(x) * (9 - i) for i, x in enumerate(body))
    if s > 101:
        s %= 101
    if s in (100, 101):
        s = 0
    return s == ctrl


def _luhn_ok(d: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(d)):
        n = int(ch)
        if i % 2 == 1:
            n *= 2
            if n > 9:
                n -= 9
        total += n
    return total % 10 == 0


_CTX_WINDOW = 40
RE_DOC_CUE = {
    "passport": re.compile(r"(?i)(паспорт|пасп\.|серия|серии|passport)"),
    "snils": re.compile(r"(?i)(снилс|страхов\w+\s+номер|пенсионн)"),
    "inn": re.compile(r"(?i)(инн|налогов|taxpayer|tin)"),
    "polis": re.compile(r"(?i)(полис|омс|дмс|страховк)"),
    "license": re.compile(r"(?i)(в/у|водительск|права\b|driver'?s?\s+licen[cs]e)"),
    "birth": re.compile(r"(?i)(свидетельств\w+\s+о\s+рождении)"),
    "account": re.compile(r"(?i)(р/с|р\.с\.|расч[её]тн\w+\s+сч[её]т|сч[её]т[ау]?\b|л/с|лицев\w+\s+сч[её]т|account|iban|бик)"),
    "card": re.compile(r"(?i)(карт[аеуы]|card|visa|mastercard|мир\b)"),
    "ssn": re.compile(r"(?i)(ssn|social\s+security)"),
    "med": re.compile(r"(?i)(медкарт|медицинск\w+\s+карт|истори\w+\s+болезни|mrn|medical\s+record)"),
}

RE_DIGIT_RUN = re.compile(r"(?<![\w\d])\d(?:[\s\- ]?\d){5,24}(?![\d\w])")
RE_SSN = re.compile(r"(?<![\d\w])\d{3}-\d{2}-\d{4}(?![\d\w])")
RE_SNILS_SHAPE = re.compile(r"(?<![\d\w])\d{3}[\-\s]\d{3}[\-\s]\d{3}[\-\s]?\d{2}(?![\d\w])")
RE_PASSPORT_SHAPE = re.compile(r"(?<![\d\w])\d{2}\s?\d{2}[\s№\-]{1,3}\d{6}(?![\d\w])")
RE_LICENSE_SHAPE = re.compile(r"(?<![\d\w])\d{2}\s?[\dА-ЯA-Z]{2}\s?\d{6}(?![\d\w])")
RE_IBAN = re.compile(r"(?<![A-Z0-9])[A-Z]{2}\d{2}(?:\s?[A-Z0-9]{4}){3,7}(?:\s?[A-Z0-9]{1,3})?(?![A-Z0-9])")


def _near(text: str, start: int, end: int, cue: str) -> bool:
    left = text[max(0, start - _CTX_WINDOW):start]
    right = text[end:end + 15]
    return bool(RE_DOC_CUE[cue].search(left) or RE_DOC_CUE[cue].search(right))


def documents(text: str) -> Iterator[Span]:
    for m in RE_SSN.finditer(text):
        d = _phone_digits(m.group(0))
        yield Span(m.start(), m.end(), Kind.DOCUMENT, d, "ssn:" + d)
    for m in RE_SNILS_SHAPE.finditer(text):
        d = _phone_digits(m.group(0))
        if _snils_ok(d) or _near(text, m.start(), m.end(), "snils"):
            yield Span(m.start(), m.end(), Kind.DOCUMENT, d, "snils:" + d)
    for m in RE_PASSPORT_SHAPE.finditer(text):
        if _near(text, m.start(), m.end(), "passport"):
            d = _phone_digits(m.group(0))
            yield Span(m.start(), m.end(), Kind.DOCUMENT, d, "passport:" + d)
    for m in RE_LICENSE_SHAPE.finditer(text):
        if _near(text, m.start(), m.end(), "license"):
            d = re.sub(r"\s", "", m.group(0))
            yield Span(m.start(), m.end(), Kind.DOCUMENT, d, "license:" + d)
    for m in RE_IBAN.finditer(text):
        d = re.sub(r"\s", "", m.group(0))
        yield Span(m.start(), m.end(), Kind.ACCOUNT, d, "iban:" + d)
    for m in RE_DIGIT_RUN.finditer(text):
        raw = m.group(0)
        d = _phone_digits(raw)
        s, e = m.start(), m.end()
        n = len(d)
        if 13 <= n <= 19 and _luhn_ok(d) and not d.startswith("0"):
            yield Span(s, e, Kind.CARD, d, "card:" + d)
            continue
        if n >= 12 and _near(text, s, e, "card"):
            yield Span(s, e, Kind.CARD, d, "card:" + d)
            continue
        if n == 20 and (d.startswith(("408", "423", "40702", "40802")) or _near(text, s, e, "account")):
            yield Span(s, e, Kind.ACCOUNT, d, "account:" + d)
            continue
        if n in (10, 12) and _inn_ok(d) and (n == 12 or _near(text, s, e, "inn")):
            yield Span(s, e, Kind.DOCUMENT, d, "inn:" + d)
            continue
        if n == 16 and _near(text, s, e, "polis"):
            yield Span(s, e, Kind.DOCUMENT, d, "polis:" + d)
            continue
        if n == 11 and _snils_ok(d) and _near(text, s, e, "snils"):
            yield Span(s, e, Kind.DOCUMENT, d, "snils:" + d)
            continue
        if n == 10 and _near(text, s, e, "passport"):
            yield Span(s, e, Kind.DOCUMENT, d, "passport:" + d)
            continue
        if n >= 6 and any(_near(text, s, e, c) for c in ("account", "med", "birth", "polis", "ssn", "inn", "license")):
            yield Span(s, e, Kind.DOCUMENT, d, "doc:" + d)


def all_rules(text: str) -> list[Span]:
    out: list[Span] = []
    out.extend(phones(text))
    out.extend(emails(text))
    out.extend(links(text))
    out.extend(documents(text))
    return out
