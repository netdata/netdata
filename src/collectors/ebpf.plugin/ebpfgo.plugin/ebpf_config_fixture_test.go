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
