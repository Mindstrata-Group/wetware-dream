"""The detection pipeline: all detectors, overlap resolution, the guard.

    detect(text, lang, known) -> Detection(spans, residual, lang, stats)

Detectors run independently over the original text; their spans are merged
(spans.resolve) and handed to the guard, which adds anything a blunt second
look still finds and reports what survives masking. A non-zero residual
means the caller must not send the text anywhere.

Language: "ru", "en" or "auto". Detection does not switch detectors by
language — Russian and Latin-script detectors both run on every text, so
mixed messages ("созвонились с Kate из Deloitte") are covered. The language
only chooses the alias labels (ЛИЦО_1 or PERSON_1).
"""

from __future__ import annotations

import re
import time
from dataclasses import dataclass, field

from . import guard, known as known_mod, ner, people, people_en, places, rules, spoken
from .kinds import LEGACY_TOKENS, Kind
from .restore import ALIAS_RE
from .policy import DEFAULT, Policy
from .spans import Span, resolve
from .text import CYR, lang_of


@dataclass
class Detection:
    spans: list[Span]
    residual: int
    lang: str
    stats: dict[str, int] = field(default_factory=dict)
    guard_added: int = 0
    elapsed_ms: float = 0.0


_LEGACY = re.compile("|".join(re.escape(t) for t in LEGACY_TOKENS.values()))


def protected(text: str) -> list[tuple[int, int]]:
    """Ranges that are already placeholders: legacy tokens and aliases.

    Detection must leave them alone, so masking an already masked text is a
    no-op and an alias the model echoes back is not masked a second time.
    """
    ranges = [(m.start(), m.end()) for m in _LEGACY.finditer(text)]
    ranges += [(m.start(), m.end()) for m in ALIAS_RE.finditer(text)]
    return ranges


def _outside(spans: list[Span], ranges: list[tuple[int, int]]) -> list[Span]:
    if not ranges:
        return spans
    return [s for s in spans if not any(s.start < b and a < s.end for a, b in ranges)]


def candidates(text: str, items: list[known_mod.Known], policy: Policy = DEFAULT) -> list[Span]:
    entities = ner.entities(text) if CYR.search(text) else []
    out: list[Span] = []
    out.extend(rules.all_rules(text))
    out.extend(spoken.all_spoken(text))
    out.extend(people.find(text, entities))
    out.extend(people_en.find(text))
    out.extend(places.addresses(text))
    out.extend(places.dates(text))
    orgs = places.organisations(text, entities)
    if not policy.acronyms:
        orgs = (s for s in orgs if s.source != "org:acronym")
    out.extend(orgs)
    out.extend(places.places(text, entities))
    out.extend(places.places_en(text))
    out.extend(known_mod.find(text, items))
    out = places.expand_quotes(text, out)
    return _outside([s for s in out if policy.enabled(s.kind)], protected(text))


def detect(text: str, lang: str = "auto", items: list[known_mod.Known] | None = None,
           policy: Policy = DEFAULT) -> Detection:
    t0 = time.perf_counter()
    items = items or []
    if lang not in ("ru", "en"):
        lang = lang_of(text)
    spans = resolve(text, candidates(text, items, policy))
    spans, verdict = guard.enforce(text, spans, items, protected(text))
    stats: dict[str, int] = {}
    for s in spans:
        stats[s.kind.value] = stats.get(s.kind.value, 0) + 1
    return Detection(
        spans=spans,
        residual=verdict.residual,
        lang=lang,
        stats=stats,
        guard_added=verdict.added,
        elapsed_ms=(time.perf_counter() - t0) * 1000,
    )


def parse_known(raw: list[dict] | None) -> list[known_mod.Known]:
    items: list[known_mod.Known] = []
    for entry in raw or []:
        try:
            kind = Kind(str(entry.get("kind", "")).upper())
        except ValueError:
            continue
        value = str(entry.get("value", ""))
        if not value.strip():
            continue
        items.append(known_mod.Known(kind, value, str(entry.get("canon", "")), str(entry.get("key", ""))))
    return items
