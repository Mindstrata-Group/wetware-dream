"""Addresses, places, organisations and dates.

In a CRM a city or a company name is business data. In a therapy chat they
narrow a person down: "работаю логистом в Уралхиммаше", "сын в 57-й школе",
"лежала в ПНД на Сибирском тракте". The default policy is strict: street
addresses, institutions with numbers or names, workplaces and places of
study, cities and exact dates are masked. Countries, years and months stay:
they carry meaning and rarely identify anyone (the HIPAA Safe Harbor rule
keeps the year for the same reason).
"""

from __future__ import annotations

import re
from typing import Iterator

from . import morph, ner
from .kinds import Kind
from .spans import Span

# ------------------------------------------------------------------ addresses

_STREET_RU = (
    r"(?:ул\.?|улиц[аеуы]|улицей|пр-т\.?|пр-кт\.?|просп\.?|проспект[аеу]?|пер\.?|переул(?:ок|ке|ка)|"
    r"б-р\.?|бульвар[аеу]?|ш\.|шоссе|наб\.?|набережн(?:ая|ой|ую)|пл\.?|площад[ьи]|проезд[аеу]?|"
    r"мкр\.?|мкрн\.?|микрорайон[аеу]?|тупик[аеу]?|аллея|аллее|тракт[аеу]?|квартал[аеу]?)"
)
# Street names are capitalized words or a lowercase adjective-like word
# ("ул. садовая"); house and flat markers are case-insensitive on their own,
# so the "д" of "ул. Ленина д. 10" is never eaten as part of the name.
_NAME_RU = r"(?:[А-ЯЁ0-9][А-Яа-яЁё0-9\-]*|[а-яё]+(?:ая|ой|ий|ый|ов|ев|ин))(?:\s+[А-ЯЁ][А-Яа-яЁё\-]*){0,2}"
_HOUSE = (r"(?:,?\s*(?i:д\.?|дом[аеу]?)?\s*\d{1,4}\s?[а-яА-Я]?(?![А-Яа-яЁё])(?:\s*[/\-]\s*\d{1,4})?"
          r"(?:\s*(?i:к\.?|корп\.?|корпус|стр\.?|строение)\s*\d{1,3})?)")
_FLAT = r"(?:,?\s*(?i:кв\.?|квартир[аеуы]|оф\.?|офис|комн\.?|комната)\s*\d{1,4})"
# The marker must end the word: "пл" is a square only as "пл." or "пл ",
# never the start of "планёрках".
RE_ADDR_RU = re.compile(
    r"(?<![А-Яа-яЁё])(?i:" + _STREET_RU + r")(?:(?<=\.)|(?![а-яёa-zА-ЯЁ]))\s*" + _NAME_RU + _HOUSE + r"?" + _FLAT + r"?"
)
# "дом 5, кв. 12", "д. 5 кв 12" without a street name
RE_HOUSE_FLAT_RU = re.compile(r"(?i)(?<![А-Яа-яЁё])(?:д\.|дом)\s*\d{1,4}\s?[а-я]?(?:\s*[,/]?\s*(?:кв\.?|квартир[аеуы])\s*\d{1,4})")
RE_FLAT_RU = re.compile(r"(?i)(?<![А-Яа-яЁё])(?:кв\.|квартир[аеуы]\s*(?:№\s*)?)\s*\d{1,4}")
# "живу на Ленина 5", "переехали на Малышева, 51"
RE_NA_STREET_RU = re.compile(
    r"(?i)(?:живу|живём|живем|жила|жил|жили|переех\w*|снима\w*|купил\w*|дом|квартир\w*|адрес\w*|"
    r"прописк\w*|прописан\w*|зарегистрирован\w*)\s+(?:\S+\s+){0,2}?на\s+([А-ЯЁ][а-яё\-]+(?:\s[А-ЯЁ][а-яё\-]+)?,?\s*\d{1,4}\s?[а-я]?(?:\s*[-/]\s*\d{1,4})?)"
)
RE_INDEX_RU = re.compile(r"(?<!\d)(?:индекс\s*)?\b[1-6]\d{5}\b(?=\s*,?\s*(?:г\.|город|[А-ЯЁ][а-яё]+,|Россия|РФ))")

