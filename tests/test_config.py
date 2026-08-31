from datetime import date
from pathlib import Path

from git_board.config import load_config, resolve_sprint

FIXTURE = Path(__file__).parent / "fixtures" / "git-board.toml"


def test_parses_repos_and_fields():
    config = load_config(FIXTURE)

    assert config.repos == [
        "EPFL-ENAC/co2-calculator",
        "EPFL-ENAC/co2-calculator-back-office-doc",
    ]

    status = config.fields["status"]
    assert status.single is True
    assert status.options[0] == "backlog"
    assert status.options[-1] == "validated"
    assert len(status.options) == 9

    est = config.fields["est"]
    assert est.numeric is True

    assert config.fields["assignee"].single is False


def test_parses_sprints():
    config = load_config(FIXTURE)

    assert config.sprints.length_days == 14
    assert config.sprints.start_weekday == "monday"
    assert [s.slug for s in config.sprints.list] == ["delivery", "post-delivery"]
    assert config.sprints.list[0].start == date(2026, 8, 24)
    assert config.sprints.list[1].title == "Post-delivery"


def test_parses_views():
    config = load_config(FIXTURE)

    assert config.views["current-sprint"].layout == "board"
    assert config.views["current-sprint"].group == "status"
    assert config.views["backlog"].sort == "prio asc"
    assert config.views["mine"].filter == "assignee:@me is:open"


def test_resolve_sprint_current_next_previous():
    config = load_config(FIXTURE)
    today = date(2026, 8, 31)  # inside the "delivery" sprint window

    current = resolve_sprint(config.sprints, "current", today)
    nxt = resolve_sprint(config.sprints, "next", today)
    previous = resolve_sprint(config.sprints, "previous", today)

    assert current.slug == "delivery"
    assert nxt.slug == "post-delivery"
    assert previous is None


def test_resolve_sprint_before_first_start():
    config = load_config(FIXTURE)
    today = date(2026, 1, 1)

    assert resolve_sprint(config.sprints, "current", today) is None
    assert resolve_sprint(config.sprints, "next", today).slug == "delivery"
    assert resolve_sprint(config.sprints, "previous", today) is None
