"""English first names and Latin spellings of Russian names.

English has no morphology to lean on, so person detection in Latin script is
gazetteer-driven: a listed first name, optionally followed by a capitalized
surname, or a title ("Dr. Smith"), or a relation cue ("my sister Kate").
Russian names typed in Latin ("Seryozha", "Natasha Ivanova") are generated
from names_ru by transliteration.
"""

from __future__ import annotations

from functools import lru_cache

from . import names_ru

_EN = """
aaron abby abigail adam adrian aidan aiden alan albert alex alexa alexander alexandra alexis alice
alicia alison allison alyssa amanda amber amelia amy andrea andrew angela angie anna anne annie
anthony antonio april ariana ashley austin ava bailey barbara becky ben benjamin beth betty bill
billy blake bob bobby bonnie brad bradley brandon brenda brendan brian brittany brooke bruce bryan
caleb cameron carl carla carlos carol caroline carolyn carter casey catherine charles charlie
charlotte chelsea cheryl chloe chris christian christina christine christopher cindy claire clara
colin connor courtney craig cynthia daisy dale dan daniel danielle danny david dean debbie deborah
denise dennis derek diana diane dominic donald donna dorothy doug douglas dylan eddie edward elena
eli elijah elizabeth ella ellen ellie emily emma eric erica erin ethan eugene evan evelyn faith
frank fred freddie gabriel gabriella gary gavin george georgia gerald grace greg gregory hailey
hannah harold harper harry hayden heather helen henry holly hunter ian isaac isabel isabella
isabelle jack jackson jacob jacqueline jake james jamie jan jane janet janice jason jasmine jay
jean jeff jeffrey jenna jennifer jenny jeremy jerry jess jesse jessica jill jim jimmy joan joanna
jodie joe joel john johnny jon jonathan jordan jose joseph josh joshua joy joyce judith judy julia
julian julie justin kaitlyn karen kate katherine kathleen kathryn kathy katie kayla keith kelly
ken kenneth kevin kim kimberly kyle laura lauren leah lee leo leonard liam lily linda lisa logan
lori louis lucas lucy luke lydia madison maggie mandy marcus margaret maria marie marilyn mark
martha martin mary mason matt matthew maureen max megan melanie melissa mia michael michelle
mike mila molly monica morgan nancy natalie nathan nicholas nick nicole noah nora oliver olivia
owen paige pamela patricia patrick paul paula peggy penny peter phil philip phoebe rachel ralph
randy rebecca riley rita rob robert robin roger ronald rose ruby russell ruth ryan sally sam
samantha samuel sandra sara sarah scott sean sebastian sharon shawn sheila shirley sophia sophie
stacy stephanie stephen steve steven sue susan sydney tammy tanya taylor teresa terry theresa
thomas tim timothy tina todd tom tommy tony tracy travis tyler valerie vanessa victor victoria
vincent virginia walter wanda wayne wendy william willow wyatt zach zachary zoe
"""

# Also everyday English words; they need a capital letter mid-sentence or a
# relation cue to count as a name.
AMBIGUOUS = frozenset("""
will may june grace hope joy faith rose amber april summer bill mark pat sue rob art jack frank
don ray gene grant wade chase dawn eve ivy jade lily ruby sky penny sandy rusty bob dean glen
lane drew max rich ken jay kim lee paige brook hunter taylor mason carter harper willow
""".split())

_TRANSLIT = {
    "а": "a", "б": "b", "в": "v", "г": "g", "д": "d", "е": "e", "ё": "yo", "ж": "zh", "з": "z",
    "и": "i", "й": "y", "к": "k", "л": "l", "м": "m", "н": "n", "о": "o", "п": "p", "р": "r",
    "с": "s", "т": "t", "у": "u", "ф": "f", "х": "kh", "ц": "ts", "ч": "ch", "ш": "sh", "щ": "shch",
    "ъ": "", "ы": "y", "ь": "", "э": "e", "ю": "yu", "я": "ya",
}
_ALT = {"kh": "h", "yo": "e", "ya": "ia", "yu": "iu", "iy": "y", "ey": "ei", "ts": "c"}


def translit(word: str) -> set[str]:
    base = "".join(_TRANSLIT.get(ch, ch) for ch in word.lower())
    out = {base}
    for a, b in _ALT.items():
        out |= {v.replace(a, b) for v in list(out)}
    if base.endswith("iy"):
        out.add(base[:-2] + "y")
        out.add(base[:-2] + "i")
    if base.endswith("ya"):
        out.add(base[:-2] + "ia")
    return {v for v in out if len(v) >= 3}


@lru_cache(maxsize=1)
def first_names() -> frozenset[str]:
    names = set(_EN.split())
    for table in (names_ru.FULL, names_ru.DIMINUTIVE):
        for name in table:
            names |= translit(name)
    return frozenset(names)


def is_first_name(word: str) -> bool:
    return word.lower() in first_names()


def ambiguous(word: str) -> bool:
    return word.lower() in AMBIGUOUS
