// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

func TestEveryRegistryInventoryFieldHasPresenceNullAndTypeSemantics(t *testing.T) {
	for _, field := range standardRegistry.Inventory {
		t.Run(fmt.Sprintf("%03d/%s/%s", field.Order, field.Kind, field.Column), func(t *testing.T) {
			node := &graphNode{Kind: string(field.Kind), Data: make(map[string]any)}
			setRegistryTestPath(node.Data, field.Path, inventoryFieldTestValue(field))
			row := make(map[string]any)
			applyRegisteredInventory(row, node)
			value, present := row[field.Column]
			if !present || value == nil {
				t.Fatalf("valid path %q produced %#v present=%t", field.Path, value, present)
			}

			row = make(map[string]any)
			applyRegisteredInventory(row, &graphNode{Kind: string(field.Kind), Data: make(map[string]any)})
			if _, present := row[field.Column]; present {
				t.Fatalf("absent path %q produced a Function value", field.Path)
			}

			node.Data = make(map[string]any)
			setRegistryTestPath(node.Data, field.Path, nil)
			row = make(map[string]any)
			applyRegisteredInventory(row, node)
			if value, present := row[field.Column]; !present || value != nil {
				t.Fatalf("null path %q = %#v present=%t, want an explicit gap", field.Path, value, present)
			}
			if row["error_class"] != "protocol" {
				t.Fatalf("null path %q error_class = %#v, want protocol", field.Path, row["error_class"])
			}

			node.Data = make(map[string]any)
			setRegistryTestPath(node.Data, field.Path, inventoryFieldWrongType(field))
			row = make(map[string]any)
			applyRegisteredInventory(row, node)
			if value, present := row[field.Column]; !present || value != nil {
				t.Fatalf("wrong-typed path %q = %#v present=%t, want an explicit gap", field.Path, value, present)
			}
			if row["error_class"] != "protocol" {
				t.Fatalf("wrong-typed path %q error_class = %#v, want protocol", field.Path, row["error_class"])
			}
		})
	}
}

func TestEveryRegistryStateHasFunctionPresenceNullAndTypeSemantics(t *testing.T) {
	columns := make(map[string]registry.ColumnSpec, len(standardRegistry.Columns))
	for _, column := range standardRegistry.Columns {
		columns[column.ID] = column
	}
	for _, state := range standardRegistry.States {
		t.Run(fmt.Sprintf("%03d/%s/%s", state.Order, state.Kind, state.Column), func(t *testing.T) {
			column, ok := columns[state.Column]
			if !ok {
				t.Fatalf("state Function column %q is not compiled", state.Column)
			}
			wantType := registry.ColumnEnum
			valid := any("SourceEnumValue")
			wrong := any(true)
			if state.BooleanFalse != "" || state.BooleanTrue != "" {
				wantType = registry.ColumnBoolean
				valid = false
				wrong = "wrong"
			}
			if column.Type != wantType {
				t.Fatalf("state Function column %q type = %q, want %q", state.Column, column.Type, wantType)
			}

			node := &graphNode{
				Kind: string(state.Kind), Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
			}
			document := node.Data
			if state.Document != "" {
				document = make(map[string]any)
				node.Enrichment[string(state.Document)+":test"] = document
			}
			setRegistryTestPath(document, state.Path, valid)
			row := make(map[string]any)
			applyRegisteredStates(row, node)
			if got, present := row[state.Column]; !present || got != valid {
				t.Fatalf("valid state = %#v present=%t, want %#v", got, present, valid)
			}

			row = make(map[string]any)
			applyRegisteredStates(row, &graphNode{
				Kind: string(state.Kind), Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
			})
			if _, present := row[state.Column]; present {
				t.Fatal("absent state produced a Function value")
			}

			setRegistryTestPath(document, state.Path, nil)
			row = make(map[string]any)
			applyRegisteredStates(row, node)
			if got, present := row[state.Column]; !present || got != nil || row["error_class"] != "protocol" {
				t.Fatalf("null state = %#v present=%t error_class=%#v", got, present, row["error_class"])
			}

			setRegistryTestPath(document, state.Path, wrong)
			row = make(map[string]any)
			applyRegisteredStates(row, node)
			if got, present := row[state.Column]; !present || got != nil || row["error_class"] != "protocol" {
				t.Fatalf("wrong-typed state = %#v present=%t error_class=%#v", got, present, row["error_class"])
			}
		})
	}
}

