// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
	"github.com/stretchr/testify/require"
)

func TestNormalizeInventoryValueHonorsColumnType(t *testing.T) {
	tests := map[string]struct {
		value any
		typ   registry.ColumnType
		want  any
	}{
		"string":             {"value", registry.ColumnString, "value"},
		"enum":               {"Ok", registry.ColumnEnum, "Ok"},
		"boolean":            {true, registry.ColumnBoolean, true},
		"integer":            {json.Number("42"), registry.ColumnInteger, int64(42)},
		"float":              {json.Number("42.5"), registry.ColumnFloat, 42.5},
		"reject string bool": {"true", registry.ColumnBoolean, nil},
		"reject fraction":    {42.5, registry.ColumnInteger, nil},
		"reject large float32 integer": {
			float32(math.MaxFloat32), registry.ColumnInteger, nil,
		},
		"reject number text": {json.Number("42"), registry.ColumnString, nil},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeInventoryValue(test.value, test.typ, false); got != test.want {
				t.Fatalf("normalizeInventoryValue() = %#v, want %#v", got, test.want)
			}
		})
	}

	if got := normalizeInventoryValue([]any{"a", "b"}, registry.ColumnString, true); got != `["a","b"]` {
		t.Fatalf("structured array = %#v", got)
	}
	if got := normalizeInventoryValue(map[string]any{"a": true}, registry.ColumnString, true); got != `{"a":true}` {
		t.Fatalf("structured object = %#v", got)
	}
	for _, value := range []any{"scalar", true, json.Number("1")} {
		if got := normalizeInventoryValue(value, registry.ColumnString, true); got != nil {
			t.Errorf("structured scalar %#v = %#v, want nil", value, got)
		}
	}
}

func TestAggregateLabelsEnforcePromotedLabelPolicy(t *testing.T) {
	client := &protocolClient{origin: "https://bmc.example.test", endpointJob: "job"}
	owner := placementTestNode("chassis", "owner", "/redfish/v1/Chassis/1", nil)
	owner.Doc.Name = strings.Repeat("x", promotedLabelLimit+1)
	owner.RollupOwner = placementTestNode("system", "previous", "/redfish/v1/Systems/1", nil)
	owner.RollupOwner.Doc.Name = "stale owner"

	labels := client.aggregateLabels(owner, aggregateSnapshot{
		Family:              " fan ",
		Role:                "   ",
		PhysicalContext:     strings.Repeat("y", promotedLabelLimit+1),
		SemanticSourceClass: " standard ",
	}, "group")

	require.Equal(t, "fan", observationLabel(labels, "component_family"))
	require.Equal(t, "standard", observationLabel(labels, "semantic_source_class"))
	require.Empty(t, observationLabel(labels, "rollup_owner_name"))
	require.Empty(t, observationLabel(labels, "aggregate_role"))
	require.Empty(t, observationLabel(labels, "physical_context"))
}

