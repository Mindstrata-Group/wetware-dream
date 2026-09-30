"""Addresses, institutions, workplaces, places and dates."""

import pytest

from tests.helpers import assert_clear, assert_masked


@pytest.mark.parametrize("text,fragment", [
    ("живу на ул. Малышева, д. 51, кв. 12 одна", "ул. Малышева, д. 51, кв. 12"),
    ("адрес: улица Ленина 5", "улица Ленина 5"),
    ("живём на Космонавтов 5, рядом парк", "Космонавтов 5"),
    ("д. 5, кв. 12 — это наш дом", "д. 5, кв. 12"),
    ("I live at 42 Maple Street, Apt 3B.", "42 Maple Street, Apt 3B"),
    ("Our address is 15 Kings Road, Brighton", "15 Kings Road"),
])
def test_addresses(text, fragment):
    assert_masked(text, fragment)


@pytest.mark.parametrize("text", [
    "Опять эти планёрках по понедельникам",
    "Перевела деньги на терапию",
    "первый эпизод депрессии был в юности",
    "плачет по ночам",
])
def test_street_markers_need_a_word_boundary(text):
    from anonymizer.detector import detect
    assert [s for s in detect(text).spans if s.kind.value == "ADDRESS"] == []


@pytest.mark.parametrize("text,fragment", [
    ("Сын учится в школе № 57", "школе № 57"),
    ("Дочь ходит в 312-й детский сад", "312-й детский сад"),
    ("Работаю в «Синаре» бухгалтером", "«Синаре»"),
    ("Работаю в ООО «Ромашка»", "ООО «Ромашка»"),
    ("Учусь в УрФУ на третьем курсе", "УрФУ"),
    ("I studied at University of Leeds", "University of Leeds"),
    ("My son goes to Lincoln High School", "Lincoln High School"),
])
def test_organisations(text, fragment):
    assert_masked(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("Диагноз ОКР поставили в ПНД", "ОКР"),
    ("Диагноз ОКР поставили в ПНД", "ПНД"),
    ("У меня СДВГ и ПТСР", "СДВГ"),
    ("Сдала анализы в поликлинике", "поликлинике"),
])
def test_generic_terms_stay(text, fragment):
    assert_clear(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("Переехали в Екатеринбург год назад", "Екатеринбург"),
    ("Бабушка живёт в Нижнем Тагиле", "Нижнем Тагиле"),
    ("We moved to Denver last year", "Denver"),
    ("I'm from Austin", "Austin"),
])
def test_places(text, fragment):
    assert_masked(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("Живу в России всю жизнь", "России"),
    ("We moved to Canada in 2019", "Canada"),
])
def test_countries_stay(text, fragment):
    assert_clear(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("Родилась 12 марта 1990 года", "12 марта 1990 года"),
    ("Дедушка умер 18.09.1995.", "18.09.1995"),
    ("Сессия была 15 сентября 2025", "15 сентября 2025"),
    ("Встреча 12.10 в 18:00", "12.10"),
    ("I was born on 03/12/1990.", "03/12/1990"),
    ("Since April 21 I can't sleep", "April 21"),
    ("Родилась в марте 1990 года", "марте 1990 года"),
])
def test_dates(text, fragment):
    assert_masked(text, fragment)


@pytest.mark.parametrize("text,fragment", [
    ("В 2019 году был первый эпизод", "2019"),
    ("В марте будет год, как я не пью", "марте"),
    ("Лекарство 1.25 мг утром", "1.25"),
    ("дозу до 0.75 таблетки", "0.75"),
    ("8 марта всегда грустно", "8 марта"),
])
def test_years_months_doses_holidays_stay(text, fragment):
    assert_clear(text, fragment)
