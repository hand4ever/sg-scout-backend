package settings

import (
	"testing"

	"sg.scout/config"
)

// restoreCfg resets the global config to defaults after each test.
func restoreCfg(t *testing.T) {
	t.Cleanup(func() {
		config.Cfg = config.Default()
	})
}

func TestRegistryComplete(t *testing.T) {
	keys := RegisteredKeys()
	want := []string{KeyDefaultEngine, KeySchedulerConcurrency, KeyStorageRoot}
	if len(keys) != len(want) {
		t.Fatalf("registered keys = %v, want %v", keys, want)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("key order mismatch at %d: %s vs %s", i, keys[i], k)
		}
		d, ok := Lookup(k)
		if !ok {
			t.Fatalf("Lookup(%s) missing", k)
		}
		if d.Default == nil || d.Effect == "" {
			t.Fatalf("key %s: default/effect incomplete: %+v", k, d)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("Lookup(nope) should be false")
	}
}

func TestSeedValueFromConfig(t *testing.T) {
	restoreCfg(t)
	// Default engine: empty TOML provider -> goquery code default.
	config.Cfg.Crawler.Engine.Provider = ""
	if v := seedValue(def(KeyDefaultEngine)); v != "goquery" {
		t.Fatalf("seed default_engine = %v, want goquery", v)
	}
	// Legacy TOML provider seeds the stored default (data-model §3).
	config.Cfg.Crawler.Engine.Provider = "firecrawl"
	if v := seedValue(def(KeyDefaultEngine)); v != "firecrawl" {
		t.Fatalf("seed default_engine = %v, want firecrawl (legacy respect)", v)
	}
	// Concurrency/storage fall back to config then code defaults.
	config.Cfg.Crawler.Concurrency = 3
	if v := seedValue(def(KeySchedulerConcurrency)); v != 3 {
		t.Fatalf("seed concurrency = %v, want 3", v)
	}
	config.Cfg.Crawler.Concurrency = 0
	if v := seedValue(def(KeySchedulerConcurrency)); v != 1 {
		t.Fatalf("seed concurrency fallback = %v, want 1", v)
	}
}

func def(key string) Def {
	d, _ := Lookup(key)
	return d
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   any
		wantErr bool
	}{
		{"unknown key", "nope", "x", true},
		{"default_engine ok", KeyDefaultEngine, "goquery", false},
		{"default_engine unknown", KeyDefaultEngine, "bogus", true},
		{"default_engine empty", KeyDefaultEngine, "", true},
		{"concurrency ok", KeySchedulerConcurrency, 2, false},
		{"concurrency zero", KeySchedulerConcurrency, 0, true},
		{"concurrency float(JSON)", KeySchedulerConcurrency, float64(3), false},
		{"concurrency string type", KeySchedulerConcurrency, "3", true},
		{"storage ok", KeyStorageRoot, "./data", false},
	}
	for _, c := range cases {
		err := Validate(c.key, c.value)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: Validate(%s, %v) err=%v, wantErr=%v", c.name, c.key, c.value, err, c.wantErr)
		}
	}
}
