"""Generator of the synthetic gold set.

Every text here is invented: templates of things people tell a therapist,
filled with random names, phones, addresses and dates. Gold spans are written
inline as {{KIND|surface}} and parsed by tests/gold/markup.py.

    python -m tests.gold.generate   # rewrites gold_ru.jsonl and gold_en.jsonl

The output is deterministic (fixed seed) and committed; test_gold.py checks
that the generator still reproduces it, so a template change is visible in
review as a change of the gold files.
"""

from __future__ import annotations

import json
import random
from pathlib import Path

import pymorphy3

HERE = Path(__file__).parent
MORPH = pymorphy3.MorphAnalyzer()

FIRST_M = ["Сергей", "Андрей", "Дмитрий", "Игорь", "Олег", "Павел", "Николай", "Артём", "Прохор",
           "Ерофей", "Добрыня", "Всеволод", "Тимофей", "Богдан", "Эдуард", "Руслан", "Ильдар", "Марат"]
FIRST_F = ["Наталья", "Людмила", "Юлия", "Екатерина", "Ольга", "Мария", "Эвелина", "Милослава",
           "Василиса", "Резеда", "Гульнара", "Злата", "Таисия", "Анастасия", "Вероника", "Полина"]
DIM_M = ["Серёжа", "Андрюша", "Дима", "Игорёк", "Паша", "Коля", "Тёма", "Лёша", "Вова", "Стёпа", "Гоша"]
DIM_F = ["Люда", "Юля", "Катя", "Оля", "Маша", "Наташа", "Настя", "Танюша", "Ксюша", "Лера", "Женя", "Наташка"]
SURNAMES = ["Петров", "Кузнецов", "Смирнов", "Ковалёв", "Прохорчук", "Кузюк", "Шмыгло", "Белоусов",
            "Гарипов", "Зайцев", "Мельниченко", "Островский", "Воронин", "Лапшин", "Тагиров", "Ким",
            "Шевчук", "Абрамян", "Фёдоров", "Крапивин"]
PATR_M = ["Сергеевич", "Викторович", "Андреевич", "Игоревич", "Олегович", "Рашидович"]
PATR_F = ["Сергеевна", "Викторовна", "Андреевна", "Игоревна", "Олеговна", "Ильинична"]
CITIES = ["Екатеринбург", "Нижний Тагил", "Каменск-Уральский", "Челябинск", "Тюмень", "Пермь",
          "Верхняя Пышма", "Асбест", "Новоуральск", "Ирбит", "Сысерть", "Берёзовский"]
STREETS = ["Малышева", "Ленина", "Сибирский тракт", "Белинского", "Космонавтов", "Бардина",
           "Крауля", "Шефская", "Амундсена", "Уральская"]
COMPANIES = ["Уралхиммаш", "Синара", "Ромашка", "ТехноПарк", "Вектор-Плюс", "Альфа-Строй", "Медсервис",
             "Горизонт", "Северный ветер", "ЭнергоСбыт"]
UNIS = ["УрФУ", "УГМУ", "УрГЭУ", "УрГПУ", "МГУ", "СПбГУ", "ВШЭ", "УрГЮУ"]
DIAGNOSES = ["ОКР", "СДВГ", "ПТСР", "БАР", "депрессивный эпизод", "генерализованное тревожное расстройство",
             "паническое расстройство", "РПП", "ПРЛ"]
DRUGS = ["сертралин 50 мг", "эсциталопрам 10 мг", "кветиапин 25 мг", "ламотриджин 100 мг", "флуоксетин 20 мг"]
MONTHS = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября",
          "октября", "ноября", "декабря"]
DOMAINS = ["mail.ru", "yandex.ru", "gmail.com", "bk.ru", "inbox.ru", "list.ru", "rambler.ru"]

EN_FIRST = ["Sarah", "Emily", "Jessica", "Hannah", "Olivia", "Megan", "Rachel", "Laura", "Chloe",
            "Michael", "David", "James", "Daniel", "Ryan", "Kevin", "Brian", "Tyler", "Nathan"]
EN_LAST = ["Johnson", "Miller", "Thompson", "Walker", "Harrison", "Brennan", "Kowalski", "Nguyen",
           "O'Connor", "Fitzgerald", "Hughes", "Patel"]
