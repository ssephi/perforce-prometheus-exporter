from pathlib import Path

from perforce_exporter.parsers import (
    parse_counters,
    parse_last_checkpoint_action,
)

FIXTURES = Path(__file__).parent / "fixtures"


def test_parses_simple_counters():
    counters = parse_counters((FIXTURES / "p4_counters.txt").read_text())
    assert counters["journal"] == "4"
    assert counters["change"] == "1"
    assert counters["maxCommitChange"] == "1"
    assert counters["upgrade"] == "60"


def test_value_with_embedded_equals_is_preserved():
    counters = parse_counters((FIXTURES / "p4_counters.txt").read_text())
    # depotStats contains '=' inside its value; we must keep it intact.
    assert "depotStats" in counters
    assert "revs(0R/0F/0B/0D/0C/0U)" in counters["depotStats"]
    assert "abc/def=ghi" in counters["depotStats"]


def test_last_checkpoint_action_timestamp():
    counters = parse_counters((FIXTURES / "p4_counters.txt").read_text())
    ts = parse_last_checkpoint_action(counters["lastCheckpointAction"])
    assert ts == 1781337732.0


def test_last_checkpoint_action_empty_or_garbage():
    assert parse_last_checkpoint_action("") is None
    assert parse_last_checkpoint_action("checkpoint completed") is None


def test_parse_counters_empty():
    assert parse_counters("") == {}