_STREET_EN = (
    r"(?:Street|St\.?|Avenue|Ave\.?|Road|Rd\.?|Lane|Ln\.?|Boulevard|Blvd\.?|Drive|Dr\.?|Court|Ct\.?|"
    r"Way|Place|Pl\.?|Terrace|Crescent|Close|Square|Sq\.?|Parkway|Pkwy\.?|Highway|Hwy\.?|Circle|Cir\.?)"
)
RE_ADDR_EN = re.compile(
    r"\b\d{1,5}\s+(?:[A-Z][a-z]+\s+){1,3}" + _STREET_EN +
    r"(?:,?\s*(?:Apt\.?|Apartment|Unit|Suite|Ste\.?|#)\s*[\w\-]+)?(?:,\s*[A-Z][a-z]+(?:\s[A-Z][a-z]+)?)?"
    r"(?:,\s*[A-Z]{2}\s*\d{5}(?:-\d{4})?)?"
)
RE_ZIP_EN = re.compile(r"\b[A-Z]{2}\s\d{5}(?:-\d{4})?\b")
RE_POSTCODE_UK = re.compile(r"\b[A-Z]{1,2}\d[A-Z\d]?\s\d[A-Z]{2}\b")


def addresses(text: str) -> Iterator[Span]:
    for rx in (RE_ADDR_RU, RE_HOUSE_FLAT_RU, RE_FLAT_RU, RE_ADDR_EN, RE_ZIP_EN, RE_POSTCODE_UK, RE_INDEX_RU):
        for m in rx.finditer(text):
            value = m.group(0).strip(" ,")
            start = m.start() + (len(m.group(0)) - len(m.group(0).lstrip(" ,")))
            if len(value) < 4:
                continue
            key = " ".join(value.lower().replace("ё", "е").split())
            yield Span(start, start + len(value), Kind.ADDRESS, value, key, "address")
    for m in RE_NA_STREET_RU.finditer(text):
        value = m.group(1).rstrip(" ,")
        key = " ".join(value.lower().replace("ё", "е").split())
        yield Span(m.start(1), m.start(1) + len(value), Kind.ADDRESS, value, key, "address")


# ------------------------------------------------------------- institutions

