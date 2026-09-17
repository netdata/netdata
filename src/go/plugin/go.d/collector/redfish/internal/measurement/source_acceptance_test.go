// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestEverySourceReadingHasPresenceNullAndTypeSemantics(t *testing.T) {
	for key, surface := range readingDescriptors {
		if key.Role == "energy_rate" {
			continue
		}
		name := strings.Join([]string{key.Family, key.Basis, key.Role, key.SemanticClass}, "/")
		t.Run(name, func(t *testing.T) {
			raw, node := syntheticRawReadingForSurface(key, surface, json.Number("0"))
			reading := normalizeReading(node, raw)
			if !reading.Valid {
				t.Fatalf("valid zero did not normalize: %+v", reading)
			}
			if reading.Value != 0 {
				t.Fatalf("valid zero reading = %v", reading.Value)
			}

			for _, test := range []struct {
				name  string
				value any
			}{
				{name: "null", value: nil},
				{name: "wrong_type", value: map[string]any{"invalid": true}},
			} {
				t.Run(test.name, func(t *testing.T) {
					invalidRaw, invalidNode := syntheticRawReadingForSurface(key, surface, test.value)
					invalid := normalizeReading(invalidNode, invalidRaw)
					if invalid.Valid {
						t.Fatalf("invalid source normalized as valid: %+v", invalid)
					}
				})
			}
		})
	}

	client := New("", "", nil)
	if readings := client.readingsForNode(
		&Resource{
			Kind: "sensor",
			Key:  "absent",
			Data: make(map[string]any),
		},
		time.Unix(100, 0),
	); len(readings) != 0 {
		t.Fatalf("absent source produced %d reading rows", len(readings))
	}
}

func TestEverySourceNumericConversionUsesItsDeclaredScale(t *testing.T) {
	for _, descriptor := range scalarFields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		descriptor := descriptor
		t.Run("scalar/"+descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("2"))
			client := New("", "", nil)
			value, ok := scalarValueByID(client.scalarValues(node, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Valid {
				t.Fatalf("scaled source did not normalize: %+v present=%t", value, ok)
			}
			scale := descriptor.Scale
			if source.Scale.Den != 0 {
				scale = source.Scale
			}
			if scale.Den == 0 {
				scale = identityScale
			}
			want := 2 * float64(scale.Num) / float64(scale.Den)
			if source.MultiplierPath != "" {
				multiplierScale := source.MultiplierScale
				if multiplierScale.Den == 0 {
					multiplierScale = identityScale
				}
				want *= 2 * float64(multiplierScale.Num) / float64(multiplierScale.Den)
			}
			if descriptor.Algorithm != algorithmAbsolute {
				setSourceTestPath(sourceTestDocument(node, source.Document), source.Path, json.Number("4"))
				next := time.Unix(101, 0)
				if descriptor.Algorithm == algorithmDurationPercent {
					next = time.Unix(1100, 0)
					want /= 10
				}
				value = requireScalarValue(t, client, node, descriptor.ID, next)
			}
			if math.Abs(value.Value-want) > 1e-9*math.Max(1, math.Abs(want)) {
				t.Fatalf("scaled scalar = %v, want %v", value.Value, want)
			}
		})
	}

	for key, readingType := range readingTypes {
		t.Run("reading/"+key.SourceType, func(t *testing.T) {
			reading := normalizeReading(
				&Resource{
					Kind: "sensor",
					Key:  key.SourceType,
				},
				rawReading{
					Path:    "Reading",
					Type:    key.SourceType,
					Units:   key.Units,
					Basis:   "Zero",
					Role:    "input",
					Value:   json.Number("2"),
					Primary: true,
				},
			)
			if !reading.Valid {
				t.Fatalf("reading conversion did not normalize: %+v", reading)
			}
			scale := readingType.Scale
			if scale.Den == 0 {
				scale = identityScale
			}
			want := 2 * float64(scale.Num) / float64(scale.Den)
			if reading.Value != want {
				t.Fatalf("scaled reading = %v, want %v", reading.Value, want)
			}
		})
	}
}