func TestScaleInventoryValueRejectsOverflowNonFiniteAndFraction(t *testing.T) {
	tests := map[string]struct {
		value any
		scale registry.Rational
		want  any
	}{
		"integer":          {int64(2), registry.Rational{Num: 1024, Den: 1}, int64(2048)},
		"integer fraction": {int64(1), registry.Rational{Num: 1, Den: 2}, nil},
		"integer overflow": {int64(math.MaxInt64), registry.Rational{Num: 2, Den: 1}, nil},
		"float":            {1.5, registry.Rational{Num: 2, Den: 1}, 3.0},
		"float overflow":   {math.MaxFloat64, registry.Rational{Num: 2, Den: 1}, nil},
		"unscaled":         {int64(7), registry.Rational{}, int64(7)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := scaleInventoryValue(test.value, test.scale); got != test.want {
				t.Fatalf("scaleInventoryValue() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestScaleInventoryNumberToIntegerIsExactAndBounded(t *testing.T) {
	tests := map[string]struct {
		value any
		scale registry.Rational
		want  any
	}{
		"fractional source with integral result": {
			json.Number("1.5"), registry.Rational{Num: 1_000_000_000, Den: 1}, int64(1_500_000_000),
		},
		"fractional result": {
			json.Number("0.0000000001"), registry.Rational{Num: 1_000_000_000, Den: 1}, nil,
		},
		"positive overflow": {
			json.Number("9223372036854775807"), registry.Rational{Num: 1_000_000_000, Den: 1}, nil,
		},
		"negative overflow": {
			json.Number("-9223372036854775808"), registry.Rational{Num: 1_000_000_000, Den: 1}, nil,
		},
		"non-finite float": {
			math.Inf(1), registry.Rational{Num: 1_000_000_000, Den: 1}, nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := scaleInventoryNumberToInteger(test.value, test.scale); got != test.want {
				t.Fatalf("scaleInventoryNumberToInteger() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestValidateReadingIdentitiesRejectsOnlyDifferentPreimages(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	require.NoError(t, validateReadingIdentities(map[string][]normalizedReading{
		"node-a": {
			{Key: key, IdentitySource: "Sensor.Reading"},
			{Key: key, IdentitySource: "Sensor.Reading"},
		},
	}))
	require.ErrorContains(t, validateReadingIdentities(map[string][]normalizedReading{
		"node-a": {{Key: key, IdentitySource: "Sensor.Reading"}},
		"node-b": {{Key: key, IdentitySource: "Sensor.Reading"}},
	}), "Redfish reading-key collision")
}

func TestReadingIdentityRegistryRejectsCrossCycleKeyReuse(t *testing.T) {
	client := &protocolClient{}
	const key = "0123456789abcdef0123456789abcdef"
	require.NoError(t, client.validateAndRegisterReadingIdentities(map[string][]normalizedReading{
		"node-a": {{Key: key, IdentitySource: "Sensor.Reading"}},
	}))
	err := client.validateAndRegisterReadingIdentities(map[string][]normalizedReading{
		"node-b": {{Key: key, IdentitySource: "Sensor.Reading"}},
	})
	require.ErrorContains(t, err, "Redfish reading-key collision")
	require.True(t, identityIntegrityError(err))
}

func TestManagerClockValueUsesRequestMidpoint(t *testing.T) {
	started := time.Now()
	finished := started.Add(2 * time.Second)
	managerTime := started.Round(0).Add(6 * time.Second)
	node := &graphNode{
		Kind: "manager",
		Data: map[string]any{"DateTime": managerTime.Format(time.RFC3339Nano)},
		Response: responseMetadata{
			StartedAt:  started,
			FinishedAt: finished,
		},
	}

	value, present, diagnostic := managerClockValue(node)
	if !present || diagnostic != "" || !value.Valid {
		t.Fatalf("managerClockValue() = (%+v, %t, %q), want a valid value", value, present, diagnostic)
	}
	if math.Abs(value.Value-5) > 0.000001 {
		t.Fatalf("clock offset = %f seconds, want 5", value.Value)
	}
}

func TestManagerClockValueRequiresExplicitOffset(t *testing.T) {
	started := time.Now()
	node := &graphNode{
		Kind: "manager",
		Data: map[string]any{"DateTime": "2026-07-30T12:00:00"},
		Response: responseMetadata{
			StartedAt:  started,
			FinishedAt: started.Add(time.Second),
		},
	}
	_, present, diagnostic := managerClockValue(node)
	if !present || diagnostic != "DateTime has no explicit UTC offset" {
		t.Fatalf("managerClockValue() = present %t diagnostic %q", present, diagnostic)
	}
}

func TestControlMetricsRequireApprovedTypeUnitPair(t *testing.T) {
	for _, controlType := range []string{"Pressure", "PressurekPa"} {
		t.Run(controlType, func(t *testing.T) {
			client := &protocolClient{}
			node := &graphNode{
				Kind: "control",
				Data: map[string]any{
					"ControlType":   controlType,
					"SetPointUnits": "kPa",
					"SetPoint":      json.Number("12.5"),
				},
			}

			value, found := scalarValueByID(client.scalarValues(node, time.Now()), "control_pressure_setpoint")
			if !found {
				t.Fatalf("approved %s/kPa control metric is missing", controlType)
			}
			if value.Value != 12_500 {
				t.Fatalf("normalized pressure set point = %f, want 12500", value.Value)
			}

			node.Data["SetPointUnits"] = "Pa"
			if _, found := scalarValueByID(client.scalarValues(node, time.Now()), "control_pressure_setpoint"); found {
				t.Fatalf("contradictory %s/Pa control pair became operational", controlType)
			}
		})
	}
}

func TestMemoryThroughputUsesTopLevelBlockSize(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	node := &graphNode{
		Kind: "memory",
		Enrichment: map[string]map[string]any{
			"memory_metrics": {
				"BlockSizeBytes": json.Number("4096"),
				"CurrentPeriod": map[string]any{
					"BlocksRead": json.Number("2"),
				},
			},
		},
	}

	value, found := scalarValueByID(client.scalarValues(node, time.Now()), "memory_current_period_blocks_read")
	if !found || !value.Valid {
		t.Fatalf("memory throughput value = %+v present=%t", value, found)
	}
	if value.Inventory != float64(8192) {
		t.Fatalf("memory throughput source total = %#v, want 8192 bytes", value.Inventory)
	}
	if value.MultiplierColumn != "memory_block_size_bytes" || value.MultiplierValue != float64(4096) {
		t.Fatalf("memory throughput multiplier = %q/%#v, want memory_block_size_bytes/4096", value.MultiplierColumn, value.MultiplierValue)
	}
}

func TestEveryRegistryScalarFieldHasRuntimeProducer(t *testing.T) {
	at := time.Now()
	for _, descriptor := range standardRegistry.Fields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			requireRegistryDescriptorRuntimeValue(t, descriptor, at)
		})
	}
}

func TestScalarFallbackRetainsSelectedSourceAndPreferredFailureProvenance(t *testing.T) {
	descriptor := registry.FieldSpec{
		ID:        "test_fallback",
		Kind:      "processor",
		Column:    "test_fallback",
		Algorithm: registry.AlgorithmAbsolute,
		Scale:     registry.Identity,
		Candidates: []registry.SourceCandidate{
			{Document: "processor_metrics", Path: "Preferred"},
			{Path: "Fallback"},
		},
	}
	node := &graphNode{
		Kind: "processor",
		Key:  "processor",
		Data: map[string]any{
			"Fallback": json.Number("42"),
		},
		Enrichment: map[string]map[string]any{
			"processor_metrics": {"Preferred": "not-a-number"},
		},
	}

	original := standardRegistry.Fields
	standardRegistry.Fields = []registry.FieldSpec{descriptor}
	t.Cleanup(func() { standardRegistry.Fields = original })

	values := (&protocolClient{}).scalarValues(node, time.Now())
	if len(values) != 1 {
		t.Fatalf("scalarValues() returned %d values, want 1", len(values))
	}
	got := values[0]
	if got.Value != 42 || got.SelectedSource != "Fallback" {
		t.Fatalf("fallback value = %+v, want value 42 from Fallback", got)
	}
	if len(got.SourceFailures) != 1 ||
		!strings.Contains(got.SourceFailures[0], "processor_metrics.Preferred") ||
		!strings.Contains(got.SourceFailures[0], "malformed") {
		t.Fatalf("fallback failure provenance = %v", got.SourceFailures)
	}
}

func TestFindEnrichmentRejectsAmbiguousDocumentKind(t *testing.T) {
	node := &graphNode{Enrichment: map[string]map[string]any{
		"memory_metrics:0": {"Reading": json.Number("1")},
		"memory_metrics:1": {"Reading": json.Number("2")},
	}}
	require.Nil(t, findEnrichment(node, "memory_metrics"))

	delete(node.Enrichment, "memory_metrics:1")
	require.Equal(t, json.Number("1"), findEnrichment(node, "memory_metrics")["Reading"])
}

func TestEveryRegistryStateAndFlagHasRuntimeProducer(t *testing.T) {
	client := &protocolClient{}
	for _, source := range standardRegistry.States {
		t.Run("state/"+source.Metric, func(t *testing.T) {
			node := &graphNode{
				Kind:       string(source.Kind),
				Key:        source.Metric,
				Data:       make(map[string]any),
				Enrichment: make(map[string]map[string]any),
			}
			document := registryTestDocument(node, source.Document)
			value := any(source.States[0])
			wantState := source.States[0]
			if source.BooleanFalse != "" || source.BooleanTrue != "" {
				value = false
				wantState = source.BooleanFalse
			}
			setRegistryTestPath(document, source.Path, value)
			observation := observationByMetric(client.statusObservations(node), source.Metric)
			if observation == nil {
				t.Fatalf("state source %s.%s has no runtime observation", source.Document, source.Path)
			}
			if observation.State != wantState {
				t.Fatalf("state = %q, want %q", observation.State, wantState)
			}
		})
	}

	for _, set := range standardRegistry.Flags {
		t.Run("flags/"+set.Metric, func(t *testing.T) {
			node := &graphNode{
				Kind:       string(set.Kind),
				Key:        set.Metric,
				Data:       make(map[string]any),
				Enrichment: make(map[string]map[string]any),
			}
			document := registryTestDocument(node, set.Document)
			for _, member := range set.Members {
				setRegistryTestPath(document, member.Path, true)
			}
			values := flagValues(node)
			observations := client.flagObservations(node, values)
			for _, member := range set.Members {
				metric := set.Metric + "_" + member.Role
				observation := observationByMetric(observations, metric)
				if observation == nil {
					t.Errorf("flag source %s.%s has no runtime observation %q", set.Document, member.Path, metric)
					continue
				}
				want := float64(1)
				if member.Invert {
					want = 0
				}
				if observation.Value != want {
					t.Errorf("%s value = %v, want %v", metric, observation.Value, want)
				}
			}
		})
	}
}

func TestEveryRegistryReadingSurfaceHasRuntimeNormalizer(t *testing.T) {
	for _, surface := range standardRegistry.Readings {
		if surface.DerivedFromEnergy {
			continue
		}
		name := strings.Join(
			[]string{surface.Family, surface.Basis, surface.Role, surface.SemanticClass},
			"/",
		)
		t.Run(name, func(t *testing.T) {
			sourceType, sourceUnits, fixed := registryReadingSource(surface.Family)
			raw := rawReading{
				Path:           "Synthetic." + name,
				IdentitySource: "Synthetic." + name,
				Type:           sourceType,
				Units:          sourceUnits,
				Basis:          surface.Basis,
				Role:           surface.Role,
				Value:          json.Number("10"),
				Primary:        true,
				ReadingScoped:  surface.AlarmMetric != "",
				Health:         "OK",
				Inventory:      make(map[string]any),
			}
			if fixed {
				raw.Inventory["fixed_family"] = surface.Family
			}
			node := &graphNode{Kind: "sensor", Key: name}
			switch surface.SemanticClass {
			case "fan":
				node.Kind = "fan"
			case "", "direct", "ambient_pressure":
			default:
				t.Fatalf("unrecognized semantic class %q", surface.SemanticClass)
			}

			reading := normalizeReading(node, raw, true)
			if !reading.Valid {
				t.Fatalf("reading is invalid: %+v", reading)
			}
			if reading.Metric != surface.Metric ||
				reading.Context != surface.Context ||
				reading.Exposure != surface.Exposure ||
				reading.AlarmMetric != surface.AlarmMetric ||
				reading.AggregateSemantic != surface.AggregateMetric {
				t.Fatalf(
					"normalized surface = metric %q context %q exposure %q alarm %q aggregate %q; want %+v",
					reading.Metric,
					reading.Context,
					reading.Exposure,
					reading.AlarmMetric,
					reading.AggregateSemantic,
					surface,
				)
			}
			observations := (&protocolClient{}).readingObservations(node, reading)
			if observationByMetric(observations, surface.Metric) == nil {
				t.Fatalf("surface metric %q has no runtime observation", surface.Metric)
			}
			if surface.AlarmMetric != "" &&
				observationByMetric(observations, surface.AlarmMetric) == nil {
				t.Fatalf("alarm metric %q has no runtime observation", surface.AlarmMetric)
			}
			for _, observation := range observations {
				if observation.Metric == "" {
					t.Fatal("runtime emitted an empty metric identifier")
				}
			}
		})
	}
}

func registryReadingSource(family string) (sourceType, sourceUnits string, fixed bool) {
	for _, source := range standardRegistry.ReadingTypes {
		if source.Family == family && len(source.SourceUnits) > 0 {
			return source.SourceType, source.SourceUnits[0], false
		}
	}
	return "Synthetic", "Synthetic", true
}

func requireRegistryDescriptorRuntimeValue(
	t *testing.T,
	descriptor registry.FieldSpec,
	at time.Time,
) {
	t.Helper()
	if len(descriptor.Candidates) == 0 {
		t.Fatal("descriptor has no source candidates")
	}
	source := descriptor.Candidates[0]
	node := &graphNode{
		Kind:       string(descriptor.Kind),
		Key:        descriptor.ID,
		Data:       make(map[string]any),
		Enrichment: make(map[string]map[string]any),
	}
	document := registryTestDocument(node, source.Document)
	for _, requirement := range source.Requires {
		setRegistryTestPath(document, requirement.Path, requirement.Value)
	}
	setRegistryTestPath(document, source.Path, json.Number("10"))
	if source.MultiplierPath != "" {
		multiplierDocument := document
		if source.MultiplierDocument != "" {
			multiplierDocument = registryTestDocument(node, source.MultiplierDocument)
		}
		setRegistryTestPath(multiplierDocument, source.MultiplierPath, json.Number("2"))
	}

	client := &protocolClient{}
	client.hardwareState.initialize()
	values := client.scalarValues(node, at)
	first, ok := scalarValueByID(values, descriptor.ID)
	if !ok {
		t.Fatalf(
			"first source %s did not reach scalarValues; node=%#v",
			sourcePath(source),
			node,
		)
	}
	if !first.Present || !first.Valid {
		t.Fatalf("first scalar = %+v, want present and valid", first)
	}
	if descriptor.Algorithm == registry.AlgorithmAbsolute {
		if !first.Emit {
			t.Fatalf("absolute scalar = %+v, want emitted", first)
		}
		return
	}
	if first.Emit {
		t.Fatalf("first counter sample = %+v, want baseline only", first)
	}

	setRegistryTestPath(document, source.Path, json.Number("11"))
	values = client.scalarValues(node, at.Add(100*time.Second))
	second, ok := scalarValueByID(values, descriptor.ID)
	if !ok || !second.Present || !second.Valid || !second.Emit {
		t.Fatalf("second counter sample = %+v present=%t, want emitted", second, ok)
	}
}

func registryTestDocument(node *graphNode, document registry.Document) map[string]any {
	if document == "" {
		return node.Data
	}
	key := string(document)
	value := node.Enrichment[key]
	if value == nil {
		value = make(map[string]any)
		node.Enrichment[key] = value
	}
	return value
}

func setRegistryTestPath(document map[string]any, path string, value any) {
	const countAnnotation = ".@odata.count"
	if before, ok := strings.CutSuffix(path, countAnnotation); ok {
		propertyPath := before
		segments := strings.Split(propertyPath, ".")
		current := document
		for _, segment := range segments[:len(segments)-1] {
			next, ok := current[segment].(map[string]any)
			if !ok {
				next = make(map[string]any)
				current[segment] = next
			}
			current = next
		}
		current[segments[len(segments)-1]+"@odata.count"] = value
		return
	}
	segments := strings.Split(path, ".")
	current := document
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}

func scalarValueByID(values []scalarValue, id string) (scalarValue, bool) {
	for _, value := range values {
		if value.Descriptor.ID == id {
			return value, true
		}
	}
	return scalarValue{}, false
}

func observationByMetric(values []hardwareObservation, metric string) *hardwareObservation {
	for index := range values {
		if values[index].Metric == metric {
			return &values[index]
		}
	}
	return nil
}

func TestNVMETemperatureArrayProducesDistinctReadings(t *testing.T) {
	client := &protocolClient{}
	parent := &graphNode{
		Kind: "drive",
		Key:  "drive-key",
		URI:  "/redfish/v1/Drives/1",
	}
	data := map[string]any{
		"NVMeSMART": map[string]any{
			"TemperatureSensorsCelsius": []any{json.Number("41"), json.Number("43")},
		},
	}
	nodes, complete, err := client.sensorExcerptArrayNodes(
		withOperationBudget(context.Background()),
		&resourceGraph{},
		parent,
		"Metrics.NVMeSMART.TemperatureSensorsCelsius",
		sensorExcerptArraySpec{
			Path: "NVMeSMART.TemperatureSensorsCelsius", Type: "Temperature", Units: "Cel",
			ScalarMembers: true,
		},
		data,
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, nodes, 2)

	var readings []normalizedReading
	for _, node := range nodes {
		values := client.readingsForNode(node, time.Now())
		require.Len(t, values, 1)
		readings = append(readings, values[0])
	}
	if readings[0].Key == readings[1].Key {
		t.Fatal("NVMe component readings share one identity")
	}
	if nodes[0].Key == nodes[1].Key {
		t.Fatal("NVMe array members share one component identity")
	}
	if readings[0].Value != 41 || readings[1].Value != 43 {
		t.Fatalf("NVMe readings = %f, %f; want 41, 43", readings[0].Value, readings[1].Value)
	}
}

func TestArrayReadingDataSourceURIStabilizesIdentityAcrossReorder(t *testing.T) {
	root, origin, err := normalizeServiceRoot("https://bmc.example/redfish/v1/")
	if err != nil {
		t.Fatal(err)
	}
	client := &protocolClient{root: root, origin: origin}
	parent := &graphNode{
		Kind: "thermal_subsystem",
		Key:  "thermal-key",
		URI:  "/redfish/v1/Chassis/1/ThermalSubsystem",
	}
	makeNodes := func(values ...map[string]any) []*graphNode {
		array := make([]any, len(values))
		for index := range values {
			array[index] = values[index]
		}
		nodes, complete, err := client.sensorExcerptArrayNodes(
			withOperationBudget(context.Background()),
			&resourceGraph{},
			parent,
			"ThermalMetrics.TemperatureReadingsCelsius",
			sensorExcerptArraySpec{
				Path: "TemperatureReadingsCelsius", Type: "Temperature", Units: "Cel",
			},
			map[string]any{"TemperatureReadingsCelsius": array},
		)
		require.NoError(t, err)
		require.True(t, complete)
		return nodes
	}
	firstSensor := map[string]any{"Reading": json.Number("41"), "DataSourceUri": "/redfish/v1/Sensors/A"}
	secondSensor := map[string]any{"Reading": json.Number("43"), "DataSourceUri": "/redfish/v1/Sensors/B"}

	var first, second []normalizedReading
	for _, node := range makeNodes(firstSensor, secondSensor) {
		first = append(first, client.readingsForNode(node, time.Now())...)
	}
	for _, node := range makeNodes(secondSensor, firstSensor) {
		second = append(second, client.readingsForNode(node, time.Now())...)
	}
	firstKeys, secondKeys := make(map[float64]string), make(map[float64]string)
	for _, reading := range first {
		firstKeys[reading.Value] = reading.Key
	}
	for _, reading := range second {
		secondKeys[reading.Value] = reading.Key
	}
	if firstKeys[41] != secondKeys[41] || firstKeys[43] != secondKeys[43] {
		t.Fatalf("reading keys changed across reorder: first=%v second=%v", firstKeys, secondKeys)
	}
}

func TestSensorExcerptArrayCardinalityGateCountsComponentsNotReadings(t *testing.T) {
	details := true
	cap := 100
	root, origin, err := normalizeServiceRoot("https://bmc.example/redfish/v1/")
	require.NoError(t, err)
	client := &protocolClient{
		root: root, origin: origin,
		config: Config{
			Charts: ChartsConfig{
				Details:                        &details,
				MaxDetailedComponentsPerFamily: &cap,
			},
		},
	}
	client.hardwareState.initialize()
	owner := &graphNode{
		Kind: "thermal_subsystem",
		Key:  "thermal",
		URI:  "/redfish/v1/Chassis/1/ThermalSubsystem",
	}
	array := make([]any, 101)
	for index := range array {
		array[index] = map[string]any{
			"Reading":       json.Number(fmt.Sprint(20 + index%10)),
			"DataSourceUri": fmt.Sprintf("/redfish/v1/Sensors/Temperature-%03d", index),
		}
	}
	nodes, complete, err := client.sensorExcerptArrayNodes(
		withOperationBudget(context.Background()),
		&resourceGraph{},
		owner,
		"ThermalMetrics.TemperatureReadingsCelsius",
		sensorExcerptArraySpec{
			Path: "TemperatureReadingsCelsius", Type: "Temperature", Units: "Cel",
		},
		map[string]any{"TemperatureReadingsCelsius": array},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, nodes, 101)

	readings := make(map[string][]normalizedReading, len(nodes))
	for _, node := range nodes {
		node.LogicalOwner = owner
		readings[node.Key] = client.readingsForNode(node, time.Now())
		require.Len(t, readings[node.Key], 1)
	}
	const gateKey = "thermal\x00sensor.temperature"
	gate := client.detailGates(
		nodes,
		readings,
		map[string]bool{gateKey: true},
		true,
	)[gateKey]
	require.Equal(t, 101, gate.Count)
	require.False(t, gate.Open)
	require.Len(t, gate.Members, 101)
}

func TestSensorExcerptDataSourceProofMergesWithAddressableSensor(t *testing.T) {
	root, origin, err := normalizeServiceRoot("https://bmc.example/redfish/v1/")
	require.NoError(t, err)
	client := &protocolClient{root: root, origin: origin}
	addressable := &graphNode{
		Kind:             "sensor",
		Key:              resourceKey(origin, "sensor", "/redfish/v1/Sensors/A"),
		URI:              "/redfish/v1/Sensors/A",
		Locator:          "/redfish/v1/Sensors/A",
		SourceModel:      "typed_resource",
		IdentityQuality:  "addressable",
		AcquisitionState: "readable",
		Data: map[string]any{
			"Reading":      json.Number("42"),
			"ReadingType":  "Temperature",
			"ReadingUnits": "Cel",
		},
	}
	graph := &resourceGraph{Nodes: []*graphNode{addressable}}
	owner := &graphNode{Kind: "thermal_subsystem", Key: "thermal", URI: "/redfish/v1/ThermalSubsystem"}
	nodes, complete, err := client.sensorExcerptArrayNodes(
		withOperationBudget(context.Background()),
		graph,
		owner,
		"ThermalMetrics.TemperatureReadingsCelsius",
		sensorExcerptArraySpec{
			Path: "TemperatureReadingsCelsius", Type: "Temperature", Units: "Cel",
		},
		map[string]any{
			"TemperatureReadingsCelsius": []any{map[string]any{
				"Reading":       json.Number("43"),
				"DataSourceUri": "/redfish/v1/Sensors/A#/Reading",
				"Thresholds": map[string]any{
					"UpperCritical": map[string]any{"Reading": json.Number("40")},
				},
			}},
		},
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, nodes, 1)
	require.Equal(t, addressable.Locator, nodes[0].Locator)

	mergeEquivalentGraphNode(addressable, nodes[0])
	readings := client.readingsForNode(addressable, time.Now())
	require.Len(t, readings, 1)
	require.Equal(t, float64(42), readings[0].Value, "the addressable Sensor reading has precedence")
	require.Equal(t, "critical", readings[0].DerivedAlarm, "the excerpt can provide complementary thresholds")
}

func TestReadingInventoryProducesTypedRawAndNormalizedValues(t *testing.T) {
	client := &protocolClient{}
	node := &graphNode{
		Kind: "sensor",
		Key:  "sensor-key",
		Data: map[string]any{
			"ReadingType":               "PressurekPa",
			"ReadingUnits":              "kPa",
			"Reading":                   json.Number("12.5"),
			"ReadingRangeMin":           json.Number("1"),
			"ReadingRangeMax":           json.Number("20"),
			"AverageReading":            json.Number("10"),
			"ReadingAccuracy":           json.Number("0.1"),
			"Calibration":               json.Number("0.2"),
			"Accuracy":                  json.Number("1.5"),
			"AveragingInterval":         "PT1M30S",
			"AveragingIntervalAchieved": true,
			"ReadingTime":               "2026-07-30T12:00:00Z",
			"Implementation":            "PhysicalSensor",
			"Thresholds": map[string]any{
				"UpperCritical": map[string]any{
					"Reading":            json.Number("18"),
					"Activation":         "Increasing",
					"DwellTime":          "PT5S",
					"HysteresisDuration": "PT2S",
					"HysteresisReading":  json.Number("0.5"),
				},
			},
		},
	}

	readings := client.readingsForNode(node, time.Now())
	if len(readings) == 0 {
		t.Fatal("sensor reading is missing")
	}
	inventory := readings[0].Inventory
	for key, want := range map[string]any{
		"reading_range_min_source":                             float64(1),
		"reading_range_min":                                    float64(1_000),
		"reading_range_max_source":                             float64(20),
		"reading_range_max":                                    float64(20_000),
		"reading_average_source":                               float64(10),
		"reading_average":                                      float64(10_000),
		"reading_accuracy_source":                              float64(0.1),
		"reading_accuracy":                                     float64(100),
		"calibration_source":                                   float64(0.2),
		"calibration":                                          float64(200),
		"accuracy_percent":                                     float64(1.5),
		"averaging_interval":                                   float64(90),
		"averaging_interval_achieved":                          true,
		"reading_time":                                         time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC).UnixMilli(),
		"threshold_upper_critical_source":                      float64(18),
		"threshold_upper_critical":                             float64(18_000),
		"threshold_upper_critical_dwell_seconds":               float64(5),
		"threshold_upper_critical_hysteresis_duration_seconds": float64(2),
		"threshold_upper_critical_hysteresis_source":           float64(0.5),
		"threshold_upper_critical_hysteresis":                  float64(500),
	} {
		if got := inventory[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := inventory["fixed_family"]; ok {
		t.Fatal("internal fixed_family marker leaked into inventory")
	}
}

func TestReadingAlarmFusionUsesSourceAbnormalPrecedence(t *testing.T) {
	tests := map[string]struct {
		source, derived string
		wantState       string
		wantSource      string
	}{
		"source warning is not elevated": {
			source: "warning", derived: "critical", wantState: "warning", wantSource: "source",
		},
		"source critical is not elevated": {
			source: "critical", derived: "emergency", wantState: "critical", wantSource: "source",
		},
		"source clear can be elevated": {
			source: "clear", derived: "critical", wantState: "critical", wantSource: "combined",
		},
		"source clear remains clear": {
			source: "clear", derived: "clear", wantState: "clear", wantSource: "source",
		},
		"absent source uses derived": {
			derived: "warning", wantState: "warning", wantSource: "derived",
		},
		"source abnormal survives derived clear": {
			source: "warning", derived: "clear", wantState: "warning", wantSource: "source",
		},
		"source survives absent derived": {
			source: "critical", wantState: "critical", wantSource: "source",
		},
		"no usable result stays absent": {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state, source := fuseAlarm(test.source, test.derived)
			if state != test.wantState || source != test.wantSource {
				t.Fatalf(
					"fuseAlarm(%q, %q) = (%q, %q), want (%q, %q)",
					test.source,
					test.derived,
					state,
					source,
					test.wantState,
					test.wantSource,
				)
			}
		})
	}
}

func TestReadingAlarmManualEvaluationPolicy(t *testing.T) {
	node := &graphNode{Kind: "sensor", Key: "sensor-key"}
	raw := rawReading{
		Path:          "Reading",
		Type:          "Temperature",
		Units:         "Cel",
		Value:         json.Number("80"),
		ReadingScoped: true,
		Health:        "Warning",
		Thresholds: map[string]rawThreshold{
			"upper_critical": {
				Value: json.Number("70"), Activation: "Increasing",
			},
		},
	}

	disabled := normalizeReading(node, raw, false)
	if disabled.SourceAlarm != "warning" ||
		disabled.DerivedAlarm != "" ||
		disabled.EffectiveAlarm != "warning" ||
		disabled.EffectiveAlarmSource != "source" {
		t.Fatalf("disabled manual evaluation = %+v", disabled)
	}

	enabled := normalizeReading(node, raw, true)
	if enabled.SourceAlarm != "warning" ||
		enabled.DerivedAlarm != "critical" ||
		enabled.EffectiveAlarm != "warning" ||
		enabled.EffectiveAlarmSource != "source" {
		t.Fatalf("source-abnormal precedence = %+v", enabled)
	}

	raw.Health = "OK"
	sourceClear := normalizeReading(node, raw, true)
	if sourceClear.EffectiveAlarm != "critical" ||
		sourceClear.EffectiveAlarmSource != "combined" ||
		sourceClear.EffectiveAlarmReason != "threshold_upper_critical" {
		t.Fatalf("source-clear threshold fallback = %+v", sourceClear)
	}

	raw.Health = ""
	sourceAbsent := normalizeReading(node, raw, true)
	if sourceAbsent.EffectiveAlarm != "critical" ||
		sourceAbsent.EffectiveAlarmSource != "derived" {
		t.Fatalf("source-absent threshold fallback = %+v", sourceAbsent)
	}

	raw.Thresholds["upper_critical"] = rawThreshold{
		Value: json.Number("70"), Activation: "Disabled",
	}
	disabledThreshold := normalizeReading(node, raw, true)
	if disabledThreshold.DerivedAlarm != "" ||
		disabledThreshold.EffectiveAlarm != "" ||
		disabledThreshold.EffectiveAlarmSource != "" {
		t.Fatalf("disabled threshold handling = %+v", disabledThreshold)
	}
}

func TestReadingSourceAlarmDiagnostics(t *testing.T) {
	node := &graphNode{Kind: "sensor", Key: "sensor-key", URI: "/redfish/v1/Chassis/1/Sensors/1"}
	base := rawReading{
		Path:          "Reading",
		Type:          "Temperature",
		Units:         "Cel",
		Value:         json.Number("42"),
		ReadingScoped: true,
	}

	tests := map[string]struct {
		health         string
		wantAlarm      string
		wantDiagnostic string
	}{
		"missing":      {wantDiagnostic: "source alarm is missing"},
		"unrecognized": {health: "VendorUnknown", wantDiagnostic: "source alarm is unrecognized"},
		"valid":        {health: "Warning", wantAlarm: "warning"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			raw := base
			raw.Health = test.health
			reading := normalizeReading(node, raw, false)
			if reading.SourceAlarm != test.wantAlarm {
				t.Fatalf("source alarm = %q, want %q", reading.SourceAlarm, test.wantAlarm)
			}
			if !strings.Contains(reading.SourceAlarmDiagnostic, test.wantDiagnostic) {
				t.Fatalf("source alarm diagnostic = %q, want substring %q", reading.SourceAlarmDiagnostic, test.wantDiagnostic)
			}
		})
	}

	notReadingScoped := base
	notReadingScoped.ReadingScoped = false
	reading := normalizeReading(node, notReadingScoped, false)
	if reading.SourceAlarmDiagnostic != "" {
		t.Fatalf("ineligible resource-level health produced reading diagnostic %q", reading.SourceAlarmDiagnostic)
	}
}

func TestReadingSourceAlarmSurvivesInvalidNumericValue(t *testing.T) {
	node := &graphNode{Kind: "sensor", Key: "sensor-key", URI: "/redfish/v1/Chassis/1/Sensors/1"}
	reading := normalizeReading(node, rawReading{
		Path:          "Reading",
		Type:          "Temperature",
		Units:         "Cel",
		Value:         map[string]any{"invalid": true},
		ReadingScoped: true,
		Health:        "Critical",
	}, true)
	if reading.Valid || reading.SourceAlarm != "critical" || reading.EffectiveAlarm != "critical" {
		t.Fatalf("invalid numeric reading lost source alarm: %+v", reading)
	}

	observations := (&protocolClient{}).readingObservations(node, reading)
	if len(observations) != 1 || observations[0].Metric != reading.AlarmMetric {
		t.Fatalf("observations = %+v, want only source alarm %q", observations, reading.AlarmMetric)
	}
}

func TestReadingAlarmEmitsEveryClosedRegistryStateWithoutFabricatingMissing(t *testing.T) {
	client := &protocolClient{}
	node := &graphNode{Kind: "sensor", Key: "sensor"}
	for _, state := range registry.AlarmStates {
		reading := normalizedReading{
			Key: "reading", Metric: "reading_temperature_zero_input",
			AlarmMetric: "reading_temperature_zero_input_alarm",
			Valid:       true, EffectiveAlarm: state,
		}
		observation := observationByMetric(client.readingObservations(node, reading), reading.AlarmMetric)
		if observation == nil || observation.State != state || !slices.Equal(observation.States, registry.AlarmStates) {
			t.Fatalf("alarm state %q observation = %#v", state, observation)
		}
	}
	reading := normalizedReading{
		Key: "reading", Metric: "reading_temperature_zero_input",
		AlarmMetric: "reading_temperature_zero_input_alarm", Valid: true,
	}
	if observation := observationByMetric(client.readingObservations(node, reading), reading.AlarmMetric); observation != nil {
		t.Fatalf("missing effective alarm fabricated observation %#v", observation)
	}
}

func TestInventoryReadingRowPreservesSourceDerivedAndEffectiveAlarmEvidence(t *testing.T) {
	client := &protocolClient{
		config: Config{
			NodeMode: "local",
		},
	}
	node := &graphNode{
		Kind:             "sensor",
		Key:              "sensor-key",
		URI:              "/redfish/v1/Chassis/1/Sensors/Intake",
		Data:             map[string]any{"Name": "Intake"},
		AcquisitionState: "readable",
		IdentityQuality:  "stable",
		Complete:         true,
		Doc: genericResource{
			ID:   "Intake",
			Name: "Intake",
			Status: genericStatus{
				Health: "OK",
			},
		},
		SystemOwners: make(map[string]*graphNode),
	}
	reading := normalizedReading{
		Key:                  "reading-key",
		SourcePath:           "Sensor.Reading",
		Family:               "temperature",
		Units:                "Celsius",
		Basis:                "zero",
		Value:                80,
		Valid:                true,
		SourceAlarm:          "clear",
		DerivedAlarm:         "critical",
		EffectiveAlarm:       "critical",
		EffectiveAlarmSource: "combined",
		EffectiveAlarmReason: "threshold_upper_critical",
	}

	row := client.inventoryReadingRow(
		node,
		detailGate{Open: true, Complete: true, Count: 1},
		time.Unix(100, 0).UTC(),
		reading,
	)
	for key, want := range map[string]any{
		"source_alarm_state":     "clear",
		"derived_alarm_state":    "critical",
		"effective_alarm_state":  "critical",
		"effective_alarm_source": "combined",
		"effective_alarm_reason": "threshold_upper_critical",
		"severity":               "critical",
		"severity_rank":          0,
	} {
		if got := row[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestDerivedAlarmThresholdTruthMatrix(t *testing.T) {
	thresholds := map[string]rawThreshold{
		"lower_caution":  {Value: json.Number("10"), Activation: "Decreasing"},
		"upper_critical": {Value: json.Number("20"), Activation: "Increasing"},
		"upper_fatal":    {Value: json.Number("30"), Activation: "Increasing"},
	}
	tests := map[string]struct {
		value      float64
		wantState  string
		wantReason string
	}{
		"strictly below lower": {
			value: 9, wantState: "warning", wantReason: "threshold_lower_caution",
		},
		"equal lower is clear": {
			value: 10, wantState: "clear", wantReason: "thresholds_clear",
		},
		"inside range is clear": {
			value: 15, wantState: "clear", wantReason: "thresholds_clear",
		},
		"equal upper is clear": {
			value: 20, wantState: "clear", wantReason: "thresholds_clear",
		},
		"above critical": {
			value: 21, wantState: "critical", wantReason: "threshold_upper_critical",
		},
		"above fatal chooses worst": {
			value: 31, wantState: "emergency", wantReason: "threshold_upper_fatal",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state, reason := deriveAlarm(test.value, thresholds, 1)
			if state != test.wantState || reason != test.wantReason {
				t.Fatalf(
					"deriveAlarm(%v) = (%q, %q), want (%q, %q)",
					test.value,
					state,
					reason,
					test.wantState,
					test.wantReason,
				)
			}
		})
	}

	unusable := map[string]rawThreshold{
		"upper_critical": {Value: "not-a-number", Activation: "Increasing"},
		"lower_caution":  {Value: json.Number("10"), Activation: "Disabled"},
	}
	if state, reason := deriveAlarm(50, unusable, 1); state != "" || reason != "" {
		t.Fatalf("unusable thresholds = (%q, %q), want no derived result", state, reason)
	}
}

func TestDerivedPowerResetsBaselineOnReadingSemanticChange(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	node := &graphNode{
		Kind: "sensor",
		Key:  "energy-sensor",
		Data: map[string]any{
			"ReadingType":  "EnergyWh",
			"ReadingUnits": "W.h",
			"Reading":      json.Number("100"),
		},
	}
	t0 := time.Now()
	if got := derivedPowerCount(client.readingsForNode(node, t0)); got != 0 {
		t.Fatalf("first sample derived power count = %d, want 0", got)
	}

	node.Data["ReadingType"] = "EnergyJoules"
	node.Data["ReadingUnits"] = "J"
	node.Data["Reading"] = json.Number("370000")
	if got := derivedPowerCount(client.readingsForNode(node, t0.Add(time.Minute))); got != 0 {
		t.Fatalf("semantic-change sample derived power count = %d, want 0", got)
	}

	node.Data["Reading"] = json.Number("370060")
	postChange := client.readingsForNode(node, t0.Add(2*time.Minute))
	if got := derivedPowerCount(postChange); got != 1 {
		t.Fatalf("post-change sample derived power count = %d, want 1; readings=%#v baselines=%#v", got, postChange, client.rateBaselines)
	}
}

func TestRateBaselineRetentionBudgetKeepsExistingContinuity(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	client.rateBaselineLimit = 1
	t0 := time.Unix(100, 0)

	_, emit := client.rateValue("first", "1", 1, t0, registry.AlgorithmRate, "epoch")
	require.False(t, emit)
	_, emit = client.rateValue("second", "1", 1, t0, registry.AlgorithmRate, "epoch")
	require.False(t, emit)
	require.Contains(t, client.rateBaselines, "first")
	require.NotContains(t, client.rateBaselines, "second")
	require.True(t, client.takeRateRetentionOverflow())
	require.False(t, client.takeRateRetentionOverflow())

	value, emit := client.rateValue(
		"first",
		"61",
		1,
		t0.Add(time.Minute),
		registry.AlgorithmRate,
		"epoch",
	)
	require.True(t, emit)
	require.Equal(t, 1.0, value)
}

func TestRateBaselineResetsWhenMultiplierChanges(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	t0 := time.Unix(100, 0)

	_, emit := client.rateValue("memory-blocks", "100", 512, t0, registry.AlgorithmRate, "epoch")
	require.False(t, emit)
	_, emit = client.rateValue(
		"memory-blocks", "110", 4096, t0.Add(time.Second), registry.AlgorithmRate, "epoch",
	)
	require.False(t, emit)

	value, emit := client.rateValue(
		"memory-blocks", "120", 4096, t0.Add(2*time.Second), registry.AlgorithmRate, "epoch",
	)
	require.True(t, emit)
	require.Equal(t, 40_960.0, value)
}

func TestOversizedProtocolNumbersFailSoftWithoutRetainedBaselines(t *testing.T) {
	integer := strings.Repeat("0", 1<<20) + "1"
	fraction := "0." + strings.Repeat("0", 1<<20) + "1"

	for _, value := range []string{integer, fraction} {
		_, _, ok := numericValue(json.Number(value))
		require.False(t, ok)
		require.Nil(t, scaleInventoryNumberToInteger(
			json.Number(value),
			registry.Rational{Num: 1, Den: 1},
		))
	}
	_, _, ok := numericSourceValue("PT"+fraction+"S", registry.AlgorithmDurationPercent)
	require.False(t, ok)

	client := &protocolClient{}
	client.hardwareState.initialize()
	client.rateBaselineLimit = 1_000
	for index := range 1_000 {
		_, emit := client.rateValue(
			fmt.Sprintf("rotating-partial-resource-%d", index),
			fraction,
			1,
			time.Unix(int64(index), 0),
			registry.AlgorithmRate,
			"epoch",
		)
		require.False(t, emit)
	}
	require.Empty(t, client.rateBaselines)
}

func TestRateEpochsDigestOversizedBMCValues(t *testing.T) {
	first := strings.Repeat("a", 1<<20)
	second := strings.Repeat("b", 1<<20)

	managerFirst := rateEpoch(map[string]any{"LifetimeStartDateTime": first})
	managerSecond := rateEpoch(map[string]any{"LifetimeStartDateTime": second})
	require.Len(t, managerFirst, digestHexChars)
	require.Len(t, managerSecond, digestHexChars)
	require.NotEqual(t, managerFirst, managerSecond)

	reading := normalizedReading{
		SourcePath: "Reading",
		Inventory:  map[string]any{"sensor_reset_time": first},
	}
	readingFirst := readingRateEpoch(reading)
	reading.Inventory["sensor_reset_time"] = second
	readingSecond := readingRateEpoch(reading)
	require.Len(t, readingFirst, digestHexChars)
	require.Len(t, readingSecond, digestHexChars)
	require.NotEqual(t, readingFirst, readingSecond)
}

func TestDerivedPowerResetsBaselineOnDecreaseAndEpochChange(t *testing.T) {
	t.Parallel()

	client := &protocolClient{}
	client.hardwareState.initialize()
	node := &graphNode{
		Kind: "sensor",
		Key:  "energy-sensor-reset",
		Data: map[string]any{
			"ReadingType":  "EnergyJoules",
			"ReadingUnits": "J",
			"Reading":      json.Number("100"),
		},
	}
	t0 := time.Unix(100, 0)
	require.Zero(t, derivedPowerCount(client.readingsForNode(node, t0)))

	node.Data["Reading"] = json.Number("160")
	require.Equal(t, 1, derivedPowerCount(client.readingsForNode(node, t0.Add(time.Minute))))

	node.Data["Reading"] = json.Number("50")
	require.Zero(t, derivedPowerCount(client.readingsForNode(node, t0.Add(2*time.Minute))))
	node.Data["Reading"] = json.Number("110")
	require.Equal(t, 1, derivedPowerCount(client.readingsForNode(node, t0.Add(3*time.Minute))))

	node.Data["LifetimeStartDateTime"] = "2026-07-31T00:00:00Z"
	node.Data["Reading"] = json.Number("170")
	require.Zero(t, derivedPowerCount(client.readingsForNode(node, t0.Add(4*time.Minute))))
	node.Data["Reading"] = json.Number("230")
	require.Equal(t, 1, derivedPowerCount(client.readingsForNode(node, t0.Add(5*time.Minute))))
}

func TestSelectedSystemRetainsPriorInclusionDuringPartialOwnershipRefresh(t *testing.T) {
	root, origin, err := normalizeServiceRoot("https://bmc.example/redfish/v1/")
	if err != nil {
		t.Fatal(err)
	}
	client := &protocolClient{
		config: Config{SystemURI: "/redfish/v1/Systems/1"},
		root:   root, origin: origin,
		selectedSystemIncluded: make(map[string]struct{}),
	}
	selected := &graphNode{Kind: "system", URI: "/redfish/v1/Systems/1", Key: "system-1"}
	other := &graphNode{Kind: "system", URI: "/redfish/v1/Systems/2", Key: "system-2"}
	component := &graphNode{
		Kind:         "fan",
		Key:          "fan-1",
		SystemOwners: map[string]*graphNode{selected.Key: selected},
	}

	complete := &resourceGraph{Complete: true}
	if got := client.filterSelectedSystem(complete, []*graphNode{selected, other, component}); !containsNode(got, component.Key) {
		t.Fatal("selected component was not included in the complete topology")
	}

	component.SystemOwners = map[string]*graphNode{other.Key: other}
	partial := &resourceGraph{Complete: false}
	if got := client.filterSelectedSystem(partial, []*graphNode{selected, other, component}); !containsNode(got, component.Key) {
		t.Fatal("partial ownership refresh excluded a previously included component")
	}

	if got := client.filterSelectedSystem(complete, []*graphNode{selected, other, component}); containsNode(got, component.Key) {
		t.Fatal("complete ownership refresh retained a component proven exclusive to another system")
	}
}

func TestSelectedSystemDoesNotAdmitNewUnownedResourceDuringPartialRefresh(t *testing.T) {
	root, origin, err := normalizeServiceRoot("https://bmc.example/redfish/v1/")
	if err != nil {
		t.Fatal(err)
	}
	client := &protocolClient{
		config: Config{SystemURI: "/redfish/v1/Systems/1"},
		root:   root, origin: origin,
		selectedSystemIncluded: make(map[string]struct{}),
	}
	selected := &graphNode{Kind: "system", URI: "/redfish/v1/Systems/1", Key: "system-1"}
	service := &graphNode{Kind: "service", URI: "/redfish/v1/", Key: "service"}
	orphan := &graphNode{Kind: "fan", Key: "new-unowned-fan"}

	complete := &resourceGraph{Complete: true}
	client.filterSelectedSystem(complete, []*graphNode{service, selected})

	partial := &resourceGraph{Complete: false}
	got := client.filterSelectedSystem(partial, []*graphNode{service, selected, orphan})
	if containsNode(got, orphan.Key) {
		t.Fatal("partial ownership refresh admitted a newly unowned component")
	}
	if !containsNode(got, service.Key) {
		t.Fatal("partial ownership refresh excluded the service scope")
	}

	got = client.filterSelectedSystem(complete, []*graphNode{service, selected, orphan})
	if !containsNode(got, orphan.Key) {
		t.Fatal("complete ownership refresh did not admit a proven service-scoped component")
	}
}

func TestDetailCardinalityGateIsAtomicPerOwnerAndComponentFamily(t *testing.T) {
	cap := 1
	details := true
	client := &protocolClient{
		config: Config{
			Charts: ChartsConfig{
				Details:                        &details,
				MaxDetailedComponentsPerFamily: &cap,
			},
		},
	}
	client.hardwareState.initialize()
	ownerA := &graphNode{Kind: "system", Key: "system-a"}
	ownerB := &graphNode{Kind: "system", Key: "system-b"}
	fanA1 := &graphNode{Kind: "fan", Key: "fan-a-1", LogicalOwner: ownerA}
	fanA2 := &graphNode{Kind: "fan", Key: "fan-a-2", LogicalOwner: ownerA}
	driveA := &graphNode{Kind: "drive", Key: "drive-a", LogicalOwner: ownerA}
	fanB := &graphNode{Kind: "fan", Key: "fan-b", LogicalOwner: ownerB}
	nodes := []*graphNode{ownerA, ownerB, fanA1, fanA2, driveA, fanB}
	evidence := map[string]bool{
		"system-a\x00fan":   true,
		"system-a\x00drive": true,
		"system-b\x00fan":   true,
	}

	gates := client.detailGates(nodes, nil, evidence, true)
	if gate := gates["system-a\x00fan"]; gate.Open || gate.Count != 2 || len(gate.Members) != 2 {
		t.Fatalf("over-cap fan gate = %+v, want the complete family atomically closed", gate)
	}
	for _, key := range []string{"system-a\x00drive", "system-b\x00fan"} {
		if gate := gates[key]; !gate.Open || gate.Count != 1 || len(gate.Members) != 1 {
			t.Fatalf("independent gate %q = %+v, want open", key, gate)
		}
	}
	if detailAllowed(fanA1, gates["system-a\x00fan"]) ||
		detailAllowed(fanA2, gates["system-a\x00fan"]) {
		t.Fatal("closed component-family gate admitted an arbitrary subset")
	}
	if !detailAllowed(driveA, gates["system-a\x00drive"]) ||
		!detailAllowed(fanB, gates["system-b\x00fan"]) {
		t.Fatal("one over-cap family closed an independent owner or component family")
	}
}

func TestDetailCardinalityGateHandlesLargeFamilyWithoutSelection(t *testing.T) {
	const componentCount = 1_001
	cap := 100
	details := true
	client := &protocolClient{
		config: Config{
			Charts: ChartsConfig{
				Details:                        &details,
				MaxDetailedComponentsPerFamily: &cap,
			},
		},
	}
	client.hardwareState.initialize()

	owner := &graphNode{Kind: "system", Key: "system"}
	nodes := make([]*graphNode, componentCount+1)
	nodes[0] = owner
	for index := range componentCount {
		nodes[index+1] = &graphNode{
			Kind:         "temperature",
			Key:          fmt.Sprintf("temperature-%04d", index),
			LogicalOwner: owner,
		}
	}

	const gateKey = "system\x00temperature"
	gates := client.detailGates(nodes, nil, map[string]bool{gateKey: true}, true)
	gate := gates[gateKey]
	if gate.Open || gate.Count != componentCount || len(gate.Members) != componentCount {
		t.Fatalf("large-family gate = %+v, want all %d components atomically closed", gate, componentCount)
	}
	for _, node := range nodes[1:] {
		if detailAllowed(node, gate) {
			t.Fatalf("closed large-family gate selected component %q", node.Key)
		}
	}

	unlimited := 0
	client.config.Charts.MaxDetailedComponentsPerFamily = &unlimited
	gate = client.detailGates(nodes, nil, map[string]bool{gateKey: true}, true)[gateKey]
	if !gate.Open || gate.Count != componentCount || len(gate.Members) != componentCount {
		t.Fatalf("unlimited large-family gate = %+v, want all %d components open", gate, componentCount)
	}
}

func TestDetailGateRetentionBudgetDoesNotFilterCurrentComponents(t *testing.T) {
	details := true
	client := &protocolClient{config: Config{Charts: ChartsConfig{Details: &details}}}
	client.hardwareState.initialize()
	owner := &graphNode{Kind: "system", Key: "system"}
	first := &graphNode{Kind: "fan", Key: "fan-1", LogicalOwner: owner}
	second := &graphNode{Kind: "fan", Key: "fan-2", LogicalOwner: owner}
	const gateKey = "system\x00fan"

	gates, retained := client.detailGatesWithinBudget(
		[]*graphNode{owner, first},
		nil,
		map[string]bool{gateKey: true},
		false,
		retainedStateBudget{entries: 1, members: 1},
	)
	require.True(t, retained)
	require.Len(t, gates[gateKey].Members, 1)

	gates, retained = client.detailGatesWithinBudget(
		[]*graphNode{owner, first, second},
		nil,
		map[string]bool{gateKey: true},
		false,
		retainedStateBudget{entries: 1, members: 1},
	)
	require.False(t, retained)
	require.Len(t, gates[gateKey].Members, 2)
	require.Empty(t, client.detailGateState)
}

func TestDetailCardinalityEvidenceRequiresCompleteUnionAcrossSlices(t *testing.T) {
	owner := &graphNode{Kind: "system", Key: "system"}
	first := &graphNode{Kind: "fan", Key: "fan-1", LogicalOwner: owner}
	second := &graphNode{Kind: "fan", Key: "fan-2", LogicalOwner: owner}
	graph := &resourceGraph{
		Complete: false,
		Nodes:    []*graphNode{owner, first, second},
		Slices: []graphSlice{
			{ParentKey: owner.Key, Path: "FansA", ChildKind: "fan", Complete: true, Members: []string{first.Key}},
			{ParentKey: owner.Key, Path: "FansB", ChildKind: "fan", Complete: false, Members: []string{second.Key}},
		},
	}
	const key = "system\x00fan"
	if evidence := graph.detailEvidence(graph.Nodes, nil); evidence[key] {
		t.Fatal("one complete slice incorrectly proved the multi-slice component-family union complete")
	}

	graph.Complete = true
	graph.Slices[1].Complete = true
	if evidence := graph.detailEvidence(graph.Nodes, nil); !evidence[key] {
		t.Fatal("all complete contributing slices did not prove the component-family union complete")
	}
}

func TestPartialTopologyRetainsAtomicLogicalPlacementDecision(t *testing.T) {
	client := &protocolClient{
		logicalOwners: map[string]logicalPlacementSnapshot{
			"sensor": {
				OwnerKey:   "chassis",
				Candidates: []string{"system-a", "system-b"},
				Reason:     "related_item_ambiguous",
			},
		},
	}
	service := &graphNode{
		Kind: "service", Key: "service", URI: "/redfish/v1/",
		Parents: make(map[string]*graphNode), RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	chassis := &graphNode{
		Kind: "chassis", Key: "chassis", URI: "/redfish/v1/Chassis/1",
		Parents: map[string]*graphNode{service.Key: service}, RollupParents: make(map[string]*graphNode),
		SystemOwners: make(map[string]*graphNode),
	}
	sensor := &graphNode{
		Kind: "sensor", Key: "sensor", URI: "/redfish/v1/Sensors/1",
		Parents: map[string]*graphNode{service.Key: service}, RollupParents: map[string]*graphNode{service.Key: service},
		SystemOwners: make(map[string]*graphNode),
	}
	graph := &resourceGraph{Nodes: []*graphNode{service, chassis, sensor}, Complete: false}

	client.resolveGraphPlacement(graph)
	if sensor.LogicalOwner != chassis {
		t.Fatalf("logical owner = %#v, want retained chassis", sensor.LogicalOwner)
	}
	if got := strings.Join(sensor.LogicalCandidates, ","); got != "system-a,system-b" {
		t.Fatalf("logical candidates = %q", got)
	}
	if sensor.LogicalReason != "related_item_ambiguous" {
		t.Fatalf("logical reason = %q", sensor.LogicalReason)
	}
}

func TestAggregateKeyIsStableAcrossSummaryContexts(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, reading := aggregateTestSurface(true)

	observations := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {reading}},
		nil,
	)
	groupKey := aggregateGroupKey(aggregateSnapshot{
		OwnerKey:        owner.Key,
		OwnerKind:       owner.Kind,
		Semantic:        aggregateReadingContext(child.Kind, reading.Context),
		Role:            reading.Role,
		Family:          "sensor." + reading.Family,
		Basis:           reading.Basis,
		Units:           reading.Units,
		Source:          reading.SourcePath,
		PhysicalContext: reading.PhysicalContext,
	})
	wantPreimage := strings.Join([]string{
		owner.Key,
		owner.Kind,
		aggregateReadingContext(child.Kind, reading.Context),
		reading.Role,
		"sensor." + reading.Family,
		reading.Basis,
		reading.Units,
		reading.SourcePath,
		reading.PhysicalContext,
	}, "\x00")
	if groupKey != wantPreimage {
		t.Fatalf("aggregate key preimage = %q, want exact public contract %q", groupKey, wantPreimage)
	}
	for _, metric := range []string{
		"aggregate_population_total",
		"aggregate_completeness_complete",
		"aggregate_temperature_minimum",
		"aggregate_temperature_average",
		"aggregate_temperature_maximum",
	} {
		observation := observationByMetric(observations, metric)
		if observation == nil {
			t.Fatalf("metric %q is missing", metric)
		}
		want := stableKey(
			"netdata:redfish:aggregate:v1",
			groupKey,
			32,
		)
		if got := observationLabel(observation.Labels, "aggregate_key"); got != want {
			t.Fatalf("%s aggregate_key = %q, want %q", metric, got, want)
		}
	}
	minimumKey := observationLabel(
		observationByMetric(observations, "aggregate_temperature_minimum").Labels,
		"aggregate_key",
	)
	averageKey := observationLabel(
		observationByMetric(observations, "aggregate_temperature_average").Labels,
		"aggregate_key",
	)
	if minimumKey != averageKey {
		t.Fatal("minimum and average summary contexts do not share the exact aggregate identity")
	}
}

func TestReadingAggregatesKeepDistinctCanonicalSourcesOnOneComponent(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, first := aggregateTestSurface(true)
	second := first
	second.Key = "second-reading-key"
	second.SourcePath = "Sensor.Excerpt.Temperature.Reading"
	second.Value = 84

	observations := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {first, second}},
		nil,
	)
	var keys, sources []string
	for _, observation := range observations {
		if observation.Metric != "aggregate_temperature_minimum" {
			continue
		}
		keys = append(keys, observationLabel(observation.Labels, "aggregate_key"))
		sources = append(sources, observationLabel(observation.Labels, "reading_source"))
	}
	if len(keys) != 2 || keys[0] == keys[1] {
		t.Fatalf("aggregate keys = %v, want two distinct source groups", keys)
	}
	sort.Strings(sources)
	if got := strings.Join(sources, ","); got != "Sensor.Excerpt.Temperature.Reading,Sensor.Reading" {
		t.Fatalf("aggregate reading sources = %q", got)
	}
}

