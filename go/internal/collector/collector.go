// Package collector implements a prometheus.Collector that runs the p4 CLI
// once per scrape. It mirrors the Python collector in metric names, labels
// and command order so the existing Grafana dashboard and Prometheus rules
// work unchanged.
package collector

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ssephi/perforce-prom-exporter/internal/config"
	"github.com/ssephi/perforce-prom-exporter/internal/p4"
	"github.com/ssephi/perforce-prom-exporter/internal/parsers"
)

// Only expose well-known low-cardinality numeric counters. Adding
// depotStats / *State counters here would either blow up cardinality or
// fail to parse — they are deliberately excluded.
var numericCounterWhitelist = []string{"journal", "change", "maxCommitChange", "upgrade"}

type errorKey struct {
	Target, Command, ErrorType string
}

type Collector struct {
	cfg config.Config

	mu          sync.Mutex
	errorCounts map[errorKey]float64

	// descriptors (one per metric family)
	up                       *prometheus.Desc
	info                     *prometheus.Desc
	cmdSuccess               *prometheus.Desc
	cmdDuration              *prometheus.Desc
	replicaJournal           *prometheus.Desc
	replicaSequence          *prometheus.Desc
	masterJournal            *prometheus.Desc
	masterSequence           *prometheus.Desc
	seqLag                   *prometheus.Desc
	jrnLag                   *prometheus.Desc
	statefileTS              *prometheus.Desc
	scrapeSuccess            *prometheus.Desc
	errorsTotal              *prometheus.Desc
	journalNumber            *prometheus.Desc
	lastCheckpointTS         *prometheus.Desc
	checkpointAge            *prometheus.Desc
	counterMetric            *prometheus.Desc
	archiveActiveTransfers   *prometheus.Desc
	archiveQueuedTransfers   *prometheus.Desc
	archiveActiveBytes       *prometheus.Desc
	archiveQueuedBytes       *prometheus.Desc
	archiveFailedTransfers   *prometheus.Desc
	archiveReplicationHealth *prometheus.Desc
}

