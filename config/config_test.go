package config

import "testing"

func TestDefaultConfig(t *testing.T) {
	if Cfg.App.Name != "SG Scout" {
		t.Fatalf("unexpected default app name: %q", Cfg.App.Name)
	}
	if Cfg.Server.Port != ":1324" {
		t.Fatalf("unexpected default port: %q", Cfg.Server.Port)
	}
}

func TestInitConfigMissingFileFailsFast(t *testing.T) {
	// Constitution VI: a missing config file MUST fail loudly (main.go exits),
	// never silently fall back to defaults.
	if err := InitConfig("definitely-not-exist.toml"); err == nil {
		t.Fatal("InitConfig MUST error on missing file (constitution VI: fail fast)")
	}
	if Cfg.App.Name != "SG Scout" {
		t.Fatalf("defaults should survive pre-load, got app name: %q", Cfg.App.Name)
	}
}
