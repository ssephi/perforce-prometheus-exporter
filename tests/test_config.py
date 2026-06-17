import pytest

from perforce_exporter.config import Target, load_config, parse_targets


def test_parse_targets_basic():
    targets = parse_targets("primary=127.0.0.1:1666,replica=127.0.0.1:1667")
    assert targets == (
        Target("primary", "127.0.0.1:1666"),
        Target("replica", "127.0.0.1:1667"),
    )


def test_parse_targets_strips_whitespace():
    targets = parse_targets(" a = 1.1.1.1:1666 , b = 2.2.2.2:1666 ")
    assert targets == (
        Target("a", "1.1.1.1:1666"),
        Target("b", "2.2.2.2:1666"),
    )


def test_parse_targets_rejects_missing_equals():
    with pytest.raises(ValueError):
        parse_targets("primary 127.0.0.1:1666")


def test_load_config_uses_defaults():
    cfg = load_config({"PERFORCE_TARGETS": "primary=127.0.0.1:1666"})
    assert cfg.listen_port == 9117
    assert cfg.p4_bin == "p4"
    assert cfg.p4_timeout_seconds == 10
    assert cfg.targets == (Target("primary", "127.0.0.1:1666"),)


def test_load_config_requires_targets():
    with pytest.raises(ValueError):
        load_config({})
