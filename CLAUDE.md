Save this as CLAUDE.md in the repo root.

CLAUDE.md

Project

Build a Prometheus exporter for Perforce / Helix Core.

The exporter should collect operational metrics from one or more Perforce servers using the p4 CLI and expose them via an HTTP /metrics endpoint.

Initial target use case:

Primary:  127.0.0.1:1666
Replica:  127.0.0.1:1667

The exporter should focus first on replication health, especially detecting whether a replica is behind the primary.

⸻

Goals

Implement a small, reliable Prometheus exporter that can report:

* Perforce server availability
* Server identity
* Server type / services
* Primary vs replica relationship
* Journal replication state
* Journal sequence lag
* Replication health
* Last successful scrape
* Command execution errors

Future metrics may include:

* Archive replication health
* Checkpoint age
* Journal size
* Depot file counts
* Changelist counts
* Pending changelists
* Open files
* User/client counts

⸻

Non-Goals

Do not build a full Perforce administration tool.

Do not mutate Perforce state.

Do not run destructive commands.

Do not require admin access for basic health metrics unless unavoidable.

Do not parse logs in the first version.

Do not require P4Python in the first version.

⸻

Technology

Preferred implementation:

Python 3
prometheus_client
subprocess
p4 CLI

The exporter should shell out to p4 rather than using P4Python initially.

Reason:

* Easier to install
* Mirrors how operators debug Perforce manually
* Keeps the first version simple
* Works well for lab and production proof-of-concept use

⸻

Runtime Model

The exporter runs as a long-lived HTTP service.

It periodically executes Perforce commands during scrape collection.

Default listen port:

9117

Default endpoint:

/metrics

⸻

Configuration

Configuration should be via environment variables initially.

Required / useful variables:

PERFORCE_TARGETS
EXPORTER_PORT
P4_BIN
P4_TIMEOUT_SECONDS

Example:

export PERFORCE_TARGETS="primary=127.0.0.1:1666,replica=127.0.0.1:1667"
export EXPORTER_PORT=9117
export P4_BIN=p4
export P4_TIMEOUT_SECONDS=10

Optional future variables:

P4USER
P4PASSWD
P4TICKETS
P4CONFIG

The exporter should pass through the existing environment where possible.

⸻

Initial Commands To Collect

p4 info

Run:

P4PORT=<target> p4 info

Parse:

Server address
Server root
Server date
Server uptime
Server version
ServerID
Server services
Replica of
Server license
Case Handling

Metrics:

perforce_up{target,server_id,server_services}
perforce_info{target,server_id,server_services,server_version,replica_of}
perforce_scrape_success{target}
perforce_scrape_duration_seconds{target,command}

perforce_info should be an info-style gauge with value 1.

⸻

p4 pull -lj

Run only where appropriate.

Command:

P4PORT=<target> p4 pull -lj

Parse:

Current replica journal state is: Journal <n>, Sequence <n>.
Current master journal state is: Journal <n>, Sequence <n>.
The statefile was last modified at: <timestamp>.
The replica server time is currently: <timestamp>.

Metrics:

perforce_replica_journal{target}
perforce_replica_sequence{target}
perforce_master_journal{target}
perforce_master_sequence{target}
perforce_replication_sequence_lag{target}
perforce_replication_journal_lag{target}
perforce_replication_statefile_modified_timestamp_seconds{target}

Lag calculation:

perforce_replication_sequence_lag = master_sequence - replica_sequence
perforce_replication_journal_lag = master_journal - replica_journal

If values cannot be parsed, export command error metrics but do not crash.

⸻

p4 pull -l

Run on replicas if useful.

Command:

P4PORT=<target> p4 pull -l

Possible metrics:

perforce_pull_threads{target}
perforce_pull_thread_up{target,thread}

This command may return empty output. Empty output should not be treated as fatal by itself.

⸻

p4 counters

Optional for v1.

Useful counters:

journal
upgrade
maxCommitChange
lastCheckpointAction

Metrics:

perforce_counter{target,name}
perforce_last_checkpoint_timestamp_seconds{target}
perforce_checkpoint_age_seconds{target}

⸻

Error Handling

Every Perforce command should have:

* Timeout
* Captured stdout
* Captured stderr
* Exit code handling

Exporter must not crash if a command fails.

Expose failures as metrics:

perforce_command_success{target,command}
perforce_command_duration_seconds{target,command}
perforce_command_errors_total{target,command,error_type}

