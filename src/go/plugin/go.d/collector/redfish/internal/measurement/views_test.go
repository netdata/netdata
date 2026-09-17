// SPDX-License-Identifier: GPL-3.0-or-later

package measurement_test

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestProjectComponentViewsUseCurrentSourceMetadata(t *testing.T) {
	observed := time.Unix(100, 0)
	resources := []*measurement.Resource{
		{Kind: "firmware", Key: "firmware", AcquisitionState: "readable", Data: map[string]any{"Version": "1.2.3"}},
		{Kind: "software", Key: "software", AcquisitionState: "readable", Data: map[string]any{"Version": "4.5.6"}},
		{Kind: "system", Key: "system", AcquisitionState: "readable", Data: map[string]any{"BiosVersion": "7.8.9"}},
		{Kind: "drive", Key: "drive", AcquisitionState: "readable", Data: map[string]any{"FirmwareVersion": "A1"}},
		{
			Kind:             "firmware",
			Key:              "unreadable",
			URI:              "/redfish/v1/UpdateService/FirmwareInventory/1",
			AcquisitionState: "unreadable",
		},
		{Kind: "service", Key: "service", AcquisitionState: "readable", Data: map[string]any{}},
	}
	projector := measurement.New("https://fixture.example", "hardware", nil)
	result, err := projector.Project(resources, true, observed)
	require.NoError(t, err)
	require.Len(t, result.Components, 5, "service root is not hardware")
	for i, version := range []string{"1.2.3", "4.5.6", "7.8.9", "A1"} {
		require.Equal(t, version, result.Components[i].Firmware)
		require.Equal(t, observed, result.Components[i].ObservedAt)
	}
	missing := result.Components[4]
	require.Equal(t, "unreadable", missing.Availability)
	require.Equal(t, resources[4].URI, missing.Name)
	require.Empty(t, missing.Health)
	require.Empty(t, missing.Firmware)
	require.True(t, missing.ObservedAt.IsZero())
	resources[0].Data["Version"] = "changed"
	require.Equal(t, "1.2.3", result.Components[0].Firmware, "published facts are independent of input documents")
}

func TestProjectPreservesNullReadingViewsWithoutNumericMetrics(t *testing.T) {
	for name, resource := range map[string]*measurement.Resource{
		"component scalar": {
			Kind: "fan", Data: map[string]any{"SpeedPercent": nil},
		},
		"environment excerpt": {
			Kind: "chassis", Enrichment: map[string]measurement.Enrichment{
				"environment_metrics": {Data: map[string]any{"PowerWatts": nil}},
			},
		},
		"polyphase member": {
			Kind: "power_supply", Enrichment: map[string]measurement.Enrichment{
				"power_supply_metrics": {Data: map[string]any{"PolyPhaseCurrentAmps": map[string]any{"Line1": nil}}},
			},
		},
		"legacy temperature": {
			Kind: "sensor", SourceModel: "deprecated_thermal", SourcePath: "Temperatures",
			Data: map[string]any{"ReadingCelsius": nil},
		},
		"legacy fan": {
			Kind: "sensor", SourceModel: "deprecated_thermal", SourcePath: "Fans",
			Data: map[string]any{"Reading": nil},
		},
		"legacy voltage": {
			Kind: "sensor", SourceModel: "deprecated_power", SourcePath: "Voltages",
			Data: map[string]any{"ReadingVolts": nil},
		},
	} {
		t.Run(name, func(t *testing.T) {
			resource.Key = "resource"
			resource.AcquisitionState = "readable"
			observed := time.Unix(100, 0)
			projector := measurement.New("https://fixture.example", "hardware", nil)
			result, err := projector.Project([]*measurement.Resource{resource}, true, observed)
			require.NoError(t, err)
			require.Len(t, result.Sensors, 1, "an explicitly null reading remains visible")
			require.False(t, result.Sensors[0].Valid)
			require.Equal(t, observed, result.Sensors[0].ObservedAt)
			absent := *resource
			absent.Data = map[string]any{}
			absent.Enrichment = nil
			withoutReading, err := projector.Project([]*measurement.Resource{&absent}, true, observed)
			require.NoError(t, err)
			require.Empty(t, withoutReading.Sensors, "absent properties do not create rows")
			require.Equal(t, withoutReading.Observations, result.Observations, "null values do not add numeric metrics")
		})
	}
}

func TestProjectLegacyNullStillAllowsNumericFallback(t *testing.T) {
	projector := measurement.New("https://fixture.example", "hardware", nil)
	resource := &measurement.Resource{
		Key:              "temperature",
		Kind:             "sensor",
		SourceModel:      "deprecated_thermal",
		SourcePath:       "Temperatures",
		AcquisitionState: "readable",
		Data:             map[string]any{"ReadingCelsius": nil, "Reading": 0},
	}
	result, err := projector.Project([]*measurement.Resource{resource}, true, time.Unix(100, 0))
	require.NoError(t, err)
	require.Len(t, result.Sensors, 1)
	require.True(t, result.Sensors[0].Valid)
	require.Zero(t, result.Sensors[0].Value)
	delete(resource.Data, "ReadingCelsius")
	withoutNull, err := projector.Project([]*measurement.Resource{resource}, true, time.Unix(100, 0))
	require.NoError(t, err)
	require.Equal(t, withoutNull, result, "an earlier null alias does not hide a later numeric value")
}