func TestNumericAggregatesStayIsolatedByParentUnitsAndRole(t *testing.T) {
	tests := []struct {
		name  string
		build func() (*resourceGraph, []*graphNode, map[string][]normalizedReading)
		label string
	}{
		{
			name:  "parent",
			label: "rollup_owner_key",
			build: func() (*resourceGraph, []*graphNode, map[string][]normalizedReading) {
				ownerA, childA, graph, readingA := aggregateTestSurface(true)
				ownerB := &graphNode{Kind: "chassis", Key: "chassis-key-b"}
				childB := &graphNode{Kind: "sensor", Key: "sensor-key-b", RollupOwner: ownerB}
				readingB := readingA
				readingB.Key = "reading-key-b"
				graph.Slices = append(graph.Slices, graphSlice{
					ParentKey: ownerB.Key, Path: "Sensors", ChildKind: childB.Kind,
					Family: "thermal", Source: "modern", Mode: relationshipComponents,
					Complete: true, Members: []string{childB.Key},
				})
				return graph,
					[]*graphNode{ownerA, childA, ownerB, childB},
					map[string][]normalizedReading{childA.Key: {readingA}, childB.Key: {readingB}}
			},
		},
		{
			name:  "units",
			label: "aggregate_units",
			build: func() (*resourceGraph, []*graphNode, map[string][]normalizedReading) {
				owner, childA, graph, readingA := aggregateTestSurface(true)
				childB := &graphNode{Kind: "sensor", Key: "sensor-key-b", RollupOwner: owner}
				readingB := readingA
				readingB.Key = "reading-key-b"
				readingB.Units = "kelvin"
				graph.Slices[0].Members = append(graph.Slices[0].Members, childB.Key)
				return graph,
					[]*graphNode{owner, childA, childB},
					map[string][]normalizedReading{childA.Key: {readingA}, childB.Key: {readingB}}
			},
		},
		{
			name:  "role",
			label: "aggregate_role",
			build: func() (*resourceGraph, []*graphNode, map[string][]normalizedReading) {
				owner, childA, graph, readingA := aggregateTestSurface(true)
				childB := &graphNode{Kind: "sensor", Key: "sensor-key-b", RollupOwner: owner}
				readingB := readingA
				readingB.Key = "reading-key-b"
				readingB.Role = "average"
				readingB.Context = "redfish.reading.temperature.zero.average"
				readingB.AggregateSemantic = "sensor_temperature_average"
				graph.Slices[0].Members = append(graph.Slices[0].Members, childB.Key)
				return graph,
					[]*graphNode{owner, childA, childB},
					map[string][]normalizedReading{childA.Key: {readingA}, childB.Key: {readingB}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &protocolClient{}
			client.hardwareState.initialize()
			graph, nodes, readings := test.build()
			observations := mustAggregateObservations(t, client, graph, nodes, readings, nil)
			values := make(map[string]struct{})
			keys := make(map[string]struct{})
			for _, observation := range observations {
				if observation.Metric != "aggregate_temperature_minimum" {
					continue
				}
				values[observationLabel(observation.Labels, test.label)] = struct{}{}
				keys[observationLabel(observation.Labels, "aggregate_key")] = struct{}{}
			}
			if len(values) != 2 || len(keys) != 2 {
				t.Fatalf("isolated %s groups: label values=%v aggregate keys=%v, want two of each", test.name, values, keys)
			}
		})
	}
}

func TestAggregateRetentionBudgetDoesNotFilterCurrentMembers(t *testing.T) {
	const (
		groupKey = "group"
		ownerKey = "owner"
		sliceKey = "slice"
	)
	snapshot := func(memberKeys ...string) aggregateSnapshot {
		members := make(map[string]struct{}, len(memberKeys))
		for _, key := range memberKeys {
			members[key] = struct{}{}
		}
		return aggregateSnapshot{
			OwnerKey:       ownerKey,
			AggregateClass: "temperature",
			Members:        cloneStringSet(members),
			SliceMembers:   map[string]map[string]struct{}{sliceKey: members},
		}
	}
	retained := make(map[string]aggregateSnapshot)
	owner := &graphNode{Kind: "system", Key: ownerKey}
	budget := retainedStateBudget{entries: 1, members: 2}

	effective, complete := reconcileAggregateSnapshots(
		retained,
		map[string]aggregateSnapshot{groupKey: snapshot("first")},
		map[string]bool{sliceKey: true},
		map[string]*graphNode{ownerKey: owner},
		false,
		budget,
	)
	require.True(t, complete)
	require.Len(t, effective[groupKey].Members, 1)
	require.Contains(t, retained, groupKey)

	effective, complete = reconcileAggregateSnapshots(
		retained,
		map[string]aggregateSnapshot{groupKey: snapshot("first", "second")},
		map[string]bool{sliceKey: true},
		map[string]*graphNode{ownerKey: owner},
		false,
		budget,
	)
	require.False(t, complete)
	require.Len(t, effective[groupKey].Members, 2)
	require.Empty(t, retained)
}

func TestAggregatesDoNotCascadeOrUseAmbiguousParents(t *testing.T) {
	t.Run("no recursive cascade", func(t *testing.T) {
		client := &protocolClient{}
		client.hardwareState.initialize()
		root := &graphNode{Kind: "system", Key: "system-key"}
		owner, child, graph, reading := aggregateTestSurface(true)
		owner.RollupOwner = root
		graph.Slices = append(graph.Slices, graphSlice{
			ParentKey: root.Key, Path: "Chassis", ChildKind: owner.Kind,
			Family: "chassis", Source: "modern", Mode: relationshipComponents,
			Complete: true, Members: []string{owner.Key},
		})
		observations := mustAggregateObservations(
			t,
			client,
			graph,
			[]*graphNode{root, owner, child},
			map[string][]normalizedReading{child.Key: {reading}},
			nil,
		)
		for _, observation := range observations {
			if strings.HasPrefix(observation.Metric, "aggregate_") &&
				observationLabel(observation.Labels, "rollup_owner_key") == root.Key {
				t.Fatalf("child aggregate recursively cascaded to %q: %#v", root.Key, observation)
			}
		}
	})

	t.Run("ambiguous parent", func(t *testing.T) {
		client := &protocolClient{}
		client.hardwareState.initialize()
		left := &graphNode{Kind: "chassis", Key: "left"}
		right := &graphNode{Kind: "chassis", Key: "right"}
		child := &graphNode{
			Kind: "sensor", Key: "sensor",
			RollupParents: map[string]*graphNode{left.Key: left, right.Key: right},
		}
		_, _, _, reading := aggregateTestSurface(true)
		graph := &resourceGraph{Complete: true, Nodes: []*graphNode{left, right, child}}
		observations := mustAggregateObservations(
			t,
			client,
			graph,
			graph.Nodes,
			map[string][]normalizedReading{child.Key: {reading}},
			nil,
		)
		for _, observation := range observations {
			if strings.HasPrefix(observation.Metric, "aggregate_") {
				t.Fatalf("ambiguous parent produced aggregate observation %#v", observation)
			}
		}
	})
}

func TestAggregateDigestCollisionFailsInsteadOfMerging(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	client.aggregateDigest = func(string) string { return strings.Repeat("0", 32) }
	owner, child, graph, first := aggregateTestSurface(true)
	second := first
	second.Key = "second-reading-key"
	second.SourcePath = "Sensor.Excerpt.Temperature.Reading"

	_, err := client.aggregateObservations(
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {first, second}},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "aggregate-key collision") {
		t.Fatalf("aggregate collision error = %v", err)
	}
}

