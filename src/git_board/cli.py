"""M1 CLI: `board`, `board view`, `board ls`, `board set`, `board get`.

Later milestones (move/close/sprint/html/lint/sync) are out of scope here.
"""

from __future__ import annotations

import json as json_lib
import os
from pathlib import Path

import typer
from rich.console import Console
from rich.table import Table

from .config import Config, load_config
from .filter import matches, parse_filter
from .labels import GitBugAdapter, apply_field

app = typer.Typer(add_completion=False, help="Fields, sprints and views over git-bug issues.")
console = Console()

CONFIG_PATH = Path(".git-board.toml")

# Option shared by ls/view/set: who "@me" resolves to. `git board --me` isn't
# a thing (no global option) to keep each command's `--help` self-explanatory.
ME_OPTION = typer.Option(None, "--me", envvar="GIT_BOARD_ME", help="Current user, for @me filters/values")


def _load_config() -> Config:
    if not CONFIG_PATH.exists():
        typer.echo(f"error: {CONFIG_PATH} not found", err=True)
        raise typer.Exit(1)
    return load_config(CONFIG_PATH)


def _field_value(issue: dict, field_name: str) -> str | None:
    for label in issue.get("labels", []):
        if label.startswith(f"{field_name}:"):
            return label.split(":", 1)[1]
    return None


def _render(issues: list[dict], *, group: str | None, sort: str | None, md: bool, as_json: bool) -> None:
    if sort:
        key, _, direction = sort.partition(" ")
        issues = sorted(issues, key=lambda i: _field_value(i, key) or "", reverse=direction.strip() == "desc")

    if as_json:
        console.print_json(json_lib.dumps(issues))
        return

    buckets: dict[str, list[dict]] = {}
    for issue in issues:
        key = (_field_value(issue, group) or "(none)") if group else ""
        buckets.setdefault(key, []).append(issue)

    for name, bucket in buckets.items():
        if md:
            if name:
                console.print(f"### {name}")
            console.print("| id | title | status | labels |")
            console.print("|---|---|---|---|")
            for issue in bucket:
                console.print(
                    f"| {issue.get('id', '')[:8]} | {issue.get('title', '')} "
                    f"| {issue.get('status', '')} | {', '.join(issue.get('labels', []))} |"
                )
            continue

        table = Table(title=name or None)
        for col in ("id", "title", "status", "labels"):
            table.add_column(col)
        for issue in bucket:
            table.add_row(
                issue.get("id", "")[:8],
                issue.get("title", ""),
                issue.get("status", ""),
                ", ".join(issue.get("labels", [])),
            )
        console.print(table)


def _list_and_render(
    filter_str: str,
    *,
    group: str | None = None,
    sort: str | None = None,
    md: bool = False,
    as_json: bool = False,
    me: str | None = None,
) -> None:
    """Plain-Python worker behind `ls`/`view`/the no-subcommand default.

    Kept separate from the `ls` typer command so it can be called with plain
    values — calling a `@app.command()`-decorated function directly would
    otherwise hand you back `typer.Option` sentinel objects for any arg you
    don't pass explicitly.
    """
    config = _load_config()
    conditions = parse_filter(filter_str)
    issues = GitBugAdapter().list_issues()
    filtered = [i for i in issues if matches(i, conditions, config, me=me or os.environ.get("GIT_BOARD_ME"))]
    _render(filtered, group=group, sort=sort, md=md, as_json=as_json)


@app.callback(invoke_without_command=True)
def main(ctx: typer.Context) -> None:
    if ctx.invoked_subcommand is None:
        _list_and_render("is:open")


@app.command()
def view(
    name: str,
    md: bool = typer.Option(False, "--md"),
    as_json: bool = typer.Option(False, "--json"),
    me: str | None = ME_OPTION,
) -> None:
    """Render a named view from .git-board.toml."""
    config = _load_config()
    view_cfg = config.views.get(name)
    if view_cfg is None:
        typer.echo(f"error: unknown view '{name}'", err=True)
        raise typer.Exit(1)
    _list_and_render(view_cfg.filter, group=view_cfg.group, sort=view_cfg.sort, md=md, as_json=as_json, me=me)


@app.command()
def ls(
    filter_str: str = typer.Argument("is:open"),
    group: str | None = typer.Option(None, "--group"),
    sort: str | None = typer.Option(None, "--sort"),
    md: bool = typer.Option(False, "--md"),
    as_json: bool = typer.Option(False, "--json"),
    me: str | None = ME_OPTION,
) -> None:
    """List issues matching a filter expression."""
    _list_and_render(filter_str, group=group, sort=sort, md=md, as_json=as_json, me=me)


@app.command("set")
def set_fields(
    issue_id: str,
    assignments: list[str] = typer.Argument(..., help="field=value pairs, e.g. status=ready prio=p1"),
    me: str | None = ME_OPTION,
) -> None:
    """Set one or more fields on an issue."""
    config = _load_config()
    adapter = GitBugAdapter()
    for assignment in assignments:
        field_name, _, value = assignment.partition("=")
        label = apply_field(adapter, config, issue_id, field_name, value, me=me or os.environ.get("GIT_BOARD_ME"))
        console.print(f"{issue_id}: {label}")


@app.command()
def get(issue_id: str) -> None:
    """Show fields (labels) for an issue."""
    for label in GitBugAdapter().get_labels(issue_id):
        console.print(label)


if __name__ == "__main__":
    app()
