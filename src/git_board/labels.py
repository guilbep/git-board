"""git-bug adapter (fields = namespaced labels `ns:value`, PRD §3) and the
single-cardinality `set` logic.

The adapter's `run` callback is injectable so tests never need a real
`git-bug` binary — pass a fake `run(args) -> str` and everything else works
unchanged.
"""

from __future__ import annotations

import json
import subprocess
from collections.abc import Callable
from pathlib import Path

from .config import Config, resolve_sprint

RunFn = Callable[[list[str]], str]


class GitBugAdapter:
    def __init__(self, run: RunFn | None = None, cwd: str | Path = "."):
        self._run = run or self._subprocess_run
        self.cwd = cwd

    def _subprocess_run(self, args: list[str]) -> str:
        # Verified against github.com/git-bug/git-bug commands/bug/*.go:
        # listing is `git bug bug --format json` (the `bug` command itself
        # lists — there is no `ls` subcommand), labels are `git bug bug
        # label new|rm <id> <label>`.
        try:
            result = subprocess.run(
                ["git", "bug", *args], cwd=self.cwd, capture_output=True, text=True, check=True
            )
        except FileNotFoundError as e:
            raise RuntimeError("git-bug is not installed or not on PATH") from e
        except subprocess.CalledProcessError as e:
            raise RuntimeError(f"git bug {' '.join(args)} failed: {e.stderr.strip()}") from e
        return result.stdout

    def list_issues(self) -> list[dict]:
        out = self._run(["bug", "--format", "json"])
        return json.loads(out) if out.strip() else []

    def get_labels(self, issue_id: str) -> list[str]:
        for issue in self.list_issues():
            if issue.get("id", "").startswith(issue_id):
                return list(issue.get("labels", []))
        return []

    def add_label(self, issue_id: str, label: str) -> None:
        self._run(["bug", "label", "new", issue_id, label])

    def remove_label(self, issue_id: str, label: str) -> None:
        self._run(["bug", "label", "rm", issue_id, label])


def apply_field(
    adapter: GitBugAdapter,
    config: Config,
    issue_id: str,
    field_name: str,
    value: str,
    *,
    me: str | None = None,
    today=None,
) -> str:
    """Set `field_name` to `value` on `issue_id`.

    Single-cardinality fields (the default, and per PRD §3 for everything but
    `assignee`) swap: any existing `field_name:*` label is removed before the
    new one is added. Multi-cardinality fields add by default; a leading
    `-` removes instead (e.g. `assignee=-guilbep`).
    """
    field_cfg = config.fields.get(field_name)
    single = field_cfg.single if field_cfg else True

    if value in ("@current", "@next", "@previous"):
        sprint = resolve_sprint(config.sprints, value[1:], today)
        if sprint is None:
            raise ValueError(f"no sprint resolves to {value}")
        resolved = sprint.slug
    elif value == "@me":
        if not me:
            raise ValueError("@me used but no current user configured")
        resolved = me
    else:
        resolved = value

    if single:
        for label in adapter.get_labels(issue_id):
            if label.startswith(f"{field_name}:") and label != f"{field_name}:{resolved}":
                adapter.remove_label(issue_id, label)
        new_label = f"{field_name}:{resolved}"
        adapter.add_label(issue_id, new_label)
        return new_label

    remove = resolved.startswith("-")
    new_label = f"{field_name}:{resolved.lstrip('+-')}"
    if remove:
        adapter.remove_label(issue_id, new_label)
    else:
        adapter.add_label(issue_id, new_label)
    return new_label
