// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfileCatalog_LoadsStockProfiles(t *testing.T) {
	catalog, err := loadProfileCatalog()
	require.NoError(t, err)

	for _, key := range []string{
		"sql_managed_instance",
		"sql_database",
		"postgres_flexible",
		"cosmos_db",
		"logic_apps",
		"virtual_machines",
		"aks",
		"storage_accounts",
		"load_balancers",
	} {
		_, ok := catalog.byName[key]
		assert.Truef(t, ok, "expected stock profile %q", key)
	}
	assert.GreaterOrEqual(t, len(catalog.stockProfileSet), 9)
}

func TestLoadProfileCatalogFromDirs_UserOverridesStock(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(userDir, "sql_database.yaml"), `
name: Azure SQL Database (User Override)
resource_type: Microsoft.Sql/servers/databases
metrics:
  - name: cpu_percent
    aggregations: [average]
    time_grain: PT1M
`))
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
name: Azure SQL Database (Stock)
resource_type: Microsoft.Sql/servers/databases
metrics:
  - name: cpu_percent
    aggregations: [average]
    time_grain: PT1M
`))
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "postgres_flexible.yaml"), `
name: Azure PostgreSQL Flexible Server
resource_type: Microsoft.DBforPostgreSQL/flexibleServers
metrics:
  - name: cpu_percent
    aggregations: [average]
    time_grain: PT1M
`))

	catalog, err := loadProfileCatalogFromDirs([]profileDirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	got, ok := catalog.byName["sql_database"]
	require.True(t, ok)
	assert.Equal(t, "Azure SQL Database (User Override)", got.Name)

	defaults := catalog.defaultProfileNames()
	assert.Equal(t, []string{"postgres_flexible", "sql_database"}, defaults)
}

func writeProfileFile(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}
