// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfileCatalog_LoadsStockProfiles(t *testing.T) {
	catalog, err := azureprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	for _, name := range []string{
		"Azure SQL Managed Instance",
		"Azure SQL Database",
		"Azure PostgreSQL Flexible Server",
		"Azure Cosmos DB Account",
		"Azure Logic Apps Workflow",
		"Azure Virtual Machine",
		"Azure Kubernetes Service Cluster",
		"Azure Storage Account",
		"Azure Load Balancer",
	} {
		profiles, err := catalog.ResolveNames([]string{name})
		require.NoErrorf(t, err, "expected stock profile %q", name)
		assert.Len(t, profiles, 1)
	}
	assert.GreaterOrEqual(t, len(catalog.DefaultProfileIDs()), 9)
}

func TestLoadProfileCatalogFromDirs_UserOverridesStock(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(userDir, "sql_database.yaml"), `
id: sql_database
name: Azure SQL Database (User Override)
resource_type: Microsoft.Sql/servers/databases
metrics:
  - id: cpu_percent
    azure_name: cpu_percent
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure SQL Database (User Override)
  context_namespace: sql_database
  charts:
    - id: am_test_sql_database_cpu
      title: Azure SQL Database CPU
      context: cpu
      family: Utilization
      type: line
      units: percentage
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: sql_database.cpu_percent_average
          name: average
`))
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
id: sql_database
name: Azure SQL Database (Stock)
resource_type: Microsoft.Sql/servers/databases
metrics:
  - id: cpu_percent
    azure_name: cpu_percent
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure SQL Database (Stock)
  context_namespace: sql_database
  charts:
    - id: am_test_sql_database_cpu
      title: Azure SQL Database CPU
      context: cpu
      family: Utilization
      type: line
      units: percentage
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: sql_database.cpu_percent_average
          name: average
`))
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "postgres_flexible.yaml"), `
id: postgres_flexible
name: Azure PostgreSQL Flexible Server
resource_type: Microsoft.DBforPostgreSQL/flexibleServers
metrics:
  - id: cpu_percent
    azure_name: cpu_percent
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure PostgreSQL Flexible Server
  context_namespace: postgres_flexible
  charts:
    - id: am_test_postgres_cpu
      title: Azure PostgreSQL CPU
      context: cpu
      family: Utilization
      type: line
      units: percentage
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: postgres_flexible.cpu_percent_average
          name: average
`))

	catalog, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	gotProfiles, err := catalog.ResolveNames([]string{"azure sql database (user override)"})
	require.NoError(t, err)
	require.Len(t, gotProfiles, 1)
	got := gotProfiles[0]
	assert.Equal(t, "Azure SQL Database (User Override)", got.Name)

	defaults := catalog.DefaultProfileIDs()
	assert.Equal(t, []string{"postgres_flexible", "sql_database"}, defaults)
}

func TestLoadProfileCatalogFromDirs_RejectsDuplicateProfileNames(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
id: sql_database
name: Azure SQL Database
resource_type: Microsoft.Sql/servers/databases
metrics:
  - id: cpu_percent
    azure_name: cpu_percent
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure SQL Database
  context_namespace: sql_database
  charts:
    - id: am_test_sql_database_cpu
      title: Azure SQL Database CPU
      context: cpu
      family: Utilization
      type: line
      units: percentage
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: sql_database.cpu_percent_average
          name: average
`))
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database_copy.yaml"), `
id: sql_database_copy
name: azure sql database
resource_type: Microsoft.Sql/servers/databases
metrics:
  - id: cpu_percent
    azure_name: cpu_percent
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure SQL Database Copy
  context_namespace: sql_database_copy
  charts:
    - id: am_test_sql_database_copy_cpu
      title: Azure SQL Database Copy CPU
      context: cpu
      family: Utilization
      type: line
      units: percentage
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: sql_database_copy.cpu_percent_average
          name: average
`))

	_, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate profile name`)
	assert.Contains(t, err.Error(), `"sql_database_copy"`)
	assert.Contains(t, err.Error(), `"sql_database"`)
}

func writeProfileFile(path, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func TestLoadProfileCatalogFromDirs_RejectsLegacyProfileWithoutCharts(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "legacy.yaml"), `
id: legacy
name: Azure Legacy
resource_type: Microsoft.Storage/storageAccounts
metrics:
  - id: used_capacity
    azure_name: UsedCapacity
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
`))

	_, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'template' must contain at least one chart")
}
