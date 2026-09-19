// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
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
				current = strings.TrimPrefix(current+"."+group.ContextNamespace, ".")
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

// chartDimensionNames resolves the dimension names a template chart renders:
// static names as written, and one dimension per declared state for a stateset
// selector without a name. chartengine infers those names from the flattened
// state series; it currently creates them in alphabetical order, so only the
// set is a runtime contract, the order here is the declaration order.
func chartDimensionNames(t *testing.T, chart charttpl.Chart) []string {
	t.Helper()
	var names []string
	for _, dimension := range chart.Dimensions {
		if dimension.Name != "" {
			names = append(names, dimension.Name)
			continue
		}
		states, ok := statesetStates()[dimension.Selector]
		require.True(t, ok, "dimension %q has no name and is not a stateset", dimension.Selector)
		names = append(names, states...)
	}
	return names
}

func statesetStates() map[string][]string {
	result := map[string][]string{"collection_status": collectionStates}
	for _, definition := range measurement.Definitions() {
		if len(definition.States) > 0 {
			result[definition.Name] = definition.States
		}
	}
	return result
}

// Every alert in health.d must target a chart the template provides.
func TestHealthAlertsTargetTemplateCharts(t *testing.T) {
	charts := templateCharts(t)
	raw, err := os.ReadFile("../../../../../health/health.d/redfish.conf")
	require.NoError(t, err)
	for _, match := range regexp.MustCompile(`(?m)^\s+on:\s+(\S+)`).FindAllStringSubmatch(string(raw), -1) {
		require.Contains(t, charts, match[1])
	}
}

func TestMetadataDocumentsChartLabels(t *testing.T) {
	var metadata struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Name   string
					Labels []struct{ Name string }
				}
			}
		}
	}
	raw, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(raw, &metadata))
	require.Len(t, metadata.Modules, 1)
	documented := make(map[string][]string)
	for _, scope := range metadata.Modules[0].Metrics.Scopes {
		for _, label := range scope.Labels {
			documented[scope.Name] = append(documented[scope.Name], label.Name)
		}
	}
	require.Equal(t, map[string][]string{
		"endpoint": {"endpoint_key"},
		"resource": slices.Sorted(slices.Values(measurement.ResourceLabelKeys)),
		"reading":  slices.Sorted(slices.Values(measurement.ReadingLabelKeys)),
	}, sortedLabelSets(documented))
}

func sortedLabelSets(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for scope, labels := range in {
		out[scope] = slices.Sorted(slices.Values(labels))
	}
	return out
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
			var actual []string
			for _, dimension := range metric.Dimensions {
				actual = append(actual, dimension.Name)
			}
			require.Equal(t, chartDimensionNames(t, chart), actual, metric.Name)
		}
	}
	require.Len(t, seen, len(charts))
}
