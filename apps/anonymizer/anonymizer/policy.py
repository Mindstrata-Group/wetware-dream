"""Masking policy: which kinds are masked and how strictly.

The default (policy.json next to this file) is strict for a therapy
context: every kind is masked, exact dates too; only years, months and
countries stay. An installation can pass its own policy file through
ANONYMIZER_POLICY_FILE; unknown keys are rejected so a typo cannot silently
switch masking off.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from pathlib import Path

from .kinds import Kind

_DEFAULT_FILE = Path(__file__).with_name("policy.json")
_ALLOWED_KEYS = {"kinds", "acronyms", "comment"}


@dataclass(frozen=True)
class Policy:
    kinds: frozenset[Kind] = field(default_factory=lambda: frozenset(Kind))
    acronyms: bool = True

    def enabled(self, kind: Kind) -> bool:
        return kind in self.kinds


def load(path: str | os.PathLike | None = None) -> Policy:
    path = path or os.environ.get("ANONYMIZER_POLICY_FILE") or _DEFAULT_FILE
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    unknown = set(raw) - _ALLOWED_KEYS
    if unknown:
        raise ValueError(f"unknown policy keys: {sorted(unknown)}")
    kinds = frozenset(Kind(k) for k, on in raw.get("kinds", {}).items() if on)
    missing = set(Kind) - {Kind(k) for k in raw.get("kinds", {})}
    if missing:
        raise ValueError(f"policy must list every kind explicitly, missing: {sorted(k.value for k in missing)}")
    return Policy(kinds=kinds, acronyms=bool(raw.get("acronyms", True)))


DEFAULT = load(_DEFAULT_FILE)
