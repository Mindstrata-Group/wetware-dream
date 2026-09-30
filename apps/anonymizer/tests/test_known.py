"""The user's dictionary: known values are masked in every form.

The property test generates word forms with pymorphy3 itself, independently
of the tables in decline.py, so a gap in our own paradigms shows up here.
"""

import random

import pymorphy3
import pytest

from anonymizer.detector import detect
from anonymizer.known import Known, find
from anonymizer.kinds import Kind
from anonymizer.spans import covered

MORPH = pymorphy3.MorphAnalyzer()
CASES = ("nomn", "gent", "datv", "accs", "ablt", "loct")
# Profile values of invented users. Deliberately includes surnames and names
# that none of the detectors know on their own.
PROFILES = [
    "Анна Кузнецова", "Игорь Шмыгло", "Злата Крапивина", "Ерофей Прохорчук", "Милослава Лапшина",
    "Сергей Белоусов", "Юлия Тагирова", "Добрыня Островский", "Резеда Гарипова", "Павел Воронин",
    "Солнышко", "Kitty", "Марат Абрамян", "Эвелина Ким",
]
FRAMES = [
    "Сегодня {} мне опять не ответил(а), и я расстроилась.",
    "я думаю про {} весь день",
    "Мне кажется, {}, что всё зря.",
    "вчера {} плакала",
]


def pymorphy_forms(word: str) -> set[str]:
    """Case forms of the word by pymorphy, from readings where the word is
    the nominative singular (so "Юлия" gives Юлии/Юлией, not the forms of
    the masculine "Юлий")."""
    out = {word}
    for p in MORPH.parse(word):
        if "nomn" not in p.tag or "plur" in p.tag:
            continue
        if not any(t in p.tag for t in ("Name", "Surn", "Patr")) and p.normal_form != word.lower():
            continue
        for case in CASES:
            f = p.inflect({case, "sing"})
            if f:
                out.add(f.word[:1].upper() + f.word[1:] if word[:1].isupper() else f.word)
    return out


@pytest.mark.parametrize("profile", PROFILES)
def test_every_form_of_every_profile_token_is_masked(profile):
    rnd = random.Random(profile)
    items = [Known(Kind.PERSON, profile)]
    for token in profile.split():
        for form in sorted(pymorphy_forms(token)):
            for variant in {form, form.lower(), form.upper() if len(form) > 3 else form}:
                text = rnd.choice(FRAMES).format(variant)
                det = detect(text, items=items)
                start = text.index(variant)
                assert covered(det.spans, start, start + len(variant)), (token, variant, case_hint(variant))


def case_hint(word: str) -> str:
    return ",".join(sorted({str(p.tag.case) for p in MORPH.parse(word)}))


def test_email_phone_handle_from_profile():
    items = [Known(Kind.EMAIL, "Anna.K@Mail.ru"), Known(Kind.PHONE, "+7 (912) 345-67-89"),
             Known(Kind.HANDLE, "@anna_k")]
    text = "пиши anna.k@mail.ru или anna.k в скайп, звони 8 912 3456789 или 345-67-89, тг anna_k"
    det = detect(text, items=items)
    for fragment in ("anna.k@mail.ru", "8 912 3456789", "anna_k"):
        start = text.index(fragment)
        assert covered(det.spans, start, start + len(fragment)), fragment
    start = text.index("anna.k в")
    assert covered(det.spans, start, start + len("anna.k"))


def test_latin_profile_matches_cyrillic_spelling():
    items = [Known(Kind.PERSON, "Anna")]
    text = "Меня зовут анна, мне 30 лет"
    det = detect(text, items=items)
    start = text.index("анна")
    assert covered(det.spans, start, start + 4)


def test_vault_values_keep_their_key():
    items = [Known(Kind.PLACE, "Сысерть", canon="Сысерть", key="сысерть")]
    spans = find("живу в Сысерти давно", items)
    assert spans and spans[0].key == "сысерть" and spans[0].kind == Kind.PLACE