func TestAggregateDigestCollisionIsRememberedAcrossCycles(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	client.aggregateDigest = func(string) string { return strings.Repeat("0", 32) }
	owner, child, graph, reading := aggregateTestSurface(true)

	_, err := client.aggregateObservations(
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {reading}},
		nil,
	)
	require.NoError(t, err)

	reading.SourcePath = "Sensor.Excerpt.Temperature.Reading"
	_, err = client.aggregateObservations(
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {reading}},
		nil,
	)
	require.ErrorContains(t, err, "aggregate-key collision")
	require.True(t, identityIntegrityError(err))
}

func TestCategoricalAggregateUsesExactChildContext(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, _ := aggregateTestSurface(true)
	child.Kind = "fan"
	graph.Slices[0].ChildKind = child.Kind
	child.Data = map[string]any{"Status": map[string]any{"Health": "OK"}}

	observations := mustAggregateObservations(t, client, graph, []*graphNode{owner, child}, nil, nil)
	observation := observationByMetric(observations, "aggregate_health_ok")
	if observation == nil {
		t.Fatal("aggregate health observation is missing")
	}
	if got := observationLabel(observation.Labels, "aggregate_semantic"); got != "redfish.fan.health" {
		t.Fatalf("aggregate semantic = %q, want exact child context", got)
	}
}

