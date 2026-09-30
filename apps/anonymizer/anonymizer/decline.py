"""Russian declension of person names, both directions.

Detection needs every case form of a name (to find "Юле" when the profile
says "Юля") and a gender-preserving nominative (so "с Петровой" is stored as
"Петрова", not "Петров"). Restoration needs the opposite: put the stored
nominative into the case the surrounding sentence asks for.

pymorphy3 is asked first, but only about readings tagged Name/Surn/Patr. It
does not know most diminutives ("Юля" parses as the noun "юла") and it picks
wrong readings for surnames in the nominative ("Тумаков" as genitive plural
of "тумак"). The deterministic tables below cover the regular Russian
paradigms and keep the stem letter for letter, including "ё".
"""

from __future__ import annotations

import re
from functools import lru_cache

from . import morph

CASES = morph.CASES
_VELAR_SIBILANT = set("гкхжшщч")
_SIBILANT_C = set("жшщчц")
_INDECLINABLE_END = ("о", "е", "и", "у", "ю", "ы", "э")


def _cap(template: str, word: str) -> str:
    """Copy capitalization of template onto word."""
    if template.isupper() and len(template) > 1:
        return word.upper()
    if template[:1].isupper():
        return word[:1].upper() + word[1:]
    return word


def _noun_a(stem: str, last: str) -> dict[str, str]:
    """First declension (-а/-я): Маша, Юля, Мария, Серёжа."""
    if last == "я":
        if stem.endswith("и"):
            return {"nomn": stem + "я", "gent": stem + "и", "datv": stem + "и",
                    "accs": stem + "ю", "ablt": stem + "ей", "loct": stem + "и"}
        return {"nomn": stem + "я", "gent": stem + "и", "datv": stem + "е",
                "accs": stem + "ю", "ablt": stem + "ей", "loct": stem + "е"}
    gent = stem + ("и" if stem[-1:] in _VELAR_SIBILANT else "ы")
    ablt = stem + ("ей" if stem[-1:] in _SIBILANT_C else "ой")
    return {"nomn": stem + "а", "gent": gent, "datv": stem + "е",
            "accs": stem + "у", "ablt": ablt, "loct": stem + "е"}


def _noun_consonant(word: str) -> dict[str, str]:
    """Second declension, masculine: Олег, Андрей, Дмитрий, Игорь."""
    if word.endswith("ий"):
        s = word[:-2]
        return {"nomn": word, "gent": s + "ия", "datv": s + "ию", "accs": s + "ия",
                "ablt": s + "ием", "loct": s + "ии"}
    if word.endswith(("й", "ь")):
        s = word[:-1]
        return {"nomn": word, "gent": s + "я", "datv": s + "ю", "accs": s + "я",
                "ablt": s + "ем", "loct": s + "е"}
    ablt = word + ("ем" if word[-1:] in _SIBILANT_C else "ом")
    return {"nomn": word, "gent": word + "а", "datv": word + "у", "accs": word + "а",
            "ablt": ablt, "loct": word + "е"}


def _surname(word: str, gender: str | None) -> dict[str, str] | None:
    w = word
    if re.search(r"(ов|ев|ёв|ин|ын)$", w) and gender != "femn":
        return {"nomn": w, "gent": w + "а", "datv": w + "у", "accs": w + "а",
                "ablt": w + "ым", "loct": w + "е"}
    if re.search(r"(ова|ева|ёва|ина|ына)$", w) and gender != "masc":
        s = w[:-1]
        return {"nomn": w, "gent": s + "ой", "datv": s + "ой", "accs": s + "у",
                "ablt": s + "ой", "loct": s + "ой"}
    if re.search(r"(ский|цкий|ской|цкой)$", w):
        s = w[:-2]
        return {"nomn": w, "gent": s + "ого", "datv": s + "ому", "accs": s + "ого",
                "ablt": s + "им", "loct": s + "ом"}
    if re.search(r"(ская|цкая)$", w):
        s = w[:-2]
        return {"nomn": w, "gent": s + "ой", "datv": s + "ой", "accs": s + "ую",
                "ablt": s + "ой", "loct": s + "ой"}
    if re.search(r"(ый|ой)$", w) and gender != "femn":
        s = w[:-2]
        return {"nomn": w, "gent": s + "ого", "datv": s + "ому", "accs": s + "ого",
                "ablt": s + "ым", "loct": s + "ом"}
    if re.search(r"ая$", w):
        s = w[:-2]
        return {"nomn": w, "gent": s + "ой", "datv": s + "ой", "accs": s + "ую",
                "ablt": s + "ой", "loct": s + "ой"}
    if re.search(r"(их|ых|ко|аго|яго)$", w) or w.endswith(_INDECLINABLE_END):
        return {c: w for c in CASES}
    if w[-1:] in ("а", "я"):
        return _noun_a(w[:-1], w[-1])
    if gender == "femn":
        return {c: w for c in CASES}
    return _noun_consonant(w)


