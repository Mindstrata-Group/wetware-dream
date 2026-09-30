"""Small assertions shared by the tests."""

from __future__ import annotations

from anonymizer.detector import detect
from anonymizer.guard import mask_for_check
from anonymizer.spans import covered


def masked(text: str, **kwargs) -> str:
    result = detect(text, **kwargs)
    return mask_for_check(text, result.spans).replace("⟦•⟧", "▇")


def assert_masked(text: str, fragment: str, **kwargs) -> None:
    result = detect(text, **kwargs)
    start = text.index(fragment)
    assert covered(result.spans, start, start + len(fragment)), (
        f"not masked: {fragment!r} -> {mask_for_check(text, result.spans)!r}")


def assert_clear(text: str, fragment: str, **kwargs) -> None:
    result = detect(text, **kwargs)
    start = text.index(fragment)
    end = start + len(fragment)
    hit = [s for s in result.spans if s.start < end and start < s.end]
    assert not hit, f"over-masked: {fragment!r} -> {mask_for_check(text, result.spans)!r}"
