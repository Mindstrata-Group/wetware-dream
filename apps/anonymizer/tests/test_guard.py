"""The output guard: adds what detectors missed, fails closed, never leaks."""

import logging

from anonymizer import guard
from anonymizer.detector import detect
from anonymizer.kinds import Kind
from anonymizer.known import Known
from anonymizer.spans import Span

SECRET_PHONE = "+7 912 000-11-22"
SECRET_NAME = "Шмыгло"


def test_guard_adds_uncovered_phone():
    text = f"мой номер {SECRET_PHONE} звоните"
    spans, verdict = guard.enforce(text, [], [])
    assert verdict.added >= 1 and verdict.residual == 0
    start = text.index(SECRET_PHONE)
    assert any(s.start <= start and s.end >= start + len(SECRET_PHONE) for s in spans)


def test_guard_adds_known_value_the_detectors_skipped():
    text = f"Сегодня {SECRET_NAME.lower()} опять не пришёл"
    spans, verdict = guard.enforce(text, [], [Known(Kind.PERSON, SECRET_NAME)])
    assert verdict.residual == 0 and verdict.added >= 1


def test_residual_is_reported_when_masking_cannot_help(monkeypatch):
    # Simulate an inconsistent detector: the blunt scan keeps finding
    # something in the masked text. The verdict must say so.
    real = guard._blunt

    def inconsistent(text, items):
        found = real(text, items)
        if guard.PLACEHOLDER in text:  # the check pass over the masked text
            found.append(Span(len(text) - 2, len(text), Kind.PHONE, "x", "x", "guard"))
        return found

    monkeypatch.setattr(guard, "_blunt", inconsistent)
    _, verdict = guard.enforce(f"мой номер {SECRET_PHONE} ok", [], [])
    assert verdict.residual >= 1 and verdict.residual_kinds == {"PHONE": 1}


def test_verdict_description_never_contains_values(monkeypatch):
    real = guard._blunt
    monkeypatch.setattr(guard, "_blunt", lambda t, i: real(t, i) + [Span(0, 2, Kind.PERSON, SECRET_NAME, "k")])
    _, verdict = guard.enforce(f"{SECRET_NAME} {SECRET_PHONE}", [], [])
    text = verdict.describe()
    assert SECRET_NAME not in text and "912" not in text


def test_detection_logs_nothing_about_content(caplog):
    caplog.set_level(logging.DEBUG)
    detect(f"Мама {SECRET_NAME} звонила с {SECRET_PHONE}", items=[Known(Kind.PERSON, SECRET_NAME)])
    for record in caplog.records:
        msg = record.getMessage()
        assert SECRET_NAME not in msg and "912" not in msg
