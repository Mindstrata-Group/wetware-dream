"""Entity kinds, their alias labels and conflict priorities.

A kind is the category of a detected fragment. The same kind is rendered as a
Russian or an English alias label depending on the language of the request,
so an LLM answering in Russian sees ``ЛИЦО_1`` and one answering in English
sees ``PERSON_1``. Restoration accepts both spellings.
"""

from __future__ import annotations

from enum import Enum


class Kind(str, Enum):
    PERSON = "PERSON"
    PHONE = "PHONE"
    EMAIL = "EMAIL"
    URL = "URL"
    HANDLE = "HANDLE"
    ADDRESS = "ADDRESS"
    PLACE = "PLACE"
    ORG = "ORG"
    DATE = "DATE"
    DOCUMENT = "DOCUMENT"
    CARD = "CARD"
    ACCOUNT = "ACCOUNT"


# Direct identifiers point at one human on their own. The quality target for
# them is full recall on the gold set; indirect ones are best effort.
DIRECT = frozenset({
    Kind.PERSON, Kind.PHONE, Kind.EMAIL, Kind.URL, Kind.HANDLE,
    Kind.ADDRESS, Kind.DOCUMENT, Kind.CARD, Kind.ACCOUNT,
})

LABELS_RU = {
    Kind.PERSON: "ЛИЦО",
    Kind.PHONE: "ТЕЛЕФОН",
    Kind.EMAIL: "ПОЧТА",
    Kind.URL: "ССЫЛКА",
    Kind.HANDLE: "НИК",
    Kind.ADDRESS: "АДРЕС",
    Kind.PLACE: "МЕСТО",
    Kind.ORG: "ОРГ",
    Kind.DATE: "ДАТА",
    Kind.DOCUMENT: "ДОКУМЕНТ",
    Kind.CARD: "КАРТА",
    Kind.ACCOUNT: "СЧЁТ",
}

LABELS_EN = {
    Kind.PERSON: "PERSON",
    Kind.PHONE: "PHONE",
    Kind.EMAIL: "EMAIL",
    Kind.URL: "LINK",
    Kind.HANDLE: "HANDLE",
    Kind.ADDRESS: "ADDRESS",
    Kind.PLACE: "PLACE",
    Kind.ORG: "ORG",
    Kind.DATE: "DATE",
    Kind.DOCUMENT: "DOCUMENT",
    Kind.CARD: "CARD",
    Kind.ACCOUNT: "ACCOUNT",
}

# Tokens of the monthly irreversible pass. They predate this service and are
# kept verbatim so already anonymized history stays uniform.
LEGACY_TOKENS = {
    Kind.PERSON: "[ИМЯ]",
    Kind.PHONE: "[ТЕЛЕФОН]",
    Kind.EMAIL: "[EMAIL]",
    Kind.URL: "[ССЫЛКА]",
    Kind.HANDLE: "[КОНТАКТ]",
    Kind.ADDRESS: "[АДРЕС]",
    Kind.PLACE: "[МЕСТО]",
    Kind.ORG: "[ОРГАНИЗАЦИЯ]",
    Kind.DATE: "[ДАТА]",
    Kind.DOCUMENT: "[ДОКУМЕНТ]",
    Kind.CARD: "[КАРТА]",
    Kind.ACCOUNT: "[СЧЁТ]",
}

# Higher wins when two fragments overlap: a link contains an e-mail, an
# address contains a surname in the street name, and so on.
PRIORITY = {
    Kind.URL: 100,
    Kind.EMAIL: 95,
    Kind.HANDLE: 90,
    Kind.CARD: 85,
    Kind.ACCOUNT: 84,
    Kind.DOCUMENT: 83,
    Kind.PHONE: 80,
    Kind.ADDRESS: 70,
    Kind.DATE: 60,
    Kind.PERSON: 50,
    Kind.ORG: 40,
    Kind.PLACE: 30,
}


def label(kind: Kind, lang: str) -> str:
    return (LABELS_EN if lang == "en" else LABELS_RU)[kind]


def kind_by_label() -> dict[str, Kind]:
    out: dict[str, Kind] = {}
    for table in (LABELS_RU, LABELS_EN):
        for kind, text in table.items():
            out[text] = kind
    # Models sometimes drop the diaeresis.
    out["СЧЕТ"] = Kind.ACCOUNT
    return out