func TestEveryRegistryFlagHasFunctionPresenceNullAndTypeSemantics(t *testing.T) {
	for _, set := range standardRegistry.Flags {
		for _, member := range set.Members {
			t.Run(fmt.Sprintf("%03d/%s/%s", set.Order, set.Kind, member.Column), func(t *testing.T) {
				node := &graphNode{
					Kind: string(set.Kind), Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
				}
				document := registryTestDocument(node, set.Document)
				setRegistryTestPath(document, member.Path, false)
				row := make(map[string]any)
				applyRegisteredFlags(row, node)
				want := false
				if member.Invert {
					want = true
				}
				if got, present := row[member.Column]; !present || got != want {
					t.Fatalf("valid flag = %#v present=%t, want %t", got, present, want)
				}

				node = &graphNode{
					Kind: string(set.Kind), Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
				}
				registryTestDocument(node, set.Document)
				row = make(map[string]any)
				applyRegisteredFlags(row, node)
				if _, present := row[member.Column]; present {
					t.Fatal("absent flag produced a Function value")
				}

				document = registryTestDocument(node, set.Document)
				setRegistryTestPath(document, member.Path, nil)
				row = make(map[string]any)
				applyRegisteredFlags(row, node)
				if got, present := row[member.Column]; !present || got != nil || row["error_class"] != "protocol" {
					t.Fatalf("null flag = %#v present=%t error_class=%#v", got, present, row["error_class"])
				}

				setRegistryTestPath(document, member.Path, "wrong")
				row = make(map[string]any)
				applyRegisteredFlags(row, node)
				if got, present := row[member.Column]; !present || got != nil || row["error_class"] != "protocol" {
					t.Fatalf("wrong-typed flag = %#v present=%t error_class=%#v", got, present, row["error_class"])
				}
			})
		}
	}
}

func TestEveryRegistryReadingHasFunctionPresenceNullAndTypeSemantics(t *testing.T) {
	for _, surface := range standardRegistry.Readings {
		if surface.DerivedFromEnergy {
			continue
		}
		surface := surface
		name := strings.Join([]string{surface.Family, surface.Basis, surface.Role, surface.SemanticClass}, "/")
		t.Run(name, func(t *testing.T) {
			raw, node := syntheticRawReadingForSurface(surface, json.Number("0"))
			reading := normalizeReading(node, raw, true)
			if !reading.Valid {
				t.Fatalf("valid zero did not normalize: %+v", reading)
			}
			row := (&protocolClient{}).inventoryReadingRow(node, detailGate{Open: true}, time.Unix(100, 0), reading)
			if got, present := row["reading_value"]; !present || got != float64(0) {
				t.Fatalf("valid zero Function reading = %#v present=%t", got, present)
			}

			for _, test := range []struct {
				name  string
				value any
			}{
				{name: "null", value: nil},
				{name: "wrong_type", value: map[string]any{"invalid": true}},
			} {
				t.Run(test.name, func(t *testing.T) {
					invalidRaw, invalidNode := syntheticRawReadingForSurface(surface, test.value)
					invalid := normalizeReading(invalidNode, invalidRaw, true)
					if invalid.Valid {
						t.Fatalf("invalid source normalized as valid: %+v", invalid)
					}
					row := (&protocolClient{}).inventoryReadingRow(
						invalidNode,
						detailGate{Open: true},
						time.Unix(100, 0),
						invalid,
					)
					if _, present := row["reading_value"]; present {
						t.Fatalf("invalid reading fabricated a Function value %#v", row["reading_value"])
					}
					if row["error_class"] != "protocol" {
						t.Fatalf("invalid reading error_class = %#v, want protocol", row["error_class"])
					}
				})
			}
		})
	}

	client := &protocolClient{}
	if readings := client.readingsForNode(
		&graphNode{Kind: "sensor", Key: "absent", Data: make(map[string]any)},
		time.Unix(100, 0),
	); len(readings) != 0 {
		t.Fatalf("absent source produced %d reading rows", len(readings))
	}
}