def _patronymic(word: str) -> dict[str, str] | None:
    if re.search(r"(ович|евич|ич)$", word):
        return _noun_consonant(word)
    if re.search(r"(овна|евна|ична|инична)$", word):
        s = word[:-1]
        return {"nomn": word, "gent": s + "ы", "datv": s + "е", "accs": s + "у",
                "ablt": s + "ой", "loct": s + "е"}
    return None


def paradigm(nominative: str, role: str, gender: str | None) -> dict[str, str]:
    """Case forms (lowercase) of a nominative name token by the tables."""
    w = nominative.lower()
    if role == "Patr":
        table = _patronymic(w)
        if table:
            return table
    if role == "Surn":
        table = _surname(w, gender)
        if table:
            return table
    if w[-1:] in ("а", "я"):
        return _noun_a(w[:-1], w[-1])
    if w.endswith(_INDECLINABLE_END) or gender == "femn":
        return {c: w for c in CASES}
    return _noun_consonant(w)


@lru_cache(maxsize=100_000)
def forms(nominative: str, role: str, gender: str | None) -> frozenset[str]:
    """Every singular case form (normalized) of a name token."""
    out = set(morph.norm(f) for f in paradigm(nominative, role, gender).values())
    out.add(morph.norm(nominative))
    out |= morph.lexeme_forms(nominative, role)
    return frozenset(out)


def to_nominative(word: str, role: str | None, gender_hint: str | None = None) -> tuple[str, str | None]:
    """Nominative singular of a name token, keeping gender. Returns (form, gender)."""
    p = morph.person_parse(word, role, gender_hint)
    if p is not None:
        gender = p.tag.gender if p.tag.gender in ("masc", "femn") else gender_hint
        want = {"nomn", "sing"}
        if gender in ("masc", "femn"):
            want.add(gender)
        infl = p.inflect(want) or p.inflect({"nomn"})
        if infl is not None:
            return _cap(word, infl.word), gender
    # Unknown to pymorphy: undo the regular endings.
    low = word.lower()
    for nom, role_guess, g in _guess_nominatives(low):
        if low in forms(nom, role_guess, g):
            return _cap(word, nom), g
    return word, gender_hint


def _guess_nominatives(low: str):
    cands = []
    for suffix, repl, g in (
        ("ой", "а", "femn"), ("ей", "я", "femn"), ("е", "а", "femn"), ("у", "а", "femn"),
        ("ю", "я", "femn"), ("и", "а", "femn"), ("ы", "а", "femn"), ("е", "я", "femn"),
        ("и", "я", "femn"),
        ("а", "", "masc"), ("у", "", "masc"), ("ом", "", "masc"), ("ем", "", "masc"),
        ("е", "", "masc"), ("ым", "", "masc"),
    ):
        if low.endswith(suffix) and len(low) - len(suffix) >= 2:
            cands.append((low[: len(low) - len(suffix)] + repl, "Name", g))
    cands.append((low, "Name", None))
    return cands


def inflect(nominative: str, case: str, role: str | None = None, gender: str | None = None) -> str:
    """Put one nominative name token into case; keeps capitalization."""
    if case == "nomn" or not nominative:
        return nominative
    if re.fullmatch(r"[А-ЯЁA-Z]\.?", nominative) or not re.search(r"[а-яё]", nominative.lower()):
        return nominative  # initials and Latin stay as they are
    role = role or morph.person_role(nominative) or "Name"
    if gender is None:
        gender = morph.gender_of(nominative)
    p = morph.person_parse(nominative, role, gender)
    if p is not None and "nomn" in p.tag and "Fixd" in p.tag:
        return nominative
    table = paradigm(nominative, role, gender)
    return _cap(nominative, table.get(case, nominative.lower()))


def person_gender(tokens: list[str]) -> str | None:
    """Gender of a full name: patronymic first, then first name, then surname."""
    for tok in tokens:
        low = tok.lower()
        if re.search(r"(овна|евна|ична)$", low):
            return "femn"
        if re.search(r"(ович|евич|ич)$", low):
            return "masc"
    for tok in tokens:
        p = morph.person_parse(tok, "Name")
        if p is not None and p.tag.gender in ("masc", "femn"):
            return p.tag.gender
    from .names_ru import gender_of_first_name
    for tok in tokens:
        g = gender_of_first_name(tok)
        if g:
            return g
    for tok in tokens:
        low = tok.lower()
        if re.search(r"(ова|ева|ина|ская|цкая|ая)$", low):
            return "femn"
        if re.search(r"(ов|ев|ин|ский|цкий)$", low):
            return "masc"
    return None
