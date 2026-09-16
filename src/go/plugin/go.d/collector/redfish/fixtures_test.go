// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDMTF2026_1SchemaTypeCoverage(t *testing.T) {
	var fixture struct {
		Release string `json:"release"`
		Schemas []struct {
			Schema    string `json:"schema"`
			ODataType string `json:"odata_type"`
			Source    string `json:"source"`
		} `json:"schemas"`
	}
	loadFixtureInto(t, "dmtf-2026.1-schema-types.json", &fixture)
	require.Equal(t, "2026.1", fixture.Release)

	evidence := make(map[string]string, len(fixture.Schemas))
	for _, item := range fixture.Schemas {
		require.NotEmpty(t, item.Schema)
		require.NotEmpty(t, item.Source)
		require.NotContains(t, item.Source, "\\")
		require.False(t, filepath.IsAbs(item.Source))
		_, exists := evidence[item.Schema]
		require.Falsef(t, exists, "duplicate schema evidence for %s", item.Schema)
		evidence[item.Schema] = item.ODataType
	}

	expected := make(map[string]struct{})
	for kind, schemas := range resourceSchemaNames {
		for _, schema := range schemas {
			expected[schema] = struct{}{}
			odataType, ok := evidence[schema]
			require.Truef(t, ok, "missing DMTF schema evidence for %s", schema)
			require.NoError(t, validateResourceSchemaType(kind, odataType))

			document := map[string]any{
				"@odata.type": odataType,
				"@odata.id":   "/redfish/v1/Synthetic/" + schema,
			}
			if kind == "legacy_power" {
				document["PowerControl"] = []any{}
			}
			raw, err := json.Marshal(document)
			require.NoError(t, err)
			_, err = decodeTypedResource(kind, raw)
			require.NoErrorf(t, err, "decode %s as %s", kind, odataType)
		}
	}
	require.Len(t, evidence, len(expected))
	for schema := range evidence {
		_, ok := expected[schema]
		require.Truef(t, ok, "unselected schema evidence %s", schema)
	}
}

func TestDMTF2026_1EmbeddedRelationshipRegistry(t *testing.T) {
	var got []string
	for _, relationship := range graphRelationships {
		if relationship.Embedded {
			got = append(got, relationship.ParentKind+"."+relationship.Path+"->"+relationship.ChildKind)
		}
	}
	sort.Strings(got)
	require.Equal(t, []string{
		"leak_detection.LeakDetectorGroups->leak_detector_group",
		"leak_detector_group.Detectors->leak_detector",
		"manager.Redundancy->redundancy",
		"power_subsystem.PowerSupplyRedundancy->redundancy",
		"sensor.SensorGroup->redundancy",
		"storage.Redundancy->redundancy",
		"system.Redundancy->redundancy",
		"thermal_subsystem.CoolantConnectorRedundancy->redundancy",
		"thermal_subsystem.FanRedundancy->redundancy",
	}, got)
}

