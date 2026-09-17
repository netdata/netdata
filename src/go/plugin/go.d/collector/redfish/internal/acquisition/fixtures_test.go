// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"net/url"
	"path/filepath"
	"sort"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
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
	testutil.LoadFixtureInto(t, "../../testdata/"+"dmtf-2026.1-schema-types.json", &fixture)
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
	fixture := testutil.LoadFixture(t, "../../testdata/"+"dmtf-2026.1-embedded-components.min.json")
	client := fixtureClient()

	thermal := fixtureGraphParent(t, "thermal_subsystem", fixture["thermal_subsystem"])
	acquired := client.acquireRelationship(
		t.Context(),
		thermal,
		fixtureRelationship(t, "thermal_subsystem", "FanRedundancy"),
		&wireStats{},
	)
	redundancy, enrichment, complete, err := acquired.children, acquired.enrichments, acquired.complete, acquired.err
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, redundancy, 1)
	require.Equal(t, "redundancy", redundancy[0].Kind)
	require.Equal(t, "positional", redundancy[0].IdentityQuality)
	require.Equal(t, "Synthetic Fan Group", redundancy[0].Doc.Name)
	require.Equal(t, "OK", redundancy[0].Doc.Status.Health)
	require.Equal(t, embeddedLocator(thermal.Locator, "FanRedundancy", "", 0), redundancy[0].Locator)
	observations := measurementTestProject(t, client, redundancy[0])
	measurementTestRequireValue(t, observations, "redfish_redundancy_members_active", 1)
	measurementTestRequireValue(t, observations, "redfish_redundancy_members_total", 2)

	withoutCount := *redundancy[0]
	withoutCount.Data = cloneJSONMap(redundancy[0].Data)
	delete(withoutCount.Data, "ActiveRedundancyGroup@odata.count")
	_, present := withoutCount.Data["ActiveRedundancyGroup@odata.count"]
	require.False(t, present)
	for _, observation := range measurementTestProject(t, client, &withoutCount) {
		require.NotEqual(t, "redfish_redundancy_members_active", observation.Metric)
	}

	leakDetection := fixtureGraphParent(t, "leak_detection", fixture["leak_detection"])
	acquired = client.acquireRelationship(
		t.Context(),
		leakDetection,
		fixtureRelationship(t, "leak_detection", "LeakDetectorGroups"),
		&wireStats{},
	)
	groups, enrichment, complete, err := acquired.children, acquired.enrichments, acquired.complete, acquired.err
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, groups, 1)
	require.Equal(t, "leak_detector_group", groups[0].Kind)
	require.Equal(t, "positional", groups[0].IdentityQuality)
	require.Equal(t, "Synthetic Detector Group", groups[0].Doc.Name)
	require.Equal(t, "Warning", groups[0].Doc.Status.Health)

	acquired = client.acquireRelationship(
		t.Context(),
		groups[0],
		fixtureRelationship(t, "leak_detector_group", "Detectors"),
		&wireStats{},
	)
	detectors, enrichment, complete, err := acquired.children, acquired.enrichments, acquired.complete, acquired.err
	require.NoError(t, err)
	require.True(t, complete)
	require.Nil(t, enrichment)
	require.Len(t, detectors, 2)
	require.Equal(t, "data_source_uri", detectors[0].IdentityQuality)
	require.Equal(t, "/redfish/v1/Chassis/Synthetic/LeakDetectors/1", detectors[0].URI)
	require.Equal(t, "positional", detectors[1].IdentityQuality)
	require.Equal(t, embeddedLocator(groups[0].Locator, "Detectors", "", 1), detectors[1].Locator)
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
	acquired := client.acquireRelationship(
		t.Context(),
		parent,
		fixtureRelationship(t, "thermal_subsystem", "FanRedundancy"),
		&wireStats{},
	)
	nodes, _, complete, err := acquired.children, acquired.enrichments, acquired.complete, acquired.err
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
				parent,
				graphRelationship{
					ChildKind: "legacy_thermal",
				},
				testutil.LoadFixture(t, "../../testdata/"+test.file),
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
				valid += len(measurementTestReadings(measurementTestProject(t, client, component)))
			}
			require.Equal(t, test.wantFans, fans)
			require.Equal(t, test.wantTemperatures, temperatures)
			require.Equal(t, test.wantValidReadings, valid)
		})
	}
}

func TestCompatibilityFixtureLegacySourceHealthIsAuthoritative(t *testing.T) {
	client := fixtureClient()
	components, complete, err := client.legacyComponents(
		fixtureParent(),
		graphRelationship{
			ChildKind: "legacy_thermal",
		},
		testutil.LoadFixture(t, "../../testdata/"+"otel-hpe-legacy-thermal.min.json"),
	)
	require.NoError(t, err)
	require.True(t, complete)
	temperature := fixtureComponentByName(t, components, "Synthetic Hot Intake")
	observations := measurementTestProject(t, client, temperature)
	readings := measurementTestReadings(observations)
	require.Len(t, readings, 1)
	require.Equal(t, float64(70), readings[0].Value)
	measurementTestRequireAlarm(t, observations, "clear")
}

func TestCompatibilityFixtureLegacyPowerAndModernDrive(t *testing.T) {
	client := fixtureClient()
	parent := fixtureParent()
	components, complete, err := client.legacyComponents(
		parent,
		graphRelationship{
			ChildKind: "legacy_power",
		},
		testutil.LoadFixture(t, "../../testdata/"+"telegraf-dell-legacy-power.min.json"),
	)
	require.NoError(t, err)
	require.True(t, complete)
	kinds := make(map[string]int)
	families := make(map[string]int)
	for _, component := range components {
		kinds[component.Kind]++
		for _, reading := range measurementTestReadings(measurementTestProject(t, client, component)) {
			families[measurementTestLabel(reading, "reading_type")]++
		}
	}
	require.Equal(t, map[string]int{"power_supply": 1, "sensor": 2}, kinds)
	require.Equal(t, map[string]int{"power": 1, "voltage": 1}, families)

	drive := &graphNode{
		Resource: measurement.Resource{
			Kind: "drive",
			Key:  "drive",
			Data: testutil.LoadFixture(t, "../../testdata/"+"telegraf-hpe-modern-drive.min.json"),
		},
	}
	measurementTestRequireValue(t, measurementTestProject(t, client, drive), "drive_media_life", 98)
}

func fixtureClient() *Client {
	root, err := url.Parse("https://fixture.example/redfish/v1/")
	if err != nil {
		panic(err)
	}
	client := &Client{
		connection: connection{
			origin: "https://fixture.example",
			root:   root,
		},
	}
	return client
}

func fixtureGraphParent(t *testing.T, kind string, value any) *graphNode {
	t.Helper()
	data, ok := value.(map[string]any)
	require.True(t, ok)
	uri, _ := measurement.Properties(data).Text("@odata.id")
	if uri == "" {
		uri = "/redfish/v1/Synthetic/" + kind
	}
	return &graphNode{
		Resource: measurement.Resource{
			Kind:             kind,
			Key:              "fixture-" + kind,
			URI:              uri,
			Data:             data,
			AcquisitionState: "readable",
		},
		Locator:         uri,
		IdentityQuality: "addressable",
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
		Resource: measurement.Resource{
			Kind: "chassis",
			Key:  "fixture-chassis",
			URI:  "/redfish/v1/Chassis/Fixture-1",
		},
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
