"""People in Russian text: names, surnames, patronymics, diminutives.

A therapy chat mentions people differently from a CRM: "мама Люда", "муж
серёжа", "с Петровой из бухгалтерии", "Наташка опять". Four signals decide
whether a word is part of a person's name:

1. the gazetteer of first names and diminutives (names_ru), in any case;
2. pymorphy3 readings tagged Name / Surn / Patr;
3. a relation cue right before the word ("мама", "муж", "начальница",
   "подруга"), which makes even a lowercase or ambiguous word a name;
4. Natasha PER spans.

Adjacent name parts are merged into one span with initials, so "Иванову
Ивану Петровичу" becomes one person. The canonical form is the nominative
with the gender preserved ("Петровой" -> "Петрова").
"""

from __future__ import annotations

import re

from . import decline, morph, names_ru, ner
from .kinds import Kind
from .spans import Span
from .text import INITIAL_RE, Token, sentence_initial, words

# Relation cues: close people, work and care roles. Matched by prefix so all
# case forms are covered ("маме", "мужем", "начальницы").
RELATION_STEMS = (
    "мам", "мамочк", "мамуль", "мать", "матер", "пап", "папочк", "отец", "отц", "муж", "жен",
    "сын", "сыноч", "доч", "брат", "братишк", "сестр", "сестрёнк", "сестренк", "бабушк",
    "бабул", "баб", "дедушк", "дед", "тёт", "тетя", "тети", "тете", "тетю", "тетей", "тетушк",
    "дяд", "подруг", "подружк", "друг", "друз", "дружищ",
    "парн", "парень", "девушк", "бывш", "начальни", "шеф", "руководител", "директор",
    "коллег", "свекров", "свёкр", "свекр", "тёщ", "тещ", "тест", "зят", "невестк", "снох",
    "племянни", "внук", "внучк", "сосед", "учител", "учительниц", "классн", "препод",
    "одноклассни", "однокурсни", "любовни", "партнёр", "партнер", "психолог", "психотерапевт",
    "терапевт", "психиатр", "врач", "доктор", "невролог", "педиатр", "гинеколог", "ребён",
    "ребен", "малыш", "крестн", "кум", "опекун", "мачех", "отчим", "падчериц", "пасынк",
    "супруг", "жених", "невест", "ухажёр", "ухажер", "бойфренд", "сожител", "подчинённ",
    "подчиненн", "клиент", "пациент", "тренер", "наставни", "куратор", "няня", "нян",
    "сиделк", "товарищ", "приятел", "знаком", "соседк", "сотрудни", "соседск",
)
# Words that look like a relation stem but are not ("жена" yes, "женщина"
# too — a cue as well; "сыр" no). Checked against the full word.
_NOT_RELATION = frozenset({
    "жест", "жесть", "другой", "другая", "другое", "другие", "других", "другим", "другому",
    "другого", "другу", "друга", "тест", "теста", "тесты", "дед", "мамонт", "папка", "папку",
    "баба", "бабки", "бабок", "женат", "женился", "женитьба", "классно", "классный", "классная",
    "классное", "классные", "знакомо", "знакомый", "знакомая", "знакомых", "знакомство",
    "доктрина", "доча",
})
_POSSESSIVE = frozenset({
    "мой", "моя", "моё", "мое", "моего", "моей", "моему", "моим", "моём", "моем", "мою", "мои",
    "моих", "наш", "наша", "нашего", "нашей", "нашему", "нашим", "нашу", "его", "её", "ее",
    "их", "твой", "твоя", "твоего", "твоей", "бывший", "бывшая", "бывшего", "бывшей",
    "бывшему", "бывшим", "бывшую", "старший", "старшая", "младший", "младшая", "старшего",
    "старшей", "младшего", "младшей", "родной", "родная", "двоюродный", "двоюродная",
    "двоюродного", "двоюродной", "лучший", "лучшая", "лучшего", "лучшей", "новый", "новая",
    "нового", "новой", "любимый", "любимая", "покойный", "покойная", "по", "имени", "зовут",
    "звали", "это",
})
_WE = frozenset({"мы", "вы", "они"})
# Capitalized words that are never a person here.
_NOT_PERSON = frozenset(morph.norm(w) for w in """
Бог Бога Богу Богом Боже Господь Господи Господа Христос Христа Иисус Аллах Будда Интернет
Январь Февраль Март Апрель Май Июнь Июль Август Сентябрь Октябрь Ноябрь Декабрь Марта Мая
Понедельник Вторник Среда Четверг Пятница Суббота Воскресенье Новый Год Рождество Пасха
Ватсап Вотсап Телеграм Телеграмм Инстаграм Инста Вконтакте Ютуб Тикток Зум Скайп Гугл Яндекс
Сбер Сбербанк Тинькофф Озон Вайлдберриз Авито Россия Москва Питер
Ок Окей Спасибо Привет Здравствуйте Добрый Доброе Пока Да Нет Ну Вот Ага Угу Хорошо Ладно
""".split())
# Geography markers before a word. One- and two-letter abbreviations count
# only with their dot: without it "с Петровой" would read as "село Петровой"
# and a person would slip through.
_GEO_LEFT = re.compile(
    r"(?i)(?:^|[\s(,;])(?:(?:г|с|д|п|ш|пр|пл|ст|ул|пер|наб|пос|обл|мкр|гор)\.|город[аеуом]*|посёлк\w*|"
    r"поселк\w*|село|деревн\w+|улиц\w+|пр-т|проспект\w*|переул\w+|б-р|бульвар\w*|площад\w+|шоссе|"
    r"набережн\w+|микрорайон\w*|р-н|район\w*|област\w+|край|краю|республик\w+|станци\w+|метро|"
    r"ул|пер|пр|пл|мкр)\s{1,2}$"
)
_SURNAME_SUFFIX = re.compile(
    r"(?i)(ов|ев|ёв|ин|ын|ова|ева|ёва|ина|ына|ский|ская|цкий|цкая|енко|ко|ук|юк|чук|ич|ян|янц|"
    r"дзе|швили|ая|ой|их|ых|ман|берг|штейн|ер)(а|у|ым|ом|ой|ую|ого|ому|им|е|и|ы)?$"
)
_PATR_RE = re.compile(r"(?i)(ович|евич|ич|овна|евна|ична|инична)(а|у|ем|ом|е|ы|ой|ою)?$")


