package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempConfig writes content to a named temp file and returns its path.
func writeTempConfig(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// parseTempConfig writes content to a named temp file, parses it, and fails
// the test on any error or missing-file result.
func parseTempConfig(t *testing.T, filename, content string) pluginConfigFile {
	t.Helper()
	path := writeTempConfig(t, filename, content)
	cfg, ok, err := parsePluginConfigFile(path)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if !ok {
		t.Fatal("expected file to be detected as found")
	}
	return cfg
}

// checkPtr asserts that got matches want: both nil, or both non-nil and equal.
func checkPtr[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s should be nil, got %v", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s = %v, want %v", name, got, *want)
	}
}

func TestParsePluginConfigFileLegacyKeys(t *testing.T) {
	// Keys present in the stock cachestat.conf (old C plugin format) must be
	// recognised and mapped to Go-plugin equivalents.
	tests := map[string]struct {
		content      string
		wantFlavor   *string
		wantPidLevel *int
		wantLoad     *LoadMethod
	}{
		"ebpf type format legacy forces legacy load": {
			content:    "[global]\nebpf type format = legacy\n",
			wantFlavor: new("tracing"),
			wantLoad:   new(LoadLegacy),
		},
		// `auto` and `co-re` emit a BLANK flavor, not a nil one.  Blank is the
		// retraction marker: apply() merges each field independently, so without
		// it a `tracing` flavor set by `legacy` in an earlier config layer would
		// survive the override and keep forcing the base object.
		// applyCommonCollectorConfig skips a blank flavor, so the collector
		// default applies.
		"ebpf type format auto retracts any earlier legacy flavor": {
			content:    "[global]\nebpf type format = auto\n",
			wantFlavor: new(""),
			wantLoad:   new(LoadPlayDice),
		},
		"ebpf type format co-re forces core load and retracts the legacy flavor": {
			content:    "[global]\nebpf type format = co-re\n",
			wantFlavor: new(""),
			wantLoad:   new(LoadCore),
		},
		// The merge order that motivated the retraction: legacy first, auto after.
		"ebpf type format legacy then auto ends blank": {
			content:    "[global]\nebpf type format = legacy\nebpf type format = auto\n",
			wantFlavor: new(""),
			wantLoad:   new(LoadPlayDice),
		},
		"explicit object flavor survives later auto in the same file": {
			content:    "[global]\nebpf object flavor = arena\nebpf type format = auto\n",
			wantFlavor: new("arena"),
			wantLoad:   new(LoadPlayDice),
		},
		"ebpf co-re tracing probe forces tracing flavor": {
			content:    "[global]\nebpf co-re tracing = probe\n",
			wantFlavor: new("tracing"),
		},
		"ebpf co-re tracing trampoline leaves flavor unchanged": {
			content: "[global]\nebpf co-re tracing = trampoline\n",
		},
		"ebpf object flavor legacy maps to tracing flavor": {
			content:    "[global]\nebpf object flavor = legacy\n",
			wantFlavor: new("tracing"),
		},
		"ebpf load mode entry is accepted as no-op": {
			content: "[global]\nebpf load mode = entry\n",
		},
		"ebpf load mode return is accepted by the generic parser": {
			content: "[global]\nebpf load mode = return\n",
		},
		"ebpf object flavor buffer ring maps to buffer flavor": {
			content:    "[global]\nebpf object flavor = buffer ring\n",
			wantFlavor: new("buffer"),
		},
		"ebpf object flavor ring buffer maps to buffer flavor": {
			content:    "[global]\nebpf object flavor = ring-buffer\n",
			wantFlavor: new("buffer"),
		},
		"type format legacy sets the load method and leaves the object flavor alone": {
			content:    "[global]\nebpf type format = legacy\nebpf object flavor = buffer\n",
			wantFlavor: new("buffer"),
			wantLoad:   new(LoadLegacy),
		},
		"collect pid real parent sets level 0": {
			content:      "[global]\ncollect pid = real parent\n",
			wantPidLevel: new(0),
		},
		"collect pid parent sets level 1": {
			content:      "[global]\ncollect pid = parent\n",
			wantPidLevel: new(1),
		},
		"collect pid all sets level 2": {
			content:      "[global]\ncollect pid = all\n",
			wantPidLevel: new(2),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := parseTempConfig(t, "cachestat.conf", tc.content)
			checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, tc.wantFlavor)
			checkPtr(t, "CollectPidLevel", cfg.CollectPidLevel, tc.wantPidLevel)
			checkPtr(t, "LoadMethod", cfg.LoadMethod, tc.wantLoad)
		})
	}
}