EN_CITIES = ["Portland", "Leeds", "Brighton", "Denver", "Toronto", "Austin"]
EN_COMPANIES = ["Deloitte", "Acme Logistics", "Brightside Clinic", "Northwind Traders", "Globex"]


def inflect(word: str, case: str, tag: str | None = None, gender: str | None = None) -> str:
    """Inflect with pymorphy, restricted to person/geo readings."""
    if case == "nomn":
        return word
    parts = word.split(" ")
    out = []
    for part in parts:
        sub = part.split("-")
        res = []
        for piece in sub:
            cands = [p for p in MORPH.parse(piece) if (tag is None or tag in p.tag)]
            if gender:
                cands = [p for p in cands if p.tag.gender == gender or "ms-f" in p.tag] or cands
            cands = [p for p in cands if "nomn" in p.tag] or cands
            form = None
            for p in cands:
                want = {case, "sing"}
                if gender and p.tag.gender in ("masc", "femn") and p.tag.POS != "ADJF":
                    want.add(p.tag.gender)
                f = p.inflect(want)
                if f:
                    form = f.word
                    break
            if form is None:
                res.append(piece)
            else:
                res.append(form[:1].upper() + form[1:] if piece[:1].isupper() else form)
        out.append("-".join(res))
    return " ".join(out)


def fem_surname(s: str) -> str:
    if s.endswith(("ов", "ев", "ёв", "ин")):
        return s + "а"
    if s.endswith("ский"):
        return s[:-2] + "ая"
    return s


