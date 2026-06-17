package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(map[string]string{
		"PERFORCE_TARGETS": "primary=127.0.0.1:1666,replica=127.0.0.1:1667",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenPort != 9117 || cfg.P4Bin != "p4" || cfg.P4TimeoutSeconds != 10 {
		t.Errorf("defaults wrong: %+v", cfg)
	}
	if len(cfg.Targets) != 2 ||
		cfg.Targets[0].Name != "primary" || cfg.Targets[0].Port != "127.0.0.1:1666" ||
		cfg.Targets[1].Name != "replica" || cfg.Targets[1].Port != "127.0.0.1:1667" {
		t.Errorf("targets wrong: %+v", cfg.Targets)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(map[string]string{
		"PERFORCE_TARGETS":   "only=10.0.0.1:1666",
		"EXPORTER_PORT":      "9000",
		"P4_BIN":             "/opt/p4",
		"P4_TIMEOUT_SECONDS": "5",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenPort != 9000 || cfg.P4Bin != "/opt/p4" || cfg.P4TimeoutSeconds != 5 {
		t.Errorf("overrides wrong: %+v", cfg)
	}
}

func TestLoadMissingTargets(t *testing.T) {
	if _, err := Load(map[string]string{}); err == nil {
		t.Fatal("expected error for missing PERFORCE_TARGETS")
	}
}

func TestParseTargetsRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"primary", "=1666", "primary=", ""} {
		if _, err := ParseTargets(raw); err == nil {
			t.Errorf("ParseTargets(%q) expected error", raw)
		}
	}
}

func TestParseTargetsSkipsBlanks(t *testing.T) {
	ts, err := ParseTargets("primary=127.0.0.1:1666, ,replica=127.0.0.1:1667")
	if err != nil {
		t.Fatalf("ParseTargets: %v", err)
	}
	if len(ts) != 2 {
		t.Errorf("got %d targets, want 2", len(ts))
	}
}