_INST_RU = (
    r"(?:школ[аеуыо]й?|гимнази[яиюей]|лице[йяюе]|лицея|колледж[аеу]?|техникум[аеу]?|училищ[еау]|"
    r"вуз[аеу]?|университет[аеу]?|институт[аеу]?|академи[яиюей]|детск(?:ий|ого|ом)\s+сад[аеу]?|"
    r"детсад[аеу]?|садик[аеу]?|д/с|ясл[иях]|больниц[аеуы]й?|поликлиник[аеуи]й?|госпитал[ьяюе]|"
    r"роддом[аеу]?|перинатальн\w+\s+центр\w*|диспансер[аеу]?|клиник[аеуи]й?|центр[аеу]?|"
    r"интернат[аеу]?|детск(?:ий|ого|ом)\s+дом[аеу]?|приют[аеу]?|кафедр[аеуы]|отделени[еияю]|"
    r"завод[аеу]?|фабрик[аеуи]|комбинат[аеу]?|цех[аеу]?|магазин[аеу]?|салон[аеу]?|кафе|ресторан[аеу]?|"
    r"офис[аеу]?|компани[яиюей]|фирм[аеуы]|банк[аеу]?|отдел[аеу]?|бюро|агентств[оеау]|"
    r"ПНД|ПНИ|ПКБ|ЦРБ|ГКБ|ДГКБ|ОКБ|РКБ|МСЧ|ОВД|УВД|МФЦ|ТЦ|ТРЦ|СОШ|МБОУ|МАОУ|ГБОУ|ДОУ|МБДОУ|ГБУЗ|ГАУЗ|НИИ)"
)
RE_INST_NUM = re.compile(
    r"(?<![А-Яа-яЁё])" + _INST_RU + r"\s*(?:№|N|No\.?|номер)?\s*\d{1,4}(?:\s?[-–]?\s?[а-яё]{1,3})?"
)
RE_NUM_INST = re.compile(r"(?<!\d)\d{1,4}\s?[-–]?\s?(?:[ояий]й|ая|ой|ую|ю|я|й)?\s+" + _INST_RU)
RE_INST_NAMED = re.compile(
    r"(?<![А-Яа-яЁё])" + _INST_RU + r"\s+(?:(?:им\.|имени)\s+)?"
    r"(?:«[^»\n]{1,60}»|\"[^\"\n]{1,60}\"|[A-ZА-ЯЁ][\w\-]*(?:\s+[A-ZА-ЯЁ][\w\-]*){0,3})"
)
RE_ORG_FORM = re.compile(
    r"(?:ООО|ОАО|ЗАО|ПАО|АО|ИП|НКО|АНО|ГУП|МУП|ФГУП|ФГБУ|ГБУ|МБУ|ТОО|LLC|Inc\.?|Ltd\.?|GmbH)\s*"
    r"(?:«[^»\n]{1,60}»|\"[^\"\n]{1,60}\"|[A-ZА-ЯЁ][\w\-]*(?:\s+[A-ZА-ЯЁ][\w\-]*){0,3})"
)
# "работаю в Уралхиммаше", "учусь в УрФУ", "уволили из «Ромашки»"
RE_WORK_STUDY = re.compile(
    r"(?i:работа\w*|работал\w*|устрои\w*|уволи\w*|увольня\w*|трудо\w*|подрабатыва\w*|стажир\w*|"
    r"учусь|учится|учился|училась|учатся|учились|учиться|поступ\w+|закончи\w*|окончи\w*|выпускни\w*|"
    r"преподаю|преподава\w*|служ\w+|лечил\w*|лечусь|лечится|лежал\w*|наблюда\w*|ходи\w*\s+в|водим|"
    r"works?\s+(?:at|for)|worked\s+(?:at|for)|study\s+at|studied\s+at|graduated\s+from|attend\w*)"
    r"\s+(?i:в[о]?\s+|на\s+|из\s+|at\s+|for\s+|from\s+)?(?:[а-яёa-z]+\s+)?"
    r"(«[^»\n]{1,60}»|\"[^\"\n]{1,60}\"|[A-ZА-ЯЁ][\w\-&]*(?:[\s\-][A-ZА-ЯЁ][\w\-&]*){0,3})"
)
RE_INST_EN = re.compile(
    r"\b(?:[A-Z][a-z]+\s+){1,3}(?:High School|Middle School|Elementary School|Primary School|School|"
    r"Academy|College|University|Hospital|Clinic|Medical Center|Institute|Inc\.?|LLC|Ltd\.?|Corp\.?)\b"
    r"|\b(?:University|College|Institute) of (?:[A-Z][a-z]+\s?){1,3}"
)
# All-caps abbreviations are masked (МГУ, УрФУ, ВШЭ) except diagnoses and
# common terms that the conversation is about.
RE_ACRONYM = re.compile(r"(?<![\wА-Яа-яЁё])[А-ЯЁ][А-ЯЁа-яё]{0,2}[А-ЯЁ]{1,6}(?![\wА-Яа-яЁё])")
SAFE_ACRONYMS = frozenset("""
РФ СССР США ЕС ООН ВОЗ СНГ ЕГЭ ОГЭ ВУЗ ВУЗа ВУЗе ЗОЖ ОКР СДВГ ПТСР КПТ ДБТ ЭМДР БАР ГТР ПРЛ
РАС ВСД ПА ОРВИ ОРЗ ВИЧ СПИД ЭКГ ЭЭГ МРТ КТ УЗИ ЖКТ ЦНС ЛФК ИВЛ ЭКО РПП НПВС СИОЗС ИМАО СМС
ММС ПМС ПМДД ЛГБТ ЛГБТК ИИ ГПТ ЧАТ ЖКХ ТВ ПК МВД ГИБДД ФСБ ЗАГС ДТП МЧС ОМС ДМС ПДД НДФЛ
ОК ТГ ВК ИП ООО АО СВО ВСУ ЧП ДР ЛС ЛК СМИ РЖД ТЦ ТРЦ МФЦ ДЗ КР ЦРУ НЛП МКБ DSM
ПНД ПНИ ПКБ ЦРБ ГКБ ОКБ РКБ МСЧ НИИ СОШ ДОУ ОВД УВД ИТ АД ЧСС ИМТ СРК ГЭРБ ХОБЛ ВПЧ ИППП
СНИЛС ИНН ОГРН ОГРНИП КПП БИК ПТС СТС ВУ ПМЖ ВНЖ РВП ЗП НДС ЕГЭ МСЭ ИПР ИПРА ТЗ ЧСВ ИМХО
""".split())