func TestEverySourceScalarFieldHasPresenceNullAndTypeSemantics(t *testing.T) {
	for _, descriptor := range scalarFields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("0"))
			client := New("", "", nil)
			value, ok := scalarValueByID(client.scalarValues(node, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || !value.Valid || value.Value != 0 {
				t.Fatalf("valid zero source = %+v present=%t", value, ok)
			}

			absent := &Resource{
				Kind:       string(descriptor.Kind),
				Key:        descriptor.ID,
				Data:       make(map[string]any),
				Enrichment: make(map[string]Enrichment),
			}
			if _, ok := scalarValueByID(client.scalarValues(absent, time.Unix(100, 0)), descriptor.ID); ok {
				t.Fatal("absent source produced a scalar value")
			}

			nullNode := scalarTestNode(descriptor, source, nil)
			value, ok = scalarValueByID(client.scalarValues(nullNode, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || value.Valid || value.Emit {
				t.Fatalf("null source = %+v present=%t, want a present gap", value, ok)
			}

			wrongNode := scalarTestNode(descriptor, source, map[string]any{"unexpected": true})
			value, ok = scalarValueByID(client.scalarValues(wrongNode, time.Unix(100, 0)), descriptor.ID)
			if !ok || !value.Present || value.Valid || value.Emit {
				t.Fatalf("wrong-typed source = %+v present=%t, want a present gap", value, ok)
			}
		})
	}
}

