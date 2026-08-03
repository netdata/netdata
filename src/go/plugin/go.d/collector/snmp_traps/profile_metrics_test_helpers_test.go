// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/require"
)

func writeRootTestProfileMetricProfile(t testing.TB, dir string) {
	t.Helper()
	writeProfileYAML(t, dir, "profile.yaml", `
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
`)
}

func rootTestProfileCatalogPaths(t testing.TB) catalog.Paths {
	t.Helper()
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "minimal.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"minimal": map[string]any{
			"file":      "minimal.yaml",
			"mibs":      []string{"IF-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.3"},
		},
	})
	return catalog.Paths{UserDirs: []string{userDir}, StockDir: stockDir}
}