func TestFlagObservationsDoNotFabricateMissingSiblingValues(t *testing.T) {
	client := &protocolClient{}
	node := &graphNode{
		Kind: "memory",
		Enrichment: map[string]map[string]any{
			"memory_metrics:0": {
				"HealthData": map[string]any{
					"DataLossDetected":    true,
					"LastShutdownSuccess": nil,
					"PerformanceDegraded": "false",
				},
			},
		},
	}

	values := flagValues(node)
	observations := client.flagObservations(node, values)
	if len(observations) != 1 {
		t.Fatalf("flag observations = %d, want only the one present valid boolean", len(observations))
	}
	if observations[0].Metric != "memory_health_flags_data_loss" || observations[0].Value != 1 {
		t.Fatalf("flag observation = %#v", observations[0])
	}
}

func TestCommonStatusPresenceAndTypeSemantics(t *testing.T) {
	client := &protocolClient{}
	node := &graphNode{
		Kind: "drive",
		Data: map[string]any{
			"Status":           map[string]any{"Health": nil, "State": json.Number("1")},
			"FailurePredicted": nil,
		},
	}

	observations := client.statusObservations(node)
	if observationByMetric(observations, "drive_health") != nil {
		t.Fatal("null Status.Health emitted a sample")
	}
	state := observationByMetric(observations, "drive_state")
	if state == nil || state.State != "unknown" {
		t.Fatalf("wrong-typed present Status.State = %#v, want unknown", state)
	}
	if observationByMetric(observations, "drive_failure_predicted") != nil {
		t.Fatal("null FailurePredicted emitted a sample")
	}

	node.Data["FailurePredicted"] = "yes"
	prediction := observationByMetric(client.statusObservations(node), "drive_failure_predicted")
	if prediction == nil || prediction.State != "unknown" {
		t.Fatalf("wrong-typed present FailurePredicted = %#v, want unknown", prediction)
	}
}

