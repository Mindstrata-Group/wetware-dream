"""Natasha NER (slovnet, CPU only) behind a lazy, thread-safe singleton.

Natasha tags PER, LOC and ORG in Russian text. It is trained on news, so it
is one signal among several, never the only one: rules, the name gazetteer
and pymorphy catch what it misses (lowercase names, diminutives, oblique
cases), and it catches unusual surnames the others do not know.

The model is ~30 MB with navec embeddings, loads in about half a second and
takes most of the detection time; set ANONYMIZER_NO_NER=1 to run without it
(tests of single layers do that to stay fast).
"""

from __future__ import annotations

import os
import threading
from dataclasses import dataclass


@dataclass(frozen=True)
class Entity:
    start: int
    end: int
    type: str  # PER, LOC, ORG


class _Natasha:
    def __init__(self) -> None:
        from natasha import Doc, NewsEmbedding, NewsNERTagger, Segmenter

        self._doc = Doc
        self._segmenter = Segmenter()
        self._tagger = NewsNERTagger(NewsEmbedding())
        self._lock = threading.Lock()

    def entities(self, text: str) -> list[Entity]:
        out: list[Entity] = []
        for offset, chunk in _chunks(text, 4000):
            with self._lock:
                doc = self._doc(chunk)
                doc.segment(self._segmenter)
                doc.tag_ner(self._tagger)
                spans = list(doc.spans)
            out.extend(Entity(s.start + offset, s.stop + offset, s.type) for s in spans)
        return out


def _chunks(text: str, size: int):
    pos = 0
    while pos < len(text):
        end = min(len(text), pos + size)
        if end < len(text):
            cut = max(text.rfind("\n", pos + size // 2, end), text.rfind(". ", pos + size // 2, end))
            if cut > pos:
                end = cut + 1
        yield pos, text[pos:end]
        pos = end


_instance: _Natasha | None = None
_init_lock = threading.Lock()


def enabled() -> bool:
    return os.environ.get("ANONYMIZER_NO_NER", "") not in ("1", "true", "yes")


def entities(text: str) -> list[Entity]:
    global _instance
    if not enabled() or not text.strip():
        return []
    if _instance is None:
        with _init_lock:
            if _instance is None:
                _instance = _Natasha()
    return _instance.entities(text)


def warm_up() -> None:
    entities("Иван Петров живёт в Москве и работает в Сбербанке.")
