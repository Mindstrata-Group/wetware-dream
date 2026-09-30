"""Identifiers written out in words."""

import pytest

from anonymizer import spoken
from anonymizer.kinds import Kind


@pytest.mark.parametrize("text,digits", [
    ("восемь девятьсот двенадцать триста сорок пять шестьдесят семь восемьдесят девять", "89123456789"),
    ("плюс семь девятьсот девяносто девять сто двадцать три ноль ноль один один", "79991230011"),
    ("восемь девять один шесть пять пять пять ноль ноль один два", "89165550012"),
    ("five five five, one two three, four five six seven", "5551234567"),
    ("double five five one two three four five six seven", "5551234567"),
])
def test_spoken_phone(text, digits):
    spans = [s for s in spoken.numbers(text) if s.kind == Kind.PHONE]
    assert spans and spans[0].canon == digits
    assert spans[0].start == 0 and spans[0].end == len(text)


@pytest.mark.parametrize("text", [
    "сто двадцать тысяч рублей в месяц",
    "мне тридцать пять лет и двое детей",
    "one of my two friends",
])
def test_amounts_and_counts_stay(text):
    assert list(spoken.numbers(text)) == []


def test_long_dictated_number_is_masked_as_document():
    text = "номер полиса один два три четыре пять шесть семь восемь девять ноль"
    spans = list(spoken.numbers(text))
    assert spans and spans[0].kind == Kind.DOCUMENT


@pytest.mark.parametrize("text,value", [
    ("почта иван точка петров собака мейл точка ру", "иван.петров@мейл.ru"),
    ("ivan dot petrov at gmail dot com", "ivan.petrov@gmail.com"),
    ("kotik нижнее подчеркивание 88 собака yandex точка ru", "kotik_88@yandex.ru"),
])
def test_spoken_email(text, value):
    spans = list(spoken.emails(text))
    assert spans and spans[0].canon == value


def test_not_an_email():
    assert list(spoken.emails("я была у мамы at home, точка")) == []