def _is_relation(tok: Token) -> bool:
    low = tok.low
    if low in _NOT_RELATION or len(low) < 3 and low not in ("муж",):
        return False
    return low.startswith(RELATION_STEMS)


def _relation_before(text: str, toks: list[Token], i: int) -> bool:
    """Is token i preceded by a relation cue (possibly with a possessive)?"""
    j = i - 1
    hops = 0
    while j >= 0 and hops < 3:
        between = text[toks[j].end:toks[j + 1].start if j + 1 < len(toks) else toks[i].start]
        if re.search(r"[.!?\n;]", between):
            return False
        if _is_relation(toks[j]):
            return True
        if toks[j].low not in _POSSESSIVE:
            return False
        j -= 1
        hops += 1
    return False


def _non_person_score(tok: Token) -> float:
    return 1.0 - morph.person_score(tok.text)


def _name_token(text: str, toks: list[Token], i: int) -> bool:
    """Decide whether a Cyrillic token is (part of) a person's name."""
    tok = toks[i]
    if not tok.cyr or len(tok.text) < 2:
        return False
    low = tok.low
    if low in _NOT_PERSON or tok.text.isupper():
        return False  # abbreviations (ИНН, ПНД) and shouting are not names
    if _GEO_LEFT.search(text[max(0, tok.start - 30):tok.start]):
        return False
    relation = _relation_before(text, toks, i)
    initial = sentence_initial(text, tok.start)
    gaz = names_ru.lookup(tok.text)
    if gaz is not None:
        ambiguous = morph.norm(gaz[0]) in names_ru.AMBIGUOUS or low in names_ru.AMBIGUOUS
        if relation:
            return True
        if tok.cap and not initial:
            return True
        if ambiguous:
            return tok.cap and initial and morph.best_is_person(tok.text)
        if tok.cap:
            return True
        # the comitative "мы с колей" / "с колей мы" names a person even when
        # the word is also an ordinary one (колея)
        if i > 0 and toks[i - 1].low in ("с", "со") and (
                (i + 1 < len(toks) and toks[i + 1].low in _WE) or (i > 1 and toks[i - 2].low in _WE)):
            return True
        # lowercase listed name: fine unless it is mostly an ordinary word
        # ("света" is the genitive of "свет" far more often than a name)
        return _non_person_score(tok) < 0.5
    role = morph.person_role(tok.text)
    if role is not None:
        if relation:
            return True
        if not tok.cap:
            return False
        if morph.best_is_person(tok.text):
            return not morph.is_geo(tok.text)
        return not initial and not morph.any_geo(tok.text)
    if relation and tok.cap:
        return True
    # an unknown capitalized word shaped like a surname: "к Крапивину"
    if tok.cap and not initial and not morph.is_known_word(tok.text) and _SURNAME_SUFFIX.search(tok.text) \
            and not morph.any_geo(tok.text):
        return True
    return False


def _surname_like(text: str, tok: Token) -> bool:
    """Capitalized word that can be a surname next to a name.

    Next to a confirmed name the bar is low: any capitalized word unknown to
    the dictionary ("Шмыгло", "Кузюк") or with a surname reading counts.
    """
    if not tok.cyr or not tok.cap or len(tok.text) < 2 or tok.text.isupper():
        return False
    if tok.low in _NOT_PERSON or morph.any_geo(tok.text) or _is_relation(tok):
        return False
    if morph.person_role(tok.text) in ("Surn", "Patr"):
        return True
    if morph.is_known_word(tok.text):
        return False
    return True


