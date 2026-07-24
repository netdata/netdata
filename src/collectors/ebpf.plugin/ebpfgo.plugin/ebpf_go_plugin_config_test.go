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
	}{
		"ebpf type format legacy forces tracing flavor": {
			content:    "[global]\nebpf type format = legacy\n",
			wantFlavor: stringPtr("tracing"),
		},
		"ebpf type format auto leaves flavor unchanged": {
			content: "[global]\nebpf type format = auto\n",
		},
		"ebpf type format co-re leaves flavor unchanged": {
			content: "[global]\nebpf type format = co-re\n",
		},
		"ebpf co-re tracing probe forces tracing flavor": {
			content:    "[global]\nebpf co-re tracing = probe\n",
			wantFlavor: stringPtr("tracing"),
		},
		"ebpf co-re tracing trampoline leaves flavor unchanged": {
			content: "[global]\nebpf co-re tracing = trampoline\n",
		},
		"ebpf object flavor legacy maps to tracing flavor": {
			content:    "[global]\nebpf object flavor = legacy\n",
			wantFlavor: stringPtr("tracing"),
		},
		"ebpf object flavor buffer ring maps to buffer flavor": {
			content:    "[global]\nebpf object flavor = buffer ring\n",
			wantFlavor: stringPtr("buffer"),
		},
		"ebpf object flavor ring buffer maps to buffer flavor": {
			content:    "[global]\nebpf object flavor = ring-buffer\n",
			wantFlavor: stringPtr("buffer"),
		},
		"collect pid real parent sets level 0": {
			content:      "[global]\ncollect pid = real parent\n",
			wantPidLevel: intPtr(0),
		},
		"collect pid parent sets level 1": {
			content:      "[global]\ncollect pid = parent\n",
			wantPidLevel: intPtr(1),
		},
		"collect pid all sets level 2": {
			content:      "[global]\ncollect pid = all\n",
			wantPidLevel: intPtr(2),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := parseTempConfig(t, "cachestat.conf", tc.content)
			checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, tc.wantFlavor)
			checkPtr(t, "CollectPidLevel", cfg.CollectPidLevel, tc.wantPidLevel)
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

	checkPtr(t, "UpdateEvery", cfg.UpdateEvery, intPtr(17))
	checkPtr(t, "PidTable", cfg.PidTable, uint32Ptr(4096))
	checkPtr(t, "MapsPerCore", cfg.MapsPerCore, boolPtr(false))
	checkPtr(t, "BTFPath", cfg.BTFPath, stringPtr("/tmp/btf"))
	checkPtr(t, "Lifetime", cfg.Lifetime, intPtr(123))
	checkPtr(t, "ObjectFlavor", cfg.ObjectFlavor, stringPtr("arena"))
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
			wantSocket: boolPtr(true),
		},
		"socket no": {
			content:    "[ebpf programs]\nsocket = no\n",
			wantSocket: boolPtr(false),
		},
		"cachestat yes socket no": {
			content:       "[ebpf programs]\ncachestat = yes\nsocket = no\n",
			wantCachestat: boolPtr(true),
			wantSocket:    boolPtr(false),
		},
		"cachestat no socket yes": {
			content:       "[ebpf programs]\ncachestat = no\nsocket = yes\n",
			wantCachestat: boolPtr(false),
			wantSocket:    boolPtr(true),
		},
		"socket absent": {
			content:       "[ebpf programs]\ncachestat = yes\n",
			wantCachestat: boolPtr(true),
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
