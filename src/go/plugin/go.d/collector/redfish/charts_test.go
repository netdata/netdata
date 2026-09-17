// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestChartTemplate(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func templateCharts(t *testing.T) map[string]charttpl.Chart {
	t.Helper()
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	result := make(map[string]charttpl.Chart)
	var visit func([]charttpl.Group, string)
	visit = func(groups []charttpl.Group, namespace string) {
		for _, group := range groups {
			current := namespace
			if group.ContextNamespace != "" {
				current = group.ContextNamespace
			}
			for _, chart := range group.Charts {
				context := chart.Context
				if current != "" {
					context = current + "." + context
				}
				require.NotContains(t, result, context)
				result[context] = chart
			}
			visit(group.Groups, current)
		}
	}
	visit(spec.Groups, spec.ContextNamespace)
	return result
}

func TestSourceChartsPreserveCommonDimensions(t *testing.T) {
	charts := templateCharts(t)
	for context, dimension := range map[string]string{
		"system.hw.sensor.temperature.input": "input",
		"system.hw.sensor.voltage.input":     "input",
		"system.hw.sensor.voltage.average":   "average",
		"system.hw.sensor.fan.input":         "input",
		"system.hw.sensor.current.input":     "input",
		"system.hw.sensor.current.average":   "average",
		"system.hw.sensor.power.input":       "input",
		"system.hw.sensor.power.average":     "average",
		"system.hw.sensor.energy.input":      "input",
		"system.hw.sensor.humidity.input":    "input",
		"system.hw.sensor.pressure.input":    "input",
	} {
		chart, ok := charts[context]
		require.True(t, ok, context)
		require.Len(t, chart.Dimensions, 1, context)
		require.Equal(t, dimension, chart.Dimensions[0].Name, context)
	}
}

func TestSourceHealthChartsAndRules(t *testing.T) {
	charts := templateCharts(t)
	var alarms int
	for context, chart := range charts {
		require.False(t, strings.HasPrefix(context, "redfish.aggregate."), context)
		require.False(t, strings.HasPrefix(context, "redfish.collection.detail_"), context)
		require.NotEqual(t, "redfish.collection.selected_system", context)
		if context != "redfish.reading.alarm" &&
			!(strings.HasPrefix(context, "system.hw.sensor.") && strings.HasSuffix(context, ".alarm")) {
			continue
		}
		alarms++
		var states []string
		for _, dimension := range chart.Dimensions {
			states = append(states, dimension.Name)
		}
		require.Equal(t, []string{"clear", "warning", "critical"}, states, context)
	}
	require.Equal(t, 9, alarms)
	// Every retained alert must attach to a chart provided by this collector.
	raw, err := os.ReadFile("../../../../../health/health.d/redfish.conf")
	require.NoError(t, err)
	for _, match := range regexp.MustCompile(`(?m)^\s+on:\s+(\S+)`).FindAllStringSubmatch(string(raw), -1) {
		require.Contains(t, charts, match[1])
	}
	require.NotContains(t, string(raw), "$cap")
	require.NotContains(t, string(raw), "$emergency")
	require.NotContains(t, string(raw), "$fault")
}

// Metadata and fixed chart definitions are maintained together after generator removal.
func TestMetadataDocumentsEveryChart(t *testing.T) {
	var metadata struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Metrics []struct {
						Name        string
						Description string
						Unit        string
						ChartType   string `yaml:"chart_type"`
						Dimensions  []struct{ Name string }
					}
				}
			}
		}
	}
	raw, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(raw, &metadata))
	require.Len(t, metadata.Modules, 1)
	charts := templateCharts(t)
	seen := make(map[string]bool)
	for _, scope := range metadata.Modules[0].Metrics.Scopes {
		for _, metric := range scope.Metrics {
			require.False(t, seen[metric.Name], metric.Name)
			seen[metric.Name] = true
			chart, ok := charts[metric.Name]
			require.True(t, ok, metric.Name)
			require.Equal(t, chart.Title, metric.Description, metric.Name)
			require.Equal(t, chart.Units, metric.Unit, metric.Name)
			require.Equal(t, string(chart.Type), metric.ChartType, metric.Name)
			var expected, actual []string
			for _, dimension := range chart.Dimensions {
				expected = append(expected, dimension.Name)
			}
			for _, dimension := range metric.Dimensions {
				actual = append(actual, dimension.Name)
			}
			require.Equal(t, expected, actual, metric.Name)
		}
	}
	require.Len(t, seen, len(charts))
}
