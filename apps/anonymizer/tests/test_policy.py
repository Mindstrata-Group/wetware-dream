"""Masking policy file."""

import json

import pytest

from anonymizer import policy
from anonymizer.detector import detect
from anonymizer.kinds import Kind


def write(tmp_path, data) -> str:
    path = tmp_path / "policy.json"
    path.write_text(json.dumps(data), encoding="utf-8")
    return str(path)


def all_kinds(on=True):
    return {k.value: on for k in Kind}


def test_default_masks_everything():
    assert policy.DEFAULT.kinds == frozenset(Kind)


def test_typo_in_key_is_rejected(tmp_path):
    with pytest.raises(ValueError):
        policy.load(write(tmp_path, {"kinds": all_kinds(), "acronym": True}))


def test_every_kind_must_be_listed(tmp_path):
    kinds = all_kinds()
    del kinds["PERSON"]
    with pytest.raises(ValueError):
        policy.load(write(tmp_path, {"kinds": kinds}))


def test_disabled_kind_is_kept(tmp_path):
    kinds = all_kinds()
    kinds["PLACE"] = False
    p = policy.load(write(tmp_path, {"kinds": kinds}))
    text = "Переехали в Екатеринбург, мама Люда осталась"
    kinds_found = {s.kind for s in detect(text, policy=p).spans}
    assert Kind.PLACE not in kinds_found and Kind.PERSON in kinds_found
