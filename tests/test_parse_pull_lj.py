from pathlib import Path

from perforce_exporter.parsers import parse_pull_lj

FIXTURES = Path(__file__).parent / "fixtures"


def test_healthy_pull_lj():
    pj = parse_pull_lj((FIXTURES / "p4_pull_lj_replica.txt").read_text())
    assert pj["replica_journal"] == 12
    assert pj["replica_sequence"] == 4521
    assert pj["master_journal"] == 12
    assert pj["master_sequence"] == 5000
    assert "statefile_modified_ts" in pj
    assert "replica_server_time_ts" in pj


def test_pull_lj_with_auth_warning_still_parses():
    pj = parse_pull_lj(
        (FIXTURES / "p4_pull_lj_auth_warning.txt").read_text()
    )
    # The auth warning line is ignored; journal lines still parse.
    assert pj["replica_journal"] == 8
    assert pj["master_sequence"] == 100


def test_empty():
    assert parse_pull_lj("") == {}


def test_only_replica_line():
    pj = parse_pull_lj(
        "Current replica journal state is: Journal 5, Sequence 100.\n"
    )
    assert pj == {"replica_journal": 5, "replica_sequence": 100}


def test_missing_fields_do_not_raise():
    pj = parse_pull_lj("garbage in\nmore garbage\n")
    assert pj == {}
