// SPDX-License-Identifier: GPL-3.0-or-later

// Package profiletest provides shared trap-profile fixtures for integration tests.
package profiletest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
)

// CatalogPaths creates a minimal usable user/stock profile tree.
func CatalogPaths(t testing.TB) catalog.Paths {
	t.Helper()
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	stockDir := filepath.Join(root, "default")
	mustMkdirAll(t, userDir)
	mustMkdirAll(t, stockDir)
	mustWrite(t, filepath.Join(stockDir, "minimal.yaml"), []byte(`
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`))
	writeCatalogue(t, stockDir, map[string]map[string]any{
		"minimal": {
			"file":      "minimal.yaml",
			"mibs":      []string{"IF-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.3"},
		},
	})
	return catalog.Paths{UserDirs: []string{userDir}, StockDir: stockDir}
}

// WriteCiscoConfigMetricProfile adds the shared Cisco profile-metric fixture.
func WriteCiscoConfigMetricProfile(t testing.TB, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "profile.yaml"), []byte(`
varbinds:
  ccmHistoryEventTerminalType:
    oid: 1.3.6.1.4.1.9.9.43.1.1.1.2
    type: INTEGER
traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    name: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    category: config_change
    severity: notice
    varbinds:
      - ccmHistoryEventTerminalType
charts:
  - id: cisco_config_changes
    title: Cisco config changes
    context: snmp.trap.cisco.config.changes
    units: events/s
    algorithm: incremental
metrics:
  - name: cisco.config.changed
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    output:
      metric: snmp_trap_cisco_config_events
      dimension: events
      chart: cisco_config_changes
`))
}

func writeCatalogue(t testing.TB, stockDir string, entries map[string]map[string]any) {
	t.Helper()
	for name, entry := range entries {
		file, _ := entry["file"].(string)
		data, err := os.ReadFile(filepath.Join(stockDir, file))
		if err != nil {
			t.Fatalf("read stock profile for catalogue entry %q: %v", name, err)
		}
		entry["sha256"] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal profile catalogue: %v", err)
	}
	mustWrite(t, filepath.Join(filepath.Dir(stockDir), "catalogue.json"), data)
}

func mustMkdirAll(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create profile fixture directory: %v", err)
	}
}

func mustWrite(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}
}
