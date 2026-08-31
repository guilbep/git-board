import shutil
from pathlib import Path

from typer.testing import CliRunner

from git_board import cli
from git_board.labels import GitBugAdapter

FIXTURE = Path(__file__).parent / "fixtures" / "git-board.toml"

FAKE_ISSUES = [
    {"id": "aaa111", "title": "Fix invoice bug", "status": "open", "labels": ["status:ready", "sprint:delivery"]},
    {"id": "bbb222", "title": "Write specs", "status": "open", "labels": ["status:backlog", "assignee:guilbep"]},
]

runner = CliRunner()


def _run(monkeypatch, tmp_path, args, env=None):
    shutil.copy(FIXTURE, tmp_path / ".git-board.toml")
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(cli, "CONFIG_PATH", Path(".git-board.toml"))
    monkeypatch.setattr(GitBugAdapter, "list_issues", lambda self: FAKE_ISSUES)
    return runner.invoke(cli.app, args, env=env)


def test_default_no_subcommand_lists_open_issues(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, [])
    assert result.exit_code == 0, result.output
    assert "aaa111" in result.output
    assert "bbb222" in result.output


def test_ls_with_filter(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, ["ls", "status:ready"])
    assert result.exit_code == 0, result.output
    assert "aaa111" in result.output
    assert "bbb222" not in result.output


def test_view_backlog(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, ["view", "backlog"])
    assert result.exit_code == 0, result.output
    assert "aaa111" in result.output


def test_view_mine_requires_me(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, ["view", "mine"], env={"GIT_BOARD_ME": ""})
    assert result.exit_code != 0


def test_view_mine_with_me_option(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, ["view", "mine", "--me", "guilbep"])
    assert result.exit_code == 0, result.output
    assert "bbb222" in result.output
    assert "aaa111" not in result.output


def test_view_mine_with_env_var(monkeypatch, tmp_path):
    result = _run(monkeypatch, tmp_path, ["view", "mine"], env={"GIT_BOARD_ME": "guilbep"})
    assert result.exit_code == 0, result.output
    assert "bbb222" in result.output