func TestParsePluginConfigFile(t *testing.T) {
	cfg := parseTempConfig(t, "ebpf.d.conf", `
[global]
    update every = 17
    pid table size = 4096
    maps per core = no
    btf path = /tmp/btf
    lifetime = 123
    ebpf object flavor = arena
`)

	checkPtr(t, "UpdateEvery", cfg.UpdateEvery, new(17))
	checkPtr(t, "PidTable", cfg.PidTable, new(uint32(4096)))
	checkPtr(t, "MapsPerCore", cfg.MapsPerCore, new(false))
	checkPtr(t, "BTFPath", cfg.BTFPath, new("/tmp/btf"))
	checkPtr(t, "Lifetime", cfg.Lifetime, new(123))
	checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, new("arena"))
}

// TestParsePluginConfigFileInvalidValuesAreIgnored verifies that unrecognized
// or malformed values for recognized config keys are silently skipped (warning
// to stderr) rather than returning an error.  This preserves the lenient
// contract of ebpf.d.conf, shared with the legacy C ebpf.plugin: a single
// typo must not crash-loop the whole Go plugin via os.Exit(1) in main.
func TestParsePluginConfigFileInvalidValuesAreIgnored(t *testing.T) {
	tests := map[string]struct {
		content string
		check   func(t *testing.T, cfg pluginConfigFile)
	}{
		"invalid collect pid leaves CollectPidLevel nil": {
			content: "[global]\ncollect pid = invalid\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "CollectPidLevel", cfg.CollectPidLevel, (*int)(nil))
			},
		},
		"invalid ebpf object flavor leaves ObjectFlavor nil": {
			content: "[global]\nebpf object flavor = bufffer\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, (*string)(nil))
			},
		},
		"invalid update every leaves UpdateEvery nil": {
			content: "[global]\nupdate every = abc\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "UpdateEvery", cfg.UpdateEvery, (*int)(nil))
			},
		},
		"invalid maps per core leaves MapsPerCore nil": {
			content: "[global]\nmaps per core = maybe\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "MapsPerCore", cfg.MapsPerCore, (*bool)(nil))
			},
		},
		"invalid cachestat in ebpf programs leaves Cachestat nil": {
			content: "[ebpf programs]\ncachestat = maybe\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "Cachestat", cfg.Cachestat, (*bool)(nil))
			},
		},
		"unrecognized ebpf type format leaves ObjectFlavor nil": {
			content: "[global]\nebpf type format = bufffer\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, (*string)(nil))
			},
		},
		"unrecognized ebpf co-re tracing leaves ObjectFlavor nil": {
			content: "[global]\nebpf co-re tracing = unknown\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, (*string)(nil))
			},
		},
		"invalid socket monitoring table size leaves field nil": {
			content: "[global]\nsocket monitoring table size = abc\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "SocketMonitoringTableSize", cfg.SocketMonitoringTableSize, (*uint32)(nil))
			},
		},
		"invalid udp connection table size leaves field nil": {
			content: "[global]\nudp connection table size = abc\n",
			check: func(t *testing.T, cfg pluginConfigFile) {
				checkPtr(t, "UDPConnectionTableSize", cfg.UDPConnectionTableSize, (*uint32)(nil))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeTempConfig(t, "ebpf.d.conf", tc.content)
			cfg, ok, err := parsePluginConfigFile(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The key was recognized so the file is marked found, even though
			// the value was invalid and the field stays nil.
			if !ok {
				t.Fatal("file should be marked found when a recognized key is present")
			}
			tc.check(t, cfg)
		})
	}
}

