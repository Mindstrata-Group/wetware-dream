"""Quality gate on the synthetic gold set.

Direct identifiers must be caught completely; indirect ones (places,
organisations, dates) at least 95 %. Texts without personal data must come
out untouched. The gold files are generated (tests/gold/generate.py) and the
generator must reproduce them byte for byte.
"""

import json

import pytest

from anonymizer.kinds import DIRECT, Kind

from .gold import generate
from .gold.markup import HERE
from .gold.score import score


@pytest.mark.parametrize("name", ["gold_ru.jsonl", "gold_en.jsonl"])
def test_recall_and_precision(name):
    total, caught, misses, false_pos, fp_examples, residual, n = score(name)
    assert residual == 0
    for kind, count in total.items():
        rate = caught[kind] / count
        if Kind(kind) in DIRECT:
            assert rate == 1.0, (kind, misses[kind][:5])
        else:
            assert rate >= 0.95, (kind, misses[kind][:5])
    negative_fp = [e for e in fp_examples if e[0].startswith("negative-")]
    assert negative_fp == [], negative_fp[:5]
    assert false_pos <= n * 0.02, fp_examples[:10]


def test_gold_size():
    ru = (HERE / "gold_ru.jsonl").read_text(encoding="utf-8").splitlines()
    en = (HERE / "gold_en.jsonl").read_text(encoding="utf-8").splitlines()
    assert len(ru) >= 300 and len(en) >= 100


def test_generator_is_deterministic(tmp_path, monkeypatch):
    monkeypatch.setattr(generate, "HERE", tmp_path)
    generate.main()
    for name in ("gold_ru.jsonl", "gold_en.jsonl"):
        fresh = [json.loads(line) for line in (tmp_path / name).read_text(encoding="utf-8").splitlines()]
        committed = [json.loads(line) for line in (HERE / name).read_text(encoding="utf-8").splitlines()]
        assert fresh == committed, f"{name} is stale: run python -m tests.gold.generate"