func TestDMTF2026_1EmbeddedComponents(t *testing.T) {
	fixture := loadFixture(t, "dmtf-2026.1-embedded-components.min.json")
	client := fixtureClient()
	for _, kind := range []string{"thermal_subsystem", "leak_detection"} {
		raw, err := json.Marshal(fixture[kind])
		require.NoError(t, err)
		typed, err := decodeTypedResource(kind, raw)
		require.NoError(t, err)
		require.NotNil(t, typed)
	}

	thermal := fixtureGraphParent(t, "thermal_subsystem", fixture["thermal_subsystem"])
	redundancy, enrichment, complete, err := client.acquireRelationship(
		t.Context(),
		thermal,
		fixtureRelationship(t, "thermal_subsystem", "FanRedundancy"),
		&wireStats{},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, redundancy, 1)
	require.Equal(t, "redundancy", redundancy[0].Kind)
	require.Equal(t, "positional", redundancy[0].IdentityQuality)
	require.Equal(t, "Synthetic Fan Group", redundancy[0].Doc.Name)
	require.Equal(t, "OK", redundancy[0].Doc.Status.Health)
	require.Contains(t, redundancy[0].Locator, ":position:0")
	scalars := make(map[string]scalarValue)
	for _, scalar := range client.scalarValues(redundancy[0], time.Now()) {
		scalars[scalar.Descriptor.ID] = scalar
	}
	require.True(t, scalars["redundancy_members_active"].Emit)
	require.Equal(t, float64(1), scalars["redundancy_members_active"].Value)
	require.True(t, scalars["redundancy_members_total"].Emit)
	require.Equal(t, float64(2), scalars["redundancy_members_total"].Value)
	inventory := make(map[string]any)
	applyRegisteredInventory(inventory, redundancy[0])
	require.Equal(t, int64(1), inventory["redundancy_active_count"])
	require.Equal(t, int64(2), inventory["redundancy_member_count"])

	withoutCount := *redundancy[0]
	withoutCount.Data = cloneJSONMap(redundancy[0].Data)
	delete(withoutCount.Data, "ActiveRedundancyGroup@odata.count")
	_, present := registeredValueAt(withoutCount.Data, "ActiveRedundancyGroup.@odata.count")
	require.False(t, present)
	for _, scalar := range client.scalarValues(&withoutCount, time.Now()) {
		require.NotEqual(t, "redundancy_members_active", scalar.Descriptor.ID)
	}

	leakDetection := fixtureGraphParent(t, "leak_detection", fixture["leak_detection"])
	groups, enrichment, complete, err := client.acquireRelationship(
		t.Context(),
		leakDetection,
		fixtureRelationship(t, "leak_detection", "LeakDetectorGroups"),
		&wireStats{},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, groups, 1)
	require.Equal(t, "leak_detector_group", groups[0].Kind)
	require.Equal(t, "positional", groups[0].IdentityQuality)
	require.Equal(t, "Synthetic Detector Group", groups[0].Doc.Name)
	require.Equal(t, "Warning", groups[0].Doc.Status.Health)

	detectors, enrichment, complete, err := client.acquireRelationship(
		t.Context(),
		groups[0],
		fixtureRelationship(t, "leak_detector_group", "Detectors"),
		&wireStats{},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, detectors, 2)
	require.Equal(t, "data_source_uri", detectors[0].IdentityQuality)
	require.Equal(t, "/redfish/v1/Chassis/Synthetic/LeakDetectors/1", detectors[0].URI)
	require.Equal(t, "positional", detectors[1].IdentityQuality)
	require.Contains(t, detectors[1].Locator, ":position:1")
}

func TestEmbeddedComponentsPublishValidSiblings(t *testing.T) {
	client := fixtureClient()
	parent := fixtureGraphParent(t, "thermal_subsystem", map[string]any{
		"FanRedundancy": []any{
			map[string]any{"GroupName": "Duplicate", "Status": map[string]any{"Health": "OK"}},
			"malformed",
			map[string]any{"GroupName": "Duplicate", "Status": map[string]any{"Health": "Warning"}},
		},
	})
	nodes, _, complete, err := client.acquireRelationship(
		t.Context(),
		parent,
		fixtureRelationship(t, "thermal_subsystem", "FanRedundancy"),
		&wireStats{},
	)
	require.Error(t, err)
	require.False(t, complete)
	require.Len(t, nodes, 2)
	require.Equal(t, "positional", nodes[0].IdentityQuality)
	require.Equal(t, "positional", nodes[1].IdentityQuality)
	require.NotEqual(t, nodes[0].Locator, nodes[1].Locator)
}

