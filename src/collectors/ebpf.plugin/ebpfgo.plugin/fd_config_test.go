package main

import (
	"slices"
	"testing"
)

func TestLoadFDConfigFilesPrefersUserAndLegacyOverlay(t *testing.T) {
	writeCollectorConfigFixture(t, "fd.conf",
		`
[global]
    update every = 11
    pid table size = 2048
    maps per core = yes
    btf path = /stock/btf
    ebpf object flavor = tracing
    ebpf load mode = entry

[ebpf programs]
    fd = no
`,
		`
[global]
    pid table size = 4096
`,
		`
[global]
    update every = 23

[ebpf programs]
    fd = yes
`,
		`
[global]
    maps per core = no
    ebpf object flavor = buffer
    collect pid = all
    ebpf load mode = return
`)

	cfg, found, err := loadFDConfigFiles()
	if err != nil {
		t.Fatalf("load config files: %v", err)
	}
	if !found {
		t.Fatal("expected config files to be detected")
	}

	if cfg.Fd == nil || !*cfg.Fd {
		t.Fatalf("unexpected fd enablement: %#v", cfg.Fd)
	}
	assertMergedCommonConfig(t, cfg, mergedCommonExpect{
		UpdateEvery:     23,
		PidTable:        4096,
		MapsPerCore:     false,
		BTFPath:         "/stock/btf",
		ObjectFlavor:    "buffer",
		CollectPidLevel: 2,
	})
	// The user overlay is the last file merged, so its `return` must win over the
	// stock `entry`.
	if cfg.LoadModeReturn == nil || !*cfg.LoadModeReturn {
		t.Fatalf("unexpected load mode: %#v", cfg.LoadModeReturn)
	}
}

// TestResolveFDLegacyConfigLoadMode pins the D1 contract: `ebpf load mode`
// reaches FDLegacyConfig.ReportErrors, which gates the error charts and nothing
// else.  `entry` (the stock default) must not enable them.
func TestResolveFDLegacyConfigLoadMode(t *testing.T) {
	tests := map[string]struct {
		global string
		want   bool
	}{
		"stock entry mode": {
			global: "[global]\n    ebpf load mode = entry\n\n[ebpf programs]\n    fd = yes\n",
			want:   false,
		},
		"return mode enables the error charts": {
			global: "[global]\n    ebpf load mode = return\n\n[ebpf programs]\n    fd = yes\n",
			want:   true,
		},
		"dev mode enables the error charts": {
			global: "[global]\n    ebpf load mode = dev\n\n[ebpf programs]\n    fd = yes\n",
			want:   true,
		},
		"absent key keeps the default": {
			global: "[ebpf programs]\n    fd = yes\n",
			want:   false,
		},
		"unrecognized value keeps the default": {
			global: "[global]\n    ebpf load mode = sideways\n\n[ebpf programs]\n    fd = yes\n",
			want:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writeCollectorConfigFixture(t, "fd.conf", tc.global, "", "", "")

			cfg, err := resolveFDLegacyConfig()
			if err != nil {
				t.Fatalf("resolveFDLegacyConfig: %v", err)
			}
			if cfg.ReportErrors != tc.want {
				t.Fatalf("ReportErrors = %t, want %t", cfg.ReportErrors, tc.want)
			}
		})
	}
}

func TestResolveFDLegacyConfigTypeFormatOverridesObjectFlavor(t *testing.T) {
	// The fixture must use a NON-default flavor.  With `ebpf object flavor = buffer`
	// this assertion was vacuous: fdDefaultObjectFlavor is "buffer" too, so a
	// regression that wrongly applied the file's flavor — or the parser's "tracing"
	// marker — still produced "buffer" and the guard passed silently.
	writeCollectorConfigFixture(t, "fd.conf", `
[global]
    ebpf object flavor = arena
    ebpf type format = legacy

[ebpf programs]
    fd = yes
`, "", "", "")

	cfg, err := resolveFDLegacyConfig()
	if err != nil {
		t.Fatalf("resolveFDLegacyConfig: %v", err)
	}
	if cfg.LoadMethod != LoadLegacy {
		t.Fatalf("LoadMethod = %v, want %v", cfg.LoadMethod, LoadLegacy)
	}
	// fd consumes LoadMethod, so the flavor merge is suppressed entirely and the
	// collector default survives — neither "arena" from the file nor "tracing"
	// from the legacy marker may leak through.
	if cfg.ObjectFlavor != fdDefaultObjectFlavor {
		t.Fatalf("ObjectFlavor = %q, want the default %q unchanged when type format wins",
			cfg.ObjectFlavor, fdDefaultObjectFlavor)
	}
}

