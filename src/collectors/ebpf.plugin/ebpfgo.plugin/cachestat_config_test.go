package main

import (
	"os"
	"path/filepath"
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
	userRoot := t.TempDir()
	stockRoot := t.TempDir()

	t.Setenv("NETDATA_USER_CONFIG_DIR", userRoot)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", stockRoot)

	for _, rel := range []string{"ebpf.d", filepath.Join("ebpf.d", "cachestat.conf")} {
		if err := os.MkdirAll(filepath.Join(userRoot, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir user %q: %v", rel, err)
		}
		if err := os.MkdirAll(filepath.Join(stockRoot, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir stock %q: %v", rel, err)
		}
	}

	write := func(root, rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write(stockRoot, "ebpf.d.conf", `
[global]
    update every = 11
    pid table size = 2048
    maps per core = yes
    btf path = /stock/btf
    ebpf object flavor = tracing
`)
	write(stockRoot, filepath.Join("ebpf.d", "cachestat.conf"), `
[global]
    pid table size = 4096
`)
	write(userRoot, "ebpf.d.conf", `
[global]
    update every = 23
`)
	write(userRoot, filepath.Join("ebpf.d", "cachestat.conf"), `
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
}

func TestLoadCachestatConfigFilesMissingReturnsNotFound(t *testing.T) {
	t.Setenv("NETDATA_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", t.TempDir())

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