func TestParsePluginConfigFileEbpfPrograms(t *testing.T) {
	tests := map[string]struct {
		content       string
		wantCachestat *bool
		wantSocket    *bool
	}{
		"socket yes": {
			content:    "[ebpf programs]\nsocket = yes\n",
			wantSocket: new(true),
		},
		"socket no": {
			content:    "[ebpf programs]\nsocket = no\n",
			wantSocket: new(false),
		},
		"cachestat yes socket no": {
			content:       "[ebpf programs]\ncachestat = yes\nsocket = no\n",
			wantCachestat: new(true),
			wantSocket:    new(false),
		},
		"cachestat no socket yes": {
			content:       "[ebpf programs]\ncachestat = no\nsocket = yes\n",
			wantCachestat: new(false),
			wantSocket:    new(true),
		},
		"socket absent": {
			content:       "[ebpf programs]\ncachestat = yes\n",
			wantCachestat: new(true),
			wantSocket:    nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := parseTempConfig(t, "ebpf.d.conf", tc.content)
			checkPtr(t, "Socket", cfg.Socket, tc.wantSocket)
			checkPtr(t, "Cachestat", cfg.Cachestat, tc.wantCachestat)
		})
	}
}

func TestApplySocketTableSizeClamp(t *testing.T) {
	tests := map[string]struct {
		value uint32
		max   uint32
		want  uint32
	}{
		"under max unchanged": {value: 1024, max: 4096, want: 1024},
		"equal max unchanged": {value: 4096, max: 4096, want: 4096},
		"over max clamped":    {value: 1 << 30, max: 65536, want: 65536},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := applySocketTableSizeClamp(tc.value, tc.max, "test"); got != tc.want {
				t.Fatalf("applySocketTableSizeClamp(%d, %d) = %d, want %d", tc.value, tc.max, got, tc.want)
			}
		})
	}
}

// TestApplyCommonCollectorConfigLegacyReachesFlavourlessCollectors pins the
// asymmetry between fd and the other collectors.
//
// `ebpf type format = legacy` emits BOTH a LoadMethod and a "tracing" flavor.
// Only fd consumes LoadMethod (it derives the base object from LoadLegacy in
// BuildFDLegacyPlan), so for fd the flavor merge must be suppressed and the
// operator's `ebpf object flavor` left intact.  cachestat, dcstat and socket
// pass no LoadMethod destination — for them the marker has to land on
// ObjectFlavor, or `legacy` selects nothing at all and silently keeps buffer.
func TestApplyCommonCollectorConfigLegacyReachesFlavourlessCollectors(t *testing.T) {
	legacy := pluginConfigFile{
		LoadMethod:   new(LoadLegacy),
		ObjectFlavor: new("tracing"),
	}

	t.Run("collector without LoadMethod gets the flavor", func(t *testing.T) {
		flavor := "buffer"
		applyCommonCollectorConfig(legacy, collectorCommonConfig{ObjectFlavor: &flavor})
		if flavor != "tracing" {
			t.Fatalf("ObjectFlavor = %q, want %q: `ebpf type format = legacy` must not be a no-op",
				flavor, "tracing")
		}
	})

	t.Run("collector with LoadMethod keeps its flavor", func(t *testing.T) {
		flavor := "buffer"
		method := LoadCore
		applyCommonCollectorConfig(legacy, collectorCommonConfig{
			ObjectFlavor: &flavor,
			LoadMethod:   &method,
		})
		if method != LoadLegacy {
			t.Fatalf("LoadMethod = %v, want %v", method, LoadLegacy)
		}
		if flavor != "buffer" {
			t.Fatalf("ObjectFlavor = %q, want %q: the load method already forces the base object, "+
				"so the operator's flavor must not be clobbered", flavor, "buffer")
		}
	})

	t.Run("blank flavor is a retraction and never applied", func(t *testing.T) {
		flavor := "arena"
		applyCommonCollectorConfig(
			pluginConfigFile{LoadMethod: new(LoadPlayDice), ObjectFlavor: new("")},
			collectorCommonConfig{ObjectFlavor: &flavor})
		if flavor != "arena" {
			t.Fatalf("ObjectFlavor = %q, want %q: a blank flavor must leave the destination alone",
				flavor, "arena")
		}
	})
}
