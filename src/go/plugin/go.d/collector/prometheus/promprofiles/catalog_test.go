// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromDirs_UserOverridesStock(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	user := filepath.Join(root, "user")
	require.NoError(t, os.MkdirAll(stock, 0o755))
	require.NoError(t, os.MkdirAll(user, 0o755))

	writeProfileFile(t, filepath.Join(stock, "demo.yaml"), validProfileYAML("demo", "Stock"))
	writeProfileFile(t, filepath.Join(user, "demo.yaml"), validProfileYAML("demo", "User"))

	catalog, err := LoadFromDirs([]DirSpec{
		{Path: stock, IsStock: true},
		{Path: user, IsStock: false},
	})
	require.NoError(t, err)

	profiles, err := catalog.Resolve([]string{"demo"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "demo", profiles[0].Name)
	require.Len(t, profiles[0].Template.Charts, 1)
	assert.Equal(t, "User", profiles[0].Template.Charts[0].Title)
}

func TestLoadFromDirs_DuplicateStockNameFails(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))

	writeProfileFile(t, filepath.Join(stock, "demo-a.yaml"), validProfileYAML("demo", "One"))
	writeProfileFile(t, filepath.Join(stock, "demo-b.yaml"), validProfileYAML("demo", "Two"))

	_, err := LoadFromDirs([]DirSpec{{Path: stock, IsStock: true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate stock profile name")
}

func TestLoadFromDirs_StrictDecodeFailsOnUnknownField(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))

	writeProfileFile(t, filepath.Join(stock, "demo.yaml"), `
name: demo
match: demo_*
unknown_field: true
template:
  family: prometheus_curated
  metrics: [demo_metric_total]
  charts:
    - id: demo_chart
      title: Demo Metric
      context: prometheus.demo.metric
      units: events
      dimensions:
        - selector: demo_metric_total
          name: value
`)

	_, err := LoadFromDirs([]DirSpec{{Path: stock, IsStock: true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_field")
}

func TestLoadFromDirs_IgnoresUnderscoredFiles(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))

	writeProfileFile(t, filepath.Join(stock, "_ignored.yaml"), validProfileYAML("demo", "Ignored"))

	catalog, err := LoadFromDirs([]DirSpec{{Path: stock, IsStock: true}})
	require.NoError(t, err)
	assert.True(t, catalog.Empty())
}

func TestLoadFromDirs_OrderedProfilesPreserveMergedLoadOrder(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	user := filepath.Join(root, "user")
	require.NoError(t, os.MkdirAll(stock, 0o755))
	require.NoError(t, os.MkdirAll(user, 0o755))

	writeProfileFile(t, filepath.Join(stock, "01-alpha.yaml"), validProfileYAML("Alpha", "Stock Alpha"))
	writeProfileFile(t, filepath.Join(stock, "02-beta.yaml"), validProfileYAML("Beta", "Stock Beta"))
	writeProfileFile(t, filepath.Join(user, "01-alpha.yaml"), validProfileYAML("Alpha", "User Alpha"))
	writeProfileFile(t, filepath.Join(user, "02-gamma.yaml"), validProfileYAML("Gamma", "User Gamma"))

	catalog, err := LoadFromDirs([]DirSpec{
		{Path: stock, IsStock: true},
		{Path: user, IsStock: false},
	})
	require.NoError(t, err)

	profiles := catalog.OrderedProfiles()
	require.Len(t, profiles, 3)
	assert.Equal(t, "Alpha", profiles[0].Name)
	require.Len(t, profiles[0].Template.Charts, 1)
	assert.Equal(t, "User Alpha", profiles[0].Template.Charts[0].Title)
	assert.Equal(t, "Beta", profiles[1].Name)
	assert.Equal(t, "Gamma", profiles[2].Name)
}

func writeProfileFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func validProfileYAML(name string, title string) string {
	return `
name: ` + name + `
match: demo_*
template:
  family: prometheus_curated
  metrics: [demo_metric_total]
  charts:
    - id: ` + name + `_chart
      title: ` + title + `
      context: prometheus.demo.metric
      units: events
      dimensions:
        - selector: demo_metric_total
          name: value
`
}
