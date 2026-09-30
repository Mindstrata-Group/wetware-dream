"""Detected fragments and overlap resolution.

Every detector works independently and emits spans over the original text.
The pipeline merges them: overlapping spans are resolved by kind priority,
then by length.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable

from .kinds import PRIORITY, Kind


@dataclass(frozen=True)
class Span:
    start: int
    end: int
    kind: Kind
    # Canonical display form stored in the vault and returned on restore
    # (a person in the nominative case, a phone as digits, ...).
    canon: str
    # Normalized identity key: two spans with the same kind and key get the
    # same alias for a user.
    key: str
    source: str = "rule"

    @property
    def length(self) -> int:
        return self.end - self.start

    def as_dict(self) -> dict:
        return {
            "start": self.start,
            "end": self.end,
            "kind": self.kind.value,
            "canon": self.canon,
            "key": self.key,
            "source": self.source,
        }


def resolve(text: str, spans: Iterable[Span]) -> list[Span]:
    """Merge overlapping spans into disjoint ones.

    Overlapping spans form a cluster. The cluster keeps the kind, canon and key
    of its winner (higher priority, then longer), but its bounds are the union
    of all members: dropping a loser would leave its uncovered tail in clear
    text, and recall matters more than a tidy canonical form. When the union
    is wider than the winner, the canon falls back to the raw union text.
    """
    items = sorted((s for s in spans if s.end > s.start), key=lambda s: (s.start, -s.end))
    clusters: list[list[Span]] = []
    end = -1
    for span in items:
        if clusters and span.start < end:
            clusters[-1].append(span)
            end = max(end, span.end)
        else:
            clusters.append([span])
            end = span.end
    out: list[Span] = []
    for cluster in clusters:
        winner = min(cluster, key=lambda s: (-PRIORITY[s.kind], -s.length, s.start))
        start = min(s.start for s in cluster)
        stop = max(s.end for s in cluster)
        if (start, stop) == (winner.start, winner.end):
            out.append(winner)
            continue
        raw = " ".join(text[start:stop].split())
        out.append(Span(start, stop, winner.kind, raw, raw.lower().replace("ё", "е"), "merged"))
    return out


def covered(spans: list[Span], start: int, end: int) -> bool:
    """Is [start, end) fully inside the union of spans?"""
    pos = start
    for span in sorted(spans, key=lambda s: s.start):
        if span.end <= pos:
            continue
        if span.start > pos:
            return False
        pos = span.end
        if pos >= end:
            return True
    return pos >= end


def apply(text: str, spans: list[Span], render) -> str:
    """Replace spans (already resolved, sorted, disjoint) with render(span)."""
    out: list[str] = []
    cursor = 0
    for span in spans:
        out.append(text[cursor:span.start])
        out.append(render(span))
        cursor = span.end
    out.append(text[cursor:])
    return "".join(out)