func New(cfg config.Config) *Collector {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &Collector{
		cfg:         cfg,
		errorCounts: map[errorKey]float64{},

		up: d("perforce_up",
			"1 if `p4 info` succeeded for the target during this scrape.",
			"target", "server_id", "server_services"),
		info: d("perforce_info",
			"Static Perforce server info; value is always 1.",
			"target", "server_id", "server_services", "server_version", "replica_of"),
		cmdSuccess: d("perforce_command_success",
			"1 if the last execution of this p4 command for the target succeeded.",
			"target", "command"),
		cmdDuration: d("perforce_command_duration_seconds",
			"Wall-clock duration of the last p4 command execution.",
			"target", "command"),
		replicaJournal: d("perforce_replica_journal",
			"Replica journal number reported by `p4 pull -lj`.", "target"),
		replicaSequence: d("perforce_replica_sequence",
			"Replica journal byte sequence reported by `p4 pull -lj`.", "target"),
		masterJournal: d("perforce_master_journal",
			"Master journal number as observed by the replica.", "target"),
		masterSequence: d("perforce_master_sequence",
			"Master journal byte sequence as observed by the replica.", "target"),
		seqLag: d("perforce_replication_sequence_lag",
			"master_sequence - replica_sequence (bytes).", "target"),
		jrnLag: d("perforce_replication_journal_lag",
			"master_journal - replica_journal.", "target"),
		statefileTS: d("perforce_replication_statefile_modified_timestamp_seconds",
			"Unix timestamp when the replica statefile was last modified.", "target"),
		scrapeSuccess: d("perforce_scrape_success",
			"1 if all commands for the target succeeded during this scrape.", "target"),
		errorsTotal: d("perforce_command_errors_total",
			"Cumulative count of p4 command errors observed by the exporter.",
			"target", "command", "error_type"),
		journalNumber: d("perforce_journal_number",
			"Current journal number from the `journal` counter.", "target"),
		lastCheckpointTS: d("perforce_last_checkpoint_timestamp_seconds",
			"Unix timestamp of the last checkpoint action.", "target"),
		checkpointAge: d("perforce_checkpoint_age_seconds",
			"Seconds since the last checkpoint action.", "target"),
		counterMetric: d("perforce_counter",
			"Selected numeric Perforce counters (whitelisted to keep cardinality low).",
			"target", "name"),
		archiveActiveTransfers: d("perforce_archive_pull_active_transfers",
			"Archive file transfers currently in flight (from `p4 pull -ls`).", "target"),
		archiveQueuedTransfers: d("perforce_archive_pull_queued_transfers",
			"Archive file transfers queued including active (from `p4 pull -ls`).", "target"),
		archiveActiveBytes: d("perforce_archive_pull_active_bytes",
			"Bytes of archive content currently transferring (from `p4 pull -ls`).", "target"),
		archiveQueuedBytes: d("perforce_archive_pull_queued_bytes",
			"Bytes of archive content queued including active (from `p4 pull -ls`).", "target"),
		archiveFailedTransfers: d("perforce_archive_pull_failed_transfers",
			"Failed archive transfers currently visible in `p4 pull -l` (heuristic).", "target"),
		archiveReplicationHealth: d("perforce_archive_replication_healthy",
			"1 if archive pull commands succeeded and no failed transfers are queued.", "target"),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{
		c.up, c.info, c.cmdSuccess, c.cmdDuration,
		c.replicaJournal, c.replicaSequence, c.masterJournal, c.masterSequence,
		c.seqLag, c.jrnLag, c.statefileTS, c.scrapeSuccess, c.errorsTotal,
		c.journalNumber, c.lastCheckpointTS, c.checkpointAge, c.counterMetric,
		c.archiveActiveTransfers, c.archiveQueuedTransfers,
		c.archiveActiveBytes, c.archiveQueuedBytes,
		c.archiveFailedTransfers, c.archiveReplicationHealth,
	} {
		ch <- desc
	}
}

// Collect is invoked once per scrape. We re-run all commands every time;
// there is no background poller, so failures surface per-request rather
// than via a stale cache.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	now := time.Now().Unix()
	opt := p4.Options{
		P4Bin:   c.cfg.P4Bin,
		Timeout: time.Duration(c.cfg.P4TimeoutSeconds) * time.Second,
	}

	for _, t := range c.cfg.Targets {
		targetOK := true

		// --- p4 info ---------------------------------------------------
		infoRes := p4.Run(t.Name, t.Port, []string{"info"}, opt)
		c.recordCommand(ch, infoRes)

		var services, serverID string
		if infoRes.OK() {
			parsed := parsers.ParseInfo(infoRes.Stdout)
			serverID = parsed["ServerID"]
			services = parsed["Server services"]
			replicaOf := parsed["Replica of"]
			version := parsed["Server version"]
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1, t.Name, serverID, services)
			ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
				t.Name, serverID, services, version, replicaOf)
		} else {
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0, t.Name, "", "")
			targetOK = false
		}

		// --- p4 pull -lj (replicas only) ------------------------------
		if services != "" && strings.Contains(strings.ToLower(services), "replica") {
			pullRes := p4.Run(t.Name, t.Port, []string{"pull", "-lj"}, opt)
			c.recordCommand(ch, pullRes)

			if pullRes.OK() {
				pj := parsers.ParsePullLJ(pullRes.Stdout)
				if !pj.Has() {
					c.bumpError(t.Name, "pull -lj", "parse_error")
					targetOK = false
				}
				if pj.HasReplicaJournal {
					ch <- prometheus.MustNewConstMetric(c.replicaJournal, prometheus.GaugeValue, float64(pj.ReplicaJournal), t.Name)
				}
				if pj.HasReplicaSequence {
					ch <- prometheus.MustNewConstMetric(c.replicaSequence, prometheus.GaugeValue, float64(pj.ReplicaSequence), t.Name)
				}
				if pj.HasMasterJournal {
					ch <- prometheus.MustNewConstMetric(c.masterJournal, prometheus.GaugeValue, float64(pj.MasterJournal), t.Name)
				}
				if pj.HasMasterSequence {
					ch <- prometheus.MustNewConstMetric(c.masterSequence, prometheus.GaugeValue, float64(pj.MasterSequence), t.Name)
				}
				if pj.HasMasterJournal && pj.HasReplicaJournal {
					ch <- prometheus.MustNewConstMetric(c.jrnLag, prometheus.GaugeValue, float64(pj.MasterJournal-pj.ReplicaJournal), t.Name)
				}
				if pj.HasMasterSequence && pj.HasReplicaSequence {
					ch <- prometheus.MustNewConstMetric(c.seqLag, prometheus.GaugeValue, float64(pj.MasterSequence-pj.ReplicaSequence), t.Name)
				}
				if pj.HasStatefileModifiedTS {
					ch <- prometheus.MustNewConstMetric(c.statefileTS, prometheus.GaugeValue, pj.StatefileModifiedTS, t.Name)
				}
			} else {
				targetOK = false
			}

			// --- p4 pull -ls / -l (archive replication) --------------
			archiveOK := true
			var failedCount int = -1 // sentinel: unknown

			lsRes := p4.Run(t.Name, t.Port, []string{"pull", "-ls"}, opt)
			c.recordCommand(ch, lsRes)
			if lsRes.OK() {
				ls := parsers.ParsePullLS(lsRes.Stdout)
				if ls.OK {
					ch <- prometheus.MustNewConstMetric(c.archiveActiveTransfers, prometheus.GaugeValue, float64(ls.ActiveTransfers), t.Name)
					ch <- prometheus.MustNewConstMetric(c.archiveQueuedTransfers, prometheus.GaugeValue, float64(ls.TotalTransfers), t.Name)
					ch <- prometheus.MustNewConstMetric(c.archiveActiveBytes, prometheus.GaugeValue, float64(ls.ActiveBytes), t.Name)
					ch <- prometheus.MustNewConstMetric(c.archiveQueuedBytes, prometheus.GaugeValue, float64(ls.TotalBytes), t.Name)
				} else {
					archiveOK = false
				}
			} else {
				archiveOK = false
				targetOK = false
			}

			lRes := p4.Run(t.Name, t.Port, []string{"pull", "-l"}, opt)
			c.recordCommand(ch, lRes)
			if lRes.OK() {
				lc := parsers.ParsePullL(lRes.Stdout)
				failedCount = lc.FailedListed
				ch <- prometheus.MustNewConstMetric(c.archiveFailedTransfers, prometheus.GaugeValue, float64(failedCount), t.Name)
			} else {
				archiveOK = false
				targetOK = false
			}

			healthy := 0.0
			if archiveOK && failedCount == 0 {
				healthy = 1
			}
			ch <- prometheus.MustNewConstMetric(c.archiveReplicationHealth, prometheus.GaugeValue, healthy, t.Name)
		}

		// --- p4 counters ----------------------------------------------
		// Only when info succeeded — otherwise the target is down and a
		// second failing command just adds noise.
		if infoRes.OK() {
			cRes := p4.Run(t.Name, t.Port, []string{"counters"}, opt)
			c.recordCommand(ch, cRes)
			if cRes.OK() {
				counters := parsers.ParseCounters(cRes.Stdout)
				for _, name := range numericCounterWhitelist {
					raw, ok := counters[name]
					if !ok {
						continue
					}
					v, err := strconv.ParseFloat(raw, 64)
					if err != nil {
						continue
					}
					ch <- prometheus.MustNewConstMetric(c.counterMetric, prometheus.GaugeValue, v, t.Name, name)
					if name == "journal" {
						ch <- prometheus.MustNewConstMetric(c.journalNumber, prometheus.GaugeValue, v, t.Name)
					}
				}
				if last, ok := counters["lastCheckpointAction"]; ok && last != "" {
					if ts, ok := parsers.ParseLastCheckpointAction(last); ok {
						ch <- prometheus.MustNewConstMetric(c.lastCheckpointTS, prometheus.GaugeValue, ts, t.Name)
						ch <- prometheus.MustNewConstMetric(c.checkpointAge, prometheus.GaugeValue, float64(now)-ts, t.Name)
					}
				}
			} else {
				targetOK = false
			}
		}

		scrapeVal := 0.0
		if targetOK {
			scrapeVal = 1
		}
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, scrapeVal, t.Name)
	}

	c.mu.Lock()
	for k, v := range c.errorCounts {
		ch <- prometheus.MustNewConstMetric(c.errorsTotal, prometheus.CounterValue, v, k.Target, k.Command, k.ErrorType)
	}
	c.mu.Unlock()
}

func (c *Collector) recordCommand(ch chan<- prometheus.Metric, r p4.CommandResult) {
	v := 0.0
	if r.OK() {
		v = 1
	}
	ch <- prometheus.MustNewConstMetric(c.cmdSuccess, prometheus.GaugeValue, v, r.Target, r.Command)
	ch <- prometheus.MustNewConstMetric(c.cmdDuration, prometheus.GaugeValue, r.DurationSeconds, r.Target, r.Command)

	switch {
	case r.MissingBinary:
		c.bumpError(r.Target, r.Command, "missing_binary")
	case r.TimedOut:
		c.bumpError(r.Target, r.Command, "timeout")
	case r.ReturnCode != 0:
		c.bumpError(r.Target, r.Command, "nonzero_exit")
	}
}

func (c *Collector) bumpError(target, command, etype string) {
	c.mu.Lock()
	c.errorCounts[errorKey{target, command, etype}]++
	c.mu.Unlock()
}