func TestEverySourceScalarFallbackHasPrecedenceAndFailureProvenance(t *testing.T) {
	for _, descriptor := range scalarFields {
		if len(descriptor.Candidates) < 2 {
			continue
		}
		for selectedIndex := range descriptor.Candidates {
			t.Run(fmt.Sprintf("%s/source-%d", descriptor.ID, selectedIndex), func(t *testing.T) {
				node := &Resource{
					Kind:       string(descriptor.Kind),
					Key:        descriptor.ID,
					Data:       make(map[string]any),
					Enrichment: make(map[string]Enrichment),
				}
				for index, source := range descriptor.Candidates {
					document := sourceTestDocument(node, source.Document)
					for _, requirement := range source.Requires {
						setSourceTestPath(document, requirement.Path, requirement.Value)
					}
					value := any(json.Number(fmt.Sprintf("%d", index+10)))
					if index < selectedIndex {
						value = map[string]any{"malformed": true}
					}
					setSourceTestPath(document, source.Path, value)
					setScalarTestMultiplier(node, source)
				}

				selected := descriptor.Candidates[selectedIndex]
				selectedDocument := sourceTestDocument(node, selected.Document)
				for _, requirement := range selected.Requires {
					setSourceTestPath(selectedDocument, requirement.Path, requirement.Value)
				}
				setSourceTestPath(selectedDocument, selected.Path, json.Number(fmt.Sprint(selectedIndex+10)))
				setScalarTestMultiplier(node, selected)

				client := New("", "", nil)
				values := client.scalarValues(node, time.Unix(100, 0))
				value, ok := scalarValueByID(values, descriptor.ID)
				if !ok || !value.Valid {
					t.Fatalf("selected fallback = %+v present=%t", value, ok)
				}
				source := descriptor.Candidates[selectedIndex]
				scale := descriptor.Scale
				if source.Scale.Den != 0 {
					scale = source.Scale
				}
				want := float64(selectedIndex+10) * float64(scale.Num) / float64(scale.Den)
				if source.MultiplierPath != "" {
					multiplierScale := source.MultiplierScale
					if multiplierScale.Den == 0 {
						multiplierScale = identityScale
					}
					want *= 2 * float64(multiplierScale.Num) / float64(multiplierScale.Den)
				}
				if descriptor.Algorithm != algorithmAbsolute {
					setSourceTestPath(
						sourceTestDocument(node, source.Document),
						source.Path,
						json.Number(fmt.Sprint(2*(selectedIndex+10))),
					)
					next := time.Unix(101, 0)
					if descriptor.Algorithm == algorithmDurationPercent {
						next = time.Unix(1100, 0)
						want /= 10
					}
					value = requireScalarValue(t, client, node, descriptor.ID, next)
				}
				if math.Abs(value.Value-want) > 1e-9*math.Max(1, math.Abs(want)) {
					t.Fatalf("selected source value = %v, want %v", value.Value, want)
				}
				expectedFailures := 0
				for _, source := range descriptor.Candidates[:selectedIndex] {
					document := sourceTestDocument(node, source.Document)
					if sourceRequirementsMatch(document, source.Requires) {
						expectedFailures++
					}
				}
				if len(value.SourceFailures) != expectedFailures {
					t.Fatalf(
						"preferred failure provenance = %v, want %d entries",
						value.SourceFailures,
						expectedFailures,
					)
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

func TestEverySourceRateFieldResetsOnDecreaseAndEpochChange(t *testing.T) {
	for _, descriptor := range scalarFields {
		if descriptor.Algorithm == algorithmAbsolute {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			source := descriptor.Candidates[0]
			node := scalarTestNode(descriptor, source, json.Number("10"))
			document := sourceTestDocument(node, source.Document)
			client := New("", "", nil)
			at := time.Unix(100, 0)

			if value := requireScalarValue(t, client, node, descriptor.ID, at); value.Emit {
				t.Fatal("first sample emitted instead of establishing a baseline")
			}
			increment := "20"
			if descriptor.Algorithm == algorithmDurationPercent {
				increment = "10.01"
			}
			setSourceTestPath(document, source.Path, json.Number(increment))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(10*time.Second)); !value.Emit {
				t.Fatal("increasing sample did not emit")
			}
			setSourceTestPath(document, source.Path, json.Number("5"))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(20*time.Second)); value.Emit {
				t.Fatal("decrease emitted instead of resetting the baseline")
			}
			postReset := "6"
			if descriptor.Algorithm == algorithmDurationPercent {
				postReset = "5.01"
			}
			setSourceTestPath(document, source.Path, json.Number(postReset))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(30*time.Second)); !value.Emit {
				t.Fatal("post-reset increase did not emit")
			}
			setSourceTestPath(document, "LifetimeStartDateTime", "2026-07-31T00:00:00Z")
			epochValue := "7"
			if descriptor.Algorithm == algorithmDurationPercent {
				epochValue = "5.02"
			}
			setSourceTestPath(document, source.Path, json.Number(epochValue))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(40*time.Second)); value.Emit {
				t.Fatal("epoch change emitted instead of resetting the baseline")
			}
			postEpoch := "8"
			if descriptor.Algorithm == algorithmDurationPercent {
				postEpoch = "5.03"
			}
			setSourceTestPath(document, source.Path, json.Number(postEpoch))
			if value := requireScalarValue(t, client, node, descriptor.ID, at.Add(50*time.Second)); !value.Emit {
				t.Fatal("post-epoch increase did not emit")
			}
		})
	}
}

func TestExplicitlyRejectedMetricClassesStayUncharted(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		data       map[string]any
		enrichment map[string]Enrichment
	}{
		{
			"ambiguous memory speed",
			"memory",
			map[string]any{"OperatingSpeedMhz": 3200},
			map[string]Enrichment{"memory_metrics": {Data: map[string]any{"OperatingSpeedMHz": 3200}}},
		},
		{
			"undefined processor bandwidth",
			"processor",
			nil,
			map[string]Enrichment{
				"processor_metrics": {
					Data: map[string]any{"LocalMemoryBandwidthBytes": 1, "RemoteMemoryBandwidthBytes": 1},
				},
			},
		},
		{
			"generic lifetime reading",
			"sensor",
			map[string]any{"LifetimeReading": 1, "ReadingType": "EnergyJoules", "ReadingUnits": "J"},
			nil,
		},
		{
			"rated efficiency curve",
			"power_supply",
			map[string]any{"EfficiencyRatings": []any{map[string]any{"EfficiencyPercent": 90}}},
			nil,
		},
		{
			"battery ratings",
			"battery",
			map[string]any{"NominalVoltage": 12, "RatedCapacity": 100, "MaxChargeRateWatts": 20},
			nil,
		},
		{"pcie identifiers", "pcie_function", map[string]any{"FunctionId": 1, "DeviceId": 2, "VendorId": 3}, nil},
		{"ethernet traffic", "ethernet_interface", map[string]any{"BytesReceived": 1, "BytesSent": 2}, nil},
		{"network port traffic", "network_port", map[string]any{"PacketsReceived": 1, "PacketsSent": 2}, nil},
		{
			"open processor cache arrays",
			"processor",
			nil,
			map[string]Enrichment{
				"processor_metrics": {Data: map[string]any{
					"Cache":             []any{map[string]any{"Level": 1}},
					"CacheMetricsTotal": map[string]any{"HitRatio": 1},
				}},
			},
		},
		{
			"nested fabric arrays",
			"network_adapter",
			map[string]any{
				"FibreChannel": []any{map[string]any{"Errors": 1}},
				"SAS":          []any{map[string]any{"Errors": 1}},
			},
			nil,
		},
		{"firmware size", "firmware", map[string]any{"SizeBytes": 1, "DeviceCount": 2}, nil},
		{
			"configuration",
			"system",
			map[string]any{
				"Boot":    map[string]any{"BootSourceOverrideEnabled": "Once"},
				"Actions": map[string]any{"Reset": true},
			},
			nil,
		},
		{
			"oem unknown",
			"drive",
			map[string]any{"Oem": map[string]any{"VendorMetric": 1}, "FutureStandardMetric": 2},
			nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := &Resource{
				Kind:       test.kind,
				Key:        test.name,
				Data:       test.data,
				Enrichment: test.enrichment,
			}
			client := New("", "", nil)
			for _, value := range client.scalarValues(node, time.Unix(100, 0)) {
				if value.Emit {
					t.Errorf("rejected class emitted scalar metric %q", value.Descriptor.Metric)
				}
			}
			for _, reading := range client.readingsForNode(node, time.Unix(100, 0)) {
				if reading.Valid {
					t.Errorf("rejected class emitted reading %q", reading.SourcePath)
				}
			}
		})
	}
}