func TestConditionSeverityAbsentNullAndUnexpectedSemantics(t *testing.T) {
	raw := []byte(`{
		"Status": {
			"Conditions": [
				{"MessageId":"ok","Severity":"OK"},
				{"MessageId":"absent"},
				{"MessageId":"null","Severity":null},
				{"MessageId":"number","Severity":7},
				{"MessageId":"other","Severity":"VendorSeverity"}
			]
		}
	}`)
	var resource genericResource
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("decode conditions: %v", err)
	}
	counts := conditionCountsFrom(resource.Status.Conditions)
	if counts.OK != 1 || counts.Warning != 0 || counts.Critical != 0 || counts.Unknown != 2 {
		t.Fatalf("condition counts = %+v, want OK=1 Unknown=2", counts)
	}
}

func TestConditionDeduplicationPreservesStructuralFieldBoundaries(t *testing.T) {
	conditions := []genericCondition{
		{
			MessageID:   "same",
			MessageArgs: []string{"argument"},
			OriginOfCondition: redfishLink{
				ODataID: "origin\x00shifted",
			},
			Timestamp: "timestamp",
			Severity:  json.RawMessage(`"Warning"`),
		},
		{
			MessageID:   "same",
			MessageArgs: []string{"argument"},
			OriginOfCondition: redfishLink{
				ODataID: "origin",
			},
			Timestamp: "shifted\x00timestamp",
			Severity:  json.RawMessage(`"Warning"`),
		},
	}

	counts := conditionCountsFrom(conditions)
	require.Equal(t, 2, counts.Warning)
}

