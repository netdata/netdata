// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/require"
)

var currentTestCatalogManager *catalog.Manager

func setTestDirs(t *testing.T, dirs ...string) {
	t.Helper()
	paths := catalog.Paths{}
	if len(dirs) > 0 {
		paths.UserDirs = append([]string(nil), dirs[:len(dirs)-1]...)
		paths.StockDir = dirs[len(dirs)-1]
	}
	currentTestCatalogManager = catalog.NewManager(paths)
	t.Cleanup(func() { currentTestCatalogManager = nil })
}

func setMinimalProfileDir(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	const oid = "1.3.6.1.6.3.1.1.5.1"
	writeProfileYAML(t, stockDir, "minimal.yaml", fmt.Sprintf(`
traps:
  - oid: %s
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: notice
`, oid))
	manifest := map[string]any{
		"minimal": map[string]any{
			"file":      "minimal.yaml",
			"mibs":      []string{"SNMPv2-MIB"},
			"trap_oids": []string{oid},
		},
	}
	writeProfileCatalogue(t, stockDir, manifest)
	currentTestCatalogManager = catalog.NewManager(catalog.Paths{StockDir: stockDir})
	t.Cleanup(func() { currentTestCatalogManager = nil })
}

func writeProfileYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func writeProfileCatalogue(t *testing.T, stockDir string, manifest any) {
	t.Helper()
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	var entries map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &entries))
	for owner, entry := range entries {
		file, _ := entry["file"].(string)
		profileData, err := os.ReadFile(filepath.Join(stockDir, file))
		require.NoError(t, err, "read stock profile for catalogue entry %q", owner)
		entry["sha256"] = fmt.Sprintf("%x", sha256.Sum256(profileData))
	}
	data, err = json.Marshal(entries)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(stockDir), "catalogue.json"), data, 0o644))
}
