from datetime import date
from pathlib import Path

from git_board.config import load_config
from git_board.filter import Condition, matches, parse_filter

FIXTURE = Path(__file__).parent / "fixtures" / "git-board.toml"
TODAY = date(2026, 8, 31)  # "delivery" sprint is current, "post-delivery" is next


def test_parse_field_condition():
    [c] = parse_filter("status:ready")
    assert c == Condition("field", "status", "ready", False)


def test_parse_negated_field_condition():
    [c] = parse_filter("-title:*spec*")
    assert c == Condition("title", None, "*spec*", True)


def test_parse_is_condition():
    [c] = parse_filter("is:open")
    assert c == Condition("is", None, "open", False)


def test_parse_free_text():
    [c] = parse_filter("bicycle")
    assert c == Condition("text", None, "bicycle", False)


def test_parse_multiple_conditions_are_anded():
    conditions = parse_filter("sprint:@current is:open -title:*spec*")
    assert [c.kind for c in conditions] == ["field", "is", "title"]
    assert conditions[2].negated is True


def test_matches_field_is_and_title_glob():
    config = load_config(FIXTURE)
    issue = {"id": "abc123", "title": "Fix invoice spec", "status": "open", "labels": ["status:ready"]}
    conditions = parse_filter("status:ready is:open title:*spec*")
    assert matches(issue, conditions, config)

    conditions_no_match = parse_filter("status:backlog")
    assert not matches(issue, conditions_no_match, config)


def test_matches_negated_title_glob_excludes():
    config = load_config(FIXTURE)
    issue = {"id": "abc123", "title": "Fix invoice spec", "status": "open", "labels": []}
    conditions = parse_filter("-title:*spec*")
    assert not matches(issue, conditions, config)


def test_matches_free_text_substring():
    config = load_config(FIXTURE)
    issue = {"id": "abc123", "title": "Fix invoice bug", "status": "open", "labels": []}
    assert matches(issue, parse_filter("invoice"), config)
    assert not matches(issue, parse_filter("purchase"), config)


def test_matches_at_me():
    config = load_config(FIXTURE)
    issue = {"id": "abc123", "title": "x", "status": "open", "labels": ["assignee:guilbep"]}
    conditions = parse_filter("assignee:@me")
    assert matches(issue, conditions, config, me="guilbep")
    assert not matches(issue, conditions, config, me="someone-else")


def test_matches_sprint_current_and_next():
    config = load_config(FIXTURE)
    current_issue = {"id": "1", "title": "x", "status": "open", "labels": ["sprint:delivery"]}
    next_issue = {"id": "2", "title": "x", "status": "open", "labels": ["sprint:post-delivery"]}

    assert matches(current_issue, parse_filter("sprint:@current"), config, today=TODAY)
    assert not matches(next_issue, parse_filter("sprint:@current"), config, today=TODAY)
    assert matches(next_issue, parse_filter("sprint:@next"), config, today=TODAY)
    assert not matches(current_issue, parse_filter("sprint:@previous"), config, today=TODAY)
