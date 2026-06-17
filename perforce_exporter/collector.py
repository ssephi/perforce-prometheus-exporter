"""Prometheus collector that scrapes p4 commands on each /metrics request.

This is intentionally a custom collector rather than a set of module-level
gauges: we want labels (server_id, server_services, replica_of) that we
only learn from `p4 info`, and we want a clean re-read on every scrape so
stale targets disappear correctly.
"""

from __future__ import annotations

import logging
import time
from collections import defaultdict
from collections.abc import Iterable

from prometheus_client.core import CounterMetricFamily, GaugeMetricFamily

from . import p4 as p4mod
from . import parsers
from .config import Config


log = logging.getLogger(__name__)

# Only expose well-known low-cardinality numeric counters as labelled
# metrics. Adding the depotStats / *State counters here would either blow
# up cardinality or fail to parse — they are deliberately excluded.
NUMERIC_COUNTER_WHITELIST: tuple[str, ...] = (
    "journal",
    "change",
    "maxCommitChange",
    "upgrade",
)


class PerforceCollector:
    """Custom prometheus_client collector.

    A fresh `collect()` is called for every scrape. We accumulate error
    counts across scrapes in ``_error_counts`` so the counter behaves like
    a normal monotonic counter.
    """

    def __init__(self, config: Config):
        self._config = config
        self._error_counts: dict[tuple[str, str, str], int] = defaultdict(int)

    def collect(self) -> Iterable:
        cfg = self._config

        up = GaugeMetricFamily(
            "perforce_up",
            "1 if `p4 info` succeeded for the target during this scrape.",
            labels=["target", "server_id", "server_services"],
        )
        info_metric = GaugeMetricFamily(
            "perforce_info",
            "Static Perforce server info; value is always 1.",
            labels=[
                "target",
                "server_id",
                "server_services",
                "server_version",
                "replica_of",
            ],
        )
        cmd_success = GaugeMetricFamily(
            "perforce_command_success",
            "1 if the last execution of this p4 command for the target succeeded.",
            labels=["target", "command"],
        )
        cmd_duration = GaugeMetricFamily(
            "perforce_command_duration_seconds",
            "Wall-clock duration of the last p4 command execution.",
            labels=["target", "command"],
        )
        replica_journal = GaugeMetricFamily(
            "perforce_replica_journal",
            "Replica journal number reported by `p4 pull -lj`.",
            labels=["target"],
        )
        replica_sequence = GaugeMetricFamily(
            "perforce_replica_sequence",
            "Replica journal byte sequence reported by `p4 pull -lj`.",
            labels=["target"],
        )
        master_journal = GaugeMetricFamily(
            "perforce_master_journal",
            "Master journal number as observed by the replica.",
            labels=["target"],
        )
        master_sequence = GaugeMetricFamily(
            "perforce_master_sequence",
            "Master journal byte sequence as observed by the replica.",
            labels=["target"],
        )
        seq_lag = GaugeMetricFamily(
            "perforce_replication_sequence_lag",
            "master_sequence - replica_sequence (bytes).",
            labels=["target"],
        )
        jrn_lag = GaugeMetricFamily(
            "perforce_replication_journal_lag",
            "master_journal - replica_journal.",
            labels=["target"],
        )
        statefile_ts = GaugeMetricFamily(
            "perforce_replication_statefile_modified_timestamp_seconds",
            "Unix timestamp when the replica statefile was last modified.",
            labels=["target"],
        )
        scrape_success = GaugeMetricFamily(
            "perforce_scrape_success",
            "1 if all commands for the target succeeded during this scrape.",
            labels=["target"],
        )
        errors_metric = CounterMetricFamily(
            "perforce_command_errors_total",
            "Cumulative count of p4 command errors observed by the exporter.",
            labels=["target", "command", "error_type"],
        )
        journal_number = GaugeMetricFamily(
            "perforce_journal_number",
            "Current journal number from the `journal` counter.",
            labels=["target"],
        )
        last_checkpoint_ts = GaugeMetricFamily(
            "perforce_last_checkpoint_timestamp_seconds",
            "Unix timestamp of the last checkpoint action.",
            labels=["target"],
        )
        checkpoint_age = GaugeMetricFamily(
            "perforce_checkpoint_age_seconds",
            "Seconds since the last checkpoint action.",
            labels=["target"],
        )
        counter_metric = GaugeMetricFamily(
            "perforce_counter",
            "Selected numeric Perforce counters (whitelisted to keep cardinality low).",
            labels=["target", "name"],
        )
        archive_active_transfers = GaugeMetricFamily(
            "perforce_archive_pull_active_transfers",
            "Archive file transfers currently in flight (from `p4 pull -ls`).",
            labels=["target"],
        )
        archive_total_transfers = GaugeMetricFamily(
            "perforce_archive_pull_queued_transfers",
            "Archive file transfers queued including active (from `p4 pull -ls`).",
            labels=["target"],
        )
        archive_active_bytes = GaugeMetricFamily(
            "perforce_archive_pull_active_bytes",
            "Bytes of archive content currently transferring (from `p4 pull -ls`).",
            labels=["target"],
        )
        archive_total_bytes = GaugeMetricFamily(
            "perforce_archive_pull_queued_bytes",
            "Bytes of archive content queued including active (from `p4 pull -ls`).",
            labels=["target"],
        )
        archive_failed_transfers = GaugeMetricFamily(
            "perforce_archive_pull_failed_transfers",
            "Failed archive transfers currently visible in `p4 pull -l` (heuristic).",
            labels=["target"],
        )
        archive_healthy = GaugeMetricFamily(
            "perforce_archive_replication_healthy",
            "1 if archive pull commands succeeded and no failed transfers are queued.",
            labels=["target"],
        )

        now = time.time()

        for target in cfg.targets:
            target_ok = True

            # --- p4 info -------------------------------------------------
            info_result = p4mod.run_p4(
                target.name,
                target.port,
                ["info"],
                p4_bin=cfg.p4_bin,
                timeout=cfg.p4_timeout_seconds,
            )
            self._record_command(info_result, cmd_success, cmd_duration)

            services = ""
            server_id = ""
            if info_result.ok:
                parsed = parsers.parse_p4_info(info_result.stdout)
                server_id = parsed.get("ServerID", "")
                services = parsed.get("Server services", "")
                replica_of = parsed.get("Replica of", "")
                version = parsed.get("Server version", "")
                up.add_metric([target.name, server_id, services], 1)
                info_metric.add_metric(
                    [target.name, server_id, services, version, replica_of], 1
                )
            else:
                up.add_metric([target.name, "", ""], 0)
                target_ok = False

            # --- p4 pull -lj (replicas only) -----------------------------
            if services and "replica" in services.lower():
                pull_result = p4mod.run_p4(
                    target.name,
                    target.port,
                    ["pull", "-lj"],
                    p4_bin=cfg.p4_bin,
                    timeout=cfg.p4_timeout_seconds,
                )
                self._record_command(pull_result, cmd_success, cmd_duration)

                if pull_result.ok:
                    try:
                        pj = parsers.parse_pull_lj(pull_result.stdout)
                    except Exception:
                        log.exception("parse_pull_lj failed for %s", target.name)
                        pj = {}
                        self._error_counts[
                            (target.name, "pull -lj", "parse_error")
                        ] += 1
                    if not pj:
                        # Command succeeded but we got nothing useful.
                        self._error_counts[
                            (target.name, "pull -lj", "parse_error")
                        ] += 1
                        target_ok = False

                    rj = pj.get("replica_journal")
                    rs = pj.get("replica_sequence")
                    mj = pj.get("master_journal")
                    ms = pj.get("master_sequence")
                    if rj is not None:
                        replica_journal.add_metric([target.name], rj)
                    if rs is not None:
                        replica_sequence.add_metric([target.name], rs)
                    if mj is not None:
                        master_journal.add_metric([target.name], mj)
                    if ms is not None:
                        master_sequence.add_metric([target.name], ms)
                    if mj is not None and rj is not None:
                        jrn_lag.add_metric([target.name], mj - rj)
                    if ms is not None and rs is not None:
                        seq_lag.add_metric([target.name], ms - rs)
                    if "statefile_modified_ts" in pj:
                        statefile_ts.add_metric(
                            [target.name], pj["statefile_modified_ts"]
                        )
                else:
                    target_ok = False

                # --- p4 pull -ls / -l (archive replication) -------------
                archive_ok = True
                failed_count: int | None = None

                ls_result = p4mod.run_p4(
                    target.name,
                    target.port,
                    ["pull", "-ls"],
                    p4_bin=cfg.p4_bin,
                    timeout=cfg.p4_timeout_seconds,
                )
                self._record_command(ls_result, cmd_success, cmd_duration)
                if ls_result.ok:
                    try:
                        ls = parsers.parse_pull_ls(ls_result.stdout)
                    except Exception:
                        log.exception("parse_pull_ls failed for %s", target.name)
                        ls = {}
                        self._error_counts[
                            (target.name, "pull -ls", "parse_error")
                        ] += 1
                    if ls:
                        archive_active_transfers.add_metric(
                            [target.name], ls["active_transfers"]
                        )
                        archive_total_transfers.add_metric(
                            [target.name], ls["total_transfers"]
                        )
                        archive_active_bytes.add_metric(
                            [target.name], ls["active_bytes"]
                        )
                        archive_total_bytes.add_metric(
                            [target.name], ls["total_bytes"]
                        )
                    else:
                        archive_ok = False
                else:
                    archive_ok = False
                    target_ok = False

                l_result = p4mod.run_p4(
                    target.name,
                    target.port,
                    ["pull", "-l"],
                    p4_bin=cfg.p4_bin,
                    timeout=cfg.p4_timeout_seconds,
                )
                self._record_command(l_result, cmd_success, cmd_duration)
                if l_result.ok:
                    try:
                        lcounts = parsers.parse_pull_l(l_result.stdout)
                    except Exception:
                        log.exception("parse_pull_l failed for %s", target.name)
                        lcounts = {"failed_listed": 0}
                        self._error_counts[
                            (target.name, "pull -l", "parse_error")
                        ] += 1
                    failed_count = int(lcounts.get("failed_listed", 0))
                    archive_failed_transfers.add_metric(
                        [target.name], failed_count
                    )
                else:
                    archive_ok = False
                    target_ok = False

                healthy = 1 if archive_ok and (failed_count == 0) else 0
                archive_healthy.add_metric([target.name], healthy)

            # --- p4 counters --------------------------------------------
            # Run only when info succeeded — otherwise the target is down
            # and a second failing command just adds noise.
            if info_result.ok:
                counters_result = p4mod.run_p4(
                    target.name,
                    target.port,
                    ["counters"],
                    p4_bin=cfg.p4_bin,
                    timeout=cfg.p4_timeout_seconds,
                )
                self._record_command(counters_result, cmd_success, cmd_duration)

                if counters_result.ok:
                    try:
                        counters = parsers.parse_counters(counters_result.stdout)
                    except Exception:
                        log.exception("parse_counters failed for %s", target.name)
                        counters = {}
                        self._error_counts[
                            (target.name, "counters", "parse_error")
                        ] += 1

                    for name in NUMERIC_COUNTER_WHITELIST:
                        raw = counters.get(name)
                        if raw is None:
                            continue
                        try:
                            value = float(raw)
                        except ValueError:
                            continue
                        counter_metric.add_metric([target.name, name], value)
                        if name == "journal":
                            journal_number.add_metric([target.name], value)

                    last_action = counters.get("lastCheckpointAction")
                    if last_action:
                        ts = parsers.parse_last_checkpoint_action(last_action)
                        if ts is not None:
                            last_checkpoint_ts.add_metric([target.name], ts)
                            checkpoint_age.add_metric([target.name], now - ts)
                else:
                    target_ok = False

            scrape_success.add_metric([target.name], 1 if target_ok else 0)

        for (tgt, cmd, etype), count in self._error_counts.items():
            errors_metric.add_metric([tgt, cmd, etype], count)

        yield up
        yield info_metric
        yield cmd_success
        yield cmd_duration
        yield replica_journal
        yield replica_sequence
        yield master_journal
        yield master_sequence
        yield seq_lag
        yield jrn_lag
        yield statefile_ts
        yield scrape_success
        yield journal_number
        yield last_checkpoint_ts
        yield checkpoint_age
        yield counter_metric
        yield archive_active_transfers
        yield archive_total_transfers
        yield archive_active_bytes
        yield archive_total_bytes
        yield archive_failed_transfers
        yield archive_healthy
        yield errors_metric

    def _record_command(
        self,
        result: p4mod.CommandResult,
        cmd_success: GaugeMetricFamily,
        cmd_duration: GaugeMetricFamily,
    ) -> None:
        cmd_success.add_metric(
            [result.target, result.command], 1 if result.ok else 0
        )
        cmd_duration.add_metric(
            [result.target, result.command], result.duration_seconds
        )
        if result.missing_binary:
            self._error_counts[
                (result.target, result.command, "missing_binary")
            ] += 1
        elif result.timed_out:
            self._error_counts[
                (result.target, result.command, "timeout")
            ] += 1
        elif result.returncode != 0:
            self._error_counts[
                (result.target, result.command, "nonzero_exit")
            ] += 1
