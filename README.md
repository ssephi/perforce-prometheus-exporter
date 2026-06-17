# perforce-prom-exporter

Prometheus exporter for Helix Core / Perforce replication health, built
against a local primary/replica lab.

The exporter shells out to the `p4` CLI, scrapes each target on demand,
and exposes operational metrics on `/metrics`. It ships with a
docker-compose stack that brings up Prometheus (with alert rules) and
Grafana (with a provisioned dashboard) so you can see the whole pipeline
working end-to-end.

![Perforce overview dashboard](screenshots/grafana-dashboard.png)

## Repo layout

```
.
├── exporter.py                       # HTTP entry point
├── perforce_exporter/                # collector / parsers / p4 wrapper / config
├── tests/                            # pytest + fixture-based parser tests
├── docker/Dockerfile                 # exporter image (p4 CLI baked in)
├── docker-compose.yml                # exporter + prometheus + grafana
├── go/                               # static Go binary (alternative to docker)
├── prometheus/
│   ├── prometheus.yml                # scrape config
│   └── rules/perforce.rules.yml      # alert rules
├── grafana/provisioning/             # datasource + dashboard provider
├── dashboards/perforce-overview.json # the dashboard itself
├── docs/
│   ├── perforce-replication-lab.md   # primary + replica + service user
│   └── troubleshooting.md            # symptoms seen in the lab
└── screenshots/                      # README links to grafana-dashboard.png
```

## Quick start

You need:

- A Perforce primary and (optionally) a replica reachable on the host.
  `docs/perforce-replication-lab.md` walks through bringing both up
  locally from the Helix Core tarball.
- Docker.

```sh
docker compose up -d --build
```

That brings up three containers:

| Service     | Port | Purpose                               |
| ----------- | ---- | ------------------------------------- |
| exporter    | 9117 | scrapes p4 targets, exposes /metrics  |
| prometheus  | 9090 | scrapes the exporter, evaluates rules |
| grafana     | 3000 | dashboard (admin/admin)               |

Open <http://127.0.0.1:3000> → `Dashboards → Perforce → Perforce overview`.

The exporter, by default, targets `host.docker.internal:1666` and
`host.docker.internal:1667`. Override via `PERFORCE_TARGETS` on the
exporter service to point at real servers.

### Authentication

`p4 info` works anonymously but `p4 counters`, `p4 pull -lj/-l/-ls`
require a logged-in identity. The compose file mounts `~/.p4tickets`
from your host into the container and reads `P4USER` from your shell
environment so the exporter picks up the same identity you have when
running `p4` interactively. If you don't have a ticket file yet:

```sh
P4PORT=127.0.0.1:1666 p4 login    # produces ~/.p4tickets
```

For a production deployment, replace the host-tickets mount with a
service-user ticket file managed by your secret store.

### Running the exporter on the host instead

If you'd rather run the exporter outside Docker (useful while iterating
on the Python code):

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt

export PERFORCE_TARGETS="primary=127.0.0.1:1666,replica=127.0.0.1:1667"
export P4_BIN=/path/to/p4
.venv/bin/python exporter.py
```

Then in `prometheus/prometheus.yml` swap `exporter:9117` for
`host.docker.internal:9117` and `docker compose up -d prometheus grafana`.

### Static Go binary (for hosts where Docker isn't an option)

A direct port lives in [`go/`](go/) — same metrics and labels, a single
statically-linked ~10 MB binary, cross-compiled to Linux amd64/arm64,
darwin/arm64 and windows/amd64. Useful when you're dropping the exporter
next to the box already running `p4d` and Docker on that host is more
trouble than it's worth.

```sh
cd go && make dist
scp dist/perforce-exporter-linux-amd64 admin@p4-primary:/usr/local/bin/perforce-exporter
```

Point Prometheus at `:9117/metrics`; the dashboard JSON and alert rules
in this repo work unchanged.

## Configuration

| Variable               | Default | Purpose                                              |
| ---------------------- | ------- | ---------------------------------------------------- |
| `PERFORCE_TARGETS`     | —       | Comma-separated `name=host:port` list (required).    |
| `EXPORTER_PORT`        | `9117`  | HTTP listen port.                                    |
| `P4_BIN`               | `p4`    | Path to the p4 binary.                               |
| `P4_TIMEOUT_SECONDS`   | `10`    | Per-command timeout.                                 |

Other `P4*` environment variables (e.g. `P4USER`, `P4TICKETS`) are
forwarded to the underlying `p4` process unchanged.

## Metrics

### Availability & identity (from `p4 info`)

- `perforce_up{target,server_id,server_services}` — 1 if `p4 info` succeeded
- `perforce_info{target,server_id,server_services,server_version,replica_of}` — static info, value 1
- `perforce_scrape_success{target}` — 1 if every command for the target succeeded this scrape
- `perforce_command_success{target,command}` — 1/0 for the most recent run of each command
- `perforce_command_duration_seconds{target,command}` — last command duration
- `perforce_command_errors_total{target,command,error_type}` — cumulative; error_type ∈ `timeout|nonzero_exit|missing_binary|parse_error`

### Replication health (from `p4 pull -lj`, replicas only)

- `perforce_replica_journal{target}`, `perforce_replica_sequence{target}`
- `perforce_master_journal{target}`, `perforce_master_sequence{target}`
- `perforce_replication_journal_lag{target}` = master − replica
- `perforce_replication_sequence_lag{target}` = master − replica (bytes)
- `perforce_replication_statefile_modified_timestamp_seconds{target}`

### Checkpoint / journal counters (from `p4 counters`)

- `perforce_journal_number{target}`
- `perforce_last_checkpoint_timestamp_seconds{target}`
- `perforce_checkpoint_age_seconds{target}`
- `perforce_counter{target,name}` — whitelisted: `journal`, `change`, `maxCommitChange`, `upgrade`

### Archive replication (from `p4 pull -ls` / `-l`, replicas only)

- `perforce_archive_pull_active_transfers{target}` / `active_bytes{target}`
- `perforce_archive_pull_queued_transfers{target}` / `queued_bytes{target}` — includes active
- `perforce_archive_pull_failed_transfers{target}` — heuristic (current queue)
- `perforce_archive_replication_healthy{target}` — 1 when commands ok and no failed transfers

## Alert rules

`prometheus/rules/perforce.rules.yml` defines:

- `PerforceTargetDown`, `PerforceScrapeDown`
- `PerforceReplicationStalled`, `PerforceReplicationJournalLag`, `PerforceReplicaStateFileStale`
- `PerforceArchiveReplicationUnhealthy`, `PerforceArchiveFailedTransfers`
- `PerforceCheckpointStale`, `PerforceCheckpointVeryStale`
- `PerforceCommandErrors`

After editing rules: `curl -X POST http://127.0.0.1:9090/-/reload`.

## Tests

```sh
.venv/bin/python -m pytest
```

Parsers are exercised against fixture files captured from the lab in
`tests/fixtures/`.

## Docs

- [docs/perforce-replication-lab.md](docs/perforce-replication-lab.md) — primary, replica, service user, ticket
- [docs/troubleshooting.md](docs/troubleshooting.md) — symptoms seen in the lab and what each one meant

## Design notes

- Shells out to `p4` rather than using P4Python — easier to install and
  mirrors how operators debug Perforce by hand.
- Parsers are pure `str → dict` functions, so they test cleanly against
  captured fixture output.
- Every scrape re-runs commands; there is no background poller. This
  keeps state simple and makes failure visible per request rather than
  through a stale cache.
- Label cardinality is kept low: no usernames, file paths, or raw error
  messages on labels.
