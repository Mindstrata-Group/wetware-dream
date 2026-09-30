"""Scoring of the detector against the gold set.

Recall is measured the way it matters for privacy: a gold fragment counts as
caught when every character of it is masked, whatever kind the detector gave
it. A false positive is a masked fragment that overlaps no gold fragment.

    python -m tests.gold.score            # prints the table, lists misses
"""

from __future__ import annotations

import sys
from collections import Counter, defaultdict

from anonymizer.detector import detect
from anonymizer.kinds import DIRECT, Kind
from anonymizer.spans import covered

from .markup import load


def score(name: str):
    rows = load(name)
    total: Counter = Counter()
    caught: Counter = Counter()
    misses: dict[str, list] = defaultdict(list)
    false_pos = 0
    fp_examples: list = []
    residual = 0
    for rid, text, gold in rows:
        det = detect(text)
        residual += det.residual
        for g in gold:
            total[g.kind] += 1
            if covered(det.spans, g.start, g.end):
                caught[g.kind] += 1
            else:
                misses[g.kind].append((rid, g.surface))
        for s in det.spans:
            if not any(s.start < g.end and g.start < s.end for g in gold):
                false_pos += 1
                fp_examples.append((rid, s.kind.value, text[s.start:s.end]))
    return total, caught, misses, false_pos, fp_examples, residual, len(rows)


def main(argv: list[str]) -> None:
    for name in argv or ["gold_ru.jsonl", "gold_en.jsonl"]:
        total, caught, misses, fp, fp_ex, residual, n = score(name)
        print(f"== {name}: {n} texts, residual={residual}, false_positive_spans={fp}")
        for kind in sorted(total):
            direct = "direct" if Kind(kind) in DIRECT else "indirect"
            print(f"  {kind:9s} {direct:8s} {caught[kind]:4d}/{total[kind]:<4d} "
                  f"{100 * caught[kind] / total[kind]:6.2f}%")
        for kind, items in misses.items():
            for rid, surface in items[:15]:
                print(f"  MISS {kind} {rid}: {surface}")
        for rid, kind, surface in fp_ex[:40]:
            print(f"  FP {rid} {kind}: {surface}")


if __name__ == "__main__":
    main(sys.argv[1:])
