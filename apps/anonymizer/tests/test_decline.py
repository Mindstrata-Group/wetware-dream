"""Russian name declension tables."""

import pytest

from anonymizer import decline


@pytest.mark.parametrize("nom,role,gender,case,expected", [
    ("Юля", "Name", "femn", "datv", "Юле"),
    ("Юля", "Name", "femn", "ablt", "Юлей"),
    ("Мария", "Name", "femn", "datv", "Марии"),
    ("Маша", "Name", "femn", "gent", "Маши"),
    ("Наташка", "Name", "femn", "ablt", "Наташкой"),
    ("Серёжа", "Name", "masc", "datv", "Серёже"),
    ("Олег", "Name", "masc", "ablt", "Олегом"),
    ("Андрей", "Name", "masc", "gent", "Андрея"),
    ("Дмитрий", "Name", "masc", "loct", "Дмитрии"),
    ("Игорь", "Name", "masc", "datv", "Игорю"),
    ("Петров", "Surn", "masc", "ablt", "Петровым"),
    ("Петрова", "Surn", "femn", "gent", "Петровой"),
    ("Ковалёв", "Surn", "masc", "datv", "Ковалёву"),
    ("Островская", "Surn", "femn", "accs", "Островскую"),
    ("Островский", "Surn", "masc", "gent", "Островского"),
    ("Ким", "Surn", "femn", "datv", "Ким"),
    ("Шевченко", "Surn", "masc", "datv", "Шевченко"),
    ("Черных", "Surn", "masc", "gent", "Черных"),
    ("Игоревна", "Patr", "femn", "ablt", "Игоревной"),
    ("Викторович", "Patr", "masc", "datv", "Викторовичу"),
])
def test_paradigm(nom, role, gender, case, expected):
    assert decline.inflect(nom, case, role, gender) == expected


def test_initials_and_latin_do_not_change():
    assert decline.inflect("И.", "datv") == "И."
    assert decline.inflect("Kate", "datv") == "Kate"


@pytest.mark.parametrize("tokens,gender", [
    (["Петрова", "Анна", "Сергеевна"], "femn"),
    (["Петров", "Игорь"], "masc"),
    (["Юля"], "femn"),
    (["Кузюк", "Олеговна"], "femn"),
])
def test_person_gender(tokens, gender):
    assert decline.person_gender(tokens) == gender