func TestFractionalInventorySourcesScaleExactlyToIntegerColumns(t *testing.T) {
	for _, field := range standardRegistry.Inventory {
		if field.SourceType != registry.ColumnFloat || field.Type != registry.ColumnInteger {
			continue
		}
		t.Run(fmt.Sprintf("%s/%s", field.Kind, field.Path), func(t *testing.T) {
			value := normalizeInventoryFieldValue(json.Number("1.5"), field)
			want := field.Scale.Num * 3 / (field.Scale.Den * 2)
			if value != want {
				t.Fatalf("scaled 1.5 = %#v, want %d", value, want)
			}
		})
	}
}

func TestEveryRegistryNumericConversionUsesItsDeclaredScale(t *testing.T) {
	for _, field := range standardRegistry.Inventory {
		sourceType := field.SourceType
		if sourceType == "" {
			sourceType = field.Type
		}
		if sourceType != registry.ColumnInteger && sourceType != registry.ColumnFloat {
			continue
		}
		t.Run("inventory/"+string(field.Kind)+"/"+field.Column, func(t *testing.T) {
			scale := field.Scale
			if scale.Den == 0 {
				scale = registry.Identity
			}
			got := normalizeInventoryFieldValue(json.Number("2"), field)
			if field.Type == registry.ColumnInteger {
				want := int64(2 * scale.Num / scale.Den)
				if got != want {
					t.Fatalf("scaled inventory value = %#v, want %d", got, want)
				}
				return
			}
			want := 2 * float64(scale.Num) / float64(scale.Den)
			if got != want {
				t.Fatalf("scaled inventory value = %#v, want %v", got, want)
			}
		})
	}

	for _, descriptor := range standardRegistry.Fields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		descriptor := descriptor
		t.Run("scalar/"+descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("2"))
			client := &protocolClient{}
			client.hardwareState.initialize()
			value, ok := scalarValueByID(client.scalarValues(node, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Valid {
				t.Fatalf("scaled source did not normalize: %+v present=%t", value, ok)
			}
			scale := descriptor.Scale
			if source.Scale.Den != 0 {
				scale = source.Scale
			}
			if scale.Den == 0 {
				scale = registry.Identity
			}
			want := 2 * float64(scale.Num) / float64(scale.Den)
			if source.MultiplierPath != "" {
				multiplierScale := source.MultiplierScale
				if multiplierScale.Den == 0 {
					multiplierScale = registry.Identity
				}
				want *= 2 * float64(multiplierScale.Num) / float64(multiplierScale.Den)
			}
			if value.Inventory != want {
				t.Fatalf("scaled scalar inventory = %#v, want %v", value.Inventory, want)
			}
		})
	}

	for _, readingType := range standardRegistry.ReadingTypes {
		t.Run("reading/"+readingType.SourceType, func(t *testing.T) {
			reading := normalizeReading(
				&graphNode{Kind: "sensor", Key: readingType.SourceType},
				rawReading{
					Path: "Reading", Type: readingType.SourceType, Units: readingType.SourceUnits[0],
					Basis: "Zero", Role: "input", Value: json.Number("2"), Primary: true,
					Inventory: make(map[string]any),
				},
				false,
			)
			if !reading.Valid {
				t.Fatalf("reading conversion did not normalize: %+v", reading)
			}
			scale := readingType.Scale
			if scale.Den == 0 {
				scale = registry.Identity
			}
			want := 2 * float64(scale.Num) / float64(scale.Den)
			if reading.Value != want {
				t.Fatalf("scaled reading = %v, want %v", reading.Value, want)
			}
		})
	}
}