func TestRangeNormalizedHistogramUsesAuthoritativeBounds(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, reading := aggregateTestSurface(true)
	reading.Histogram = "range_percentage"
	minimum, maximum := 0.0, 200.0
	reading.RangeMin, reading.RangeMax = &minimum, &maximum
	reading.Value = 100

	observations := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {reading}},
		nil,
	)
	if got := aggregateMetricValue(observations, "aggregate_range_percentage_distribution_50_60"); got != 1 {
		t.Fatalf("range-normalized 50-60 bucket = %v, want 1", got)
	}
	if got := aggregateMetricValue(observations, "aggregate_population_histogram_eligible"); got != 1 {
		t.Fatalf("histogram eligible population = %v, want 1", got)
	}

	reading.RangeMax = nil
	observations = mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		map[string][]normalizedReading{child.Key: {reading}},
		nil,
	)
	if got := aggregateMetricValue(observations, "aggregate_population_histogram_ineligible"); got != 1 {
		t.Fatalf("histogram ineligible population = %v, want 1", got)
	}
}

func TestNormalizeReadingRejectsValueOutsideAdvertisedRange(t *testing.T) {
	reading := normalizeReading(
		&graphNode{Kind: "sensor", Key: "sensor-key"},
		rawReading{
			Path: "Sensor.Reading", Type: "Temperature", Units: "Cel", Basis: "Zero",
			Role: "input", Value: json.Number("101"), Primary: true,
			RangeMin: json.Number("0"), RangeMax: json.Number("100"),
		},
		true,
	)
	if reading.Valid {
		t.Fatal("reading outside ReadingRangeMax remained valid")
	}
	if reading.RangeMin == nil || *reading.RangeMin != 0 ||
		reading.RangeMax == nil || *reading.RangeMax != 100 {
		t.Fatalf("normalized reading range = [%v,%v], want [0,100]", reading.RangeMin, reading.RangeMax)
	}
}

