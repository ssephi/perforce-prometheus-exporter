// Package parsers contains pure functions that turn p4 CLI output into
// structured values. Keeping these decoupled from the collector makes them
// easy to test against captured fixture files.
package parsers

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PullLJ holds the fields parsed from `p4 pull -lj`. Any field may be unset
// (zero) — the caller checks the bool returned by Parse to know whether the
// command produced anything usable.
type PullLJ struct {
	ReplicaJournal      int64
	ReplicaSequence     int64
	MasterJournal       int64
	MasterSequence      int64
	StatefileModifiedTS float64
	ReplicaServerTimeTS float64

	HasReplicaJournal      bool
	HasReplicaSequence     bool
	HasMasterJournal       bool
	HasMasterSequence      bool
	HasStatefileModifiedTS bool
	HasReplicaServerTimeTS bool
}

type PullLS struct {
	ActiveTransfers int64
	TotalTransfers  int64
	ActiveBytes     int64
	TotalBytes      int64
	OK              bool
}

type PullL struct {
	TotalListed  int
	FailedListed int
}

// ParseInfo turns "Key: value" lines from `p4 info` into a map.
//
// Splitting on the first colon only preserves values that themselves
// contain colons, e.g. "Server address: 127.0.0.1:1666".
func ParseInfo(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

// `p4 pull -lj` prints two journal-state lines whose payload looks like
// "Journal 12, Sequence 4521". Spacing varies between server versions.
var journalStateRE = regexp.MustCompile(`(?i)Journal\s+(\d+),\s*Sequence\s+(\d+)`)

func ParsePullLJ(text string) PullLJ {
	var out PullLJ
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "current replica journal state"):
			if m := journalStateRE.FindStringSubmatch(line); m != nil {
				out.ReplicaJournal, _ = strconv.ParseInt(m[1], 10, 64)
				out.ReplicaSequence, _ = strconv.ParseInt(m[2], 10, 64)
				out.HasReplicaJournal = true
				out.HasReplicaSequence = true
			}
		case strings.HasPrefix(lower, "current master journal state"):
			if m := journalStateRE.FindStringSubmatch(line); m != nil {
				out.MasterJournal, _ = strconv.ParseInt(m[1], 10, 64)
				out.MasterSequence, _ = strconv.ParseInt(m[2], 10, 64)
				out.HasMasterJournal = true
				out.HasMasterSequence = true
			}
		case strings.HasPrefix(lower, "the statefile was last modified at"):
			if ts, ok := parseP4Timestamp(afterMarker(line, "at:")); ok {
				out.StatefileModifiedTS = ts
				out.HasStatefileModifiedTS = true
			}
		case strings.HasPrefix(lower, "the replica server time is currently"):
			if ts, ok := parseP4Timestamp(afterMarker(line, "currently:")); ok {
				out.ReplicaServerTimeTS = ts
				out.HasReplicaServerTimeTS = true
			}
		}
	}
	return out
}

// Has reports whether any field was populated. Mirrors the Python "empty
// dict" sentinel used by the collector to detect parse failure.
func (p PullLJ) Has() bool {
	return p.HasReplicaJournal || p.HasReplicaSequence ||
		p.HasMasterJournal || p.HasMasterSequence ||
		p.HasStatefileModifiedTS || p.HasReplicaServerTimeTS
}

// ParseCounters splits "name = value" lines. Values may themselves contain
// "=" (e.g. depotStats), so we split on the first "=" only.
func ParseCounters(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name == "" {
			continue
		}
		out[name] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

var leadingIntRE = regexp.MustCompile(`^\s*(\d+)\b`)

// ParseLastCheckpointAction reads the leading unix timestamp from a
// `lastCheckpointAction` value like:
//
//	"1781337732 (2026/06/13 09:02:12 +0100 BST) checkpoint completed"
func ParseLastCheckpointAction(value string) (float64, bool) {
	m := leadingIntRE.FindStringSubmatch(value)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return float64(n), true
}

// `p4 pull -ls` prints a single summary line like:
//
//	"File transfers: 0 active/0 total, bytes: 0 active/0 total."
var pullLSRE = regexp.MustCompile(
	`(?i)File transfers:\s*(\d+)\s*active\s*/\s*(\d+)\s*total` +
		`\s*,\s*bytes:\s*(\d+)\s*active\s*/\s*(\d+)\s*total`,
)

func ParsePullLS(text string) PullLS {
	m := pullLSRE.FindStringSubmatch(text)
	if m == nil {
		return PullLS{}
	}
	at, _ := strconv.ParseInt(m[1], 10, 64)
	tt, _ := strconv.ParseInt(m[2], 10, 64)
	ab, _ := strconv.ParseInt(m[3], 10, 64)
	tb, _ := strconv.ParseInt(m[4], 10, 64)
	return PullLS{
		ActiveTransfers: at,
		TotalTransfers:  tt,
		ActiveBytes:     ab,
		TotalBytes:      tb,
		OK:              true,
	}
}

// ParsePullL counts non-empty lines and flags any containing "fail"
// (case-insensitive). Heuristic — the only signal available without log
// parsing. Current-queue gauge, not a historical count.
func ParsePullL(text string) PullL {
	var out PullL
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		out.TotalListed++
		if strings.Contains(strings.ToLower(s), "fail") {
			out.FailedListed++
		}
	}
	return out
}

func afterMarker(line, marker string) string {
	idx := strings.Index(strings.ToLower(line), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(line[idx+len(marker):])
}

// parseP4Timestamp accepts either "2026/06/13 09:00:01" or
// "2026/06/13 09:00:01 +0000 UTC" and returns unix seconds. Tested shapes
// match the lab fixtures verbatim.
func parseP4Timestamp(value string) (float64, bool) {
	s := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value), "."))
	if s == "" {
		return 0, false
	}
	// Strip a trailing timezone abbreviation (e.g. "UTC") after the
	// numeric offset — Go's time.Parse with the layouts below tolerates
	// the offset but not an extra word after it.
	parts := strings.Fields(s)
	candidate := s
	if len(parts) == 4 && (strings.HasPrefix(parts[2], "+") || strings.HasPrefix(parts[2], "-")) {
		candidate = strings.Join(parts[:3], " ")
	}
	for _, layout := range []string{
		"2006/01/02 15:04:05 -0700",
		"2006/01/02 15:04:05",
	} {
		t, err := time.Parse(layout, candidate)
		if err == nil {
			return float64(t.Unix()), true
		}
	}
	return 0, false
}
