"""People written in Latin script: English names and transliterated Russian ones.

No NER model: a listed first name (names_en), a capitalized surname after it,
a title ("Dr. Smith", "Mrs Brown") or a relation cue ("my wife Sarah",
"my boss, Tom Baker") make a person. This runs on every text, because Latin
names show up inside Russian messages too ("мой терапевт — Dr. Anna Klein").
"""

from __future__ import annotations

import re

from . import names_en
from .kinds import Kind
from .spans import Span
from .text import sentence_initial

_CAP = r"[A-Z][a-z]+(?:[-'’][A-Z]?[a-z]+)?"
RE_TITLE = re.compile(
    r"\b(?:Mr|Mrs|Ms|Miss|Mx|Dr|Prof|Professor|Doctor|Sir|Madam|Aunt|Uncle|Coach|Nurse)\.?\s+("
    + _CAP + r"(?:\s+" + _CAP + r")?)"
)
RE_CAP_WORD = re.compile(_CAP)
RELATION_EN = (
    "mom", "mum", "mother", "dad", "father", "husband", "wife", "son", "daughter", "brother",
    "sister", "grandma", "grandmother", "grandpa", "grandfather", "aunt", "uncle", "cousin",
    "friend", "boyfriend", "girlfriend", "partner", "fiance", "fiancee", "ex", "boss", "manager",
    "colleague", "coworker", "co-worker", "neighbor", "neighbour", "teacher", "therapist",
    "psychologist", "psychiatrist", "doctor", "counselor", "counsellor", "stepmom", "stepdad",
    "stepmother", "stepfather", "mother-in-law", "father-in-law", "roommate", "classmate",
    "niece", "nephew", "kid", "child", "baby", "bestie", "bff", "supervisor", "mentor", "coach",
    "nanny", "client", "patient", "named", "called",
)
RE_RELATION = re.compile(
    r"(?i)\b(?:" + "|".join(re.escape(r) for r in RELATION_EN) + r")s?\b[,:]?\s+(?:is\s+|was\s+)?"
    r"([A-Za-z][a-z]+(?:\s+[A-Z][a-z]+)?)"
)
# Capitalized words that are not surnames after a first name.
_NOT_SURNAME = frozenset("""
I I'm I've I'll I'd The A An And But Or So If When While Because Then Than That This These
Those My Your His Her Our Their Its We You He She They It Me Him Us Them Is Are Was Were Be
Been Have Has Had Do Does Did Not No Yes Ok Okay Hi Hello Hey Thanks Thank Please Sorry Well
Monday Tuesday Wednesday Thursday Friday Saturday Sunday January February March April May June
July August September October November December Christmas Easter God Jesus Mom Dad Mum
""".split())


def _surname_after(text: str, end: int) -> int:
    m = re.match(r"[ \t]{1,2}(" + _CAP + r")", text[end:end + 40])
    if m and m.group(1) not in _NOT_SURNAME:
        return end + m.end(1)
    return end


def canonical(value: str) -> tuple[str, str]:
    canon = " ".join(value.split())
    return canon, canon.lower()


def find(text: str) -> list[Span]:
    ranges: list[tuple[int, int]] = []
    for m in re.finditer(r"[A-Za-z][a-z]+(?:[-'’][A-Za-z]?[a-z]+)?", text):
        word = m.group(0)
        if not names_en.is_first_name(word):
            continue
        cap = word[:1].isupper()
        if not cap:
            continue  # lowercase names count only after a relation cue (below)
        if names_en.ambiguous(word) and sentence_initial(text, m.start()):
            # "May I ...", "Will you ..." at the start of a sentence
            nxt = re.match(r"[ \t]{1,2}(" + _CAP + r")", text[m.end():m.end() + 30])
            if not nxt or nxt.group(1) in _NOT_SURNAME:
                continue
        if names_en.ambiguous(word):
            prev = text[max(0, m.start() - 12):m.start()].lower()
            if re.search(r"\b(?:in|of|by|since|until|early|late|next|last|this)\s+$", prev) and word in (
                    "May", "June", "April", "August", "Summer", "Will", "Grace"):
                continue
        ranges.append((m.start(), _surname_after(text, m.end())))
    for m in RE_TITLE.finditer(text):
        ranges.append((m.start(), m.end()))
    for m in RE_RELATION.finditer(text):
        name = m.group(1)
        first = name.split()[0]
        if first.lower() in ("a", "an", "the", "is", "was", "and", "who", "that", "said", "told",
                             "has", "had", "will", "would", "can", "does", "did", "not", "just",
                             "always", "never", "also", "really", "still", "gave", "got", "took"):
            continue
        if not (first[:1].isupper() or names_en.is_first_name(first)):
            continue
        if first[:1].islower() and not names_en.is_first_name(first):
            continue
        end = m.start(1) + len(name) if name.split()[-1][:1].isupper() or len(name.split()) == 1 else m.start(1) + len(first)
        ranges.append((m.start(1), end))
    spans = []
    for start, end in ranges:
        canon, key = canonical(text[start:end])
        spans.append(Span(start, end, Kind.PERSON, canon, key, "people_en"))
    return spans
