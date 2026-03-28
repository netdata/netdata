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
	assert.Equal(t, "User", profiles[0].Name)
}

func TestLoadFromDirs_DuplicateStockIDFails(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))

	writeProfileFile(t, filepath.Join(stock, "demo-a.yaml"), validProfileYAML("demo", "One"))
	writeProfileFile(t, filepath.Join(stock, "demo-b.yaml"), validProfileYAML("demo", "Two"))

	_, err := LoadFromDirs([]DirSpec{{Path: stock, IsStock: true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate stock profile id")
}

func TestLoadFromDirs_StrictDecodeFailsOnUnknownField(t *testing.T) {
	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))

	writeProfileFile(t, filepath.Join(stock, "demo.yaml"), `
id: demo
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

func writeProfileFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func validProfileYAML(id, name string) string {
	return `
id: ` + id + `
name: ` + name + `
match: demo_*
template:
  family: prometheus_curated
  metrics: [demo_metric_total]
  charts:
    - id: ` + id + `_chart
      title: Demo Metric
      context: prometheus.demo.metric
      units: events
      dimensions:
        - selector: demo_metric_total
          name: value
`
}
