"""Inline gold markup: "Мама {{PERSON|Люда}} звонила" -> text + spans."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path

MARK = re.compile(r"\{\{([A-Z]+)\|(.+?)\}\}")
HERE = Path(__file__).parent


@dataclass(frozen=True)
class Gold:
    start: int
    end: int
    kind: str
    surface: str


def parse(marked: str) -> tuple[str, list[Gold]]:
    out: list[str] = []
    gold: list[Gold] = []
    cursor = 0
    pos = 0
    for m in MARK.finditer(marked):
        chunk = marked[cursor:m.start()]
        out.append(chunk)
        pos += len(chunk)
        surface = m.group(2)
        gold.append(Gold(pos, pos + len(surface), m.group(1), surface))
        out.append(surface)
        pos += len(surface)
        cursor = m.end()
    out.append(marked[cursor:])
    return "".join(out), gold


def load(name: str) -> list[tuple[str, str, list[Gold]]]:
    rows = []
    with open(HERE / name, encoding="utf-8") as fh:
        for line in fh:
            row = json.loads(line)
            text, gold = parse(row["text"])
            rows.append((row["id"], text, gold))
    return rows
