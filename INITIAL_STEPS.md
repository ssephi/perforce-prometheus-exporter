Here’s a starting point. I’ve written it as a runbook rather than notes, so future-you can repeat the process.

Helix Core Lab - Primary, Replica and Replication Troubleshooting

Environment

Server	ServerID	Port	Root
Primary	primary	127.0.0.1:1666	dbfiles
Replica	replica	127.0.0.1:1667	dbfiles2

Version:

P4D/MACOSX12ARM64/2026.1

⸻

Initial Primary Setup

Create server root

mkdir dbfiles

Start server

./p4d -r dbfiles -p 127.0.0.1:1666

Set server identity

p4 serverid primary

Restart server.

Verify

P4PORT=127.0.0.1:1666 p4 info

Expected:

ServerID: primary
Server services: standard

Create admin user

p4 user

Set password

p4 passwd

Create depot

p4 depot quake

Create workspace

p4 client

Example:

Root: /Users/sephi/Documents/development/quake
View:
    //quake/... //Garys-MacBook-Pro/...

Import repository

cd /Users/sephi/Documents/development/quake
p4 reconcile
p4 submit

⸻

Creating a Replica

This process should be repeatable for any additional replica.

Examples:

replica-lon
replica-ny
replica-aws

⸻

Create service user

P4PORT=127.0.0.1:1666 p4 user -f replica_svc

Set:

Type: service

Set password:

P4PORT=127.0.0.1:1666 p4 passwd replica_svc

Grant permissions

P4PORT=127.0.0.1:1666 p4 protect

Example:

super user replica_svc * //...

⸻

Create replica server specification

P4PORT=127.0.0.1:1666 p4 server replica

Example:

ServerID: replica
Type: server
Address: 127.0.0.1:1667
Services: replica
User: replica_svc
DistributedConfig:
    db.replication=readonly
    lbr.replication=readonly
    P4TARGET=127.0.0.1:1666

⸻

Create checkpoint

P4PORT=127.0.0.1:1666 p4 admin checkpoint

⸻

Create replica root

mkdir dbfiles2

⸻

Restore checkpoint

./p4d -r dbfiles2 -jr dbfiles/checkpoint.<latest>

⸻

Set replica identity

./p4d -r dbfiles2 -xD replica

⸻

Create service user ticket

P4PORT=127.0.0.1:1666 \
P4USER=replica_svc \
P4TICKETS=dbfiles2/.p4tickets \
p4 login

⸻

Start replica

P4TICKETS=dbfiles2/.p4tickets \
./p4d \
-r dbfiles2 \
-p 127.0.0.1:1667 \
-u replica_svc

⸻

Verify

P4PORT=127.0.0.1:1667 p4 info

Expected:

ServerID: replica
Server services: replica
Replica of: 127.0.0.1:1666

⸻

Creating Additional Replicas

Repeat the above process using:

replica-lon
127.0.0.1:1668
replica-ny
127.0.0.1:1669

Each replica requires:

* Unique ServerID
* Unique root directory
* Unique port
* Corresponding server specification
* Restored checkpoint
* Service-user ticket

⸻

Replication Health Checks

Check replica registration

Primary:

p4 servers

Expected:

replica server 127.0.0.1:1667 replica

⸻

Check replica identity

Replica:

p4 info

Expected:

Server services: replica

⸻

Check current journal state

Replica:

p4 pull -lj

Shows:

* Current replica journal
* Current master journal
* Pull status

⸻

Verify data movement

Primary:

p4 counter repltest 1

Replica:

p4 counter repltest

Values should match.

⸻

Initial Replication Troubleshooting

Problem

Server services: standard

Cause

Replica spec missing or not restored from checkpoint.

Fix

* Verify server spec exists on primary.
* Create new checkpoint.
* Rebuild replica.

⸻

Problem

Missing replication mode

Cause

Replica configured with P4TARGET but no replication mode.

Fix

db.replication=readonly

⸻

Problem

This server requires service user authentication for remote server access

Cause

2026 secure-by-default behaviour.

Fix

* Create service user.
* Configure User: in server spec.
* Create service ticket.
* Start replica using service user.

⸻

Problem

No entries made in db.topology

Cause

Missing ServerID.

Fix

p4 serverid primary

or

./p4d -r <root> -xD <serverid>

⸻

Problem

file(s) not in client view

Cause

Workspace mapping incorrect.

Fix

Verify:

p4 where .
p4 client -o

⸻

Problem

Replica not receiving updates

Checks

p4 info
p4 pull -lj
p4 servers

Verify:

* ServerID correct
* Services = replica
* P4TARGET correct
* Service ticket valid
* Replica started with service user
* Journal sequence advancing

:::
I'd also add a future section once you've created a third replica covering:
- Checkpoint rotation
- Journal rotation
- Replica rebuild from latest checkpoint
- Simulated primary failure and recovery
Those are exactly the sorts of scenarios that tend to come up in senior Perforce interviews.
