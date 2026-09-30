"""Quality run on real conversations. Prints aggregates only.

Input: CSV on stdin, one row per user message, columns
    user_id, message_id, content, display_name, email, phone, telegram_username
(produced by psql COPY on a read-only connection, see apps/anonymizer/README.md).

The recall oracle is the user's own profile: if a message contains the
user's name, e-mail, phone or Telegram username (in any case form), that
fragment must be gone after masking. Two modes are measured:

  A. "with profile": the profile is passed as known values, as the Go API does
     in production. Expected residual: 0 — this checks the plumbing on real text.
  B. "blind": no known values. The residual here is the honest recall of the
     detectors on real names, phones and e-mails that nobody told them about.

A second oracle scans the masked output with independent strict patterns
(10-11 digit phone shapes, anything@domain, Luhn-valid card numbers).

This tool must never print message text, profile values or spans. There is
no flag to do so. Errors are counted by exception type only.
"""

from __future__ import annotations

import csv
import json
import re
import resource
import statistics
import sys
import time
from collections import Counter

from . import known as known_mod
from . import morph, names_en, names_ru
from .detector import detect
from .guard import PLACEHOLDER, mask_for_check
from .kinds import Kind

_PHONE_SHAPE = re.compile(r"(?<!\d)(?:\+?7|8)?[\s\-()]*9\d{2}(?:[\s\-()]*\d){7}(?!\d)")
_EMAIL_SHAPE = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)+")
_CARD_SHAPE = re.compile(r"(?<!\d)\d(?:[\s\-]?\d){12,18}(?!\d)")


def _luhn(d: str) -> bool:
    total = 0
    for i, ch in enumerate(reversed(d)):
        n = int(ch)
        if i % 2:
            n = n * 2 - 9 if n * 2 > 9 else n * 2
        total += n
    return total % 10 == 0


def strict_hits(masked: str) -> int:
    n = 0
    for rx in (_PHONE_SHAPE, _EMAIL_SHAPE):
        n += sum(1 for m in rx.finditer(masked) if PLACEHOLDER not in m.group(0))
    for m in _CARD_SHAPE.finditer(masked):
        d = re.sub(r"\D", "", m.group(0))
        if 13 <= len(d) <= 19 and _luhn(d):
            n += 1
    return n


def _name_like(token: str) -> bool:
    return bool(names_ru.lookup(token) or morph.person_role(token) or names_en.is_first_name(token))


def profile_items(row: dict) -> dict[str, list[known_mod.Known]]:
    """Profile values grouped by oracle category."""
    groups: dict[str, list[known_mod.Known]] = {"name": [], "nick": [], "email": [], "phone": [], "handle": []}
    for tok in re.findall(r"[A-Za-zА-Яа-яЁё][A-Za-zА-Яа-яЁё'\-]{2,}", row.get("display_name") or ""):
        groups["name" if _name_like(tok) else "nick"].append(known_mod.Known(Kind.PERSON, tok))
    if (row.get("email") or "").strip():
        groups["email"].append(known_mod.Known(Kind.EMAIL, row["email"].strip()))
    if len(re.sub(r"\D", "", row.get("phone") or "")) >= 10:
        groups["phone"].append(known_mod.Known(Kind.PHONE, row["phone"]))
    if (row.get("telegram_username") or "").strip():
        groups["handle"].append(known_mod.Known(Kind.HANDLE, row["telegram_username"].strip()))
    return groups


def _shape(text: str, start: int, end: int) -> str:
    """Privacy-safe shape of a missed fragment: context class and script, never characters."""
    frag = text[start:end]
    left = text[max(0, start - 1):start]
    right = text[end:end + 1]
    script = "lat" if re.fullmatch(r"[A-Za-z0-9_.\-]+", frag) else ("cyr" if re.search(r"[А-Яа-яЁё]", frag) else "other")
    ctx = ("at" if left == "@" else "word" if left.isalnum() else "space" if left.isspace() or not left else "punct")
    after = "word" if right.isalnum() else "end" if not right else "space" if right.isspace() else "punct"
    window = text[max(0, start - 40):end + 15].lower()
    in_url = "plain"
    if re.search(r"(?:https?://|www\.|t\.me/|\.com|\.ru)", window):
        # a fixed list of public platforms; any other domain is just "other"
        platforms = ("t.me", "telegram", "vk.com", "instagram", "youtube", "tiktok", "mindstrata",
                     "mail.ru", "gmail", "yandex", "@")
        found = [p for p in platforms if p in window]
        in_url = "url:" + (",".join(found) if found else "other")
    length = "1-3" if len(frag) <= 3 else "4-6" if len(frag) <= 6 else "7-12" if len(frag) <= 12 else "13+"
    return f"{script}|left:{ctx}|right:{after}|{in_url}|len:{length}"


