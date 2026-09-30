"""Thin cached wrapper over pymorphy3.

pymorphy3 knows three tags that matter here: Name (first name), Surn
(surname) and Patr (patronymic), plus Geox for geography. Parsing is the
hot path of detection, so every call is memoized.
"""

from __future__ import annotations

import threading
from functools import lru_cache

import pymorphy3

_lock = threading.Lock()
_analyzer: pymorphy3.MorphAnalyzer | None = None

PERSON_TAGS = ("Name", "Surn", "Patr")
CASES = ("nomn", "gent", "datv", "accs", "ablt", "loct")


def analyzer() -> pymorphy3.MorphAnalyzer:
    global _analyzer
    if _analyzer is None:
        with _lock:
            if _analyzer is None:
                _analyzer = pymorphy3.MorphAnalyzer()
    return _analyzer


def norm(word: str) -> str:
    return word.lower().replace("ё", "е")


@lru_cache(maxsize=200_000)
def parses(word: str) -> tuple:
    return tuple(analyzer().parse(word.lower()))


def person_role(word: str) -> str | None:
    """Name / Surn / Patr if any parse carries that tag, strongest first."""
    for p in parses(word):
        for tag in PERSON_TAGS:
            if tag in p.tag:
                return tag
    return None


def best_is_person(word: str) -> bool:
    ps = parses(word)
    return bool(ps) and any(tag in ps[0].tag for tag in PERSON_TAGS)


def person_score(word: str) -> float:
    """Total pymorphy score of person readings (0..1)."""
    return sum(p.score for p in parses(word) if any(t in p.tag for t in PERSON_TAGS))


def is_known_word(word: str) -> bool:
    """Is the word in the dictionary (not guessed by the predictor)?"""
    for p in parses(word):
        if p.is_known:
            return True
    return False


def is_geo(word: str) -> bool:
    ps = parses(word)
    return bool(ps) and "Geox" in ps[0].tag


def any_geo(word: str) -> bool:
    return any("Geox" in p.tag for p in parses(word))


def is_org(word: str) -> bool:
    return any("Orgn" in p.tag for p in parses(word))


def pos(word: str) -> str | None:
    ps = parses(word)
    return ps[0].tag.POS if ps else None


def is_verb_like(word: str) -> bool:
    ps = parses(word)
    return bool(ps) and ps[0].tag.POS in {"VERB", "INFN", "PRTF", "PRTS", "GRND"}


def person_parse(word: str, role: str | None = None, gender: str | None = None):
    """Best parse with a person tag, optionally restricted to role and gender."""
    for p in parses(word):
        if not any(t in p.tag for t in PERSON_TAGS):
            continue
        if role and role not in p.tag:
            continue
        if gender and not (p.tag.gender == gender or "ms-f" in p.tag):
            continue
        return p
    return None


def gender_of(word: str) -> str | None:
    p = person_parse(word)
    if p is None:
        return None
    return p.tag.gender


def lexeme_forms(word: str, role: str | None = None) -> set[str]:
    """All singular case forms of every person reading of the word.

    Only readings where the word itself is the nominative singular count:
    "Александр" is also the genitive plural of "Александра", and that lexeme
    must not leak into the forms of the masculine name.
    """
    out: set[str] = set()
    for p in parses(word):
        if not any(t in p.tag for t in PERSON_TAGS):
            continue
        if role and role not in p.tag:
            continue
        if "nomn" not in p.tag or "plur" in p.tag:
            continue
        for form in p.lexeme:
            if form.tag.number == "plur":
                continue
            out.add(norm(form.word))
    return out
