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

	for _, baseName := range []string{
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
		profiles, err := catalog.ResolveBaseNames([]string{baseName})
		require.NoErrorf(t, err, "expected stock profile %q", baseName)
		assert.Len(t, profiles, 1)
	}
	assert.GreaterOrEqual(t, len(catalog.ResourceTypes()), 9)
}

func TestLoadProfileCatalogFromDirs_UserOverridesStock(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(userDir, "sql_database.yaml"), `
display_name: Azure SQL Database (User Override)
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
display_name: Azure SQL Database (Stock)
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
display_name: Azure PostgreSQL Flexible Server
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

	gotProfiles, err := catalog.ResolveBaseNames([]string{"sql_database"})
	require.NoError(t, err)
	require.Len(t, gotProfiles, 1)
	got := gotProfiles[0]
	assert.Equal(t, "Azure SQL Database (User Override)", got.DisplayName)
}

func TestLoadProfileCatalogFromDirs_RejectsDuplicateProfileBasenames(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
display_name: Azure SQL Database
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
	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "nested", "sql_database.yml"), `
display_name: azure sql database
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
        - selector: sql_database.cpu_percent_average
          name: average
`))

	_, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate stock profile basename`)
	assert.Contains(t, err.Error(), `"sql_database"`)
}

func TestLoadProfileCatalogFromDirs_RejectsInvalidProfileBasename(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "SQL_DATABASE.yaml"), `
display_name: Azure SQL Database Copy
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
      context: cpu_copy
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

	_, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `basename must match`)
	assert.Contains(t, err.Error(), `SQL_DATABASE.yaml`)
}

func TestLoadProfileCatalogFromDirs_NormalizesLocalSelectorShorthand(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
display_name: Azure SQL Database
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
        - selector: cpu_percent_average
          name: average
`))

	catalog, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	profiles, err := catalog.ResolveBaseNames([]string{"sql_database"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].Template.Charts, 1)
	require.Len(t, profiles[0].Template.Charts[0].Dimensions, 1)
	assert.Equal(t, "sql_database.cpu_percent_average", profiles[0].Template.Charts[0].Dimensions[0].Selector)
}

func TestLoadProfileCatalogFromDirs_RejectsUnknownSelectorShorthand(t *testing.T) {
	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	require.NoError(t, writeProfileFile(filepath.Join(stockDir, "sql_database.yaml"), `
display_name: Azure SQL Database
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
        - selector: missing_average
          name: average
`))

	_, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{
		{Path: stockDir, IsStock: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template")
	assert.Contains(t, err.Error(), "missing_average")
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
display_name: Azure Legacy
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