def _miss_shapes(text: str, items: list[known_mod.Known]) -> list[str]:
    return [_shape(text, s.start, s.end) for s in known_mod.find(text, items)
            if PLACEHOLDER not in text[s.start:s.end]]


def _hits(text: str, items: list[known_mod.Known]) -> int:
    """Number of distinct fragments (overlapping matches count once)."""
    ranges = sorted((s.start, s.end) for s in known_mod.find(text, items)
                    if PLACEHOLDER not in text[s.start:s.end])
    count, end = 0, -1
    for a, b in ranges:
        if a >= end:
            count += 1
        end = max(end, b)
    return count


def run(stream) -> dict:
    csv.field_size_limit(10_000_000)
    reader = csv.DictReader(stream, fieldnames=[
        "user_id", "message_id", "content", "display_name", "email", "phone", "telegram_username"])
    report: dict = {
        "messages": 0, "chars": 0, "users": 0, "errors": Counter(),
        "oracle": {k: {"messages_with_value": 0, "occurrences": 0, "residual_with_profile": 0,
                       "residual_blind": 0} for k in ("name", "nick", "email", "phone", "handle")},
        "strict_hits_with_profile": 0, "strict_hits_blind": 0,
        "detector_residual_with_profile": 0, "detector_residual_blind": 0,
        "guard_added_blind": 0, "masked_chars_blind": 0, "masked_chars_with_profile": 0,
        "spans_blind": Counter(), "spans_blind_by_source": Counter(), "messages_with_any_span_blind": 0,
        "blind_miss_shapes": Counter(),
    }
    users: set[str] = set()
    timings: list[float] = []
    for row in reader:
        text = row.get("content") or ""
        if not text.strip():
            continue
        report["messages"] += 1
        report["chars"] += len(text)
        users.add(row.get("user_id") or "")
        try:
            groups = profile_items(row)
            all_items = [i for g in groups.values() for i in g]
            t0 = time.perf_counter()
            blind = detect(text)
            timings.append((time.perf_counter() - t0) * 1000)
            full = detect(text, items=all_items)
            masked_blind = mask_for_check(text, blind.spans)
            masked_full = mask_for_check(text, full.spans)
            for cat, items in groups.items():
                if not items:
                    continue
                n = _hits(text, items)
                if n:
                    o = report["oracle"][cat]
                    o["messages_with_value"] += 1
                    o["occurrences"] += n
                    o["residual_blind"] += _hits(masked_blind, items)
                    o["residual_with_profile"] += _hits(masked_full, items)
                    for shape in _miss_shapes(masked_blind, items):
                        report["blind_miss_shapes"][f"{cat}:{shape}"] += 1
            report["strict_hits_blind"] += strict_hits(masked_blind)
            report["strict_hits_with_profile"] += strict_hits(masked_full)
            report["detector_residual_blind"] += blind.residual
            report["detector_residual_with_profile"] += full.residual
            report["guard_added_blind"] += blind.guard_added
            report["masked_chars_blind"] += sum(s.length for s in blind.spans)
            report["masked_chars_with_profile"] += sum(s.length for s in full.spans)
            if blind.spans:
                report["messages_with_any_span_blind"] += 1
            for s in blind.spans:
                report["spans_blind"][s.kind.value] += 1
                report["spans_blind_by_source"][f"{s.kind.value}/{s.source}"] += 1
        except Exception as exc:  # noqa: BLE001 - count, never print the row
            report["errors"][type(exc).__name__] += 1
    report["users"] = len(users)
    if timings:
        timings.sort()
        report["ms_per_message_blind"] = {
            "p50": round(statistics.median(timings), 2),
            "p95": round(timings[int(len(timings) * 0.95) - 1 if len(timings) > 1 else 0], 2),
            "max": round(timings[-1], 2),
        }
    if report["chars"]:
        report["masked_share_blind"] = round(report["masked_chars_blind"] / report["chars"], 4)
        report["masked_share_with_profile"] = round(report["masked_chars_with_profile"] / report["chars"], 4)
    report["max_rss_mb"] = round(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024, 1)
    report["errors"] = dict(report["errors"])
    report["spans_blind"] = dict(report["spans_blind"])
    report["spans_blind_by_source"] = dict(report["spans_blind_by_source"])
    report["blind_miss_shapes"] = dict(report["blind_miss_shapes"])
    return report


def main() -> None:
    try:
        report = run(sys.stdin)
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"fatal": type(exc).__name__}))
        sys.exit(1)
    print(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
