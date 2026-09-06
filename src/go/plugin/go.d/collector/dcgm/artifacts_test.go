// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type metadataMetric struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Unit        string `yaml:"unit"`
	ChartType   string `yaml:"chart_type"`
	Dimensions  []struct {
		Name string `yaml:"name"`
	} `yaml:"dimensions"`
}

func readMetadata(t *testing.T) (map[string]metadataMetric, map[string]map[string]string) {
	t.Helper()
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var doc struct {
		Modules []struct {
			Alerts  []map[string]string `yaml:"alerts"`
			Metrics struct {
				Scopes []struct {
					Metrics []metadataMetric `yaml:"metrics"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Modules, 1)
	rows := make(map[string]metadataMetric)
	for _, scope := range doc.Modules[0].Metrics.Scopes {
		for _, row := range scope.Metrics {
			require.NotContains(t, rows, row.Name)
			rows[row.Name] = row
		}
	}
	alerts := make(map[string]map[string]string)
	for _, alert := range doc.Modules[0].Alerts {
		alerts[alert["name"]] = alert
	}
	return rows, alerts
}
func TestCatalogMetadata(t *testing.T) {
	rows, _ := readMetadata(t)
	for name, row := range rows {
		spec, ok := contextCatalog[name]
		require.True(t, ok, name)
		assert.Equal(t, spec.Title, row.Description, name)
		assert.Equal(t, spec.Units, row.Unit, name)
		assert.Equal(t, string(spec.Type), row.ChartType, name)
	}
	// Every supported field has a documented default hardware scope. Other
	// exporter-discovered entity combinations use the declared dynamic prefix.
	for name, def := range fieldCatalog {
		if def.Raw {
			continue
		}
		entity := entityGPU
		switch {
		case def.Host:
			entity = entityHost
		case def.Link != "" || strings.HasPrefix(name, "DCGM_FI_DEV_NVSWITCH_LINK_"):
			entity = entityNVLink
		case strings.HasPrefix(name, "DCGM_FI_DEV_CPU_"):
			entity = entityCPU
		case strings.HasPrefix(def.Group, "interconnect.nvswitch."):
			entity = entityNVSwitch
		}
		spec := classifyMetric(entity, name, "", def.SourceKind)
		if def.Direction == "total" {
			if entity != entityNVLink {
				continue
			}
			spec.Context = contextCatalog["dcgm.nvlink.interconnect.combined.throughput"]
		}
		row, ok := rows[spec.Context.ID]
		require.True(t, ok, name+" -> "+spec.Context.ID)
		if len(def.States) > 0 {
			continue
		}
		dim := def.DimName
		if len(def.Labels) > 0 {
			dim += "_" + def.Labels[0] + "=*"
		}
		var dimensions []string
		for _, d := range row.Dimensions {
			dimensions = append(dimensions, d.Name)
		}
		assert.Contains(t, dimensions, dim, name)
	}
}
func TestExporterCSVSemantics(t *testing.T) {
	data, err := os.ReadFile("dcgm-exporter-netdata.csv")
	require.NoError(t, err)
	rows, _ := readMetadata(t)
	annotation := ""
	enabled := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "# context=") {
			annotation = strings.Fields(strings.TrimPrefix(line, "# context="))[0]
			continue
		}
		if strings.HasPrefix(line, "# numeric=") || strings.HasPrefix(line, "# metadata=") {
			annotation = ""
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "# "), ",", 3)
		if len(parts) != 3 || !strings.HasPrefix(parts[0], "DCGM_") {
			continue
		}
		name, kind := parts[0], strings.TrimSpace(parts[1])
		if !strings.HasPrefix(line, "#") {
			enabled++
		}
		if def, ok := fieldCatalog[name]; ok && kind != "label" {
			expected := "gauge"
			if def.SourceKind == sampleCounter {
				expected = "counter"
			}
			assert.Equal(t, expected, kind, name)
		}
		if annotation != "" {
			assert.Contains(t, rows, annotation, name)
		}
	}
	assert.Equal(t, 123, enabled, "preserve the stock field selection")
}
func TestStockAlertContracts(t *testing.T) {
	rows, metadataAlerts := readMetadata(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "health", "health.d", "dcgm.conf"))
	require.NoError(t, err)
	var rules []map[string]string
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		if key == "template" {
			rules = append(rules, make(map[string]string))
		}
		if len(rules) > 0 {
			rules[len(rules)-1][key] = strings.TrimSpace(value)
		}
	}
	require.Len(t, rules, 5)
	for _, rule := range rules {
		name := rule["template"]
		row, ok := rows[rule["on"]]
		require.True(t, ok, name)
		assert.Equal(t, row.Unit, rule["units"], name)
		assert.Equal(t, rule["on"], metadataAlerts[name]["metric"], name)
		assert.Equal(t, rule["info"], metadataAlerts[name]["info"], name)
		assert.Equal(t, "$this > 0", rule["warn"], name)
		assert.Equal(t, "30s", rule["every"], name)
		assert.Equal(t, "sysadmin", rule["to"], name)
		lookup := strings.Fields(rule["lookup"])
		require.GreaterOrEqual(t, len(lookup), 6)
		var dims []string
		for _, d := range row.Dimensions {
			dims = append(dims, d.Name)
		}
		assert.Contains(t, dims, lookup[len(lookup)-1], name)
		if strings.Contains(rule["lookup"], "-5m") {
			assert.Equal(t, "average", lookup[0], name)
		} else {
			assert.Equal(t, "max", lookup[0], name)
		}
	}
}

func TestNVSwitchLinkMetadata(t *testing.T) {
	body := "# TYPE DCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS gauge\nDCGM_FI_DEV_NVSWITCH_LINK_FATAL_ERRORS{nvlink=\"0\",nvswitch=\"switch0\"} 42\n"
	c := collectorWithMetrics(t, body)
	mx := c.Collect(nil)
	ch := chartByContext(c, "dcgm.nvlink.interconnect.nvswitch.link_sxid")
	require.NotNil(t, ch)
	require.NotEmpty(t, mx)
	rows, _ := readMetadata(t)
	row, ok := rows[ch.Ctx]
	require.True(t, ok, "real switch-link context must be documented")
	assert.Equal(t, ch.Title, row.Description)
	assert.Equal(t, ch.Units, row.Unit)
}
