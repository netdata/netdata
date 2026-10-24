package main

import (
	"testing"
)

func TestApplyPidTableSizeClamp(t *testing.T) {
	tests := map[string]struct {
		in   uint32
		want uint32
	}{
		"zero stays zero":       {0, 0},
		"small value unchanged": {100, 100},
		"at cap unchanged":      {cachestatMaxPIDTableSize, cachestatMaxPIDTableSize},
		"above cap clamped":     {cachestatMaxPIDTableSize + 1, cachestatMaxPIDTableSize},
		"far above cap clamped": {4194304, cachestatMaxPIDTableSize},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := applyPidTableSizeClamp(tc.in)
			if got != tc.want {
				t.Fatalf("applyPidTableSizeClamp(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoadCachestatConfigFilesPrefersUserAndLegacyOverlay(t *testing.T) {
	writeCollectorConfigFixture(t, "cachestat.conf",
		`
[global]
    update every = 11
    pid table size = 2048
    maps per core = yes
    btf path = /stock/btf
    ebpf object flavor = tracing
`,
		`
[global]
    pid table size = 4096
`,
		`
[global]
    update every = 23
`,
		`
[global]
    maps per core = no
    ebpf object flavor = buffer
`)

	cfg, found, err := loadCachestatConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if !found {
		t.Fatal("expected config files to be detected")
	}

	assertMergedCommonConfig(t, cfg, mergedCommonExpect{
		UpdateEvery:     23,
		PidTable:        4096,
		MapsPerCore:     false,
		BTFPath:         "/stock/btf",
		ObjectFlavor:    "buffer",
		CollectPidLevel: -1,
	})
}

func TestLoadCachestatConfigFilesMissingReturnsNotFound(t *testing.T) {
	useEmptyConfigRoots(t)

	cfg, found, err := loadCachestatConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if found {
		t.Fatal("expected no config files to be found")
	}
	if cfg.UpdateEvery != nil || cfg.PidTable != nil || cfg.MapsPerCore != nil || cfg.BTFPath != nil || cfg.Lifetime != nil || cfg.ObjectFlavor != nil {
		t.Fatalf("expected empty config, got %#v", cfg)
	}
}

func TestCachestatConfigAutoPreservesEarlierExplicitFlavor(t *testing.T) {
	tests := map[string]string{
		"auto":  "auto",
		"co-re": "co-re",
	}

	for name, typeFormat := range tests {
		t.Run(name, func(t *testing.T) {
			writeCollectorConfigFixture(t, "cachestat.conf",
				"[global]\n    ebpf object flavor = arena\n", "", "",
				"[global]\n    ebpf type format = "+typeFormat+"\n")

			cfg, err := resolveCachestatLegacyConfig()
			if err != nil {
				t.Fatalf("resolveCachestatLegacyConfig(): %v", err)
			}
			if cfg.ObjectFlavor != "arena" {
				t.Fatalf("ObjectFlavor = %q, want arena", cfg.ObjectFlavor)
			}
		})
	}
}
