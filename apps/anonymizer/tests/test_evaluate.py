"""The real-data evaluator must print aggregates only."""

import csv
import io
import json

from anonymizer import evaluate

ROWS = [
    ("1", "10", "Меня зовут Злата, пишите на zlata.k@mail.ru или 8 912 000 11 22", "Злата Крапивина",
     "zlata.k@mail.ru", "+79120001122", "zlata_k"),
    ("1", "11", "Сегодня тревожно, Крапивиной звонить не хочу", "Злата Крапивина", "zlata.k@mail.ru",
     "+79120001122", "zlata_k"),
    ("2", "12", "Солнышко светит, а мне грустно", "Солнышко", "", "", ""),
]
SECRETS = ["Злата", "Крапивин", "zlata", "912", "0001122", "Солнышко", "грустно", "тревожно"]


def _csv(rows) -> io.StringIO:
    buf = io.StringIO()
    writer = csv.writer(buf)
    for row in rows:
        writer.writerow(row)
    buf.seek(0)
    return buf


def test_report_contains_no_input_text():
    report = evaluate.run(_csv(ROWS))
    dumped = json.dumps(report, ensure_ascii=False)
    for secret in SECRETS:
        assert secret.lower() not in dumped.lower(), secret


def test_profile_oracle_with_profile_is_complete():
    report = evaluate.run(_csv(ROWS))
    assert report["messages"] == 3 and report["users"] == 2
    for cat, o in report["oracle"].items():
        assert o["residual_with_profile"] == 0, cat
    assert report["oracle"]["name"]["occurrences"] >= 2
    assert report["oracle"]["email"]["occurrences"] == 1
    assert report["strict_hits_with_profile"] == 0


def test_errors_are_counted_by_type_only(monkeypatch):
    def boom(*a, **k):
        raise RuntimeError("Злата Крапивина")

    monkeypatch.setattr(evaluate, "detect", boom)
    report = evaluate.run(_csv(ROWS[:1]))
    assert report["errors"] == {"RuntimeError": 1}
    assert "Злата" not in json.dumps(report, ensure_ascii=False)
