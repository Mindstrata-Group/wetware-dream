"""Restoration of real values into the model's answer."""

import pytest

from anonymizer.kinds import Kind
from anonymizer.restore import Entry, restore

LUDA = Entry(Kind.PERSON, 1, "Люда")
PETROV = Entry(Kind.PERSON, 2, "Петров Игорь Викторович")
PETROVA = Entry(Kind.PERSON, 3, "Петрова")
CITY = Entry(Kind.PLACE, 1, "Екатеринбург")
PHONE = Entry(Kind.PHONE, 1, "89123456789")
ENTRIES = [LUDA, PETROV, PETROVA, CITY, PHONE]


@pytest.mark.parametrize("answer,expected", [
    ("Поговорите с ЛИЦО_1 спокойно.", "Поговорите с Людой спокойно."),
    ("Напишите ЛИЦО_1 письмо.", "Напишите Люде письмо."),
    ("У ЛИЦО_1 тоже есть чувства.", "У Люды тоже есть чувства."),
    ("Думаю о ЛИЦО_1 часто.", "Думаю о Люде часто."),
    ("ЛИЦО_1 вас любит.", "Люда вас любит."),
    ("С подругой ЛИЦО_3 вы давно знакомы?", "С подругой Петровой вы давно знакомы?"),
    ("Позвоните ЛИЦО_2 завтра.", "Позвоните Петрову Игорю Викторовичу завтра."),
    ("Вы боитесь ЛИЦО_2?", "Вы боитесь Петрова Игоря Викторовича?"),
    ("Когда вы переехали в МЕСТО_1?", "Когда вы переехали в Екатеринбург?"),
    ("Жизнь в МЕСТО_1 вам нравится?", "Жизнь в Екатеринбурге вам нравится?"),
    ("Из МЕСТО_1 вы уехали давно?", "Из Екатеринбурга вы уехали давно?"),
    ("Наберите ТЕЛЕФОН_1.", "Наберите 89123456789."),
    ("Talk to PERSON_1 tomorrow.", "Talk to Люда tomorrow."),
])
def test_case_from_context(answer, expected):
    assert restore(answer, ENTRIES).text == expected


def test_glued_ending_is_used_when_context_is_silent():
    result = restore("Скажите ЛИЦО_1е спасибо, а ЛИЦО_2у — нет.", ENTRIES)
    assert "Люде" in result.text


def test_unknown_alias_is_marked_not_replaced():
    result = restore("Как дела у ЛИЦО_9?", ENTRIES)
    assert result.unknown == ["ЛИЦО_9"]
    assert "ЛИЦО_9" in result.text and "⟨" in result.text


def test_counts():
    result = restore("С ЛИЦО_1 и ЛИЦО_3 всё сложно, МЕСТО_1 далеко.", ENTRIES)
    assert result.restored == 3 and result.inflected >= 2


def test_context_chooses_the_case_but_is_not_returned():
    # streaming: "Поговорите с " was already sent, the alias arrives next
    result = restore("ЛИЦО_1 спокойно.", ENTRIES, context="Поговорите с ")
    assert result.text == "Людой спокойно."


def test_aliases_in_context_are_not_restored_again():
    result = restore(" и ЛИЦО_3.", ENTRIES, context="Встретьтесь с ЛИЦО_1")
    assert result.text == " и Петровой." and result.restored == 1


def test_alias_inside_word_is_not_touched():
    assert restore("XЛИЦО_1Y", ENTRIES).text == "XЛИЦО_1Y"
