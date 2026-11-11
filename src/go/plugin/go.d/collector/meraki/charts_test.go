// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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
		map[string][]string{metricDeviceAvailabilityStatus: availabilityStates, metricApplianceUplinkStatus: uplinkStates},
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

func TestMetadataDocumentsEmittedLabels(t *testing.T) {
	_, c, _ := newHealthFixture(t)
	require.NoError(t, collectCycle(t.Context(), c))

	var orgLabels []string
	c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
		if name == metricOrganizationAssignedDevices {
			orgLabels = slices.Sorted(maps.Keys(labels.CloneMap()))
		}
	})
	emitted := map[string][]string{
		"organization": orgLabels,
		"device":       slices.Sorted(maps.Keys(readDevices(c)["S1"].Labels)),
	}
	for scope, metric := range map[string]string{
		"switch port":             metricSwitchPortTrafficSent,
		"appliance uplink":        metricApplianceUplinkTrafficSent,
		"appliance uplink health": metricApplianceUplinkStatus,
		"appliance uplink probe":  metricApplianceUplinkLatency,
	} {
		points := metricPoints(c, metric)
		require.NotEmpty(t, points)
		labels := points[0].Labels
		delete(labels, metricApplianceUplinkStatus) // The flattened state is a dimension, not a chart label.
		emitted[scope] = slices.Sorted(maps.Keys(labels))
	}

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

	require.Equal(t, emitted, documented)
}
