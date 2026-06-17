"""HTTP entry point for the Perforce Prometheus exporter.

Boring on purpose: load config, register the collector, start the
prometheus_client HTTP server, sleep forever.
"""

from __future__ import annotations

import logging
import signal
import sys
import time

from prometheus_client import REGISTRY, start_http_server

from perforce_exporter.collector import PerforceCollector
from perforce_exporter.config import load_config


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    log = logging.getLogger("perforce_exporter")

    try:
        cfg = load_config()
    except ValueError as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        return 2

    REGISTRY.register(PerforceCollector(cfg))
    start_http_server(cfg.listen_port)
    log.info(
        "listening on :%d targets=%s",
        cfg.listen_port,
        ",".join(f"{t.name}={t.port}" for t in cfg.targets),
    )

    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
    try:
        while True:
            time.sleep(3600)
    except KeyboardInterrupt:
        return 0


if __name__ == "__main__":
    sys.exit(main())
