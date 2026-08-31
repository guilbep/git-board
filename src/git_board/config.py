"""Parse `.git-board.toml` (PRD §4) into typed config objects."""

from __future__ import annotations

import tomllib
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path


@dataclass
class Field:
    single: bool = True
    options: list[str] = field(default_factory=list)
    numeric: bool = False


@dataclass
class Sprint:
    slug: str
    title: str
    start: date


@dataclass
class SprintsConfig:
    length_days: int = 14
    start_weekday: str = "monday"
    list: list[Sprint] = field(default_factory=list)


@dataclass
class View:
    layout: str = "table"
    filter: str = ""
    group: str | None = None
    sort: str | None = None


@dataclass
class Config:
    repos: list[str] = field(default_factory=list)
    fields: dict[str, Field] = field(default_factory=dict)
    sprints: SprintsConfig = field(default_factory=SprintsConfig)
    views: dict[str, View] = field(default_factory=dict)


def parse_config(data: dict) -> Config:
    """Build a Config from an already-parsed TOML dict (handy in tests)."""
    repos = list(data.get("repos", {}).get("list", []))

    fields = {
        name: Field(
            single=raw.get("single", True),
            options=list(raw.get("options", [])),
            numeric=raw.get("numeric", False),
        )
        for name, raw in data.get("fields", {}).items()
    }

    sprints_raw = data.get("sprints", {})
    sprint_list = [
        Sprint(
            slug=s["slug"],
            title=s.get("title", s["slug"]),
            start=s["start"] if isinstance(s["start"], date) else date.fromisoformat(str(s["start"])),
        )
        for s in sprints_raw.get("list", [])
    ]
    sprints = SprintsConfig(
        length_days=sprints_raw.get("length_days", 14),
        start_weekday=sprints_raw.get("start_weekday", "monday"),
        list=sprint_list,
    )

    views = {
        name: View(
            layout=raw.get("layout", "table"),
            filter=raw.get("filter", ""),
            group=raw.get("group"),
            sort=raw.get("sort"),
        )
        for name, raw in data.get("views", {}).items()
    }

    return Config(repos=repos, fields=fields, sprints=sprints, views=views)


def load_config(path: str | Path) -> Config:
    with open(path, "rb") as f:
        data = tomllib.load(f)
    return parse_config(data)


def resolve_sprint(sprints: SprintsConfig, which: str, today: date | None = None) -> Sprint | None:
    """Resolve 'current' / 'next' / 'previous' against the sprint calendar.

    The "current" sprint is the last one whose start date is on or before
    `today`; next/previous are its neighbours in start-date order.
    """
    today = today or date.today()
    ordered = sorted(sprints.list, key=lambda s: s.start)

    idx = None
    for i, s in enumerate(ordered):
        if s.start <= today:
            idx = i
        else:
            break

    if which == "current":
        return ordered[idx] if idx is not None else None
    if which == "next":
        nxt = idx + 1 if idx is not None else 0
        return ordered[nxt] if nxt < len(ordered) else None
    if which == "previous":
        return ordered[idx - 1] if idx is not None and idx > 0 else None
    raise ValueError(f"unknown sprint alias: {which!r}")
