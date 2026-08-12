from __future__ import annotations

from datetime import date, datetime
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError
import re

SCHEMA_VERSION = "1.0"
_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$")
_TIME = re.compile(r"^(?:[01][0-9]|2[0-3]):[0-5][0-9]$")
_TYPES = {"lecture", "practice", "lab", "seminar", "exam", "credit", "consultation"}
_RECURRENCES = {"every", "odd", "even", "date", "cycle"}
MAX_GROUPS = 10_000
MAX_LESSONS = 500_000


class SnapshotValidationError(ValueError):
    def __init__(self, problems: list[str]):
        self.problems = problems
        super().__init__("connector snapshot is invalid: " + "; ".join(problems))


def validate_snapshot(snapshot: dict) -> None:
    problems: list[str] = []
    if snapshot.get("schema_version") != SCHEMA_VERSION:
        problems.append(f"schema_version must be {SCHEMA_VERSION}")
    _external_id(snapshot.get("snapshot_id"), "snapshot_id", problems)
    try:
        datetime.fromisoformat(str(snapshot.get("generated_at", "")).replace("Z", "+00:00"))
    except ValueError:
        problems.append("generated_at must be RFC3339")

    institution = snapshot.get("institution") or {}
    _external_id(institution.get("external_id"), "institution.external_id", problems)
    if not str(institution.get("name", "")).strip():
        problems.append("institution.name is required")
    try:
        ZoneInfo(str(institution.get("timezone", "")))
    except ZoneInfoNotFoundError:
        problems.append("institution.timezone must be an IANA timezone")

    term = snapshot.get("term") or {}
    _external_id(term.get("external_id"), "term.external_id", problems)
    term_start = _date(term.get("starts_on"), "term.starts_on", problems)
    term_end = _date(term.get("ends_on"), "term.ends_on", problems)
    if term_start and term_end and term_end < term_start:
        problems.append("term.ends_on must not be before starts_on")

    groups = snapshot.get("groups")
    if not isinstance(groups, list) or not groups:
        problems.append("groups must contain at least one group")
        groups = []
    if len(groups) > MAX_GROUPS:
        problems.append(f"groups exceeds the limit of {MAX_GROUPS}")
    group_ids: set[str] = set()
    group_names: set[str] = set()
    lesson_ids: set[str] = set()
    lesson_count = 0
    for group_index, group in enumerate(groups):
        prefix = f"groups[{group_index}]"
        group_id = str(group.get("external_id", ""))
        _external_id(group_id, prefix + ".external_id", problems)
        if group_id in group_ids:
            problems.append(prefix + ".external_id is duplicated")
        group_ids.add(group_id)
        if not str(group.get("name", "")).strip():
            problems.append(prefix + ".name is required")
        normalized_name = str(group.get("name", "")).strip().casefold()
        if normalized_name in group_names:
            problems.append(prefix + ".name is duplicated")
        group_names.add(normalized_name)
        lessons = group.get("lessons") or []
        if not isinstance(lessons, list):
            problems.append(prefix + ".lessons must be an array")
            continue
        for lesson_index, lesson in enumerate(lessons):
            lesson_count += 1
            lesson_prefix = f"{prefix}.lessons[{lesson_index}]"
            lesson_id = str(lesson.get("external_id", ""))
            _external_id(lesson_id, lesson_prefix + ".external_id", problems)
            if lesson_id in lesson_ids:
                problems.append(lesson_prefix + ".external_id is duplicated")
            lesson_ids.add(lesson_id)
            if not str(lesson.get("subject", "")).strip():
                problems.append(lesson_prefix + ".subject is required")
            if lesson.get("type") not in _TYPES:
                problems.append(lesson_prefix + ".type is not supported")
            subgroup = lesson.get("subgroup", 0)
            if not isinstance(subgroup, int) or subgroup < 0 or subgroup > 100:
                problems.append(lesson_prefix + ".subgroup must be between 0 and 100")
            schedule = lesson.get("schedule") or {}
            if not _TIME.match(str(schedule.get("starts_at", ""))) or not _TIME.match(str(schedule.get("ends_at", ""))):
                problems.append(lesson_prefix + ".schedule time must use HH:MM")
            elif schedule["starts_at"] >= schedule["ends_at"]:
                problems.append(lesson_prefix + ".schedule.ends_at must be after starts_at")
            recurrence = schedule.get("recurrence") or {}
            kind = recurrence.get("kind")
            if kind not in _RECURRENCES:
                problems.append(lesson_prefix + ".schedule.recurrence.kind is not supported")
            elif kind == "date":
                lesson_date = _date(schedule.get("date"), lesson_prefix + ".schedule.date", problems)
                if lesson_date and term_start and term_end and not term_start <= lesson_date <= term_end:
                    problems.append(lesson_prefix + ".schedule.date is outside the term")
            else:
                day = schedule.get("day_of_week", 0)
                if not isinstance(day, int) or day < 1 or day > 7:
                    problems.append(lesson_prefix + ".schedule.day_of_week must be between 1 and 7")
            if kind == "cycle":
                length = recurrence.get("cycle_length", 0)
                weeks = recurrence.get("cycle_weeks") or []
                if not isinstance(length, int) or length < 2 or length > 16:
                    problems.append(lesson_prefix + ".schedule.recurrence.cycle_length must be between 2 and 16")
                if not weeks or any(not isinstance(week, int) or week < 1 or week > length for week in weeks):
                    problems.append(lesson_prefix + ".schedule.recurrence.cycle_weeks is invalid")
                if len(weeks) != len(set(weeks)):
                    problems.append(lesson_prefix + ".schedule.recurrence.cycle_weeks contains duplicates")
            valid_from = recurrence.get("valid_from")
            valid_to = recurrence.get("valid_to")
            if bool(valid_from) != bool(valid_to):
                problems.append(lesson_prefix + ".schedule.recurrence validity dates must be supplied together")
            elif valid_from and valid_to:
                start = _date(valid_from, lesson_prefix + ".schedule.recurrence.valid_from", problems)
                end = _date(valid_to, lesson_prefix + ".schedule.recurrence.valid_to", problems)
                if start and end and (end < start or (term_start and start < term_start) or (term_end and end > term_end)):
                    problems.append(lesson_prefix + ".schedule.recurrence validity must be ordered and inside the term")
    if lesson_count > MAX_LESSONS:
        problems.append(f"lessons exceeds the limit of {MAX_LESSONS}")
    if problems:
        raise SnapshotValidationError(problems[:100])


def _external_id(value: object, path: str, problems: list[str]) -> None:
    if not _ID.match(str(value or "").strip()):
        problems.append(path + " has an invalid format")


def _date(value: object, path: str, problems: list[str]) -> date | None:
    try:
        return date.fromisoformat(str(value))
    except ValueError:
        problems.append(path + " must use YYYY-MM-DD")
        return None