class Filler:
    def __init__(self, rnd: random.Random) -> None:
        self.r = rnd

    def g(self, kind: str, surface: str) -> str:
        return "{{" + kind + "|" + surface + "}}"

    def first(self, case: str, dim: bool | None = None, lower: bool = False):
        female = self.r.random() < 0.5
        dim = self.r.random() < 0.5 if dim is None else dim
        pool = (DIM_F if female else DIM_M) if dim else (FIRST_F if female else FIRST_M)
        name = self.r.choice(pool)
        form = inflect(name, case, "Name", "femn" if female else "masc")
        if form == name and case != "nomn":
            form = _dim_case(name, case)
        if lower:
            form = form.lower()
        return self.g("PERSON", form), female

    def full(self, case: str):
        female = self.r.random() < 0.5
        first = self.r.choice(FIRST_F if female else FIRST_M)
        sur = self.r.choice(SURNAMES)
        sur = fem_surname(sur) if female else sur
        g = "femn" if female else "masc"
        style = self.r.choice(["fs", "sfp", "fps", "si"])
        f_ = inflect(first, case, "Name", g)
        s_ = inflect(sur, case, "Surn", g)
        if s_ == sur and case != "nomn":
            s_ = _surname_case(sur, case, g)
        if style == "fs":
            text = f"{f_} {s_}"
        elif style == "sfp":
            p = self.r.choice(PATR_F if female else PATR_M)
            text = f"{s_} {f_} {inflect(p, case, 'Patr', g)}"
        elif style == "fps":
            p = self.r.choice(PATR_F if female else PATR_M)
            text = f"{f_} {inflect(p, case, 'Patr', g)}"
        else:
            text = f"{s_} {first[0]}.{self.r.choice('АВГДИКМНОПС')}."
        return self.g("PERSON", text)

    def surname(self, case: str):
        female = self.r.random() < 0.5
        sur = self.r.choice(SURNAMES)
        sur = fem_surname(sur) if female else sur
        g = "femn" if female else "masc"
        s_ = inflect(sur, case, "Surn", g)
        if s_ == sur and case != "nomn":
            s_ = _surname_case(sur, case, g)
        return self.g("PERSON", s_)

    def phone(self):
        d = "9" + "".join(self.r.choice("0123456789") for _ in range(9))
        fmt = self.r.choice([
            "+7 {0}{1}{2} {3}{4}{5}-{6}{7}-{8}{9}", "8 ({0}{1}{2}) {3}{4}{5}-{6}{7}-{8}{9}",
            "8{0}{1}{2}{3}{4}{5}{6}{7}{8}{9}", "+7{0}{1}{2}{3}{4}{5}{6}{7}{8}{9}",
            "8-{0}{1}{2}-{3}{4}{5}-{6}{7}-{8}{9}", "{0}{1}{2} {3}{4}{5} {6}{7} {8}{9}",
            "+7 ({0}{1}{2}) {3}{4}{5} {6}{7} {8}{9}", "8 {0}{1}{2} {3}{4}{5} {6}{7}{8}{9}",
        ])
        return self.g("PHONE", fmt.format(*d))

    def phone_spoken(self):
        return self.g("PHONE", self.r.choice([
            "восемь девятьсот двенадцать триста сорок пять шестьдесят семь восемьдесят девять",
            "плюс семь девятьсот девяносто девять сто двадцать три ноль ноль один один",
            "восемь девять один шесть пять пять пять ноль ноль один два",
        ]))

    def email(self):
        user = self.r.choice(["yulia.k", "kotik_88", "sergey.petrov", "natasha1990", "i.ivanova",
                              "maria-smirnova", "dima.ural", "lena_v"])
        return self.g("EMAIL", f"{user}@{self.r.choice(DOMAINS)}")

    def handle(self):
        return self.g("HANDLE", "@" + self.r.choice(["yulka_k", "serega_ekb", "natali_psy", "kot_begemot",
                                                     "dimon1990", "masha.art"]))

    def url(self):
        return self.g("URL", self.r.choice([
            "https://vk.com/id12345678", "vk.com/masha_art", "https://t.me/serega_ekb",
            "instagram.com/natali.psy", "https://docs.google.com/document/d/1AbCdEf/edit",
            "https://disk.yandex.ru/d/Xy12Ab34",
        ]))

    def address(self):
        st = self.r.choice(STREETS)
        n = self.r.randint(1, 150)
        flat = self.r.randint(1, 300)
        return self.g("ADDRESS", self.r.choice([
            f"ул. {st}, д. {n}, кв. {flat}", f"улица {st} {n}", f"ул. {st} {n}-{flat}",
            f"проспект Космонавтов, {n}", f"д. {n}, кв. {flat}",
        ]))

    def city(self, case: str):
        c = self.r.choice(CITIES)
        return self.g("PLACE", inflect(c, case, None))

    def company(self):
        c = self.r.choice(COMPANIES)
        return self.g("ORG", self.r.choice([f"«{c}»", f"ООО «{c}»", c]))

    def school(self):
        return self.g("ORG", self.r.choice([
            f"школе № {self.r.randint(1, 200)}", f"{self.r.randint(1, 200)}-й школе",
            f"гимназии № {self.r.randint(1, 50)}", f"детском саду № {self.r.randint(1, 500)}",
            f"больнице № {self.r.randint(1, 40)}", f"поликлинике № {self.r.randint(1, 60)}",
        ]))

    def uni(self):
        return self.g("ORG", self.r.choice(UNIS))

    def date(self):
        d, m, y = self.r.randint(1, 28), self.r.randint(1, 12), self.r.randint(1995, 2025)
        return self.g("DATE", self.r.choice([
            f"{d} {MONTHS[m - 1]} {y}", f"{d:02d}.{m:02d}.{y}", f"{d} {MONTHS[m - 1]}", f"{d:02d}.{m:02d}.{y % 100:02d}",
        ]))

    def birth(self):
        d, m, y = self.r.randint(1, 28), self.r.randint(1, 12), self.r.randint(1960, 2010)
        return self.g("DATE", self.r.choice([f"{d} {MONTHS[m - 1]} {y} года", f"{d:02d}.{m:02d}.{y}"]))

    def snils(self):
        while True:
            body = "".join(self.r.choice("0123456789") for _ in range(9))
            if int(body) > 1001998:
                break
        s = sum(int(x) * (9 - i) for i, x in enumerate(body))
        s = s % 101 if s > 101 else s
        s = 0 if s in (100, 101) else s
        d = body + f"{s:02d}"
        return self.g("DOCUMENT", f"{d[:3]}-{d[3:6]}-{d[6:9]} {d[9:]}")

    def card(self):
        digits = [4, 2, 7, 6] + [self.r.randint(0, 9) for _ in range(11)]
        total = 0
        for i, n in enumerate(reversed(digits)):
            if i % 2 == 0:
                n *= 2
                n = n - 9 if n > 9 else n
            total += n
        digits.append((10 - total % 10) % 10)
        s = "".join(map(str, digits))
        return self.g("CARD", " ".join(s[i:i + 4] for i in range(0, 16, 4)))

    def passport(self):
        return self.g("DOCUMENT", f"{self.r.randint(10, 99)}{self.r.randint(10, 99)} {self.r.randint(100000, 999999)}")

    def inn(self):
        while True:
            d = [self.r.randint(0, 9) for _ in range(10)]
            c1 = sum(x * c for x, c in zip(d, [7, 2, 4, 10, 3, 5, 9, 4, 6, 8])) % 11 % 10
            c2 = sum(x * c for x, c in zip(d + [c1], [3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8])) % 11 % 10
            return self.g("DOCUMENT", "".join(map(str, d + [c1, c2])))


