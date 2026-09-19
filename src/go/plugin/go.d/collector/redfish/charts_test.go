// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"os"
	"slices"
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

// statesetStates maps every stateset metric to its declared states, the names
// chartengine renders for a stateset selector without an explicit name.
func statesetStates() map[string][]string {
	result := map[string][]string{"collection_status": collectionStates}
	for _, definition := range measurement.Definitions() {
		if len(definition.States) > 0 {
			result[definition.Name] = definition.States
		}
	}
	return result
}

func readArtifact(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

const healthConfigPath = "../../../../../health/health.d/redfish.conf"

func TestMetadataDocumentsChartTemplate(t *testing.T) {
	collecttest.AssertMetadataDocumentsChartTemplate(t, readArtifact(t, "metadata.yaml"), chartTemplateYAML, statesetStates())
}

func TestHealthAlertsTargetChartTemplate(t *testing.T) {
	collecttest.AssertHealthAlertsTargetChartTemplate(t, readArtifact(t, healthConfigPath), chartTemplateYAML)
}

func TestMetadataAlertsMatchHealthConfig(t *testing.T) {
	collecttest.AssertMetadataAlertsMatchHealthConfig(t, readArtifact(t, "metadata.yaml"), readArtifact(t, healthConfigPath))
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
