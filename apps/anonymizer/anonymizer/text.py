"""Tokenization and small text helpers shared by the detectors."""

from __future__ import annotations

import re
from dataclasses import dataclass

WORD_RE = re.compile(r"[A-Za-zА-Яа-яЁё][A-Za-zА-Яа-яЁё'’]*(?:-[A-Za-zА-Яа-яЁё]+)*")
INITIAL_RE = re.compile(r"(?<![A-Za-zА-Яа-яЁё])[А-ЯЁA-Z]\.(?:\s?[А-ЯЁA-Z]\.)?")
CYR = re.compile(r"[А-Яа-яЁё]")
LAT = re.compile(r"[A-Za-z]")


@dataclass(frozen=True)
class Token:
    start: int
    end: int
    text: str

    @property
    def low(self) -> str:
        return self.text.lower().replace("ё", "е")

    @property
    def cap(self) -> bool:
        return self.text[:1].isupper()

    @property
    def cyr(self) -> bool:
        return bool(CYR.match(self.text))


def words(text: str) -> list[Token]:
    return [Token(m.start(), m.end(), m.group(0)) for m in WORD_RE.finditer(text)]


def sentence_initial(text: str, start: int) -> bool:
    """Does a sentence (or a line, or a quote) start right before position?"""
    i = start - 1
    while i >= 0 and text[i] in " \t«\"'“„(—–-*•":
        i -= 1
    return i < 0 or text[i] in ".!?…\n:;"


def gap(text: str, a: int, b: int) -> str:
    return text[a:b]


def single_space(text: str, a: int, b: int) -> bool:
    return re.fullmatch(r"[ \t ]{1,2}", text[a:b]) is not None


def lang_of(text: str) -> str:
    """Dominant script: 'ru' or 'en'."""
    c = len(CYR.findall(text))
    lat = len(LAT.findall(text))
    return "en" if lat > c else "ru"


def cue_before(text: str, start: int, window: int = 40) -> str:
    """Lowercased text before position, up to window chars, single-spaced."""
    return " ".join(text[max(0, start - window):start].lower().replace("ё", "е").split())
