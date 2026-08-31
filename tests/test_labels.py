import json
from datetime import date
from pathlib import Path

from git_board.config import load_config
from git_board.labels import GitBugAdapter, apply_field

FIXTURE = Path(__file__).parent / "fixtures" / "git-board.toml"
TODAY = date(2026, 8, 31)


class FakeGitBug:
    """Records label add/remove calls; backs GitBugAdapter.run for tests."""

    def __init__(self, issues: dict[str, list[str]]):
        self.issues = issues
        self.calls: list[list[str]] = []

    def __call__(self, args: list[str]) -> str:
        self.calls.append(args)
        if args[:2] == ["bug", "--format"]:
            return json.dumps(
                [
                    {"id": iid, "title": "t", "status": "open", "labels": labels}
                    for iid, labels in self.issues.items()
                ]
            )
        if args[:3] == ["bug", "label", "new"]:
            issue_id, label = args[3], args[4]
            self.issues.setdefault(issue_id, [])
            if label not in self.issues[issue_id]:
                self.issues[issue_id].append(label)
            return ""
        if args[:3] == ["bug", "label", "rm"]:
            issue_id, label = args[3], args[4]
            if label in self.issues.get(issue_id, []):
                self.issues[issue_id].remove(label)
            return ""
        raise AssertionError(f"unexpected git-bug invocation: {args}")


def _adapter(issues):
    fake = FakeGitBug(issues)
    return GitBugAdapter(run=fake), fake


def test_get_labels_via_mocked_adapter():
    adapter, _ = _adapter({"abc123": ["status:backlog", "prio:p1"]})
    assert adapter.get_labels("abc123") == ["status:backlog", "prio:p1"]
    assert adapter.get_labels("abc1") == ["status:backlog", "prio:p1"]  # prefix match


def test_apply_field_single_cardinality_swaps_label():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": ["status:backlog", "prio:p1"]})

    label = apply_field(adapter, config, "abc123", "status", "ready")

    assert label == "status:ready"
    assert fake.issues["abc123"] == ["prio:p1", "status:ready"]


def test_apply_field_single_no_op_when_already_set():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": ["status:ready"]})

    apply_field(adapter, config, "abc123", "status", "ready")

    assert fake.issues["abc123"] == ["status:ready"]


def test_apply_field_multi_cardinality_adds_without_removing():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": ["assignee:alice"]})

    apply_field(adapter, config, "abc123", "assignee", "+bob")

    assert sorted(fake.issues["abc123"]) == ["assignee:alice", "assignee:bob"]


def test_apply_field_multi_cardinality_remove():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": ["assignee:alice", "assignee:bob"]})

    apply_field(adapter, config, "abc123", "assignee", "-bob")

    assert fake.issues["abc123"] == ["assignee:alice"]


def test_apply_field_resolves_sprint_alias():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": []})

    label = apply_field(adapter, config, "abc123", "sprint", "@next", today=TODAY)

    assert label == "sprint:post-delivery"
    assert fake.issues["abc123"] == ["sprint:post-delivery"]


def test_apply_field_resolves_me():
    config = load_config(FIXTURE)
    adapter, fake = _adapter({"abc123": []})

    label = apply_field(adapter, config, "abc123", "assignee", "@me", me="guilbep")

    assert label == "assignee:guilbep"