func TestEveryRegistryScalarFieldHasPresenceNullAndTypeSemantics(t *testing.T) {
	for _, descriptor := range standardRegistry.Fields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("0"))
			client := &protocolClient{}
			client.hardwareState.initialize()
			value, ok := scalarValueByID(client.scalarValues(node, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || !value.Valid || value.Inventory != float64(0) {
				t.Fatalf("valid zero source = %+v present=%t", value, ok)
			}

			absent := &graphNode{
				Kind: string(descriptor.Kind), Key: descriptor.ID,
				Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
			}
			if _, ok := scalarValueByID(client.scalarValues(absent, time.Unix(100, 0)), descriptor.ID); ok {
				t.Fatal("absent source produced a scalar value")
			}

			nullNode := scalarTestNode(descriptor, source, nil)
			value, ok = scalarValueByID(client.scalarValues(nullNode, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || value.Valid || value.Emit || value.Inventory != nil {
				t.Fatalf("null source = %+v present=%t, want a present Function gap", value, ok)
			}

			wrongNode := scalarTestNode(descriptor, source, map[string]any{"unexpected": true})
			value, ok = scalarValueByID(client.scalarValues(wrongNode, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || value.Valid || value.Emit {
				t.Fatalf("wrong-typed source = %+v present=%t, want a present gap", value, ok)
			}
		})
	}
}

func TestEveryRegistryScalarFallbackHasPrecedenceAndFailureProvenance(t *testing.T) {
	for _, descriptor := range standardRegistry.Fields {
		if len(descriptor.Candidates) < 2 {
			continue
		}
		for selectedIndex := range descriptor.Candidates {
			t.Run(fmt.Sprintf("%s/source-%d", descriptor.ID, selectedIndex), func(t *testing.T) {
				node := &graphNode{
					Kind: string(descriptor.Kind), Key: descriptor.ID,
					Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
				}
				for index, source := range descriptor.Candidates {
					document := registryTestDocument(node, source.Document)
					for _, requirement := range source.Requires {
						setRegistryTestPath(document, requirement.Path, requirement.Value)
					}
					value := any(json.Number(fmt.Sprintf("%d", index+10)))
					if index < selectedIndex {
						value = map[string]any{"malformed": true}
					}
					setRegistryTestPath(document, source.Path, value)
					setScalarTestMultiplier(node, source)
				}

				client := &protocolClient{}
				client.hardwareState.initialize()
				values := client.scalarValues(node, time.Unix(100, 0))
				value, ok := scalarValueByID(values, descriptor.ID)
				if !ok || !value.Valid || value.SelectedSource != sourcePath(descriptor.Candidates[selectedIndex]) {
					t.Fatalf("selected fallback = %+v present=%t", value, ok)
				}
				expectedFailures := 0
				for _, source := range descriptor.Candidates[:selectedIndex] {
					document := registryTestDocument(node, source.Document)
					if sourceRequirementsMatch(document, source.Requires) {
						expectedFailures++
					}
				}
				if len(value.SourceFailures) != expectedFailures {
					t.Fatalf("preferred failure provenance = %v, want %d entries", value.SourceFailures, expectedFailures)
				}
				count := 0
				for _, candidate := range values {
					if candidate.Descriptor.ID == descriptor.ID {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("fallback produced %d values, want proof-only selection of one", count)
				}
			})
		}
	}
}

func TestEveryRegistryRateFieldResetsOnDecreaseAndEpochChange(t *testing.T) {
	for _, descriptor := range standardRegistry.Fields {
		if descriptor.Algorithm == registry.AlgorithmAbsolute {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("10"))
			document := registryTestDocument(node, source.Document)
			client := &protocolClient{}
			client.hardwareState.initialize()
			at := time.Unix(100, 0)

			if value := requireScalarValue(t, client, node, descriptor.ID, at); value.Emit {
				t.Fatal("first sample emitted instead of establishing a baseline")
			}
			increment := "20"
			if descriptor.Algorithm == registry.AlgorithmDurationPercent {
				increment = "10.01"
			}
			setRegistryTestPath(document, source.Path, json.Number(increment))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(10*time.Second)); !value.Emit {
				t.Fatal("increasing sample did not emit")
			}
			setRegistryTestPath(document, source.Path, json.Number("5"))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(20*time.Second)); value.Emit {
				t.Fatal("decrease emitted instead of resetting the baseline")
			}
			postReset := "6"
			if descriptor.Algorithm == registry.AlgorithmDurationPercent {
				postReset = "5.01"
			}
			setRegistryTestPath(document, source.Path, json.Number(postReset))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(30*time.Second)); !value.Emit {
				t.Fatal("post-reset increase did not emit")
			}
			setRegistryTestPath(document, "LifetimeStartDateTime", "2026-07-31T00:00:00Z")
			epochValue := "7"
			if descriptor.Algorithm == registry.AlgorithmDurationPercent {
				epochValue = "5.02"
			}
			setRegistryTestPath(document, source.Path, json.Number(epochValue))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(40*time.Second)); value.Emit {
				t.Fatal("epoch change emitted instead of resetting the baseline")
			}
			postEpoch := "8"
			if descriptor.Algorithm == registry.AlgorithmDurationPercent {
				postEpoch = "5.03"
			}
			setRegistryTestPath(document, source.Path, json.Number(postEpoch))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(50*time.Second)); !value.Emit {
				t.Fatal("post-epoch increase did not emit")
			}
		})
	}
}