func TestCompatibilityFixturesLegacyThermal(t *testing.T) {
	tests := map[string]struct {
		file              string
		wantFans          int
		wantTemperatures  int
		wantValidReadings int
	}{
		"dell": {
			file:              "otel-dell-legacy-thermal.min.json",
			wantFans:          1,
			wantTemperatures:  2,
			wantValidReadings: 2,
		},
		"hpe": {
			file:              "otel-hpe-legacy-thermal.min.json",
			wantFans:          1,
			wantTemperatures:  2,
			wantValidReadings: 3,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := fixtureClient()
			parent := fixtureParent()
			components, complete, err := client.legacyComponents(
				withOperationBudget(context.Background()),
				parent,
				graphRelationship{ChildKind: "legacy_thermal"},
				loadFixture(t, test.file),
			)
			require.NoError(t, err)
			require.True(t, complete)
			fans, temperatures, valid := 0, 0, 0
			for _, component := range components {
				switch component.Kind {
				case "fan":
					fans++
				case "sensor":
					temperatures++
				}
				for _, reading := range client.readingsForNode(component, time.Now()) {
					if reading.Valid {
						valid++
					}
				}
			}
			require.Equal(t, test.wantFans, fans)
			require.Equal(t, test.wantTemperatures, temperatures)
			require.Equal(t, test.wantValidReadings, valid)
		})
	}
}

func TestCompatibilityFixtureManualThresholdEvaluationCanBeDisabled(t *testing.T) {
	client := fixtureClient()
	parent := fixtureParent()
	components, complete, err := client.legacyComponents(
		withOperationBudget(context.Background()),
		parent,
		graphRelationship{ChildKind: "legacy_thermal"},
		loadFixture(t, "otel-hpe-legacy-thermal.min.json"),
	)
	require.NoError(t, err)
	require.True(t, complete)
	temperature := fixtureComponentByName(t, components, "Synthetic Hot Intake")

	readings := client.readingsForNode(temperature, time.Now())
	require.Len(t, readings, 1)
	require.Equal(t, "emergency", readings[0].EffectiveAlarm)
	require.Equal(t, "combined", readings[0].EffectiveAlarmSource)

	disabled := false
	client.config.Alarms.EvaluateThresholds = &disabled
	readings = client.readingsForNode(temperature, time.Now())
	require.Len(t, readings, 1)
	require.Equal(t, "clear", readings[0].EffectiveAlarm)
	require.Equal(t, "source", readings[0].EffectiveAlarmSource)
}

func TestCompatibilityFixturesModernReadings(t *testing.T) {
	client := fixtureClient()
	tests := map[string]struct {
		node         *graphNode
		wantFamilies map[string]int
	}{
		"thermal metrics": {
			node: &graphNode{
				Kind: "thermal_subsystem",
				Key:  "thermal-subsystem",
				Enrichment: map[string]map[string]any{
					"thermal_metrics:0": loadFixture(
						t,
						"telegraf-hpe-modern-thermal-metrics.min.json",
					),
				},
			},
			wantFamilies: map[string]int{"temperature": 1},
		},
		"fan": {
			node: &graphNode{
				Kind: "fan",
				Key:  "fan",
				Data: loadFixture(t, "telegraf-hpe-modern-fan.min.json"),
			},
			wantFamilies: map[string]int{"percentage": 1},
		},
		"power supply metrics": {
			node: &graphNode{
				Kind: "power_supply",
				Key:  "power-supply",
				Enrichment: map[string]map[string]any{
					"power_supply_metrics:0": loadFixture(
						t,
						"telegraf-hpe-modern-power-supply-metrics.min.json",
					),
				},
			},
			wantFamilies: map[string]int{
				"current": 1, "energy": 1, "frequency": 1, "percentage": 1,
				"power": 2, "rotational_speed": 1, "temperature": 1, "voltage": 1,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := make(map[string]int)
			identities := make(map[string]struct{})
			for _, reading := range client.readingsForNode(test.node, time.Now()) {
				if !reading.Valid {
					continue
				}
				got[reading.Family]++
				if _, ok := identities[reading.Key]; ok {
					t.Fatalf("duplicate reading identity %q", reading.Key)
				}
				identities[reading.Key] = struct{}{}
			}
			require.Equal(t, test.wantFamilies, got)
		})
	}
}

