"""Filter grammar (PRD §5): `field:value`, `-field:value`, `field:@alias`,
`is:open|closed`, `title:*glob*`, free text. Conditions are AND'd together.
"""

from __future__ import annotations

import fnmatch
from dataclasses import dataclass
from datetime import date

from .config import Config, resolve_sprint

_SPRINT_ALIASES = ("@current", "@next", "@previous")


@dataclass
class Condition:
    kind: str  # "field" | "is" | "title" | "text"
    field: str | None
    value: str
    negated: bool = False


def parse_filter(text: str) -> list[Condition]:
    conditions = []
    for token in text.split():
        negated = token.startswith("-")
        body = token[1:] if negated else token
        field_name, sep, value = body.partition(":")
        if not sep:
            conditions.append(Condition("text", None, body, negated))
        elif field_name == "is":
            conditions.append(Condition("is", None, value, negated))
        elif field_name == "title":
            conditions.append(Condition("title", None, value, negated))
        else:
            conditions.append(Condition("field", field_name, value, negated))
    return conditions


def _resolve_value(field_name: str, value: str, config: Config, me: str | None, today: date | None) -> str:
    if value == "@me":
        if not me:
            raise ValueError("@me used in filter but no current user configured")
        return me
    if value in _SPRINT_ALIASES:
        sprint = resolve_sprint(config.sprints, value[1:], today)
        return sprint.slug if sprint else "\0no-such-sprint"
    return value


def matches(
    issue: dict,
    conditions: list[Condition],
    config: Config,
    *,
    me: str | None = None,
    today: date | None = None,
) -> bool:
    labels = set(issue.get("labels", []))
    title = issue.get("title", "")
    status = issue.get("status", "open")

    for c in conditions:
        if c.kind == "is":
            ok = status == c.value
        elif c.kind == "title":
            ok = fnmatch.fnmatch(title.lower(), c.value.lower())
        elif c.kind == "text":
            ok = c.value.lower() in title.lower()
        elif c.kind == "field":
            value = _resolve_value(c.field, c.value, config, me, today)
            ok = f"{c.field}:{value}" in labels
        else:  # pragma: no cover - exhaustive by construction
            raise ValueError(f"unknown condition kind: {c.kind!r}")

        if c.negated:
            ok = not ok
        if not ok:
            return False
    return True
