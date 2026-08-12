package main

import (
	"testing"
)

func TestLoadDCStatConfigFilesPrefersUserAndLegacyOverlay(t *testing.T) {
	writeCollectorConfigFixture(t, "dcstat.conf",
		`
[global]
    update every = 11
    pid table size = 2048
    maps per core = yes
    btf path = /stock/btf
    ebpf object flavor = tracing

[ebpf programs]
    dcstat = no
`,
		`
[global]
    pid table size = 4096
`,
		`
[global]
    update every = 23

[ebpf programs]
    dcstat = yes
`,
		`
[global]
    maps per core = no
    ebpf object flavor = buffer
    collect pid = all
`)

	cfg, found, err := loadDCStatConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if !found {
		t.Fatal("expected config files to be detected")
	}

	if cfg.Dcstat == nil || !*cfg.Dcstat {
		t.Fatalf("unexpected dcstat enablement: %#v", cfg.Dcstat)
	}
	if cfg.UpdateEvery == nil || *cfg.UpdateEvery != 23 {
		t.Fatalf("unexpected update every: %#v", cfg.UpdateEvery)
	}
	if cfg.PidTable == nil || *cfg.PidTable != 4096 {
		t.Fatalf("unexpected pid table size: %#v", cfg.PidTable)
	}
	if cfg.MapsPerCore == nil || *cfg.MapsPerCore {
		t.Fatalf("unexpected maps per core: %#v", cfg.MapsPerCore)
	}
	if cfg.BTFPath == nil || *cfg.BTFPath != "/stock/btf" {
		t.Fatalf("unexpected btf path: %#v", cfg.BTFPath)
	}
	if cfg.ObjectFlavor == nil || *cfg.ObjectFlavor != "buffer" {
		t.Fatalf("unexpected object flavor: %#v", cfg.ObjectFlavor)
	}
	if cfg.CollectPidLevel == nil || *cfg.CollectPidLevel != 2 {
		t.Fatalf("unexpected collect pid level: %#v", cfg.CollectPidLevel)
	}
}

func TestLoadDCStatConfigFilesMissingReturnsNotFound(t *testing.T) {
	useEmptyConfigRoots(t)

	cfg, found, err := loadDCStatConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if found {
		t.Fatal("expected no config files to be found")
	}
	if cfg.Dcstat != nil || cfg.UpdateEvery != nil || cfg.PidTable != nil || cfg.MapsPerCore != nil ||
		cfg.BTFPath != nil || cfg.Lifetime != nil || cfg.ObjectFlavor != nil || cfg.CollectPidLevel != nil {
		t.Fatalf("expected empty config, got %#v", cfg)
	}
}

// TestDCStatDefaultsMatchStockConfig pins the two defaults that differ from
// cachestat: dcstat ships disabled in ebpf.d.conf, and its base-flavor object
// family stops at kernel 5.14.
func TestDCStatDefaultsMatchStockConfig(t *testing.T) {
	cfg := defaultDCStatLegacyConfig()

	if cfg.Enabled {
		t.Fatal("dcstat must default to disabled to match the stock ebpf.d.conf key")
	}
	if cfg.ObjectFlavor != dcstatDefaultObjectFlavor {
		t.Fatalf("default object flavor = %q, want %q", cfg.ObjectFlavor, dcstatDefaultObjectFlavor)
	}
	if cfg.UpdateEvery != dcstatDefaultUpdateEvery {
		t.Fatalf("default update every = %d, want %d", cfg.UpdateEvery, dcstatDefaultUpdateEvery)
	}
	if cfg.PidTableSize != dcstatDefaultPIDTableSize {
		t.Fatalf("default pid table size = %d, want %d", cfg.PidTableSize, dcstatDefaultPIDTableSize)
	}
	if dcstatMaxBaseSelector != 7 {
		t.Fatalf("dcstatMaxBaseSelector = %d, want 7 (5.14 is the newest base dc object)", dcstatMaxBaseSelector)
	}
}