func TestCompatibilityFixtureModernStandaloneSensor(t *testing.T) {
	client := fixtureClient()
	node := &graphNode{
		Kind: "sensor",
		Key:  "standalone-sensor",
		Data: loadFixture(t, "checkmk-nvidia-modern-sensor.min.json"),
	}
	readings := client.readingsForNode(node, time.Now())
	require.Len(t, readings, 1)
	reading := readings[0]
	require.True(t, reading.Valid)
	require.Equal(t, "temperature", reading.Family)
	require.Equal(t, "system.hw.sensor.temperature.input", reading.Context)
	require.Equal(t, "clear", reading.SourceAlarm)
	require.Equal(t, "critical", reading.DerivedAlarm)
	require.Equal(t, "critical", reading.EffectiveAlarm)
	require.Equal(t, "combined", reading.EffectiveAlarmSource)
	require.Equal(t, "threshold_upper_critical", reading.EffectiveAlarmReason)
}

func TestCompatibilityFixtureLegacyPowerAndModernDrive(t *testing.T) {
	client := fixtureClient()
	parent := fixtureParent()
	components, complete, err := client.legacyComponents(
		withOperationBudget(context.Background()),
		parent,
		graphRelationship{ChildKind: "legacy_power"},
		loadFixture(t, "telegraf-dell-legacy-power.min.json"),
	)
	require.NoError(t, err)
	require.True(t, complete)
	kinds := make(map[string]int)
	families := make(map[string]int)
	for _, component := range components {
		kinds[component.Kind]++
		for _, reading := range client.readingsForNode(component, time.Now()) {
			if reading.Valid {
				families[reading.Family]++
			}
		}
	}
	require.Equal(t, map[string]int{"power_supply": 1, "sensor": 2}, kinds)
	require.Equal(t, map[string]int{"power": 1, "voltage": 1}, families)

	drive := &graphNode{
		Kind: "drive",
		Key:  "drive",
		Data: loadFixture(t, "telegraf-hpe-modern-drive.min.json"),
	}
	fields := make(map[string]scalarValue)
	for _, value := range client.scalarValues(drive, time.Now()) {
		fields[value.Descriptor.ID] = value
	}
	require.True(t, fields["drive_capacitybytes"].Emit)
	require.Equal(t, float64(1_600_000_000_000), fields["drive_capacitybytes"].Value)
	require.True(t, fields["drive_predictedmedialifeleftpercent"].Emit)
	require.Equal(t, float64(98), fields["drive_predictedmedialifeleftpercent"].Value)
}

func fixtureClient() *protocolClient {
	enabled := true
	root, err := url.Parse("https://fixture.example/redfish/v1/")
	if err != nil {
		panic(err)
	}
	client := &protocolClient{
		origin: "https://fixture.example",
		root:   root,
		config: Config{
			Alarms: AlarmsConfig{EvaluateThresholds: &enabled},
		},
	}
	client.hardwareState.initialize()
	return client
}

func fixtureGraphParent(t *testing.T, kind string, value any) *graphNode {
	t.Helper()
	data, ok := value.(map[string]any)
	require.True(t, ok)
	uri, _ := stringValue(data["@odata.id"])
	if uri == "" {
		uri = "/redfish/v1/Synthetic/" + kind
	}
	return &graphNode{
		Kind:             kind,
		Key:              "fixture-" + kind,
		URI:              uri,
		Locator:          uri,
		Data:             data,
		AcquisitionState: "readable",
		Complete:         true,
		IdentityQuality:  "addressable",
	}
}

func fixtureRelationship(t *testing.T, parent, path string) graphRelationship {
	t.Helper()
	for _, relationship := range graphRelationships {
		if relationship.ParentKind == parent && relationship.Path == path {
			require.True(t, relationship.Embedded)
			return relationship
		}
	}
	t.Fatalf("fixture relationship %s.%s is missing", parent, path)
	return graphRelationship{}
}

func fixtureParent() *graphNode {
	return &graphNode{
		Kind: "chassis",
		Key:  "fixture-chassis",
		URI:  "/redfish/v1/Chassis/Fixture-1",
	}
}

func fixtureComponentByName(
	t *testing.T,
	components []*graphNode,
	name string,
) *graphNode {
	t.Helper()
	for _, component := range components {
		if component.Doc.Name == name {
			return component
		}
	}
	t.Fatalf("fixture component %q is missing", name)
	return nil
}

func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	var document map[string]any
	loadFixtureInto(t, name, &document)
	return document
}

func loadFixtureInto(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, target))
}
