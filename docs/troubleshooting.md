# Troubleshooting

Symptoms seen in the lab while building this exporter, and what each one
actually meant.

## Containerised exporter: `perforce_up = 1` but other commands fail

`perforce_command_success{command="counters"}` and the various
`pull -lj/-l/-ls` rows are 0 even though `perforce_up` is 1.

`p4 info` works anonymously; the others need a ticket. The compose file
mounts `${HOME}/.p4tickets` from your host into the container at
`/root/.p4tickets` and forwards `P4USER` from your shell. If you have no
ticket file yet:

```sh
P4PORT=127.0.0.1:1666 p4 login    # creates ~/.p4tickets
docker compose up -d --force-recreate exporter
```

## `perforce_up{target="…"} = 0`

`p4 info` failed for the target. Causes ranked by frequency:

1. **p4d not running.** Check with `lsof -nP -iTCP:<port> -sTCP:LISTEN`.
2. **Wrong port.** `PERFORCE_TARGETS` is `name=host:port`. A typo here
   silently routes the exporter to a closed socket; `perforce_up` is 0
   and `perforce_command_errors_total{error_type="nonzero_exit"}`
   increments.
3. **Auth required.** A secure server can reject anonymous `p4 info`;
   in practice `p4 info` works without auth even on Helix Core 2026.1,
   but other commands won't. Export `P4USER` and ensure a ticket exists.

## `perforce_command_errors_total{error_type="missing_binary"}` ticking

The exporter could not find a `p4` binary. Either:

- `P4_BIN` is unset and `p4` is not on `PATH`, **or**
- the path you provided doesn't exist or isn't executable.

Set `P4_BIN` to a full path (e.g. `/Users/you/Downloads/helix-core-server/p4`).

## Replica won't start

```
Perforce server error: Server must have a unique ServerID
```

The replica server spec was not created on the primary. Re-run the
`./p4 server -i` step from the lab guide. The `ServerID` in the spec
must match what the replica process believes its identity to be.

```
Perforce password (P4PASSWD) invalid or unset.
```

The replica started without a valid service-user ticket. Re-login as
`replica_svc` with `P4TICKETS` pointing at the file the replica reads,
then restart with the same `P4TICKETS=…` in front of `p4d`.

## `p4 pull -lj` returns the auth-warning line and nothing else

The replica is up but its pull thread can't authenticate. The parser
ignores the warning line and returns an empty dict, which the collector
treats as a parse failure: `perforce_scrape_success{target=replica} = 0`.
Fix by re-logging in as the service user.

## Replication sequence lag never falls

If `perforce_replication_sequence_lag` is positive and stable, the
replica is connected but stuck. Common causes:

- **Failed archive transfer blocking the journal pull**: check
  `perforce_archive_pull_failed_transfers`. A non-zero value here points
  at a specific file that the replica can't fetch.
- **Disk full on the replica.** `df -h dbfiles2`.
- **Service user ticket expired.** Re-login and restart.

If lag *grows* steadily, the pull thread isn't keeping up with master's
write rate — usually a sign of network or disk bottleneck, not a config
bug.

## `perforce_replication_statefile_modified_timestamp_seconds` getting stale

The statefile timestamp not advancing means the replica process isn't
making forward progress at all. Most often the `p4d` for the replica has
been stopped. Confirm with `ps`.

## Prometheus target shows as `DOWN` in the UI

If `perforce_up` is 1 inside the exporter's `/metrics` but Prometheus
itself shows the scrape as down:

- From the prometheus container, hit
  `host.docker.internal:9117/metrics` — should return text.
- On Linux (no Docker Desktop), `host.docker.internal` doesn't resolve;
  the compose file sets `extra_hosts: host.docker.internal:host-gateway`
  which works on Docker 20.10+. If you're on something older, replace
  the target with the host's real IP.

## Grafana datasource is missing

If the "Perforce" folder is empty or the dashboard shows "No data":

1. `docker compose logs grafana | grep provision` — look for errors
   loading `/etc/grafana/provisioning/...`.
2. Confirm the datasource uid is `prometheus` (the dashboard panels
   reference it explicitly). It's set in
   `grafana/provisioning/datasources/datasource.yml`.

## Port already in use when starting the exporter

`OSError: [Errno 48] Address already in use`. Another exporter instance
is bound to `:9117`. Find it:

```sh
lsof -nP -iTCP:9117 -sTCP:LISTEN
kill <pid>
```

## Dashboard shows duplicate rows for the same target

A stat panel or graph shows e.g. two `replica` rows, one UP and one DOWN.
This is a stale `instance` label left behind after a scrape-config change
(for example, moving the exporter from running on the host to running
inside compose changes `instance` from `host.docker.internal:9117` to
`exporter:9117`). Prometheus keeps the old series until its samples roll
off the dashboard's time window.

Either wait — the stale series will fall off when the dashboard time
range no longer covers any of its samples — or tombstone it explicitly
(requires `--web.enable-admin-api`, enabled in this repo's compose):

```sh
curl -X POST -g \
  'http://127.0.0.1:9090/api/v1/admin/tsdb/delete_series?match[]={instance="host.docker.internal:9117"}'
curl -X POST 'http://127.0.0.1:9090/api/v1/admin/tsdb/clean_tombstones'
```

## Alert is `pending` long after the underlying problem cleared

Rules using `rate(...)` over a 5-minute window stay non-zero for that
full 5 minutes after the last error sample. That's expected — the
counter increment hasn't aged out of the window yet. The alert flips
back to `inactive` automatically.