def _dim_case(name: str, case: str) -> str:
    stem, last = name[:-1], name[-1]
    if last == "я":
        table = {"gent": "и", "datv": "е", "accs": "ю", "ablt": "ей", "loct": "е"}
    elif last == "а":
        soft = stem[-1] in "гкхжшщч"
        table = {"gent": "и" if soft else "ы", "datv": "е", "accs": "у",
                 "ablt": "ей" if stem[-1] in "жшщчц" else "ой", "loct": "е"}
    else:
        return name + {"gent": "а", "datv": "у", "accs": "а", "ablt": "ом", "loct": "е"}[case]
    return stem + table[case]


def _surname_case(sur: str, case: str, g: str) -> str:
    if g == "femn" and sur.endswith("а"):
        return sur[:-1] + {"gent": "ой", "datv": "ой", "accs": "у", "ablt": "ой", "loct": "ой"}[case]
    if g == "femn":
        return sur
    if sur.endswith(("о", "ук", "юк", "ян", "м")) and not sur.endswith(("ук", "юк", "ян", "м")):
        return sur
    return sur + {"gent": "а", "datv": "у", "accs": "а", "ablt": "ом" if not sur.endswith(("ов", "ев", "ин")) else "ым", "loct": "е"}[case]


RU_TEMPLATES = [
    lambda f: f"Вчера опять поругалась с мамой {f.first('ablt')[0]}, она звонила мне с {f.phone()} раз десять.",
    lambda f: f"Муж {f.first('nomn', True, True)[0]} говорит, что я всё придумываю.",
    lambda f: f"Мой начальник {f.full('nomn')} кричит на планёрках, я работаю в {f.company()} уже третий год.",
    lambda f: f"Написала {f.first('datv')[0]} на почту {f.email()}, но она так и не ответила.",
    lambda f: f"Живём сейчас по адресу {f.address()}, соседи шумят ночами.",
    lambda f: f"Сын учится в {f.school()}, классная руководительница {f.full('nomn')} постоянно жалуется.",
    lambda f: f"Родилась {f.birth()}, в детстве много болела.",
    lambda f: f"С {f.date()} не сплю нормально, психиатр назначил {f.r.choice(DRUGS)}.",
    lambda f: f"Диагноз {f.r.choice(DIAGNOSES)} мне поставили в ПНД, врач {f.surname('nomn')} был очень холоден.",
    lambda f: f"Подруга {f.first('nomn')[0]} уехала в {f.city('accs')}, и мне стало совсем одиноко.",
    lambda f: f"Мы переехали из {f.city('gent')} два года назад, я до сих пор скучаю.",
    lambda f: f"Мой телеграм {f.handle()}, пишите туда, если что.",
    lambda f: f"Вот моя страница {f.url()}, там всё про меня.",
    lambda f: f"Продиктую номер: {f.phone_spoken()}.",
    lambda f: f"Сестра {f.first('nomn', True)[0]} считает, что у меня {f.r.choice(DIAGNOSES)}, но это не так.",
    lambda f: f"Свекровь {f.full('nomn')} опять приехала без предупреждения.",
    lambda f: f"Я учусь в {f.uni()} на третьем курсе и почти не хожу на пары.",
    lambda f: f"Бывший муж {f.surname('nomn')} не платит алименты с {f.date()}.",
    lambda f: f"Созвонилась с {f.full('ablt')}, договорились встретиться в {f.city('loct')}.",
    lambda f: f"СНИЛС {f.snils()}, если нужно для справки.",
    lambda f: f"Паспорт {f.passport()}, выдан в прошлом году.",
    lambda f: f"Перевела деньги на карту {f.card()}, но психолог их не получил.",
    lambda f: f"ИНН {f.inn()} нужен для вычета за терапию.",
    lambda f: f"Дочка {f.first('nomn', True)[0]} ходит в {f.school()}, её обижают одноклассники.",
    lambda f: f"Коллега {f.first('nomn')[0]} {f.surname('nomn')} постоянно сваливает на меня свою работу.",
    lambda f: f"Я боюсь звонить {f.first('datv')[0]}, вдруг она опять накричит.",
    lambda f: f"Разговаривала с {f.first('ablt', True)[0]} про папу, стало легче.",
    lambda f: f"У {f.first('gent')[0]} день рождения, а я даже не хочу поздравлять.",
    lambda f: f"Папа {f.first('nomn', False)[0]} пьёт с тех пор, как его уволили из {f.company()}.",
    lambda f: f"Меня зовут {f.full('nomn')}, мне 34 года, у меня {f.r.choice(DIAGNOSES)}.",
    lambda f: f"Можно мне на почту {f.email()} прислать запись сессии?",
    lambda f: f"Позвоните мне, пожалуйста, {f.phone()}, я не могу писать.",
    lambda f: f"Брат {f.first('nomn', True, True)[0]} сказал, что я истеричка.",
    lambda f: f"С {f.first('ablt', True, True)[0]} мы расстались в {f.r.choice(['марте', 'мае', 'октябре'])}, до сих пор больно.",
    lambda f: f"Терапевт {f.full('nomn')} из клиники {f.company()} посоветовала вести дневник.",
    lambda f: f"Работаю медсестрой в {f.school()}, график ужасный.",
    lambda f: f"Приезжайте ко мне: {f.address()}, код домофона скажу.",
    lambda f: f"Тёща {f.first('nomn')[0]} звонит каждый день с {f.phone()} и контролирует жену.",
    lambda f: f"Мой партнёр {f.first('nomn')[0]} против того, чтобы я ходила к психологу.",
    lambda f: f"Ребёнку {f.first('datv', True)[0]} шесть лет, он не спит без света.",
    lambda f: f"Вчера видела {f.surname('accs')} в торговом центре и чуть не расплакалась.",
    lambda f: f"Дедушка {f.full('nomn')} умер {f.date()}, я не успела попрощаться.",
    lambda f: f"Подружка {f.first('nomn', True, True)[0]} пишет мне в {f.url()} каждый вечер.",
    lambda f: f"Уволилась из {f.company()}, теперь сижу дома в {f.city('loct')}.",
    lambda f: f"Про {f.first('accs')[0]} даже думать не хочу после того разговора.",
    lambda f: f"Записалась на приём к {f.surname('datv')} на {f.date()}.",
    lambda f: f"Племянник {f.first('nomn', True)[0]} живёт у нас, пока его мама в больнице.",
    lambda f: f"мой номер {f.phone()} пишите в вотсап",
    lambda f: f"Руководитель {f.full('nomn')} угрожает уволить, если я возьму больничный.",
    lambda f: f"Меня травил одноклассник {f.surname('nomn')} в {f.school()} много лет.",
    lambda f: f"Оформили опеку, суд был {f.date()} в {f.city('loct')}.",
    lambda f: f"почта для связи {f.email()} телефон {f.phone()}",
    lambda f: f"Моя психотерапевт {f.full('nomn')} ушла в декрет, и я не знаю, к кому идти.",
    lambda f: f"Созвонились с {f.g('PERSON', 'Kate')} из {f.company()}, она сказала, что {f.g('PERSON', 'Ivan Petrov')} уволился.",
    lambda f: f"Я сказала {f.first('datv')[0]}, что больше так не могу.",
    lambda f: f"Ходили с {f.first('ablt')[0]} к семейному психологу, стало только хуже.",
    lambda f: f"Живу с родителями на {f.g('ADDRESS', 'Малышева, 51')}, своей квартиры нет.",
    lambda f: f"Принимаю {f.r.choice(DRUGS)} с {f.date()}, побочки сильные.",
    lambda f: f"Одногруппница {f.first('nomn')[0]} выложила мои фото в {f.url()}.",
]

