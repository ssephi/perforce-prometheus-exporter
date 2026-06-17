from pathlib import Path

from perforce_exporter.parsers import parse_pull_l, parse_pull_ls

FIXTURES = Path(__file__).parent / "fixtures"


def test_pull_ls_idle():
    summary = parse_pull_ls((FIXTURES / "p4_pull_ls_idle.txt").read_text())
    assert summary == {
        "active_transfers": 0,
        "total_transfers": 0,
        "active_bytes": 0,
        "total_bytes": 0,
    }


def test_pull_ls_busy():
    summary = parse_pull_ls((FIXTURES / "p4_pull_ls_busy.txt").read_text())
    assert summary["active_transfers"] == 3
    assert summary["total_transfers"] == 27
    assert summary["active_bytes"] == 524288
    assert summary["total_bytes"] == 10485760


def test_pull_ls_empty_input():
    assert parse_pull_ls("") == {}


def test_pull_ls_unparseable_input():
    assert parse_pull_ls("Perforce password (P4PASSWD) invalid or unset.\n") == {}


def test_pull_l_empty():
    assert parse_pull_l("") == {"total_listed": 0, "failed_listed": 0}


def test_pull_l_with_failure():
    counts = parse_pull_l(
        (FIXTURES / "p4_pull_l_with_failure.txt").read_text()
    )
    assert counts == {"total_listed": 3, "failed_listed": 1}
