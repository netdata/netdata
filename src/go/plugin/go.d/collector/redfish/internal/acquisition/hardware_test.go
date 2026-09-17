// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestNVMETemperatureArrayProducesDistinctReadings(t *testing.T) {
	client := &Client{}
	parent := &graphNode{
		Resource: measurement.Resource{
			Kind: "drive",
			Key:  "drive-key",
			URI:  "/redfish/v1/Drives/1",
		},
	}
	data := map[string]any{
		"NVMeSMART": map[string]any{
			"TemperatureSensorsCelsius": []any{json.Number("41"), json.Number("43")},
		},
	}
	nodes, complete, err := client.sensorExcerptArrayNodes(
		parent,
		"Metrics.NVMeSMART.TemperatureSensorsCelsius",
		sensorExcerptArraySpec{
			Path:          "NVMeSMART.TemperatureSensorsCelsius",
			Type:          "Temperature",
			Units:         "Cel",
			ScalarMembers: true,
		},
		data,
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, nodes, 2)

	readings := measurementTestReadings(measurementTestProject(t, client, nodes...))
	require.Len(t, readings, 2)
	require.NotEmpty(t, measurementTestLabel(readings[0], "reading_key"))
	require.NotEqual(
		t,
		measurementTestLabel(readings[0], "reading_key"),
		measurementTestLabel(readings[1], "reading_key"),
	)
	require.NotEqual(t, nodes[0].Key, nodes[1].Key)
	require.Equal(t, float64(41), readings[0].Value)
	require.Equal(t, float64(43), readings[1].Value)
}

func TestArrayReadingDataSourceURIStabilizesIdentityAcrossReorder(t *testing.T) {
	root, origin, err := NormalizeServiceRoot("https://bmc.example/redfish/v1/")
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		root:   root,
		origin: origin,
	}
	parent := &graphNode{
		Resource: measurement.Resource{
			Kind: "thermal_subsystem",
			Key:  "thermal-key",
			URI:  "/redfish/v1/Chassis/1/ThermalSubsystem",
		},
	}
	makeNodes := func(values ...map[string]any) []*graphNode {
		array := make([]any, len(values))
		for index := range values {
			array[index] = values[index]
		}
		nodes, complete, err := client.sensorExcerptArrayNodes(
			parent,
			"ThermalMetrics.TemperatureReadingsCelsius",
			sensorExcerptArraySpec{
				Path:  "TemperatureReadingsCelsius",
				Type:  "Temperature",
				Units: "Cel",
			},
			map[string]any{"TemperatureReadingsCelsius": array},
		)
		require.NoError(t, err)
		require.True(t, complete)
		return nodes
	}
	firstSensor := map[string]any{"Reading": json.Number("41"), "DataSourceUri": "/redfish/v1/Sensors/A"}
	secondSensor := map[string]any{"Reading": json.Number("43"), "DataSourceUri": "/redfish/v1/Sensors/B"}

	first := measurementTestReadings(measurementTestProject(t, client, makeNodes(firstSensor, secondSensor)...))
	second := measurementTestReadings(measurementTestProject(t, client, makeNodes(secondSensor, firstSensor)...))
	require.Len(t, first, 2)
	require.Len(t, second, 2)
	firstKeys, secondKeys := make(map[float64]string), make(map[float64]string)
	for _, reading := range first {
		firstKeys[reading.Value] = measurementTestLabel(reading, "reading_key")
		require.NotEmpty(t, firstKeys[reading.Value])
	}
	for _, reading := range second {
		secondKeys[reading.Value] = measurementTestLabel(reading, "reading_key")
		require.NotEmpty(t, secondKeys[reading.Value])
	}
	if firstKeys[41] != secondKeys[41] || firstKeys[43] != secondKeys[43] {
		t.Fatalf("reading keys changed across reorder: first=%v second=%v", firstKeys, secondKeys)
	}
}

func TestSensorExcerptDataSourceProofMergesWithAddressableSensor(t *testing.T) {
	root, origin, err := NormalizeServiceRoot("https://bmc.example/redfish/v1/")
	require.NoError(t, err)
	client := &Client{
		root:   root,
		origin: origin,
	}
	addressable := &graphNode{
		Resource: measurement.Resource{
			Kind:             "sensor",
			Key:              identity.ResourceKey(origin, "sensor", "/redfish/v1/Sensors/A"),
			URI:              "/redfish/v1/Sensors/A",
			SourceModel:      "typed_resource",
			AcquisitionState: "readable",
			Data: map[string]any{
				"Reading":      json.Number("42"),
				"ReadingType":  "Temperature",
				"ReadingUnits": "Cel",
			},
		},
		Locator:         "/redfish/v1/Sensors/A",
		IdentityQuality: "addressable",
	}
	graph := &resourceGraph{
		ByIdentity: make(map[string]*graphNode),
		KeySources: make(map[string]string),
	}
	addressable.Parents = make(map[string]*graphNode)
	require.NoError(t, graph.add(addressable))
	owner := &graphNode{
		Resource: measurement.Resource{
			Kind: "thermal_subsystem",
			Key:  "thermal",
			URI:  "/redfish/v1/ThermalSubsystem",
		},
	}
	nodes, complete, err := client.sensorExcerptArrayNodes(
		owner,
		"ThermalMetrics.TemperatureReadingsCelsius",
		sensorExcerptArraySpec{
			Path:  "TemperatureReadingsCelsius",
			Type:  "Temperature",
			Units: "Cel",
		},
		map[string]any{
			"TemperatureReadingsCelsius": []any{map[string]any{
				"Reading":       json.Number("43"),
				"DataSourceUri": "/redfish/v1/Sensors/A#/Reading",
				"Status":        map[string]any{"Health": "Critical"},
			}},
		},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, nodes, 1)
	require.NoError(t, graph.add(nodes[0]))
	client.reconcileSensorExcerptIdentities(graph)
	require.Len(t, graph.Nodes, 1)
	require.Same(t, addressable, graph.Nodes[0])
	observations := measurementTestProject(t, client, addressable)
	readings := measurementTestReadings(observations)
	require.Len(t, readings, 1)
	require.Equal(t, float64(42), readings[0].Value, "the addressable Sensor reading has precedence")
	measurementTestRequireAlarm(t, observations, "critical")
}
