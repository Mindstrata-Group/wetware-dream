"""Temporal activity of the monthly irreversible pass.

The contract (activity name, argument, result fields) is shared with the Go
workflow in apps/api/internal/anonymizer and must not change.
"""

from __future__ import annotations

import dataclasses

from temporalio import activity

from .irreversible import anonymize


def _heartbeat(details: int) -> None:
    try:
        activity.heartbeat(details)
    except RuntimeError:
        pass  # called outside an activity context (tests)


@dataclasses.dataclass
class AnonymizeResult:
    texts: list[str]
    stats: dict[str, int]  # replacement token -> count over the whole batch


@activity.defn(name="anonymize_texts")
async def anonymize_texts(texts: list[str]) -> AnonymizeResult:
    result_texts: list[str] = []
    batch_stats: dict[str, int] = {}
    for index, text in enumerate(texts, start=1):
        masked, stats = anonymize(text)
        result_texts.append(masked)
        for key, count in stats.items():
            batch_stats[key] = batch_stats.get(key, 0) + count
        if index % 50 == 0:
            _heartbeat(index)
    _heartbeat(len(texts))
    return AnonymizeResult(texts=result_texts, stats=batch_stats)
