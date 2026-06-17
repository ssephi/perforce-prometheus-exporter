from pathlib import Path

from perforce_exporter.parsers import parse_p4_info

FIXTURES = Path(__file__).parent / "fixtures"


def test_primary_info():
    info = parse_p4_info((FIXTURES / "p4_info_primary.txt").read_text())
    assert info["ServerID"] == "master.1"
    assert info["Server services"] == "standard"
    assert info["Server address"] == "127.0.0.1:1666"
    assert info["Server version"].startswith("P4D/")
    assert "Replica of" not in info


def test_replica_info():
    info = parse_p4_info((FIXTURES / "p4_info_replica.txt").read_text())
    assert info["ServerID"] == "replica.1"
    assert info["Server services"] == "replica"
    assert info["Replica of"] == "127.0.0.1:1666"


def test_empty_input():
    assert parse_p4_info("") == {}


def test_lines_without_colon_are_skipped():
    info = parse_p4_info("not a key value line\nServerID: x.1\n")
    assert info == {"ServerID": "x.1"}