# Handwritten tricky cases: typos, lowercase, mixed scripts, dictation,
# things that must NOT be masked (diagnoses, doses, generic institutions).
RU_TRICKY = [
    "мама {{PERSON|люда}} опять звонила, я не взяла трубку",
    "{{PERSON|Сережа}} сказал что ему всё равно",
    "Мой бывший, {{PERSON|Кравцов}}, пишет с новых номеров: {{PHONE|+7 912 000-11-22}}.",
    "Врач сказала принимать 0.75 таблетки кветиапина, СДВГ под вопросом.",
    "Сдала анализы в поликлинике, в ПНД сказали прийти через месяц.",
    "Поговорила с {{PERSON|Натальей Сергеевной}} после сессии.",
    "Пишите на {{EMAIL|kotik_88@mail.ru}} или в тг {{HANDLE|@kot_begemot}}.",
    "Мне 28 лет, в 2019 году был первый эпизод депрессии.",
    "Родилась {{DATE|12.03.1990}}, живу в {{PLACE|Нижнем Тагиле}}.",
    "Номер карты {{CARD|4276 3801 2345 6787}}, переведите предоплату.",
    "моя почта {{EMAIL|иван точка петров собака мейл точка ру}}",
    "перезвоните на {{PHONE|восемь девятьсот двенадцать триста сорок пять шестьдесят семь восемьдесят девять}}",
    "С {{PERSON|Юлей}} и {{PERSON|Настей}} больше не общаюсь.",
    "{{PERSON|Наташка}} опять не отвечает, а с {{PERSON|Петровой}} из бухгалтерии я не разговариваю.",
    "Отец, {{PERSON|Игорь Викторович}}, всегда был холоден.",
    "Muж {{PERSON|Серёжа}} против терапии.",
    "Моя дочь {{PERSON|Ксюша}} ходит в {{ORG|детский сад № 312}}.",
    "Работаю в {{ORG|«Синаре»}} бухгалтером.",
    "Учусь в {{ORG|УрФУ}}, живу в общаге на {{ADDRESS|ул. Комсомольская, 70}}.",
    "Позвоните по номеру {{PHONE|89123456789}}, спросите {{PERSON|Людмилу}}.",
    "У меня ПТСР после ДТП, лечусь у психиатра.",
    "Подруга {{PERSON|Лена}} и её муж {{PERSON|Вадик}} зовут на дачу.",
    "Классная руководительница {{PERSON|Ольга Петровна}} вызвала меня в школу.",
    "Сессия была {{DATE|15 сентября 2025}}, после неё стало легче.",
    "Мой ник в инсте {{HANDLE|natali.psy}}, там дневник эмоций.",
    "Мне нравится читать про ОКР и СДВГ на форумах.",
    "Со мной работала {{PERSON|Резеда Ринатовна}}, очень тёплый специалист.",
    "Когда {{PERSON|Тёма}} плачет, я не знаю, что делать.",
    "Созвонилась с {{PERSON|Kate}} из {{ORG|Deloitte}}, она обещала помочь.",
    "{{PERSON|Анна Ивановна Шмыгло}} — моя бабушка.",
    "Паспорт серия {{DOCUMENT|6512 345678}}.",
    "полис омс {{DOCUMENT|6655443322110099}}",
    "Живу на {{ADDRESS|Космонавтов 5}}, это рядом с парком.",
    "Сегодня говорили с терапевтом про тревогу и границы.",
    "Лекарство 1.25 мг утром, 2.5 мг вечером.",
    "Встреча {{DATE|12.10}} в 18:00, не забыть.",
    "Бабушка {{PERSON|Зина}} живёт в {{PLACE|Ирбите}}, я езжу к ней раз в месяц.",
    "Тётя {{PERSON|Галя}} и дядя {{PERSON|Боря}} опять поссорились.",
    "Мой {{PHONE|+44 7911 123456}}, я сейчас в {{PLACE|Лондоне}}.",
    "Пиши в личку {{URL|t.me/serega_ekb}}.",
]

