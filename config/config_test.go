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

func TestInitConfigMissingFileFallsBackToDefaults(t *testing.T) {
	if err := InitConfig("definitely-not-exist.toml"); err != nil {
		t.Fatalf("InitConfig should not error on missing file, got: %v", err)
	}
	if Cfg.App.Name != "SG Scout" {
		t.Fatalf("defaults should survive, got app name: %q", Cfg.App.Name)
	}
}