func TestRegistryCommonContextsHaveExactDimensions(t *testing.T) {
	want := map[string]string{
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
	}
	for _, chart := range standardRegistry.Charts {
		dimension, ok := want[chart.Context]
		if !ok {
			continue
		}
		for _, candidate := range chart.Dimensions {
			if candidate.ID == "present" || candidate.Metric == "present" {
				t.Errorf("common chart %q has prohibited synthetic present dimension", chart.Context)
			}
		}
		if len(chart.Dimensions) != 1 || chart.Dimensions[0].ID != dimension {
			t.Errorf("common context %q dimensions = %#v, want exactly %q", chart.Context, chart.Dimensions, dimension)
		}
		delete(want, chart.Context)
	}
	for context := range want {
		t.Errorf("common context %q is missing", context)
	}
}

func TestExplicitlyRejectedMetricClassesStayUncharted(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		data       map[string]any
		enrichment map[string]map[string]any
	}{
		{"ambiguous memory speed", "memory", map[string]any{"OperatingSpeedMhz": 3200}, map[string]map[string]any{"memory_metrics": {"OperatingSpeedMHz": 3200}}},
		{"undefined processor bandwidth", "processor", nil, map[string]map[string]any{"processor_metrics": {"LocalMemoryBandwidthBytes": 1, "RemoteMemoryBandwidthBytes": 1}}},
		{"generic lifetime reading", "sensor", map[string]any{"LifetimeReading": 1, "ReadingType": "EnergyJoules", "ReadingUnits": "J"}, nil},
		{"rated efficiency curve", "power_supply", map[string]any{"EfficiencyRatings": []any{map[string]any{"EfficiencyPercent": 90}}}, nil},
		{"battery ratings", "battery", map[string]any{"NominalVoltage": 12, "RatedCapacity": 100, "MaxChargeRateWatts": 20}, nil},
		{"pcie identifiers", "pcie_function", map[string]any{"FunctionId": 1, "DeviceId": 2, "VendorId": 3}, nil},
		{"ethernet traffic", "ethernet_interface", map[string]any{"BytesReceived": 1, "BytesSent": 2}, nil},
		{"network port traffic", "network_port", map[string]any{"PacketsReceived": 1, "PacketsSent": 2}, nil},
		{"open processor cache arrays", "processor", nil, map[string]map[string]any{"processor_metrics": {"Cache": []any{map[string]any{"Level": 1}}, "CacheMetricsTotal": map[string]any{"HitRatio": 1}}}},
		{"nested fabric arrays", "network_adapter", map[string]any{"FibreChannel": []any{map[string]any{"Errors": 1}}, "SAS": []any{map[string]any{"Errors": 1}}}, nil},
		{"firmware size", "firmware", map[string]any{"SizeBytes": 1, "DeviceCount": 2}, nil},
		{"configuration", "system", map[string]any{"Boot": map[string]any{"BootSourceOverrideEnabled": "Once"}, "Actions": map[string]any{"Reset": true}}, nil},
		{"oem unknown", "drive", map[string]any{"Oem": map[string]any{"VendorMetric": 1}, "FutureStandardMetric": 2}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := &graphNode{
				Kind: test.kind, Key: test.name,
				Data: test.data, Enrichment: test.enrichment,
			}
			client := &protocolClient{}
			client.hardwareState.initialize()
			for _, value := range client.scalarValues(node, time.Unix(100, 0)) {
				if value.Emit {
					t.Errorf("rejected class emitted scalar metric %q", value.Descriptor.Metric)
				}
			}
			for _, reading := range client.readingsForNode(node, time.Unix(100, 0)) {
				if reading.Valid && reading.Exposure == registry.ExposureOperationalReading {
					t.Errorf("rejected class emitted reading %q", reading.SourcePath)
				}
			}
		})
	}
}

