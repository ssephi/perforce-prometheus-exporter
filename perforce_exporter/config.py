"""Environment-driven configuration for the exporter."""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Target:
    name: str
    port: str


@dataclass(frozen=True)
class Config:
    targets: tuple[Target, ...]
    listen_port: int
    p4_bin: str
    p4_timeout_seconds: int


def parse_targets(raw: str) -> tuple[Target, ...]:
    targets: list[Target] = []
    for item in raw.split(","):
        item = item.strip()
        if not item:
            continue
        if "=" not in item:
            raise ValueError(
                f"invalid target {item!r}, expected name=host:port"
            )
        name, port = item.split("=", 1)
        name, port = name.strip(), port.strip()
        if not name or not port:
            raise ValueError(
                f"invalid target {item!r}, expected name=host:port"
            )
        targets.append(Target(name=name, port=port))
    if not targets:
        raise ValueError("PERFORCE_TARGETS must contain at least one target")
    return tuple(targets)


def load_config(env: dict | None = None) -> Config:
    env = env if env is not None else os.environ
    raw = env.get("PERFORCE_TARGETS", "")
    if not raw:
        raise ValueError(
            "PERFORCE_TARGETS is required, "
            "e.g. primary=127.0.0.1:1666,replica=127.0.0.1:1667"
        )
    return Config(
        targets=parse_targets(raw),
        listen_port=int(env.get("EXPORTER_PORT", "9117")),
        p4_bin=env.get("P4_BIN", "p4"),
        p4_timeout_seconds=int(env.get("P4_TIMEOUT_SECONDS", "10")),
    )
