package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCollectorConfigFixture points the config loader at two temporary roots and
// writes the four files a collector merges, in load order:
// stock ebpf.d.conf, stock overlay, user ebpf.d.conf, user overlay.
//
// An empty string skips that file, so a test can exercise any subset of the
// merge chain.  legacyName is the collector's overlay, e.g. "dcstat.conf".
func writeCollectorConfigFixture(t *testing.T, legacyName, stockGlobal, stockOverlay, userGlobal, userOverlay string) {
	t.Helper()

	userRoot := t.TempDir()
	stockRoot := t.TempDir()

	t.Setenv("NETDATA_USER_CONFIG_DIR", userRoot)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", stockRoot)

	for _, root := range []string{userRoot, stockRoot} {
		if err := os.MkdirAll(filepath.Join(root, "ebpf.d"), 0o755); err != nil {
			t.Fatalf("mkdir %s/ebpf.d: %v", root, err)
		}
	}

	write := func(root, rel, content string) {
		if content == "" {
			return
		}
		path := filepath.Join(root, rel)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	overlay := filepath.Join("ebpf.d", legacyName)
	write(stockRoot, pluginPrimaryConfigFile, stockGlobal)
	write(stockRoot, overlay, stockOverlay)
	write(userRoot, pluginPrimaryConfigFile, userGlobal)
	write(userRoot, overlay, userOverlay)
}

// useEmptyConfigRoots points the loader at two empty directories, so every
// collector default must survive untouched.
func useEmptyConfigRoots(t *testing.T) {
	t.Helper()

	t.Setenv("NETDATA_USER_CONFIG_DIR", t.TempDir())
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", t.TempDir())
}

// TestResolveUpdateEvery pins the precedence every module's startup path depends
// on: config wins over the legacy numeric CLI argument, which wins over the
// module default. All four collectors route through this one function
// (main.go:113,137,169,206), so a silent inversion would change every module's
// interval at once.
func TestResolveUpdateEvery(t *testing.T) {
	const fallback = 10

	tests := map[string]struct {
		cliArg, cfgVal, want int
	}{
		"config wins over CLI":            {cliArg: 5, cfgVal: 7, want: 7},
		"CLI used when config unset":      {cliArg: 5, cfgVal: 0, want: 5},
		"fallback when neither is set":    {cliArg: 0, cfgVal: 0, want: fallback},
		"config wins even when lower":     {cliArg: 60, cfgVal: 1, want: 1},
		"negative config is not positive": {cliArg: 5, cfgVal: -1, want: 5},
		"negative CLI falls through":      {cliArg: -1, cfgVal: 0, want: fallback},
		"both negative yields fallback":   {cliArg: -3, cfgVal: -3, want: fallback},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolveUpdateEvery(tc.cliArg, tc.cfgVal, fallback); got != tc.want {
				t.Fatalf("resolveUpdateEvery(cli=%d, cfg=%d, fallback=%d) = %d, want %d",
					tc.cliArg, tc.cfgVal, fallback, got, tc.want)
			}
		})
	}
}