# Texts without any personal data: what a therapy chat is mostly about.
# Every span found here is a false positive.
RU_NEGATIVE = [
    "Сегодня весь день тревога, не могу сосредоточиться на работе.",
    "Психиатр сказал, что при ОКР помогает экспозиция с предотвращением реакции.",
    "Принимаю сертралин 50 мг уже три месяца, побочки почти прошли.",
    "Мне кажется, что я недостаточно хороша для этих отношений.",
    "Когда начальник повышает голос, у меня трясутся руки.",
    "Вера в себя появилась только после второго года терапии.",
    "Надежда есть, но сил пока нет.",
    "Любовь к себе — это не эгоизм, я только сейчас это поняла.",
    "В детстве меня часто оставляли одну дома.",
    "Панические атаки начались после родов, примерно полгода назад.",
    "Я боюсь, что если скажу правду, от меня все отвернутся.",
    "Сон 4-5 часов, просыпаюсь в 3 ночи и не могу уснуть.",
    "Лечащий врач предложил увеличить дозу до 1.5 таблетки.",
    "Мне 35 лет, у меня двое детей и ипотека на 20 лет.",
    "В 2020 году я потеряла работу и с тех пор не могу найти себя.",
    "Хочу разобраться, почему я всё время извиняюсь.",
    "Группа поддержки по вторникам мне очень помогает.",
    "Прочитала книгу «Тело помнит всё», много узнала о травме.",
    "Мама всегда говорила, что я ничего не добьюсь.",
    "Папа ушёл из семьи, когда мне было семь.",
    "Муж не понимает, почему я плачу без причины.",
    "С подругой поссорились из-за ерунды, теперь не общаемся.",
    "Начальница опять задержала зарплату на неделю.",
    "Врач в поликлинике отправил к неврологу, а невролог — к психиатру.",
    "Я понимаю головой, что это иррационально, но тело реагирует.",
    "Стало лучше после того, как я начала гулять по 30 минут в день.",
    "ПОМОГИТЕ, я не знаю, что делать дальше.",
    "Мне поставили СДВГ во взрослом возрасте, и многое встало на места.",
    "Говорили на сессии про границы и чувство вины.",
    "В марте будет год, как я не пью.",
]
EN_NEGATIVE = [
    "I feel anxious every morning before work.",
    "My therapist suggested CBT for the intrusive thoughts.",
    "I have been on 20 mg of fluoxetine since last spring.",
    "Hope is the only thing that keeps me going.",
    "My mom always said I was too sensitive.",
    "In March I will be one year sober.",
    "Will it ever get better?",
    "I talked to my boss about the workload and it went fine.",
    "Sleep has been terrible, maybe 4 hours a night.",
    "The group session on Tuesday helped a lot.",
]

