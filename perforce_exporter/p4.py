"""Thin wrapper around the p4 CLI.

Goal: never raise on a normal command failure. Return a structured result
that the collector can turn into metrics. Secrets must not appear in
returned fields beyond what p4 itself prints to stderr.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import time
from dataclasses import dataclass


@dataclass(frozen=True)
class CommandResult:
    target: str
    command: str
    args: tuple[str, ...]
    returncode: int
    stdout: str
    stderr: str
    duration_seconds: float
    timed_out: bool
    missing_binary: bool

    @property
    def ok(self) -> bool:
        return (
            not self.missing_binary
            and not self.timed_out
            and self.returncode == 0
        )


def run_p4(
    target_name: str,
    port: str,
    args: list[str],
    *,
    p4_bin: str = "p4",
    timeout: int = 10,
    env_overrides: dict | None = None,
) -> CommandResult:
    """Run a p4 subcommand against ``port`` and return a structured result.

    target_name is the operator-friendly label used for metrics, port is the
    P4PORT value. We deliberately do not interpret the command output here.
    """
    command_str = " ".join(args)

    if shutil.which(p4_bin) is None:
        return CommandResult(
            target=target_name,
            command=command_str,
            args=tuple(args),
            returncode=-1,
            stdout="",
            stderr=f"{p4_bin}: not found",
            duration_seconds=0.0,
            timed_out=False,
            missing_binary=True,
        )

    env = os.environ.copy()
    env["P4PORT"] = port
    if env_overrides:
        env.update(env_overrides)

    start = time.monotonic()
    try:
        proc = subprocess.run(
            [p4_bin, *args],
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as e:
        return CommandResult(
            target=target_name,
            command=command_str,
            args=tuple(args),
            returncode=-1,
            stdout=(e.stdout or "") if isinstance(e.stdout, str) else "",
            stderr=((e.stderr or "") if isinstance(e.stderr, str) else "")
            + f"\ntimeout after {timeout}s",
            duration_seconds=time.monotonic() - start,
            timed_out=True,
            missing_binary=False,
        )

    return CommandResult(
        target=target_name,
        command=command_str,
        args=tuple(args),
        returncode=proc.returncode,
        stdout=proc.stdout,
        stderr=proc.stderr,
        duration_seconds=time.monotonic() - start,
        timed_out=False,
        missing_binary=False,
    )
