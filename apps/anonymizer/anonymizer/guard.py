"""Output guard: a second, independent look before anything leaves.

After the detectors have chosen their spans the guard scans the original
text with blunt rules (long digit runs, anything with "@" inside a word,
the user's known values, the strict format rules) and adds whatever is not
covered yet. Then it masks the text and scans the result once more; a
finding in the masked text means detection is inconsistent, and the caller
must refuse to send the message rather than send it as is (fail closed).

The guard reports counts and kinds only. It never returns, logs or formats
the values it found: the original guard of this design put the value into
its verdict string and from there into the service log.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

from . import known as known_mod
from . import rules, spoken
from .kinds import Kind
from .spans import Span, covered, resolve

_DIGITS = re.compile(r"(?<![\w])\+?\d(?:[\s\-().]{0,2}\d){8,}(?![\w])")
_AT_WORD = re.compile(r"[\w.%+\-]+\s?@\s?[\w\-]+(?:\.[\w\-]+)+")
PLACEHOLDER = "⟦•⟧"


@dataclass
class Verdict:
    added: int = 0
    residual: int = 0
    residual_kinds: dict[str, int] = field(default_factory=dict)

    def describe(self) -> str:
        # Counts and kinds only — never values.
        kinds = ",".join(f"{k}:{n}" for k, n in sorted(self.residual_kinds.items()))
        return f"added={self.added} residual={self.residual} kinds=[{kinds}]"


def _blunt(text: str, items: list[known_mod.Known]) -> list[Span]:
    out: list[Span] = []
    for m in _DIGITS.finditer(text):
        d = re.sub(r"\D", "", m.group(0))
        kind = Kind.PHONE if len(d) in (10, 11) and d[-10] in "3456789" else Kind.DOCUMENT
        out.append(Span(m.start(), m.end(), kind, d, d[-10:] if kind == Kind.PHONE else "doc:" + d, "guard"))
    for m in _AT_WORD.finditer(text):
        v = re.sub(r"\s", "", m.group(0)).lower()
        out.append(Span(m.start(), m.end(), Kind.EMAIL, v, v, "guard"))
    out.extend(rules.phones(text))
    out.extend(rules.emails(text))
    out.extend(rules.links(text))
    out.extend(rules.documents(text))
    out.extend(spoken.numbers(text))
    out.extend(known_mod.find(text, items))
    return out


def mask_for_check(text: str, spans: list[Span]) -> str:
    out = []
    cursor = 0
    for s in spans:
        out.append(text[cursor:s.start])
        out.append(PLACEHOLDER)
        cursor = s.end
    out.append(text[cursor:])
    return "".join(out)


def enforce(text: str, spans: list[Span], items: list[known_mod.Known],
            protected: list[tuple[int, int]] | None = None) -> tuple[list[Span], Verdict]:
    """Add uncovered findings, then count what survives masking.

    `protected` ranges are existing placeholders (aliases, legacy tokens);
    findings that touch them are ignored.
    """
    verdict = Verdict()
    current = list(spans)
    keep_out = protected or []

    def free(s: Span) -> bool:
        return not any(s.start < b and a < s.end for a, b in keep_out)

    for _ in range(3):
        extra = [s for s in _blunt(text, items) if free(s) and not covered(current, s.start, s.end)]
        if not extra:
            break
        verdict.added += len(extra)
        current = resolve(text, current + extra)
    masked = mask_for_check(text, current)
    leftovers = [s for s in _blunt(masked, items) if PLACEHOLDER not in masked[s.start:s.end]]
    verdict.residual = len(leftovers)
    for s in leftovers:
        verdict.residual_kinds[s.kind.value] = verdict.residual_kinds.get(s.kind.value, 0) + 1
    return current, verdict
