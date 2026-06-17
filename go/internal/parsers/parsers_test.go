package parsers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParseInfoPrimary(t *testing.T) {
	info := ParseInfo(readFixture(t, "p4_info_primary.txt"))

	checks := map[string]string{
		"ServerID":         "master.1",
		"Server services":  "standard",
		"Server address":   "127.0.0.1:1666", // value with embedded colon
		"Server version":   "P4D/LINUX26X86_64/2026.1/2589505 (2026/03/15)",
	}
	for k, want := range checks {
		if got := info[k]; got != want {
			t.Errorf("info[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseInfoReplica(t *testing.T) {
	info := ParseInfo(readFixture(t, "p4_info_replica.txt"))
	if info["ServerID"] != "replica.1" {
		t.Errorf("ServerID = %q, want replica.1", info["ServerID"])
	}
	if info["Server services"] != "replica" {
		t.Errorf("Server services = %q, want replica", info["Server services"])
	}
	if info["Replica of"] != "127.0.0.1:1666" {
		t.Errorf("Replica of = %q", info["Replica of"])
	}
}

func TestParsePullLJReplica(t *testing.T) {
	pj := ParsePullLJ(readFixture(t, "p4_pull_lj_replica.txt"))

	if !pj.HasReplicaJournal || pj.ReplicaJournal != 12 {
		t.Errorf("ReplicaJournal = %d (has=%v), want 12", pj.ReplicaJournal, pj.HasReplicaJournal)
	}
	if !pj.HasReplicaSequence || pj.ReplicaSequence != 4521 {
		t.Errorf("ReplicaSequence = %d, want 4521", pj.ReplicaSequence)
	}
	if !pj.HasMasterJournal || pj.MasterJournal != 12 {
		t.Errorf("MasterJournal = %d, want 12", pj.MasterJournal)
	}
	if !pj.HasMasterSequence || pj.MasterSequence != 5000 {
		t.Errorf("MasterSequence = %d, want 5000", pj.MasterSequence)
	}
	if !pj.HasStatefileModifiedTS {
		t.Fatalf("expected statefile_modified_ts")
	}
	want := time.Date(2026, 6, 13, 9, 0, 1, 0, time.UTC).Unix()
	if int64(pj.StatefileModifiedTS) != want {
		t.Errorf("statefile_modified_ts = %v, want %v", pj.StatefileModifiedTS, want)
	}
	if !pj.HasReplicaServerTimeTS {
		t.Errorf("expected replica_server_time_ts")
	}
}

func TestParsePullLJAuthWarning(t *testing.T) {
	pj := ParsePullLJ(readFixture(t, "p4_pull_lj_auth_warning.txt"))
	// auth warning is the first line; the parser must skip it and pick up
	// the journal-state lines that follow.
	if !pj.HasReplicaJournal || pj.ReplicaJournal != 8 {
		t.Errorf("ReplicaJournal = %d, want 8", pj.ReplicaJournal)
	}
	if !pj.HasMasterSequence || pj.MasterSequence != 100 {
		t.Errorf("MasterSequence = %d, want 100", pj.MasterSequence)
	}
}

func TestParseCounters(t *testing.T) {
	c := ParseCounters(readFixture(t, "p4_counters.txt"))
	if c["journal"] != "4" {
		t.Errorf("journal = %q, want 4", c["journal"])
	}
	if c["change"] != "1" {
		t.Errorf("change = %q, want 1", c["change"])
	}
	// depotStats contains '=' in its value; split-on-first-'=' should preserve it.
	if got := c["depotStats"]; got == "" || got[:4] != "revs" {
		t.Errorf("depotStats = %q, want value starting with 'revs'", got)
	}
}

func TestParseLastCheckpointAction(t *testing.T) {
	c := ParseCounters(readFixture(t, "p4_counters.txt"))
	ts, ok := ParseLastCheckpointAction(c["lastCheckpointAction"])
	if !ok {
		t.Fatal("ParseLastCheckpointAction returned !ok")
	}
	if int64(ts) != 1781337732 {
		t.Errorf("ts = %v, want 1781337732", ts)
	}
}

func TestParsePullLSIdleAndBusy(t *testing.T) {
	idle := ParsePullLS(readFixture(t, "p4_pull_ls_idle.txt"))
	if !idle.OK {
		t.Fatal("idle: expected OK")
	}
	if idle.ActiveTransfers != 0 || idle.TotalTransfers != 0 {
		t.Errorf("idle transfers: %+v", idle)
	}

	busy := ParsePullLS(readFixture(t, "p4_pull_ls_busy.txt"))
	if !busy.OK {
		t.Fatal("busy: expected OK")
	}
	if busy.ActiveTransfers != 3 || busy.TotalTransfers != 27 {
		t.Errorf("busy transfers: %+v", busy)
	}
	if busy.ActiveBytes != 524288 || busy.TotalBytes != 10485760 {
		t.Errorf("busy bytes: %+v", busy)
	}
}

func TestParsePullLWithFailure(t *testing.T) {
	l := ParsePullL(readFixture(t, "p4_pull_l_with_failure.txt"))
	if l.TotalListed != 3 {
		t.Errorf("TotalListed = %d, want 3", l.TotalListed)
	}
	if l.FailedListed != 1 {
		t.Errorf("FailedListed = %d, want 1", l.FailedListed)
	}
}

func TestParsePullLSMissingLine(t *testing.T) {
	if got := ParsePullLS("nothing useful here\n"); got.OK {
		t.Errorf("expected !OK on missing summary, got %+v", got)
	}
}
