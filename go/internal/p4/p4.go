// Package p4 is a thin wrapper around the p4 CLI. It never returns an
// error on a normal command failure — the collector turns CommandResult
// into metrics directly.
package p4

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	Target          string
	Command         string
	Args            []string
	ReturnCode      int
	Stdout          string
	Stderr          string
	DurationSeconds float64
	TimedOut        bool
	MissingBinary   bool
}

func (r CommandResult) OK() bool {
	return !r.MissingBinary && !r.TimedOut && r.ReturnCode == 0
}

type Options struct {
	P4Bin         string
	Timeout       time.Duration
	EnvOverrides  map[string]string // applied on top of the inherited env
	LookPathExtra func(name string) (string, error)
}

// Run executes `p4 <args...>` against the given P4PORT. It captures stdout
// and stderr and synthesises a CommandResult for any failure shape we
// expect to see in practice: missing binary, timeout, non-zero exit.
func Run(targetName, port string, args []string, opt Options) CommandResult {
	commandStr := strings.Join(args, " ")
	bin := opt.P4Bin
	if bin == "" {
		bin = "p4"
	}

	resolved, err := exec.LookPath(bin)
	if err != nil {
		return CommandResult{
			Target:        targetName,
			Command:       commandStr,
			Args:          append([]string(nil), args...),
			ReturnCode:    -1,
			Stderr:        bin + ": not found",
			MissingBinary: true,
		}
	}

	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, resolved, args...)
	cmd.Env = mergeEnv(os.Environ(), map[string]string{"P4PORT": port}, opt.EnvOverrides)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start).Seconds()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return CommandResult{
			Target:          targetName,
			Command:         commandStr,
			Args:            append([]string(nil), args...),
			ReturnCode:      -1,
			Stdout:          stdout.String(),
			Stderr:          stderr.String() + "\ntimeout after " + timeout.String(),
			DurationSeconds: dur,
			TimedOut:        true,
		}
	}

	rc := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			rc = ee.ExitCode()
		} else {
			rc = -1
		}
	}

	return CommandResult{
		Target:          targetName,
		Command:         commandStr,
		Args:            append([]string(nil), args...),
		ReturnCode:      rc,
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		DurationSeconds: dur,
	}
}

func mergeEnv(base []string, overlays ...map[string]string) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(base))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok {
			seen[k] = len(out)
		}
		out = append(out, kv)
	}
	for _, overlay := range overlays {
		for k, v := range overlay {
			kv := k + "=" + v
			if idx, ok := seen[k]; ok {
				out[idx] = kv
			} else {
				seen[k] = len(out)
				out = append(out, kv)
			}
		}
	}
	return out
}
