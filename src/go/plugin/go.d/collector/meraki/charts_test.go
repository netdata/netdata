// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestIntegrationArtifacts(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, defaultUpdateEvery)
	require.NoError(t, err)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(
		t,
		metadata,
		chartTemplateYAML,
		map[string][]string{"device_availability_status": availabilityStates},
	)
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
}

func TestDocumentedLabels(t *testing.T) {
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
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &metadata))
	require.Len(t, metadata.Modules, 1)
	documented := make(map[string][]string)
	for _, scope := range metadata.Modules[0].Metrics.Scopes {
		for _, label := range scope.Labels {
			documented[scope.Name] = append(documented[scope.Name], label.Name)
		}
		slices.Sort(documented[scope.Name])
	}
	want := map[string][]string{
		"organization": slices.Clone(OrganizationLabelKeys),
		"device":       slices.Clone(DeviceLabelKeys),
	}
	for _, labels := range want {
		slices.Sort(labels)
	}
	require.Equal(t, want, documented)
}