_PLACE_ALLOW = frozenset(morph.norm(w) for w in """
Россия России Россию Россией РФ Украина Украины Украине Украину Беларусь Белоруссия Белоруссии
Казахстан Казахстана Казахстане Европа Европы Европе Европу Азия Азии Америка Америки Америке
Америку США Германия Германии Франция Франции Италия Италии Испания Испании Китай Китая Китае
Япония Японии Турция Турции Турцию Грузия Грузии Армения Армении Израиль Израиле Израиля
Англия Англии Великобритания Великобритании Канада Канаде Канады Мир Земля Земле Земли
Запад Запада Востоке Восток Север Юг Сибирь Сибири Урал Урале Урала Кавказ Кавказе
Russia Europe Asia America USA UK Canada Germany France Italy Spain China Japan Turkey Israel
""".split())


def _org_value(text: str, start: int, end: int, source: str) -> Span:
    value = text[start:end].strip(" ,.")
    s = start + (len(text[start:end]) - len(text[start:end].lstrip(" ,.")))
    key = " ".join(value.lower().replace("ё", "е").strip("«»\"").split())
    return Span(s, s + len(value), Kind.ORG, value, key, source)


def organisations(text: str, entities: list[ner.Entity]) -> Iterator[Span]:
    for rx, source in ((RE_INST_NUM, "org:inst"), (RE_NUM_INST, "org:inst"), (RE_INST_NAMED, "org:inst"),
                       (RE_ORG_FORM, "org:form"), (RE_INST_EN, "org:inst")):
        for m in rx.finditer(text):
            yield _org_value(text, m.start(), m.end(), source)
    for m in RE_WORK_STUDY.finditer(text):
        name = m.group(1)
        if morph.norm(name.strip("«»\"")) in _PLACE_ALLOW:
            continue
        yield _org_value(text, m.start(1), m.end(1), "org:work")
    for m in RE_ACRONYM.finditer(text):
        word = m.group(0)
        if word in SAFE_ACRONYMS or word.upper() in SAFE_ACRONYMS or len(word) < 2:
            continue
        if sum(ch.isupper() for ch in word) < 2 or len(word) > 7:
            continue
        if word.isupper() and morph.is_known_word(word) and len(word) > 3:
            continue  # shouting ("ПОМОГИТЕ"), not an abbreviation
        yield _org_value(text, m.start(), m.end(), "org:acronym")
    for ent in entities:
        if ent.type == "ORG":
            value = text[ent.start:ent.end]
            if value.upper() in SAFE_ACRONYMS:
                continue
            yield _org_value(text, ent.start, ent.end, "org:natasha")