EN_TEMPLATES = [
    lambda f: f"My wife {f.g('PERSON', f.r.choice(EN_FIRST))} says I overreact to everything.",
    lambda f: f"I had another fight with my mom, {f.g('PERSON', f.r.choice(EN_FIRST) + ' ' + f.r.choice(EN_LAST))}.",
    lambda f: f"You can reach me at {f.g('PHONE', f.r.choice(['(555) 123-4567', '555-867-5309', '+1 415 555 0132', '+44 7911 123456']))}.",
    lambda f: f"My email is {f.g('EMAIL', f.r.choice(['sarah.j@gmail.com', 'dmiller88@yahoo.com', 'kate_w@outlook.com']))}.",
    lambda f: f"I work at {f.g('ORG', f.r.choice(EN_COMPANIES))} and my boss {f.g('PERSON', f.r.choice(EN_FIRST))} micromanages me.",
    lambda f: f"We moved to {f.g('PLACE', f.r.choice(EN_CITIES))} last year and I still have no friends.",
    lambda f: f"I live at {f.g('ADDRESS', f.r.choice(['42 Maple Street, Apt 3B', '1200 Oak Avenue', '7 Rosewood Lane']))}.",
    lambda f: f"My therapist, {f.g('PERSON', 'Dr. ' + f.r.choice(EN_LAST))}, suggested journaling.",
    lambda f: f"I was born on {f.g('DATE', f.r.choice(['March 12, 1990', '12 March 1990', '03/12/1990']))}.",
    lambda f: f"My son {f.g('PERSON', f.r.choice(EN_FIRST))} goes to {f.g('ORG', 'Lincoln High School')}.",
    lambda f: f"I talked to {f.g('PERSON', f.r.choice(EN_FIRST) + ' ' + f.r.choice(EN_LAST))} about the panic attacks.",
    lambda f: f"My SSN is {f.g('DOCUMENT', '123-45-6789')}, do you need it for insurance?",
    lambda f: f"Find me on Instagram {f.g('HANDLE', '@kate.draws')} or {f.g('URL', 'facebook.com/kate.walker')}.",
    lambda f: f"I have ADHD and take 20 mg of Adderall in the morning.",
    lambda f: f"Since {f.g('DATE', f.r.choice(['June 3rd', 'April 21', 'September 9, 2024']))} I can't sleep.",
    lambda f: f"My ex, {f.g('PERSON', f.r.choice(EN_FIRST))}, keeps texting me from {f.g('PHONE', '555.234.9876')}.",
    lambda f: f"I studied at {f.g('ORG', 'University of Leeds')} and hated every minute.",
    lambda f: f"Call my sister {f.g('PERSON', f.r.choice(EN_FIRST).lower())} if I don't show up.",
    lambda f: f"Мой терапевт — {f.g('PERSON', 'Dr. Anna Klein')}, она из {f.g('PLACE', 'Берлина')}.",
    lambda f: f"My friend {f.g('PERSON', 'Natasha Ivanova')} moved from {f.g('PLACE', 'Moscow')} to {f.g('PLACE', 'Toronto')}.",
]
EN_TRICKY = [
    "I feel sad in May and happy in June.",
    "Will you help me with my anxiety?",
    "Grace under pressure is not my thing.",
    "My daughter {{PERSON|Lily}} is scared of the dark.",
    "{{PERSON|Mrs. Hughes}} from next door keeps calling the police.",
    "Text me: {{PHONE|+1 (646) 555-0199}}.",
    "I met {{PERSON|Jessica Brennan}} at {{ORG|Brightside Clinic}} on {{DATE|April 3, 2025}}.",
    "My husband {{PERSON|Tom}} and I are thinking about couples therapy.",
    "Email me at {{EMAIL|tyler.nguyen@proton.me}} after the session.",
    "I take 50 mg of sertraline and 0.5 mg of clonazepam.",
    "My card is {{CARD|4111 1111 1111 1111}}, charge the copay.",
    "Our address is {{ADDRESS|15 Kings Road}}, {{PLACE|Brighton}}.",
    "I'm from {{PLACE|Austin}} but I live in {{PLACE|Denver}} now.",
    "My boss, {{PERSON|Kevin Harrison}}, said I'm too emotional.",
    "The kids, {{PERSON|Emma}} and {{PERSON|Noah}}, are with their dad this week.",
]


def build(templates, tricky, negative, n: int, seed: int) -> list[dict]:
    rnd = random.Random(seed)
    f = Filler(rnd)
    rows = []
    for i in range(n):
        tpl = templates[i % len(templates)]
        rows.append({"id": f"gen-{i:04d}", "text": tpl(f)})
    for i, line in enumerate(tricky):
        rows.append({"id": f"tricky-{i:03d}", "text": line})
    for i, line in enumerate(negative):
        rows.append({"id": f"negative-{i:03d}", "text": line})
    return rows


def main() -> None:
    ru = build(RU_TEMPLATES, RU_TRICKY, RU_NEGATIVE, 300, 20260930)
    en = build(EN_TEMPLATES, EN_TRICKY, EN_NEGATIVE, 100, 20260931)
    for name, rows in (("gold_ru.jsonl", ru), ("gold_en.jsonl", en)):
        with open(HERE / name, "w", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps(row, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
