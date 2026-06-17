// Package config loads exporter settings from the environment.
//
// PERFORCE_TARGETS is the only required variable; everything else has a
// sensible default. Keeping the loader pure (input: map[string]string) makes
// it trivial to unit test.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Target struct {
	Name string
	Port string
}

type Config struct {
	Targets          []Target
	ListenPort       int
	P4Bin            string
	P4TimeoutSeconds int
}

// ParseTargets splits a "name=host:port,name=host:port" string into Targets.
func ParseTargets(raw string) ([]Target, error) {
	var out []Target
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, port, ok := strings.Cut(item, "=")
		name = strings.TrimSpace(name)
		port = strings.TrimSpace(port)
		if !ok || name == "" || port == "" {
			return nil, fmt.Errorf("invalid target %q, expected name=host:port", item)
		}
		out = append(out, Target{Name: name, Port: port})
	}
	if len(out) == 0 {
		return nil, errors.New("PERFORCE_TARGETS must contain at least one target")
	}
	return out, nil
}

// Load reads configuration from env, falling back to os.Environ() if nil.
func Load(env map[string]string) (Config, error) {
	if env == nil {
		env = map[string]string{}
		for _, kv := range os.Environ() {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
	}

	raw := env["PERFORCE_TARGETS"]
	if raw == "" {
		return Config{}, errors.New(
			"PERFORCE_TARGETS is required, " +
				"e.g. primary=127.0.0.1:1666,replica=127.0.0.1:1667",
		)
	}
	targets, err := ParseTargets(raw)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Targets:          targets,
		ListenPort:       getInt(env, "EXPORTER_PORT", 9117),
		P4Bin:            getStr(env, "P4_BIN", "p4"),
		P4TimeoutSeconds: getInt(env, "P4_TIMEOUT_SECONDS", 10),
	}, nil
}

func getStr(env map[string]string, key, def string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return def
}

func getInt(env map[string]string, key string, def int) int {
	v, ok := env[key]
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