# Latin-script places: no model, so a preposition of place followed by a
# capitalized word is enough (strict), minus words that are obviously not
# places. A short list of big cities covers mentions without a preposition.
RE_PLACE_EN = re.compile(
    r"\b(?:in|from|to|near|around|outside|visit(?:ed|ing)?|moved to|live in|lives in|living in|"
    r"born in|grew up in|back in|flew to|drove to|went to)\s+((?:[A-Z][a-z]+)(?:[\s\-](?:[A-Z][a-z]+)){0,2})"
)
_NOT_PLACE_EN = frozenset("""
I Me My The A An This That Monday Tuesday Wednesday Thursday Friday Saturday Sunday January February
March April May June July August September October November December English Russian Spanish French
German Chinese Italian Math Therapy Group Session Church God Christmas Easter Summer Winter Spring
Fall Autumn Love Hell Heaven Bed Work School College University Hospital Mom Dad Mum Instagram
Facebook Telegram Zoom Google Discord Reddit Twitter TikTok YouTube Netflix Russia Europe Asia
America USA UK Canada Germany France Italy Spain China Japan Turkey Israel Africa Australia
""".split())
_CITIES_EN = frozenset("""
London Paris Berlin Moscow Toronto Vancouver Montreal Sydney Melbourne Dublin Boston Chicago Seattle
Denver Austin Portland Dallas Houston Phoenix Atlanta Miami Brooklyn Manhattan Oakland Brighton
Leeds Manchester Liverpool Bristol Glasgow Edinburgh Cardiff Belfast Oxford Cambridge Madrid
Barcelona Rome Milan Vienna Prague Warsaw Budapest Amsterdam Brussels Lisbon Istanbul Tbilisi
Yerevan Almaty Astana Tashkent Minsk Kyiv Kiev Riga Vilnius Tallinn Helsinki Stockholm Oslo
Copenhagen Zurich Geneva Munich Hamburg Dubai Bangkok Tokyo Seoul Beijing Shanghai Delhi Mumbai
Petersburg Yekaterinburg Ekaterinburg Novosibirsk Kazan Sochi
""".split())


def places_en(text: str) -> Iterator[Span]:
    for m in RE_PLACE_EN.finditer(text):
        value = m.group(1)
        words = value.split()
        while words and words[-1] in _NOT_PLACE_EN:
            words.pop()
        if not words or words[0] in _NOT_PLACE_EN:
            continue
        value = " ".join(words)
        yield Span(m.start(1), m.start(1) + len(value), Kind.PLACE, value, value.lower(), "place_en")
    for m in re.finditer(r"\b[A-Z][a-z]+\b", text):
        if m.group(0) in _CITIES_EN:
            yield Span(m.start(), m.end(), Kind.PLACE, m.group(0), m.group(0).lower(), "place_en")


def expand_quotes(text: str, spans: list[Span]) -> list[Span]:
    """Grow organisation and place spans over the quotes around them."""
    out = []
    for s in spans:
        if s.kind in (Kind.ORG, Kind.PLACE) and s.start > 0 and s.end < len(text):
            left, right = text[s.start - 1], text[s.end]
            if (left, right) in (("«", "»"), ('"', '"'), ("“", "”"), ("'", "'")):
                s = Span(s.start - 1, s.end + 1, s.kind, text[s.start - 1:s.end + 1], s.key, s.source)
        out.append(s)
    return out


def places(text: str, entities: list[ner.Entity]) -> Iterator[Span]:
    seen: set[tuple[int, int]] = set()
    for ent in entities:
        if ent.type != "LOC":
            continue
        value = text[ent.start:ent.end]
        if all(morph.norm(w) in _PLACE_ALLOW for w in re.findall(r"[\wЁё]+", value)):
            continue
        seen.add((ent.start, ent.end))
        key = " ".join(morph.norm(w) for w in value.split())
        yield Span(ent.start, ent.end, Kind.PLACE, value, key, "place")
    # pymorphy geography for what Natasha misses (lowercase-insensitive for
    # capitalized words only; "урал" the river and "Урал" the region are fine)
    for m in re.finditer(r"(?<![А-Яа-яЁё])(?:(?:г\.|город[аеу]?|пос\.|посёлок|поселок|село|деревн[яеи]|д\.|с\.)\s*)?"
                         r"([А-ЯЁ][а-яё]+(?:[\s\-][А-ЯЁ][а-яё]+)?)", text):
        word = m.group(1)
        first = word.split()[0].split("-")[0]
        marked = m.group(0) != word
        if morph.norm(first) in _PLACE_ALLOW:
            continue
        if not (marked or morph.is_geo(first)):
            continue
        if marked and not morph.any_geo(first) and morph.best_is_person(first):
            continue
        start = m.start(1)
        if any(a <= start < b for a, b in seen):
            continue
        key = " ".join(morph.norm(w) for w in word.split())
        yield Span(start, m.end(1), Kind.PLACE, word, key, "place")


