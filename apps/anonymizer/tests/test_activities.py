"""Irreversible monthly pass: the Temporal activity contract."""

import pytest

from anonymizer.activities import AnonymizeResult, anonymize_texts
from anonymizer.irreversible import anonymize


@pytest.mark.asyncio
async def test_anonymize_texts_returns_result_struct():
    result = await anonymize_texts(["Иван, test@example.com", "Пиши @ivan_petrov"])
    assert isinstance(result, AnonymizeResult)
    assert result.texts[0] == "[ИМЯ], [EMAIL]"
    assert result.texts[1] == "Пиши [КОНТАКТ]"
    assert result.stats.get("EMAIL") == 1
    assert result.stats.get("КОНТАКТ") == 1


@pytest.mark.asyncio
async def test_anonymize_texts_empty_batch():
    result = await anonymize_texts([])
    assert result.texts == [] and result.stats == {}


@pytest.mark.asyncio
async def test_anonymize_texts_preserves_order():
    result = await anonymize_texts(["Привет", "Карта 4276 3801 2345 6787", "ИНН 500100732259"])
    assert result.texts == ["Привет", "Карта [КАРТА]", "ИНН [ДОКУМЕНТ]"]


@pytest.mark.parametrize("text,expected", [
    ("Позвони +7 999 123-45-67", "Позвони [ТЕЛЕФОН]"),
    ("Мой email test@example.com", "Мой email [EMAIL]"),
    ("Живу ул. Ленина д. 10", "Живу [АДРЕС]"),
    ("Привет как дела", "Привет как дела"),
    ("[ИМЯ] [EMAIL] уже заменены", "[ИМЯ] [EMAIL] уже заменены"),
])
def test_legacy_tokens(text, expected):
    assert anonymize(text)[0] == expected


def test_already_anonymized_text_is_stable():
    once, _ = anonymize("Мама Люда звонила с 89123456789")
    twice, _ = anonymize(once)
    assert once == twice
