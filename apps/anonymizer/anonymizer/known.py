"""The user's own dictionary: values we already know belong to this person.

Two sources feed it: the Mindstrata profile (display name, e-mail, phone,
Telegram username) and every value already stored in the user's vault. A
known value is masked wherever it appears, in any case form and spelling,
whatever the other detectors think. This is what makes recall on the user's
own data complete: a name the NER never recognised is still caught once it is
in the dictionary.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from functools import lru_cache

from . import decline, morph, names_en, names_ru, people, spoken
from .kinds import Kind
from .spans import Span


@dataclass(frozen=True)
class Known:
    kind: Kind
    value: str
    canon: str = ""
    key: str = ""


_STOP = frozenset("""
the and not you your for with this that from are was were have has had user admin test
пользователь админ тест гость клиент человек друг мама папа привет
""".split())
_RU_FROM_LAT = [
    ("shch", "щ"), ("sch", "щ"), ("yo", "ё"), ("zh", "ж"), ("kh", "х"), ("ts", "ц"), ("ch", "ч"),
    ("sh", "ш"), ("yu", "ю"), ("ya", "я"), ("ia", "ия"), ("iy", "ий"), ("a", "а"), ("b", "б"),
    ("v", "в"), ("w", "в"), ("g", "г"), ("d", "д"), ("e", "е"), ("z", "з"), ("i", "и"), ("y", "й"),
    ("k", "к"), ("l", "л"), ("m", "м"), ("n", "н"), ("o", "о"), ("p", "п"), ("r", "р"), ("s", "с"),
    ("t", "т"), ("u", "у"), ("f", "ф"), ("h", "х"), ("c", "к"), ("x", "кс"), ("q", "к"), ("j", "дж"),
]


def lower_same_length(text: str) -> str:
    """Lowercase and ё->е without changing any offset."""
    out = []
    for ch in text:
        low = ch.lower()
        out.append(low if len(low) == 1 else ch)
    return "".join(out).replace("ё", "е")


def _cyr_from_latin(word: str) -> str:
    w = word.lower()
    out = ""
    i = 0
    while i < len(w):
        for lat, cyr in _RU_FROM_LAT:
            if w.startswith(lat, i):
                out += cyr
                i += len(lat)
                break
        else:
            out += w[i]
            i += 1
    if out.endswith("ы"):
        out = out[:-1] + "й"
    return out


@lru_cache(maxsize=50_000)
def token_forms(token: str) -> frozenset[str]:
    """Every spelling of one name token worth searching for (normalized)."""
    tok = token.strip(".,;:!?()\"'«»")
    if len(tok) < 3 or tok.lower() in _STOP:
        return frozenset()
    out: set[str] = {morph.norm(tok)}
    if re.search(r"[А-Яа-яЁё]", tok):
        hit = names_ru.lookup(tok)
        if hit:
            out |= decline.forms(hit[0], "Name", hit[1])
        role = morph.person_role(tok) or "Surn"
        gender = decline.person_gender([tok])
        out |= decline.forms(tok, role, gender)
        out |= morph.lexeme_forms(tok)  # every person reading, whatever its role
        if role == "Surn":
            # a surname is shared by both genders of a family
            out |= decline.forms(tok, "Surn", "masc" if gender == "femn" else "femn")
        # a nickname that is an ordinary word ("Солнышко") in all its forms
        ps = morph.parses(tok)
        if ps and ps[0].is_known:
            out |= {morph.norm(f.word) for f in ps[0].lexeme if f.tag.number != "plur"}
        for latin in names_en.translit(tok):
            out.add(latin)
    else:
        out.add(tok.lower() + "'s")
        cyr = _cyr_from_latin(tok)
        if names_ru.lookup(cyr) or morph.person_role(cyr):
            out |= token_forms(cyr[:1].upper() + cyr[1:])
    return frozenset(f for f in out if len(f) >= 3)


@lru_cache(maxsize=50_000)
def _word_regex(forms: frozenset[str]) -> re.Pattern | None:
    if not forms:
        return None
    alt = "|".join(sorted((re.escape(f) for f in forms), key=len, reverse=True))
    return re.compile(r"(?<![a-zа-я0-9_])(?:" + alt + r")(?![a-zа-я0-9_])")


def _phone_spans(text: str, digits: str, item: Known) -> list[Span]:
    if len(digits) < 7:
        return []
    tail = digits[-10:]
    out = []
    for m in re.finditer(r"\+?\d(?:[\s\-().]{0,3}\d){6,14}", text):
        d = re.sub(r"\D", "", m.group(0))
        if len(d) >= 7 and (d[-10:] == tail or tail.endswith(d)):
            out.append(Span(m.start(), m.end(), Kind.PHONE, tail, tail, "known"))
    for sp in spoken.numbers(text):
        if sp.kind == Kind.PHONE and sp.key == tail:
            out.append(Span(sp.start, sp.end, Kind.PHONE, tail, tail, "known"))
    return out


def find(text: str, items: list[Known]) -> list[Span]:
    if not items:
        return []
    low = lower_same_length(text)
    out: list[Span] = []
    for item in items:
        value = item.value.strip()
        if not value:
            continue
        if item.kind == Kind.PHONE or (item.kind == Kind.DOCUMENT and re.fullmatch(r"[\d\s\-+()]+", value)):
            digits = re.sub(r"\D", "", value)
            if item.kind == Kind.PHONE:
                out.extend(_phone_spans(text, digits, item))
            else:
                for m in re.finditer(r"\d(?:[\s\-]?\d){5,}", text):
                    if re.sub(r"\D", "", m.group(0)) == digits:
                        out.append(Span(m.start(), m.end(), item.kind, item.canon or digits,
                                        item.key or digits, "known"))
            continue
        if item.kind == Kind.EMAIL:
            v = value.lower()
            for m in re.finditer(re.escape(v), low):
                out.append(Span(m.start(), m.end(), Kind.EMAIL, v, v, "known"))
            local = v.split("@")[0]
            if len(local) >= 4 and local not in _STOP:
                for m in re.finditer(r"(?<![a-z0-9._%+\-])" + re.escape(local) + r"(?![a-z0-9_])", low):
                    out.append(Span(m.start(), m.end(), Kind.HANDLE, local, local, "known"))
            continue
        if item.kind == Kind.HANDLE:
            v = value.lstrip("@").lower()
            if len(v) < 3:
                continue
            for m in re.finditer(r"(?<![a-z0-9_])@?" + re.escape(v) + r"(?![a-z0-9_])", low):
                out.append(Span(m.start(), m.end(), Kind.HANDLE, "@" + v, v, "known"))
            continue
        if item.kind == Kind.PERSON:
            for tok in re.findall(r"[A-Za-zА-Яа-яЁё][A-Za-zА-Яа-яЁё'\-]*", value):
                rx = _word_regex(token_forms(tok))
                if rx is None:
                    continue
                for m in rx.finditer(low):
                    canon, key = people.canonical(text[m.start():m.end()])
                    out.append(Span(m.start(), m.end(), Kind.PERSON, canon, key, "known"))
            continue
        # Places, organisations, addresses, dates and documents from the vault:
        # the whole value, whitespace-insensitive, plus case forms of one-word
        # places ("Екатеринбург" -> "Екатеринбурге").
        norm_value = " ".join(lower_same_length(value).split())
        variants = {norm_value}
        if item.kind in (Kind.PLACE, Kind.ORG) and " " not in norm_value and re.search(r"[а-я]", norm_value):
            for p in morph.parses(norm_value):
                if p.normal_form == norm_value or "Geox" in p.tag or "Orgn" in p.tag:
                    variants |= {morph.norm(f.word) for f in p.lexeme if f.tag.number != "plur"}
        for var in variants:
            if len(var) < 3:
                continue
            pattern = r"\s+".join(re.escape(part) for part in var.split())
            for m in re.finditer(r"(?<![a-zа-я0-9_])" + pattern + r"(?![a-zа-я0-9_])", low):
                out.append(Span(m.start(), m.end(), item.kind, item.canon or value,
                                item.key or norm_value, "known"))
    return out