func inventoryFieldTestValue(field registry.InventoryFieldSpec) any {
	if field.Structured {
		return []any{map[string]any{"value": "synthetic"}}
	}
	typ := field.SourceType
	if typ == "" {
		typ = field.Type
	}
	switch typ {
	case registry.ColumnString, registry.ColumnEnum:
		return "synthetic"
	case registry.ColumnBoolean:
		return false
	case registry.ColumnTimestamp:
		return "2026-07-31T00:00:00Z"
	case registry.ColumnInteger:
		return json.Number("0")
	case registry.ColumnFloat:
		return json.Number("0")
	default:
		panic("unsupported inventory column type " + field.Type)
	}
}

func inventoryFieldWrongType(field registry.InventoryFieldSpec) any {
	if field.Structured {
		return "wrong"
	}
	typ := field.SourceType
	if typ == "" {
		typ = field.Type
	}
	switch typ {
	case registry.ColumnString, registry.ColumnEnum, registry.ColumnTimestamp:
		return true
	case registry.ColumnBoolean:
		return "wrong"
	case registry.ColumnInteger, registry.ColumnFloat:
		return map[string]any{"wrong": true}
	default:
		panic("unsupported inventory column type " + field.Type)
	}
}

func syntheticRawReadingForSurface(
	surface registry.ReadingSurfaceSpec,
	value any,
) (rawReading, *graphNode) {
	sourceType, sourceUnits, fixed := registryReadingSource(surface.Family)
	raw := rawReading{
		Path:           "Synthetic." + surface.Family + "." + surface.Basis + "." + surface.Role,
		IdentitySource: "Synthetic." + surface.Family + "." + surface.Basis + "." + surface.Role,
		Type:           sourceType,
		Units:          sourceUnits,
		Basis:          surface.Basis,
		Role:           surface.Role,
		Value:          value,
		Primary:        true,
		ReadingScoped:  surface.AlarmMetric != "",
		Health:         "OK",
		Inventory:      make(map[string]any),
	}
	if fixed {
		raw.Inventory["fixed_family"] = surface.Family
	}
	node := &graphNode{Kind: "sensor", Key: raw.IdentitySource, Data: make(map[string]any)}
	if surface.SemanticClass == "fan" {
		node.Kind = "fan"
	}
	return raw, node
}

func scalarTestNode(descriptor registry.FieldSpec, source registry.SourceCandidate, value any) *graphNode {
	node := &graphNode{
		Kind: string(descriptor.Kind), Key: descriptor.ID,
		Data: make(map[string]any), Enrichment: make(map[string]map[string]any),
	}
	document := registryTestDocument(node, source.Document)
	for _, requirement := range source.Requires {
		setRegistryTestPath(document, requirement.Path, requirement.Value)
	}
	setRegistryTestPath(document, source.Path, value)
	setScalarTestMultiplier(node, source)
	return node
}

func setScalarTestMultiplier(node *graphNode, source registry.SourceCandidate) {
	if source.MultiplierPath == "" {
		return
	}
	document := registryTestDocument(node, source.Document)
	if source.MultiplierDocument != "" {
		document = registryTestDocument(node, source.MultiplierDocument)
	}
	setRegistryTestPath(document, source.MultiplierPath, json.Number("2"))
}

func requireScalarValue(
	t *testing.T,
	client *protocolClient,
	node *graphNode,
	id string,
	at time.Time,
) scalarValue {
	t.Helper()
	value, ok := scalarValueByID(client.scalarValues(node, at), id)
	if !ok || !value.Present || !value.Valid || math.IsNaN(value.Value) || math.IsInf(value.Value, 0) {
		t.Fatalf("scalar %q = %+v present=%t", id, value, ok)
	}
	return value
}