Suggested error types:

timeout
nonzero_exit
parse_error
missing_binary

⸻

Security

Do not log passwords.

Do not expose tickets.

Do not expose full environment variables.

Do not include command-line secrets in labels.

Use labels only for stable, low-cardinality data.

Avoid labels such as file path, changelist description, username, or raw error message.

⸻

Prometheus Metric Style

Use Prometheus naming conventions:

* Prefix all metrics with perforce_
* Use seconds for timestamps/durations
* Use _total for counters
* Use _seconds for durations/timestamps where appropriate
* Keep label cardinality low

Preferred labels:

target
server_id
server_services
command

Avoid high-cardinality labels.

⸻

Initial File Layout

Suggested:

perforce-prom-exporter/
├── CLAUDE.md
├── README.md
├── requirements.txt
├── exporter.py
├── perforce_exporter/
│   ├── __init__.py
│   ├── collector.py
│   ├── p4.py
│   ├── parsers.py
│   └── config.py
├── tests/
│   ├── test_parse_info.py
│   ├── test_parse_pull_lj.py
│   └── fixtures/
│       ├── p4_info_primary.txt
│       ├── p4_info_replica.txt
│       └── p4_pull_lj_replica.txt
└── docker/
    └── Dockerfile

⸻

Implementation Notes

Command wrapper

Create a reusable command runner:

run_p4(target: str, args: list[str], timeout: int) -> CommandResult

It should:

* Set P4PORT for the command
* Run p4
* Capture stdout/stderr
* Return structured result
* Never raise for normal Perforce command failure

⸻

Parser design

Parsers should be pure functions.

Example:

parse_p4_info(text: str) -> dict
parse_pull_lj(text: str) -> dict

This makes testing simple.

⸻

Testing

Use fixture files based on real command output from the lab.

Test cases:

* Primary p4 info
* Replica p4 info
* p4 pull -lj healthy output
* p4 pull -lj with service-user auth warning
* Empty p4 pull -l
* Missing fields
* Non-zero command exit

⸻

Lab Commands

Primary:

./p4d -r dbfiles -p 127.0.0.1:1666

Replica:

P4TICKETS=dbfiles2/.p4tickets \
./p4d -r dbfiles2 -p 127.0.0.1:1667 -u replica_svc

Exporter:

export PERFORCE_TARGETS="primary=127.0.0.1:1666,replica=127.0.0.1:1667"
python exporter.py

Scrape:

curl http://127.0.0.1:9117/metrics

⸻

First Milestone

Build a working exporter that outputs:

perforce_up
perforce_info
perforce_command_success
perforce_command_duration_seconds
perforce_replica_journal
perforce_replica_sequence
perforce_master_journal
perforce_master_sequence
perforce_replication_sequence_lag
perforce_replication_journal_lag

Acceptance test:

1. Run primary and replica locally.
2. Start exporter.
3. Confirm /metrics returns data for both targets.
4. Submit a change or increment a counter on primary.
5. Confirm lag metric changes or remains zero once replica catches up.

⸻

Second Milestone

Add checkpoint and journal metrics.

Suggested metrics:

perforce_journal_number
perforce_last_checkpoint_timestamp_seconds
perforce_checkpoint_age_seconds

⸻

Third Milestone

Add archive replication visibility.

Investigate:

p4 pull -l
p4 pull -lj
server logs
journal / archive pull thread state

Goal metrics:

perforce_archive_pull_threads
perforce_archive_pull_errors_total
perforce_archive_replication_healthy

Do not invent archive backlog metrics unless they can be reliably derived.

⸻

Important Behaviour Observed In Lab

Helix Core 2026.1 is secure by default.

Replica setup required:

* Unique ServerID
* Server spec with Services: replica
* db.replication=readonly
* lbr.replication=readonly
* P4TARGET=127.0.0.1:1666
* Service user with Type: service
* Valid service-user ticket
* Replica started with -u replica_svc
* Replica started with matching P4TICKETS

Example working replica start:

P4TICKETS=dbfiles2/.p4tickets \
./p4d -r dbfiles2 -p 127.0.0.1:1667 -u replica_svc

⸻

Tone / Style

Keep the implementation small, boring, and operational.

This is infrastructure software.

Prefer explicit code over clever abstractions.

Prefer clear metrics over excessive metrics.

Prefer reliable parsing over fragile regex where possible.

Add comments where Perforce behaviour is non-obvious.