def _join_parts(text: str, toks: list[Token], flags: list[bool]) -> list[tuple[int, int]]:
    """Group flagged tokens into name spans, growing over neighbours."""
    groups: list[tuple[int, int]] = []
    i = 0
    n = len(toks)
    while i < n:
        if not flags[i]:
            i += 1
            continue
        a = b = i
        # grow right over flagged tokens or surname-like/patronymic words
        while b + 1 < n and re.fullmatch(r"[ \t ]{1,2}", text[toks[b].end:toks[b + 1].start]):
            nxt = toks[b + 1]
            if flags[b + 1] or _PATR_RE.search(nxt.text) and nxt.cap or _surname_like(text, nxt):
                b += 1
                continue
            break
        # grow left over a surname-like word ("Кузюк Настя")
        while a - 1 >= 0 and re.fullmatch(r"[ \t ]{1,2}", text[toks[a - 1].end:toks[a].start]):
            if _surname_like(text, toks[a - 1]):
                a -= 1
                continue
            break
        groups.append((toks[a].start, toks[b].end))
        i = b + 1
    return groups


def _with_initials(text: str, start: int, end: int) -> tuple[int, int]:
    right = re.match(r"[ \t ]{0,2}([А-ЯЁ]\.\s?(?:[А-ЯЁ]\.)?)", text[end:end + 8])
    if right:
        end = end + right.end()
    left = re.search(r"([А-ЯЁ]\.\s?(?:[А-ЯЁ]\.)?)[ \t ]{0,2}$", text[max(0, start - 8):start])
    if left:
        start = start - (len(text[max(0, start - 8):start]) - left.start())
    return start, end


def canonical(value: str) -> tuple[str, str]:
    """(canon, key) of a person value: nominative tokens, gender kept."""
    parts = re.findall(r"[А-ЯЁA-Zа-яёa-z][А-ЯЁа-яёA-Za-z'’\-]*\.?", value)
    gender = None
    for tok in parts:
        if _PATR_RE.search(tok):
            p = morph.person_parse(tok.rstrip("."), "Patr")
            if p is not None and p.tag.gender in ("masc", "femn"):
                gender = p.tag.gender
                break
    if gender is None:
        for tok in parts:
            hit = names_ru.lookup(tok.rstrip("."))
            if hit:
                gender = hit[1]
                break
            p = morph.person_parse(tok.rstrip("."), "Name")
            if p is not None and p.tag.gender in ("masc", "femn"):
                gender = p.tag.gender
                break
    out: list[str] = []
    for tok in parts:
        if tok.endswith(".") or not re.search(r"[а-яё]", tok.lower()):
            out.append(tok)
            continue
        hit = names_ru.lookup(tok)
        if hit:
            # keep the diminutive the person used, just in the nominative
            out.append(hit[0])
            continue
        nom, _ = decline.to_nominative(tok, morph.person_role(tok), gender)
        out.append(nom[:1].upper() + nom[1:])
    canon = " ".join(out)
    return canon, " ".join(morph.norm(p) for p in out)


def find(text: str, entities: list[ner.Entity] | None = None) -> list[Span]:
    toks = words(text)
    flags = [_name_token(text, toks, i) for i in range(len(toks))]
    ranges = _join_parts(text, toks, flags)
    # Natasha PER spans, trimmed to capitalized non-verb tokens
    for ent in entities or ():
        if ent.type != "PER":
            continue
        inner = [t for t in toks if t.start >= ent.start and t.end <= ent.end]
        keep = [t for t in inner if t.cap and t.low not in _NOT_PERSON and not morph.is_verb_like(t.text)
                and not t.text.isupper() and not _is_relation(t)
                and not (sentence_initial(text, t.start) and morph.is_known_word(t.text)
                         and not morph.person_role(t.text))
                and not _GEO_LEFT.search(text[max(0, t.start - 30):t.start])]
        # "Вера в себя...", "Любовь к себе..." at the start of a sentence are
        # words, not people, whatever the model thinks
        if len(keep) == 1 and sentence_initial(text, keep[0].start) and (
                keep[0].low in names_ru.AMBIGUOUS or morph.norm(keep[0].text) in names_ru.AMBIGUOUS):
            continue
        if keep:
            ranges.append((keep[0].start, keep[-1].end))
    # initials with a surname: "Иванов И.И.", "И. И. Иванов"
    for m in re.finditer(r"(?<![А-Яа-яЁё])([А-ЯЁ]\.\s?[А-ЯЁ]?\.?)\s?([А-ЯЁ][а-яё]{2,})", text):
        if _surname_like(text, Token(m.start(2), m.end(2), m.group(2))):
            ranges.append((m.start(), m.end()))
    for m in re.finditer(r"([А-ЯЁ][а-яё]{2,})\s([А-ЯЁ]\.\s?[А-ЯЁ]\.?)", text):
        if _surname_like(text, Token(m.start(1), m.end(1), m.group(1))):
            ranges.append((m.start(), m.end()))
    spans: list[Span] = []
    for start, end in ranges:
        start, end = _with_initials(text, start, end)
        value = text[start:end]
        canon, key = canonical(value)
        spans.append(Span(start, end, Kind.PERSON, canon, key, "people"))
    return spans
