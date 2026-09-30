"""Identifiers written out in words.

Voice input and people who distrust digits produce "восемь девятьсот
двенадцать триста сорок пять шестьдесят семь восемьдесят девять" or
"ivan dot petrov at gmail dot com". The digit rules cannot see these.
Number words are turned into digits keeping their positions, and the digit
string is judged the same way a typed one is: 10-11 digits shaped like a phone
become PHONE, any other dictated run of 9+ digits is masked as DOCUMENT.
Amounts ("сто двадцать тысяч") do not reach nine digits and stay.
"""

from __future__ import annotations

import re
from typing import Iterator

from .kinds import Kind
from .spans import Span

RU_UNITS = {"ноль": 0, "нуль": 0, "один": 1, "одна": 1, "одно": 1, "два": 2, "две": 2,
            "три": 3, "четыре": 4, "пять": 5, "шесть": 6, "семь": 7, "восемь": 8, "девять": 9}
RU_TEENS = {"десять": 10, "одиннадцать": 11, "двенадцать": 12, "тринадцать": 13,
            "четырнадцать": 14, "пятнадцать": 15, "шестнадцать": 16, "семнадцать": 17,
            "восемнадцать": 18, "девятнадцать": 19}
RU_TENS = {"двадцать": 20, "тридцать": 30, "сорок": 40, "пятьдесят": 50,
           "шестьдесят": 60, "семьдесят": 70, "восемьдесят": 80, "девяносто": 90}
RU_HUNDREDS = {"сто": 100, "двести": 200, "триста": 300, "четыреста": 400, "пятьсот": 500,
               "шестьсот": 600, "семьсот": 700, "восемьсот": 800, "девятьсот": 900}
EN_UNITS = {"zero": 0, "oh": 0, "o": 0, "nought": 0, "one": 1, "two": 2, "three": 3,
            "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9}
EN_TEENS = {"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14,
            "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19}
EN_TENS = {"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60,
           "seventy": 70, "eighty": 80, "ninety": 90}

UNITS = {**RU_UNITS, **EN_UNITS}
TEENS = {**RU_TEENS, **EN_TEENS}
TENS = {**RU_TENS, **EN_TENS}
HUNDREDS = RU_HUNDREDS
PLUS = {"плюс", "plus"}
DOUBLE = {"double": 2, "triple": 3}

_TOKEN = re.compile(r"[A-Za-zА-Яа-яЁё]+|\d+|\+")
_GAP = re.compile(r"^[\s,.\-–—:;]{0,4}$")


def _tokens(text: str):
    for m in _TOKEN.finditer(text):
        yield m.start(), m.end(), m.group(0).lower().replace("ё", "е")


def _is_number_token(tok: str) -> bool:
    return (tok.isdigit() or tok in UNITS or tok in TEENS or tok in TENS
            or tok in HUNDREDS or tok in PLUS or tok in DOUBLE or tok == "hundred")


def _runs(text: str):
    run: list[tuple[int, int, str]] = []
    for start, end, tok in _tokens(text):
        if not _is_number_token(tok):
            if run:
                yield run
            run = []
            continue
        if run and not _GAP.match(text[run[-1][1]:start]):
            yield run
            run = []
        run.append((start, end, tok))
    if run:
        yield run


def _to_digits(run: list[tuple[int, int, str]]) -> str:
    out: list[str] = []
    i = 0
    toks = [t for _, _, t in run]
    while i < len(toks):
        t = toks[i]
        if t in PLUS:
            out.append("+")
            i += 1
        elif t.isdigit():
            out.append(t)
            i += 1
        elif t in DOUBLE and i + 1 < len(toks) and toks[i + 1] in UNITS:
            out.append(str(UNITS[toks[i + 1]]) * DOUBLE[t])
            i += 2
        elif t in HUNDREDS:
            value = HUNDREDS[t]
            i += 1
            if i < len(toks) and toks[i] in TENS:
                value += TENS[toks[i]]
                i += 1
                if i < len(toks) and toks[i] in UNITS and UNITS[toks[i]] > 0:
                    value += UNITS[toks[i]]
                    i += 1
            elif i < len(toks) and toks[i] in TEENS:
                value += TEENS[toks[i]]
                i += 1
            elif i < len(toks) and toks[i] in UNITS and UNITS[toks[i]] > 0:
                value += UNITS[toks[i]]
                i += 1
            out.append(f"{value:03d}")
        elif t in TENS:
            value = TENS[t]
            i += 1
            if i < len(toks) and toks[i] in UNITS and UNITS[toks[i]] > 0:
                value += UNITS[toks[i]]
                i += 1
            out.append(str(value))
        elif t in TEENS:
            out.append(str(TEENS[t]))
            i += 1
        elif t in UNITS:
            out.append(str(UNITS[t]))
            i += 1
        else:  # "hundred" without a preceding unit
            out.append("100")
            i += 1
    return "".join(out)


def numbers(text: str) -> Iterator[Span]:
    for run in _runs(text):
        words = [t for _, _, t in run if not t.isdigit() and t != "+"]
        if not words:
            continue  # purely typed digits are the business of rules.py
        digits = _to_digits(run)
        plain = digits.lstrip("+")
        start, end = run[0][0], run[-1][1]
        english = all(re.fullmatch(r"[a-z]+", w) for w in words)
        if (len(plain) == 11 and plain[0] in "78") or (len(plain) == 10 and (plain[0] == "9" or english)) \
                or (digits.startswith("+") and 10 <= len(plain) <= 13):
            yield Span(start, end, Kind.PHONE, plain, plain[-10:], "spoken")
        elif len(plain) >= 9:
            yield Span(start, end, Kind.DOCUMENT, plain, "doc:" + plain, "spoken")


_AT = r"(?:собака|собачка|собаку|эт|at)"
_DOT = r"(?:точка|dot)"
_UNDERSCORE = r"(?:нижнее\s+подчерк\w*|подчерк\w*|underscore|дефис|тире|dash|hyphen)"
_PART = r"[A-Za-zА-Яа-яЁё0-9]+"
RE_SPOKEN_EMAIL = re.compile(
    r"(?i)(?<![\w])(" + _PART + r"(?:\s+(?:" + _DOT + "|" + _UNDERSCORE + r")\s+" + _PART + r"){0,4})"
    r"\s+" + _AT + r"\s+(" + _PART + r"(?:\s+" + _DOT + r"\s+" + _PART + r"){1,3})(?![\w])"
)
_TLD = {"ру": "ru", "ком": "com", "нет": "net", "орг": "org", "рф": "рф", "ру.": "ru",
        "ру,": "ru"}


def emails(text: str) -> Iterator[Span]:
    for m in RE_SPOKEN_EMAIL.finditer(text):
        domain_parts = re.split(r"(?i)\s+" + _DOT + r"\s+", m.group(2))
        tld = domain_parts[-1].lower()
        if not (re.fullmatch(r"[a-z]{2,6}", tld) or tld in _TLD):
            continue
        local = re.sub(r"(?i)\s+" + _DOT + r"\s+", ".", m.group(1))
        local = re.sub(r"(?i)\s+" + _UNDERSCORE + r"\s+", "_", local)
        value = (local + "@" + ".".join(domain_parts[:-1]) + "." + _TLD.get(tld, tld)).lower()
        yield Span(m.start(), m.end(), Kind.EMAIL, value, value, "spoken")


def all_spoken(text: str) -> list[Span]:
    return [*numbers(text), *emails(text)]