func TestAbsentConditionsDoNotCreateFalseZeroAggregate(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, _ := aggregateTestSurface(true)
	child.Kind = "fan"
	graph.Slices[0].ChildKind = child.Kind
	child.Data = map[string]any{"Status": map[string]any{"Health": "OK"}}

	observations := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		nil,
		nil,
	)
	if observationByMetric(observations, "aggregate_conditions_ok") != nil {
		t.Fatal("absent Status.Conditions created a false all-zero aggregate")
	}
	if observationByMetric(client.statusObservations(child), "fan_conditions_ok") != nil {
		t.Fatal("absent Status.Conditions created a false direct condition chart")
	}

	child.Data = map[string]any{
		"Status": map[string]any{
			"Health":     "OK",
			"Conditions": []any{},
		},
	}
	observations = mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		nil,
		nil,
	)
	condition := observationByMetric(observations, "aggregate_conditions_ok")
	if condition == nil || condition.Value != 0 {
		t.Fatalf("present empty Status.Conditions observation = %#v, want explicit zero", condition)
	}
	if got := observationLabel(condition.Labels, "aggregate_units"); got != "conditions" {
		t.Fatalf("condition aggregate units = %q, want conditions", got)
	}
	if direct := observationByMetric(client.statusObservations(child), "fan_conditions_ok"); direct == nil || direct.Value != 0 {
		t.Fatalf("present empty direct condition observation = %#v, want explicit zero", direct)
	}
}

func TestCategoricalAggregateRetainsMembershipAcrossPartialSlice(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, _ := aggregateTestSurface(true)
	child.Kind = "fan"
	graph.Slices[0].ChildKind = child.Kind
	child.Data = map[string]any{"Status": map[string]any{"Health": "OK"}}

	initial := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		nil,
		nil,
	)
	if got := aggregateMetricValue(initial, "aggregate_health_ok"); got != 1 {
		t.Fatalf("initial aggregate health OK count = %v, want 1", got)
	}

	graph.Complete = false
	graph.Slices[0].Complete = false
	graph.Slices[0].Members = nil
	partial := mustAggregateObservations(t, client, graph, []*graphNode{owner}, nil, nil)
	if got := aggregateMetricValue(partial, "aggregate_population_unknown"); got != 1 {
		t.Fatalf("partial categorical unknown population = %v, want 1", got)
	}
	if got := aggregateMetricValue(partial, "aggregate_health_unknown"); got != 1 {
		t.Fatalf("partial categorical health unknown count = %v, want 1", got)
	}
	if got := aggregateMetricValue(partial, "aggregate_completeness_incomplete"); got != 1 {
		t.Fatalf("partial categorical completeness = %v, want incomplete", got)
	}
}

func TestAggregateMembershipFollowsItsContributingSlice(t *testing.T) {
	client := &protocolClient{}
	client.hardwareState.initialize()
	owner, child, graph, reading := aggregateTestSurface(true)
	readings := map[string][]normalizedReading{child.Key: {reading}}

	initial := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		readings,
		nil,
	)
	if got := aggregateMetricValue(initial, "aggregate_population_total"); got != 1 {
		t.Fatalf("initial aggregate population = %v, want 1", got)
	}

	graph.Complete = false
	graph.Slices[0].Complete = false
	partial := mustAggregateObservations(t, client,
		graph,
		[]*graphNode{owner, child},
		nil,
		nil,
	)
	if got := aggregateMetricValue(partial, "aggregate_population_total"); got != 1 {
		t.Fatalf("partial aggregate population = %v, want retained 1", got)
	}
	if got := aggregateMetricValue(partial, "aggregate_population_unknown"); got != 1 {
		t.Fatalf("partial aggregate unknown = %v, want 1", got)
	}

	graph.Slices[0].Complete = true
	graph.Slices[0].Members = nil
	graph.Slices = append(graph.Slices, graphSlice{
		ParentKey: "other-owner",
		Path:      "Unrelated",
		ChildKind: "fan",
		Family:    "thermal",
		Source:    "modern",
		Mode:      relationshipComponents,
		Complete:  false,
	})
	removed := mustAggregateObservations(t, client, graph, []*graphNode{owner}, nil, nil)
	if observationByMetric(
		removed,
		"aggregate_population_total",
	) != nil {
		t.Fatal("a complete contributing slice did not remove the aggregate after its member disappeared")
	}
}

func aggregateTestSurface(complete bool) (
	*graphNode,
	*graphNode,
	*resourceGraph,
	normalizedReading,
) {
	owner := &graphNode{Kind: "chassis", Key: "chassis-key"}
	child := &graphNode{
		Kind:        "sensor",
		Key:         "sensor-key",
		RollupOwner: owner,
	}
	graph := &resourceGraph{
		Complete: complete,
		Slices: []graphSlice{{
			ParentKey: owner.Key,
			Path:      "Sensors",
			ChildKind: child.Kind,
			Family:    "thermal",
			Source:    "modern",
			Mode:      relationshipComponents,
			Complete:  complete,
			Members:   []string{child.Key},
		}},
	}
	reading := normalizedReading{
		Key:                 "reading-key",
		SourcePath:          "Sensor.Reading",
		Family:              "temperature",
		Units:               "Celsius",
		Basis:               "zero",
		Role:                "input",
		Value:               42,
		Valid:               true,
		Primary:             true,
		AggregateClass:      "temperature",
		AggregateSemantic:   "sensor_temperature_input",
		Context:             "redfish.reading.temperature.zero.input",
		AggregateKinds:      []registry.Kind{"chassis"},
		SemanticSourceClass: "standard",
		PhysicalContext:     "Intake",
	}
	return owner, child, graph, reading
}

func mustAggregateObservations(
	t *testing.T,
	client *protocolClient,
	graph *resourceGraph,
	nodes []*graphNode,
	readings map[string][]normalizedReading,
	scalars map[string][]scalarValue,
) []hardwareObservation {
	t.Helper()
	observations, err := client.aggregateObservations(graph, nodes, readings, scalars)
	if err != nil {
		t.Fatalf("aggregate observations: %v", err)
	}
	return observations
}

func aggregateMetricValue(observations []hardwareObservation, metric string) float64 {
	observation := observationByMetric(observations, metric)
	if observation == nil {
		return math.NaN()
	}
	return observation.Value
}

func observationLabel(labels []metrix.Label, key string) string {
	for _, label := range labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
}

func containsNode(nodes []*graphNode, key string) bool {
	for _, node := range nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func derivedPowerCount(readings []normalizedReading) int {
	count := 0
	for _, reading := range readings {
		if reading.Role == "energy_rate" {
			count++
		}
	}
	return count
}
