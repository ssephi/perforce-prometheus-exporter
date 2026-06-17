# Perforce / Helix Core replication lab

A minimal two-node setup the exporter targets locally:

```
primary  127.0.0.1:1666     ServerID=master.1   services=standard
replica  127.0.0.1:1667     ServerID=replica.1  services=replica   Replica of=127.0.0.1:1666
```

Both p4d processes run as the current user against on-disk db roots. No
init scripts, no system service — start them in a terminal and stop them
with Ctrl-C.

Helix Core 2026.1 is **secure by default**. A replica won't start
without a unique `ServerID`, a server spec marked `Services: replica`,
the right `db.replication`/`lbr.replication` config, and a valid
service-user ticket. The steps below reproduce the working setup.

## 1. Primary

```sh
cd ~/Downloads/helix-core-server
mkdir -p dbfiles
./p4d -r dbfiles -p 127.0.0.1:1666
```

Sanity-check from another shell:

```sh
P4PORT=127.0.0.1:1666 ./p4 info
```

## 2. Create a service user on the primary

A "service" user (`Type: service`) is the identity the replica
authenticates as when pulling. Create the user spec:

```sh
P4PORT=127.0.0.1:1666 ./p4 user -f replica_svc
```

Edit the form so it contains:

```
User:   replica_svc
Type:   service
FullName: Replica service user
```

Then set its password and obtain a ticket the replica will use:

```sh
P4PORT=127.0.0.1:1666 ./p4 passwd replica_svc
mkdir -p dbfiles2
P4TICKETS=dbfiles2/.p4tickets P4PORT=127.0.0.1:1666 \
  ./p4 -u replica_svc login
```

The login writes a ticket into `dbfiles2/.p4tickets`. The replica reads
that file at startup.

## 3. Server spec for the replica

```sh
P4PORT=127.0.0.1:1666 ./p4 server -i <<'EOF'
ServerID:       replica.1
Type:           server
Services:       replica
Name:           replica.1
Address:        127.0.0.1:1667
Description:
    Local lab replica
EOF
```

## 4. Configure replication on the primary

These config values tell the replica to pull from the primary and stay
read-only.

```sh
P4PORT=127.0.0.1:1666 ./p4 configure set replica.1#P4TARGET=127.0.0.1:1666
P4PORT=127.0.0.1:1666 ./p4 configure set replica.1#db.replication=readonly
P4PORT=127.0.0.1:1666 ./p4 configure set replica.1#lbr.replication=readonly
P4PORT=127.0.0.1:1666 ./p4 configure set replica.1#serviceUser=replica_svc
```

## 5. Replica db root

The first time, seed the replica from a checkpoint of the primary:

```sh
P4PORT=127.0.0.1:1666 ./p4 admin checkpoint
cp dbfiles/checkpoint.* dbfiles2/
cp dbfiles/journal      dbfiles2/   # optional, for journal continuity
( cd dbfiles2 && ./p4d -r . -jr checkpoint.<N>.gz )
```

## 6. Start the replica

```sh
P4TICKETS=dbfiles2/.p4tickets \
  ./p4d -r dbfiles2 -p 127.0.0.1:1667 -u replica_svc
```

Important flags:

- `-r dbfiles2` — replica's own db root
- `-p 127.0.0.1:1667` — what the replica listens on
- `-u replica_svc` — the service user identity
- `P4TICKETS=...` — points at the ticket file from step 2

## 7. Validate replication

```sh
P4PORT=127.0.0.1:1667 ./p4 pull -lj
```

Expected:

```
Current replica journal state is:  Journal N, Sequence M.
Current master journal state is:   Journal N, Sequence M.
The statefile was last modified at: <recent timestamp>.
The replica server time is currently: <recent timestamp>.
```

When the journal/sequence numbers match, replication is caught up.

## 8. Point the exporter at it

```sh
export PERFORCE_TARGETS="primary=127.0.0.1:1666,replica=127.0.0.1:1667"
export P4_BIN=~/Downloads/helix-core-server/p4
.venv/bin/python exporter.py
```

Then bring up Prometheus + Grafana:

```sh
docker compose up -d
```

See `docs/troubleshooting.md` if any of this misbehaves.
