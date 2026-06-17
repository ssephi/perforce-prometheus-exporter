"""Pure-function parsers for p4 command output.

Keeping these as pure str -> dict functions makes them easy to test from
fixture files and keeps Perforce-specific quirks contained.
"""

from __future__ import annotations

import re
from datetime import datetime, timezone

# `p4 pull -lj` prints two journal-state lines whose payload looks like
# "Journal 12, Sequence 4521". Spacing varies between server versions, so
# match liberally on whitespace.
_JOURNAL_STATE_RE = re.compile(
    r"Journal\s+(\d+),\s*Sequence\s+(\d+)", re.IGNORECASE
)


def parse_p4_info(text: str) -> dict[str, str]:
    """Parse `p4 info` "Key: value" lines into a dict.

    p4 info uses unique keys per line, with values that may themselves
    contain colons (e.g. "Server address: 127.0.0.1:1666"). Splitting on
    the first colon only is the correct behaviour.
    """
    out: dict[str, str] = {}
    for line in text.splitlines():
        if ":" not in line:
            continue
        key, _, value = line.partition(":")
        key = key.strip()
        if not key:
            continue
        out[key] = value.strip()
    return out


def parse_pull_lj(text: str) -> dict:
    """Parse `p4 pull -lj` output.

    Returned keys (any subset may be present):

        replica_journal, replica_sequence  (int)
        master_journal,  master_sequence   (int)
        statefile_modified_ts              (unix seconds, float)
        replica_server_time_ts             (unix seconds, float)
    """
    result: dict = {}
    for raw in text.splitlines():
        line = raw.strip()
        if not line:
            continue
        lower = line.lower()
        if lower.startswith("current replica journal state"):
            m = _JOURNAL_STATE_RE.search(line)
            if m:
                result["replica_journal"] = int(m.group(1))
                result["replica_sequence"] = int(m.group(2))
        elif lower.startswith("current master journal state"):
            m = _JOURNAL_STATE_RE.search(line)
            if m:
                result["master_journal"] = int(m.group(1))
                result["master_sequence"] = int(m.group(2))
        elif lower.startswith("the statefile was last modified at"):
            ts = _parse_p4_timestamp(_after_marker(line, "at:"))
            if ts is not None:
                result["statefile_modified_ts"] = ts
        elif lower.startswith("the replica server time is currently"):
            ts = _parse_p4_timestamp(_after_marker(line, "currently:"))
            if ts is not None:
                result["replica_server_time_ts"] = ts
    return result


def parse_counters(text: str) -> dict[str, str]:
    """Parse `p4 counters` output into a name->value dict.

    Each line has the shape ``name = value``. Values may themselves contain
    `=` (e.g. the internal ``depotStats`` counter), so we split on the first
    ``=`` only and trim whitespace from both sides.
    """
    out: dict[str, str] = {}
    for line in text.splitlines():
        if "=" not in line:
            continue
        name, _, value = line.partition("=")
        name = name.strip()
        if not name:
            continue
        out[name] = value.strip()
    return out


# `lastCheckpointAction` values look like:
#   "1781337732 (2026/06/13 09:02:12 +0100 BST) checkpoint completed"
# The leading integer is a unix timestamp — much easier to consume than the
# bracketed local time, which we ignore.
_LEADING_INT_RE = re.compile(r"^\s*(\d+)\b")


def parse_last_checkpoint_action(value: str) -> float | None:
    """Return the unix timestamp of the last checkpoint action, or None."""
    m = _LEADING_INT_RE.match(value)
    if not m:
        return None
    try:
        return float(int(m.group(1)))
    except ValueError:
        return None


# `p4 pull -ls` prints a single summary line like:
#   "File transfers: 0 active/0 total, bytes: 0 active/0 total."
# Numbers can grow large (multi-gigabyte) on busy systems, so accept any
# integer width.
_PULL_LS_RE = re.compile(
    r"File transfers:\s*(\d+)\s*active\s*/\s*(\d+)\s*total"
    r"\s*,\s*bytes:\s*(\d+)\s*active\s*/\s*(\d+)\s*total",
    re.IGNORECASE,
)


def parse_pull_ls(text: str) -> dict:
    """Parse `p4 pull -ls`.

    Returns keys: active_transfers, total_transfers, active_bytes, total_bytes.
    Returns an empty dict if the summary line is absent.
    """
    m = _PULL_LS_RE.search(text)
    if not m:
        return {}
    return {
        "active_transfers": int(m.group(1)),
        "total_transfers": int(m.group(2)),
        "active_bytes": int(m.group(3)),
        "total_bytes": int(m.group(4)),
    }


def parse_pull_l(text: str) -> dict:
    """Parse `p4 pull -l` output (queued/in-flight file transfer list).

    Output format varies between p4d versions; the only thing we treat as
    structurally meaningful is one record per non-empty line. We flag any
    line containing the substring "fail" (case-insensitive) as a failed
    transfer — a heuristic, but the only signal available without log
    parsing. This metric is a current-queue gauge, not a historical count.

    Returns keys: total_listed, failed_listed.
    """
    total = 0
    failed = 0
    for line in text.splitlines():
        s = line.strip()
        if not s:
            continue
        total += 1
        if "fail" in s.lower():
            failed += 1
    return {"total_listed": total, "failed_listed": failed}


def _after_marker(line: str, marker: str) -> str:
    """Return the substring after a case-insensitive marker, or ''."""
    idx = line.lower().find(marker.lower())
    if idx < 0:
        return ""
    return line[idx + len(marker):].strip()


def _parse_p4_timestamp(value: str) -> float | None:
    """Parse a p4-style timestamp into unix seconds.

    Known shapes observed in the lab:
        "2026/06/13 09:00:01"
        "2026/06/13 09:00:01 +0000 UTC"
    Timestamps with no timezone are treated as UTC, which matches the
    server's own behaviour when P4D runs with UTC.
    """
    s = value.strip().rstrip(".").strip()
    if not s:
        return None
    # Strip a trailing timezone abbreviation (e.g. "UTC") after the offset;
    # strptime can't consume it on macOS.
    parts = s.split()
    if len(parts) == 4 and parts[2].startswith(("+", "-")):
        s_no_tz_name = " ".join(parts[:3])
    else:
        s_no_tz_name = s
    for fmt in ("%Y/%m/%d %H:%M:%S %z", "%Y/%m/%d %H:%M:%S"):
        try:
            dt = datetime.strptime(s_no_tz_name, fmt)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=timezone.utc)
            return dt.timestamp()
        except ValueError:
            continue
    return None