// TestResolveFDLegacyConfigTypeFormatAutoRetractsLegacy pins the retraction: a
// later config layer setting `auto` must be able to restore the normal object
// family, not stay pinned to the base object by a stale legacy marker.
func TestResolveFDLegacyConfigTypeFormatAutoRetractsLegacy(t *testing.T) {
	// The later layer sets ONLY the type format.  If it also set an object flavor
	// that value would overwrite the stale "tracing" marker on its own and the
	// test would pass even with the retraction missing.
	writeCollectorConfigFixture(t, "fd.conf",
		"[global]\n    ebpf type format = legacy\n\n[ebpf programs]\n    fd = yes\n",
		"",
		"[global]\n    ebpf type format = auto\n",
		"")

	cfg, err := resolveFDLegacyConfig()
	if err != nil {
		t.Fatalf("resolveFDLegacyConfig: %v", err)
	}
	if cfg.LoadMethod == LoadLegacy {
		t.Fatalf("LoadMethod = %v, want the legacy marker retracted by `auto`", cfg.LoadMethod)
	}
	if cfg.ObjectFlavor != fdDefaultObjectFlavor {
		t.Fatalf("ObjectFlavor = %q, want the default %q: `auto` must clear the stale legacy flavor",
			cfg.ObjectFlavor, fdDefaultObjectFlavor)
	}
}

// TestResolveFDLegacyConfigDefaults pins the stock defaults: fd is opt-in and its
// apps/cgroups integrations are off, so an operator enabling nothing pays no
// shared-memory cost.
func TestResolveFDLegacyConfigDefaults(t *testing.T) {
	useEmptyConfigRoots(t)

	cfg, err := resolveFDLegacyConfig()
	if err != nil {
		t.Fatalf("resolveFDLegacyConfig: %v", err)
	}

	if cfg.Enabled {
		t.Error("fd must be disabled by default")
	}
	if cfg.AppsEnabled || cfg.CgroupsEnabled {
		t.Error("fd apps/cgroups integration must be off by default")
	}
	if cfg.ReportErrors {
		t.Error("fd error charts must be off by default (ebpf load mode = entry)")
	}
	if cfg.UpdateEvery != fdDefaultUpdateEvery {
		t.Errorf("UpdateEvery = %d, want %d", cfg.UpdateEvery, fdDefaultUpdateEvery)
	}
	if cfg.PidTableSize != fdDefaultPIDTableSize {
		t.Errorf("PidTableSize = %d, want %d", cfg.PidTableSize, fdDefaultPIDTableSize)
	}
	if cfg.ObjectFlavor != fdDefaultObjectFlavor {
		t.Errorf("ObjectFlavor = %q, want %q", cfg.ObjectFlavor, fdDefaultObjectFlavor)
	}
	if !cfg.MapsPerCore {
		t.Error("MapsPerCore must default to true")
	}
	// Disabled means /proc/kallsyms is never read, so the targets stay empty and
	// LoadFDLegacy refuses rather than attaching to a placeholder name.
	if cfg.Targets != (FDTargets{}) {
		t.Errorf("Targets = %+v, want zero while fd is disabled", cfg.Targets)
	}
}

// TestResolveFDLegacyConfigResolvesTargetsWhenEnabled complements the test above:
// once fd is enabled the symbol table IS consulted.  It is skipped where
// /proc/kallsyms is unreadable (an unprivileged container), because the degrade
// path is covered by the fd_targets tests instead.
func TestResolveFDLegacyConfigResolvesTargetsWhenEnabled(t *testing.T) {
	writeCollectorConfigFixture(t, "fd.conf", "[ebpf programs]\n    fd = yes\n", "", "", "")

	cfg, err := resolveFDLegacyConfig()
	if err != nil {
		t.Fatalf("resolveFDLegacyConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatal("fd should be enabled by the fixture")
	}
	if cfg.Targets == (FDTargets{}) {
		t.Skip("/proc/kallsyms did not yield fd targets on this host")
	}

	if !slices.Contains(fdOpenCandidates, cfg.Targets.Open) {
		t.Errorf("Targets.Open = %q, want one of %v", cfg.Targets.Open, fdOpenCandidates)
	}
	if !slices.Contains(fdCloseCandidates, cfg.Targets.Close) {
		t.Errorf("Targets.Close = %q, want one of %v", cfg.Targets.Close, fdCloseCandidates)
	}
}
