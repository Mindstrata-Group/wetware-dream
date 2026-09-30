"""Format rules: phones, e-mails, links, handles, documents."""

import pytest

from anonymizer import rules
from anonymizer.kinds import Kind
from tests.helpers import assert_clear, assert_masked


@pytest.mark.parametrize("phone", [
    "+7 912 345-67-89", "8 (912) 345-67-89", "89123456789", "+79123456789", "8-912-345-67-89",
    "912 345 67 89", "+7 (343) 385 00 19", "8 912 345 6789", "+44 7911 123456", "+1 415 555 0132",
    "(555) 123-4567", "555-867-5309",
])
def test_phone_shapes(phone):
    assert_masked(f"Звоните: {phone}, я на связи.", phone)


def test_local_number_needs_a_phone_word():
    assert_masked("тел. 45-67-89 после шести", "45-67-89")
    assert_clear("код 45-67-89 на коробке", "45-67-89")


@pytest.mark.parametrize("text,fragment", [
    ("Номер заказа 1234567890 пришёл", "1234567890"),
    ("Лекарство 0.75 мг утром", "0.75"),
    ("Мне 35 лет, ипотека на 20 лет", "35"),
    ("8 800 555-35-35 горячая линия", "8 800 555-35-35"),
])
def test_not_a_personal_phone(text, fragment):
    spans = [s for s in rules.phones(text) if s.kind == Kind.PHONE and text[s.start:s.end] == fragment]
    assert not spans


@pytest.mark.parametrize("email", [
    "yulia.k@mail.ru", "kotik_88@gmail.com", "i.ivanova@yandex.ru", "a-b.c+tag@sub.example.co.uk",
])
def test_emails(email):
    assert_masked(f"пишите на {email} вечером", email)


def test_email_glued_to_cyrillic():
    text = "почтаyulia.k@mail.ruспасибо"
    spans = list(rules.emails(text))
    assert spans and text[spans[0].start:spans[0].end] == "yulia.k@mail.ru"


@pytest.mark.parametrize("link", [
    "https://vk.com/id12345678", "vk.com/masha_art", "t.me/serega_ekb", "instagram.com/natali.psy",
    "https://docs.google.com/document/d/1AbCdEf/edit", "https://example.com/reset?token=abc123",
])
def test_links(link):
    assert_masked(f"вот ссылка {link} смотрите", link)


def test_reference_links_and_bare_domains_stay():
    assert_clear("читала статью на https://ru.wikipedia.org/wiki/Тревога вчера", "wikipedia")
    assert_clear("нашла в google.com и на ozon.ru", "google.com")
    assert_clear("нашла в google.com и на ozon.ru", "ozon.ru")


@pytest.mark.parametrize("link", [
    "https://example.com/kotik88", "kotik88.tilda.ws", "https://myblog.ru/about", "www.site.ru/u/kotik",
])
def test_personal_links_on_any_domain(link):
    assert_masked(f"вот моя страница {link}, заходите", link)


def test_email_domain_is_not_a_link():
    spans = [s for s in rules.links("пишите на ivan@mail.ru") if s.kind == Kind.URL]
    assert spans == []


def test_handles():
    assert_masked("пишите в тг @kot_begemot", "@kot_begemot")
    assert_masked("мой ник в инсте natali.psy", "natali.psy")
    assert_clear("кот@ дома", "кот")


def test_snils_checksum_or_cue():
    assert_masked("СНИЛС 112-233-445 95", "112-233-445 95")
    assert [s for s in rules.documents("код 112-233-445 00 на коробке")] == []


def test_inn_card_account_passport():
    assert_masked("ИНН 500100732259", "500100732259")
    assert_masked("карта 4276 3801 2345 6787", "4276 3801 2345 6787")
    assert_masked("р/с 40817810099910004312", "40817810099910004312")
    assert_masked("паспорт 4510 123456", "4510 123456")
    assert_masked("My SSN is 123-45-6789", "123-45-6789")
    # without a document word the rule stays silent (the guard may still mask
    # a ten-digit run, which is its job)
    assert list(rules.documents("артикул 4510 123456 на складе")) == []


def test_card_requires_luhn_without_cue():
    assert list(rules.documents("номер 4276 3801 2345 6788 в заявке")) == []