func syntheticRawReadingForSurface(
	key readingKey,
	surface readingDescriptor,
	value any,
) (rawReading, *Resource) {
	sourceType, sourceUnits, fixed := sourceReadingSource(key.Family)
	raw := rawReading{
		Path:           "Synthetic." + key.Family + "." + key.Basis + "." + key.Role,
		IdentitySource: "Synthetic." + key.Family + "." + key.Basis + "." + key.Role,
		Type:           sourceType,
		Units:          sourceUnits,
		Basis:          key.Basis,
		Role:           key.Role,
		Value:          value,
		Primary:        true,
		ReadingScoped:  surface.AlarmMetric != "",
		Health:         "OK",
	}
	if fixed {
		raw.FixedFamily = key.Family
	}
	node := &Resource{
		Kind: "sensor",
		Key:  raw.IdentitySource,
		Data: make(map[string]any),
	}
	if key.SemanticClass == "fan" {
		node.Kind = "fan"
	}
	return raw, node
}

func scalarTestNode(descriptor sourceField, source scalarSource, value any) *Resource {
	node := &Resource{
		Kind:       string(descriptor.Kind),
		Key:        descriptor.ID,
		Data:       make(map[string]any),
		Enrichment: make(map[string]Enrichment),
	}
	document := sourceTestDocument(node, source.Document)
	for _, requirement := range source.Requires {
		setSourceTestPath(document, requirement.Path, requirement.Value)
	}
	setSourceTestPath(document, source.Path, value)
	setScalarTestMultiplier(node, source)
	return node
}

func setScalarTestMultiplier(node *Resource, source scalarSource) {
	if source.MultiplierPath == "" {
		return
	}
	document := sourceTestDocument(node, source.Document)
	if source.MultiplierDocument != "" {
		document = sourceTestDocument(node, source.MultiplierDocument)
	}
	setSourceTestPath(document, source.MultiplierPath, json.Number("2"))
}

func requireScalarValue(
	t *testing.T,
	client *Projector,
	node *Resource,
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

var benchmarkReading readingDescriptor
var benchmarkReadingFound bool

func BenchmarkMatchReadingSurface(b *testing.B) {
	for name, key := range map[string]readingKey{
		"common":   {"temperature", "zero", "input", "direct"},
		"fallback": {"power", "delta", "average", "direct"},
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkReading, benchmarkReadingFound = matchReading(
					key.Family,
					key.Basis,
					key.Role,
					key.SemanticClass,
				)
			}
			if !benchmarkReadingFound {
				b.Fatal("source mapping missing")
			}
		})
	}
}

// Scalar selection is linear in the fixed per-kind candidate inventory.
// Allocations track observations and diagnostics; ns/op is a local-machine trend.
func BenchmarkHardwareScalarValues(b *testing.B) {
	client := New("", "", nil)
	node := &Resource{
		Kind: "memory",
		Key:  "dimm-1",
		Enrichment: map[string]Enrichment{
			"memory_metrics": {Data: map[string]any{"CapacityUtilizationPercent": json.Number("50")}},
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		values := client.scalarValues(node, time.Unix(int64(i)+1, 0))
		if len(values) == 0 {
			b.Fatal("capacity observation missing")
		}
	}
}
