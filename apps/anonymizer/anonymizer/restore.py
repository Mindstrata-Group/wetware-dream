"""Putting real values back into the model's answer.

The model sees "позвони ЛИЦО_1" and answers "поговорите с ЛИЦО_1 спокойно".
Restoration replaces the alias with the stored value in the case the
sentence asks for: "поговорите с Людой спокойно". The case is read from the
text the model wrote, not from the original message: the model rephrases,
and only its own sentence knows which case is needed.

How the case is chosen, first match wins:

1. a preposition right before the alias ("с" -> instrumental, "к" -> dative);
2. a noun in an oblique case right before it, the alias being its
   apposition ("с подругой ЛИЦО_2" -> instrumental, like "подругой");
3. a verb with unambiguous government ("позвонить" -> dative);
4. an ending the model glued to the alias ("ЛИЦО_1у", "ЛИЦО_1ой");
5. otherwise nominative — exactly what a plain replacement would give, so a
   miss never makes the text worse.

Aliases the model invented (no such entry for this user) are not replaced
with anything; they are marked and counted.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from . import decline, morph
from .kinds import Kind, kind_by_label

_LABELS = kind_by_label()
ALIAS_RE = re.compile(
    r"(?<![A-Za-zА-Яа-яЁё0-9_])(" + "|".join(sorted(map(re.escape, _LABELS), key=len, reverse=True)) +
    r")_(\d{1,5})([а-яё]{1,3})?(?![A-Za-zА-Яа-яЁё0-9_])"
)

PREPOSITIONS = {
    "без": "gent", "у": "gent", "от": "gent", "для": "gent", "из": "gent", "до": "gent",
    "около": "gent", "возле": "gent", "кроме": "gent", "вместо": "gent", "против": "gent",
    "среди": "gent", "ради": "gent", "мимо": "gent", "после": "gent", "насчет": "gent",
    "из-за": "gent", "помимо": "gent", "относительно": "gent", "со стороны": "gent",
    "к": "datv", "ко": "datv", "благодаря": "datv", "вопреки": "datv", "навстречу": "datv",
    "по": "datv", "про": "accs", "через": "accs", "на": "accs", "за": "accs",
    "с": "ablt", "со": "ablt", "над": "ablt", "перед": "ablt", "между": "ablt", "рядом с": "ablt",
    "вместе с": "ablt", "о": "loct", "об": "loct", "обо": "loct", "при": "loct",
}
# Verbs with one possible case for a person object. Kept short on purpose:
# a wrong verb here is a confident mistake where a harmless nominative was.
VERBS = {
    "datv": ("позвон", "звон", "напиш", "написа", "пиш", "скаж", "сказа", "говор", "сообщ",
             "переда", "отправ", "ответ", "помог", "помоч", "довер", "объясн", "признай",
             "признат", "пожалуй", "жалова", "улыбн", "подари", "дари", "купи", "обеща", "предлож"),
    "accs": ("спроси", "спраш", "поблагодар", "благодар", "обнимит", "обнял", "обним", "позов",
             "пригласи", "приглаш", "поддерж", "выслуш", "слуша", "попрос", "прост", "прощ",
             "уважа", "люб", "ненавид", "понима", "поним", "вижу", "видит", "встрет", "ждет", "жду",
             "ждал", "ждать", "бои", "боит", "обид", "ценит", "цени"),
    "ablt": ("поговор", "разговар", "общат", "общал", "обща", "встрет", "увид", "познаком",
             "дружи", "поссор", "мири", "помири", "советова", "посовет", "гордит", "горжус",
             "восхища"),
}
# Case by a suffix glued to an alias, depending on gender. Only where one
# reading is possible.
GLUED = {
    "masc": {"а": "gent", "я": "gent", "у": "datv", "ю": "datv", "ом": "ablt", "ем": "ablt",
             "ым": "ablt", "е": "loct"},
    "femn": {"ы": "gent", "и": "gent", "у": "accs", "ю": "accs", "ой": "ablt", "ей": "ablt"},
}
UNKNOWN_MARK = "⟨{}⟩"


@dataclass
class Entry:
    kind: Kind
    number: int
    canon: str


@dataclass
class Restored:
    text: str
    restored: int = 0
    inflected: int = 0
    unknown: list[str] = field(default_factory=list)


def _words_before(text: str, pos: int, n: int = 3) -> list[str]:
    left = text[max(0, pos - 80):pos]
    if re.search(r"[.!?\n:;]\s*$", left):
        return []
    left = re.split(r"[.!?\n:;]", left)[-1]
    return [w.lower().replace("ё", "е") for w in re.findall(r"[A-Za-zА-Яа-яЁё\-]+", left)][-n:]


_MOTION = ("переех", "переезж", "поех", "поед", "уех", "уезж", "приех", "приезж", "вернул", "верн",
           "лет", "полет", "улет", "еду", "едем", "едет", "ехать", "съезд", "съезж", "отправ", "перебра",
           "попал", "поступ", "пошл", "пошел", "пошла", "иду", "идти", "ход")


def place_case(text: str, pos: int) -> str | None:
    """"в"/"на" before a place: accusative after motion, locative otherwise."""
    words = _words_before(text, pos, 4)
    if not words or words[-1] not in ("в", "во", "на"):
        return None
    if any(w.startswith(_MOTION) for w in words[:-1]):
        return "accs"
    return "loct"


def context_case(text: str, pos: int) -> str | None:
    words = _words_before(text, pos)
    if not words:
        return None
    last = words[-1]
    if len(words) >= 2 and f"{words[-2]} {last}" in PREPOSITIONS:
        return PREPOSITIONS[f"{words[-2]} {last}"]
    if last in PREPOSITIONS:
        return PREPOSITIONS[last]
    # apposition to a noun in an oblique case ("с подругой ЛИЦО_2")
    ps = morph.parses(last)
    if ps and ps[0].tag.POS == "NOUN" and ps[0].tag.animacy == "anim" and ps[0].tag.case in (
            "gent", "datv", "accs", "ablt", "loct") and ps[0].score >= 0.3:
        if len(words) >= 2 and words[-2] in PREPOSITIONS:
            return PREPOSITIONS[words[-2]] if PREPOSITIONS[words[-2]] != "accs" else ps[0].tag.case
        return ps[0].tag.case
    for case, stems in VERBS.items():
        if last.startswith(stems) and morph.is_verb_like(last):
            return case
    return None


def _inflect_person(canon: str, case: str) -> str:
    tokens = canon.split()
    gender = decline.person_gender(tokens)
    out = []
    for i, tok in enumerate(tokens):
        role = morph.person_role(tok)
        if role is None:
            # position heuristics: "Surname Name Patronymic" or "Name Surname"
            role = "Patr" if re.search(r"(вич|вна|чна)$", tok.lower()) else "Name"
        out.append(decline.inflect(tok, case, role, gender))
    return " ".join(out)


def _inflect_place(canon: str, case: str) -> str:
    words = canon.split()
    out = []
    for w in words:
        p = next((p for p in morph.parses(w) if "Geox" in p.tag), None)
        if p is None:
            out.append(w)
            continue
        infl = p.inflect({case})
        out.append(decline._cap(w, infl.word) if infl else w)
    return " ".join(out)


def restore(text: str, entries: list[Entry], context: str = "") -> Restored:
    """Restore aliases in text. `context` is the raw model text right before
    it (streaming: what was already emitted); it is used to choose cases and
    is not part of the result."""
    table = {(e.kind, e.number): e for e in entries}
    result = Restored(text="")
    offset = len(context)
    text = context + text
    out: list[str] = []
    cursor = offset
    prev_end, prev_case = -1, None
    for m in ALIAS_RE.finditer(text):
        label, number, glued = m.group(1), int(m.group(2)), m.group(3) or ""
        kind = _LABELS[label]
        entry = table.get((kind, number))
        case = None
        if entry is not None and kind in (Kind.PERSON, Kind.PLACE):
            case = place_case(text, m.start()) if kind == Kind.PLACE else None
            # coordination: "с ЛИЦО_1 и ЛИЦО_3" — the second takes the case of the first
            if case is None and prev_case and re.fullmatch(r"\s*(?:,|и|или|,\s*а\s+также)\s*",
                                                            text[prev_end:m.start()]):
                case = prev_case
            case = case or context_case(text, m.start())
            if case is None and glued:
                gender = decline.person_gender(entry.canon.split()) if kind == Kind.PERSON else "masc"
                case = GLUED.get(gender or "masc", {}).get(glued)
            prev_end, prev_case = m.end(), case
        if m.start() < offset:
            continue  # already emitted; only its case matters for what follows
        out.append(text[cursor:m.start()])
        cursor = m.end()
        if entry is None:
            result.unknown.append(f"{label}_{number}")
            out.append(UNKNOWN_MARK.format(f"{label}_{number}") + glued)
            continue
        value = entry.canon
        if kind in (Kind.PERSON, Kind.PLACE):
            if case and case != "nomn":
                value = _inflect_person(value, case) if kind == Kind.PERSON else _inflect_place(value, case)
                if value != entry.canon:
                    result.inflected += 1
        elif glued:
            value += glued  # a phone or a date does not decline; keep the model's text
        result.restored += 1
        out.append(value)
    out.append(text[cursor:])
    result.text = "".join(out)
    return result


def parse_entries(raw: list[dict] | None) -> list[Entry]:
    entries: list[Entry] = []
    for item in raw or []:
        try:
            kind = Kind(str(item.get("kind", "")).upper())
            number = int(item.get("number"))
        except (ValueError, TypeError):
            continue
        canon = str(item.get("canon", ""))
        if canon:
            entries.append(Entry(kind, number, canon))
    return entries
