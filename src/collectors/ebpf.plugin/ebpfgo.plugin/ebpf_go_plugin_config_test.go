package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePluginConfigFileLegacyKeys(t *testing.T) {
	// Keys present in the stock cachestat.conf (old C plugin format) must be
	// recognised and mapped to Go-plugin equivalents.
	tests := map[string]struct {
		content      string
		wantFlavor   string // "" means "don't care / unchanged"
		wantPidLevel int    // -1 means "not set"
	}{
		"ebpf type format legacy forces tracing flavor": {
			content:      "[global]\nebpf type format = legacy\n",
			wantFlavor:   "tracing",
			wantPidLevel: -1,
		},
		"ebpf type format auto leaves flavor unchanged": {
			content:      "[global]\nebpf type format = auto\n",
			wantFlavor:   "",
			wantPidLevel: -1,
		},
		"ebpf type format co-re leaves flavor unchanged": {
			content:      "[global]\nebpf type format = co-re\n",
			wantFlavor:   "",
			wantPidLevel: -1,
		},
		"ebpf co-re tracing probe forces tracing flavor": {
			content:      "[global]\nebpf co-re tracing = probe\n",
			wantFlavor:   "tracing",
			wantPidLevel: -1,
		},
		"ebpf co-re tracing trampoline leaves flavor unchanged": {
			content:      "[global]\nebpf co-re tracing = trampoline\n",
			wantFlavor:   "",
			wantPidLevel: -1,
		},
		"collect pid real parent sets level 0": {
			content:      "[global]\ncollect pid = real parent\n",
			wantFlavor:   "",
			wantPidLevel: 0,
		},
		"collect pid parent sets level 1": {
			content:      "[global]\ncollect pid = parent\n",
			wantFlavor:   "",
			wantPidLevel: 1,
		},
		"collect pid all sets level 2": {
			content:      "[global]\ncollect pid = all\n",
			wantFlavor:   "",
			wantPidLevel: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "cachestat.conf")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, ok, err := parsePluginConfigFile(path)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if !ok {
				t.Fatal("expected file to be detected as found")
			}

			if tc.wantFlavor != "" {
				if cfg.ObjectFlavor == nil || *cfg.ObjectFlavor != tc.wantFlavor {
					t.Fatalf("ObjectFlavor = %v, want %q", cfg.ObjectFlavor, tc.wantFlavor)
				}
			} else if cfg.ObjectFlavor != nil {
				t.Fatalf("ObjectFlavor should be nil, got %q", *cfg.ObjectFlavor)
			}

			if tc.wantPidLevel >= 0 {
				if cfg.CollectPidLevel == nil || *cfg.CollectPidLevel != tc.wantPidLevel {
					t.Fatalf("CollectPidLevel = %v, want %d", cfg.CollectPidLevel, tc.wantPidLevel)
				}
			} else if cfg.CollectPidLevel != nil {
				t.Fatalf("CollectPidLevel should be nil, got %d", *cfg.CollectPidLevel)
			}
		})
	}
}

func TestParsePluginConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ebpf.d.conf")

	content := []byte(`
[global]
    update every = 17
    pid table size = 4096
    maps per core = no
    btf path = /tmp/btf
    lifetime = 123
    ebpf object flavor = arena
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, ok, err := parsePluginConfigFile(path)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if !ok {
		t.Fatal("expected config to be detected")
	}

	if cfg.UpdateEvery == nil || *cfg.UpdateEvery != 17 {
		t.Fatalf("unexpected update every: %#v", cfg.UpdateEvery)
	}
	if cfg.PidTable == nil || *cfg.PidTable != 4096 {
		t.Fatalf("unexpected pid table size: %#v", cfg.PidTable)
	}
	if cfg.MapsPerCore == nil || *cfg.MapsPerCore {
		t.Fatalf("unexpected maps per core: %#v", cfg.MapsPerCore)
	}
	if cfg.BTFPath == nil || *cfg.BTFPath != "/tmp/btf" {
		t.Fatalf("unexpected btf path: %#v", cfg.BTFPath)
	}
	if cfg.Lifetime == nil || *cfg.Lifetime != 123 {
		t.Fatalf("unexpected lifetime: %#v", cfg.Lifetime)
	}
	if cfg.ObjectFlavor == nil || *cfg.ObjectFlavor != "arena" {
		t.Fatalf("unexpected object flavor: %#v", cfg.ObjectFlavor)
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
			dir := t.TempDir()
			path := filepath.Join(dir, "ebpf.d.conf")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, ok, err := parsePluginConfigFile(path)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if !ok {
				t.Fatal("expected file to be detected as found")
			}

			if tc.wantSocket == nil {
				if cfg.Socket != nil {
					t.Fatalf("Socket should be nil, got %v", *cfg.Socket)
				}
			} else {
				if cfg.Socket == nil || *cfg.Socket != *tc.wantSocket {
					t.Fatalf("Socket = %v, want %v", cfg.Socket, *tc.wantSocket)
				}
			}

			if tc.wantCachestat == nil {
				if cfg.Cachestat != nil {
					t.Fatalf("Cachestat should be nil, got %v", *cfg.Cachestat)
				}
			} else {
				if cfg.Cachestat == nil || *cfg.Cachestat != *tc.wantCachestat {
					t.Fatalf("Cachestat = %v, want %v", cfg.Cachestat, *tc.wantCachestat)
				}
			}
		})
	}
}
