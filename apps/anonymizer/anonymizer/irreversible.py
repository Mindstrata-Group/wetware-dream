"""The monthly irreversible pass: replace personal data with generic tokens.

Old dialog messages are rewritten in place by the Go workflow
(apps/api/internal/anonymizer); this module does the text part. It uses the
same detection as the reversible service, only the replacement differs:
every fragment becomes a fixed token ("[ИМЯ]", "[ТЕЛЕФОН]") and nothing is
kept to restore it. A residual from the guard is not an error here: the
fragment the guard still sees is masked with the generic token too.
"""

from __future__ import annotations

from .detector import detect
from .kinds import LEGACY_TOKENS
from .spans import apply


def anonymize(text: str) -> tuple[str, dict[str, int]]:
    if not text.strip():
        return text, {}
    result = detect(text)
    masked = apply(text, result.spans, lambda s: LEGACY_TOKENS[s.kind])
    stats: dict[str, int] = {}
    for s in result.spans:
        token = LEGACY_TOKENS[s.kind].strip("[]")
        stats[token] = stats.get(token, 0) + 1
    return masked, stats