# ---------------------------------------------------------------------- dates

_MONTHS_RU = r"(?:январ[ьяе]|феврал[ьяе]|март[аеу]?|апрел[ьяе]|ма[йяе]|июн[ьяе]|июл[ьяе]|август[аеу]?|сентябр[ьяе]|октябр[ьяе]|ноябр[ьяе]|декабр[ьяе])"
_MONTHS_EN = r"(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|June?|July?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\.?"
# A sentence may end right after the date ("умер 18.09.1995."), so a dot is
# a boundary unless a digit follows it.
_END = r"(?!\d)(?![.,]\d)"
RE_DATE_NUM = re.compile(r"(?<![\d.])(?:[0-2]?\d|3[01])[./\-](?:0?[1-9]|1[0-2])[./\-](?:(?:19|20)\d{2}|\d{2})" + _END)
RE_DATE_ISO = re.compile(r"(?<!\d)(?:19|20)\d{2}-(?:0[1-9]|1[0-2])-(?:[0-2]\d|3[01])(?!\d)")
RE_DATE_RU = re.compile(
    r"(?i)(?<!\d)(?:[0-2]?\d|3[01])(?:-?(?:го|е|ого))?\s+" + _MONTHS_RU + r"(?:\s+(?:19|20)\d{2}(?:\s*(?:г\.|года?))?)?"
)
RE_DATE_EN = re.compile(
    r"\b(?:" + _MONTHS_EN + r"\s+(?:[0-2]?\d|3[01])(?:st|nd|rd|th)?(?:,?\s+(?:19|20)\d{2})?"
    r"|(?:[0-2]?\d|3[01])(?:st|nd|rd|th)?\s+(?:of\s+)?" + _MONTHS_EN + r"(?:,?\s+(?:19|20)\d{2})?)\b"
)
# "12.03" is a date, "0.05" and "1.25 мг" are doses: two-digit day, no unit.
RE_DAY_MONTH_NUM = re.compile(
    r"(?<![\d.,])(?:0[1-9]|[12]\d|3[01])\.(?:0[1-9]|1[0-2])(?![\d.,])(?!\s*(?:мг|mg|мл|ml|г\b|%|мкг|ед))"
)
# Birth: here even month and year identify, so they are masked too.
RE_BIRTH = re.compile(
    r"(?i)(?:родил(?:ся|ась|ись)|рожден\w*|рождён\w*|день\s+рождени\w+|д\.\s?р\.|др\b|born|birthday|date\s+of\s+birth|dob)"
    r"[\s:—\-]*(?:в|on|in)?\s*((?:(?:[0-2]?\d|3[01])\s+)?" + _MONTHS_RU + r"(?:\s+(?:19|20)\d{2}(?:\s*(?:г\.|года?))?)?|"
    r"(?:19|20)\d{2}(?:\s*(?:г\.|года?))?|" + _MONTHS_EN + r"(?:\s+\d{1,2})?(?:,?\s+(?:19|20)\d{2})?|"
    r"(?:[0-2]?\d|3[01])[./\-](?:0?[1-9]|1[0-2])(?:[./\-](?:(?:19|20)\d{2}|\d{2}))?" + _END + r")"
)


def dates(text: str) -> Iterator[Span]:
    for rx in (RE_DATE_NUM, RE_DATE_ISO, RE_DATE_RU, RE_DATE_EN, RE_DAY_MONTH_NUM):
        for m in rx.finditer(text):
            value = m.group(0)
            if rx is RE_DATE_RU and value.lower().startswith(("8 март", "8 мая", "9 мая", "1 мая", "23 феврал")) \
                    and not re.search(r"\d{4}", value):
                continue  # holidays, not someone's date
            key = " ".join(value.lower().split())
            yield Span(m.start(), m.end(), Kind.DATE, value, key, "date")
    for m in RE_BIRTH.finditer(text):
        value = m.group(1).strip()
        if not value:
            continue
        start = m.start(1) + (len(m.group(1)) - len(m.group(1).lstrip()))
        key = " ".join(value.lower().split())
        yield Span(start, start + len(value), Kind.DATE, value, key, "birth")
