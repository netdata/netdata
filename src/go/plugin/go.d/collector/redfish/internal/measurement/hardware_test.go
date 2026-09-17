// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/stretchr/testify/require"
)

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
	client := New("", "", nil)
	const key = "0123456789abcdef0123456789abcdef"
	require.NoError(t, client.validateAndRegisterReadingIdentities(map[string][]normalizedReading{
		"node-a": {{Key: key, IdentitySource: "Sensor.Reading"}},
	}))
	err := client.validateAndRegisterReadingIdentities(map[string][]normalizedReading{
		"node-b": {{Key: key, IdentitySource: "Sensor.Reading"}},
	})
	require.ErrorContains(t, err, "Redfish reading-key collision")
	require.True(t, identity.IsIntegrityError(err))
}

func TestManagerClockValueUsesRequestMidpoint(t *testing.T) {
	started := time.Now()
	finished := started.Add(2 * time.Second)
	managerTime := started.Round(0).Add(6 * time.Second)
	node := &Resource{
		Kind: "manager",
		Data: map[string]any{"DateTime": managerTime.Format(time.RFC3339Nano)},
		Response: ResponseTiming{
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
	node := &Resource{
		Kind: "manager",
		Data: map[string]any{"DateTime": "2026-07-30T12:00:00"},
		Response: ResponseTiming{
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
			client := New("", "", nil)
			node := &Resource{
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
	client := New("", "", nil)
	node := &Resource{
		Kind: "memory",
		Enrichment: map[string]Enrichment{
			"memory_metrics": {Data: map[string]any{
				"BlockSizeBytes": json.Number("4096"),
				"CurrentPeriod": map[string]any{
					"BlocksRead": json.Number("2"),
				},
			}},
		},
	}

	at := time.Unix(100, 0)
	value, found := scalarValueByID(client.scalarValues(node, at), "memory_current_period_blocks_read")
	require.True(t, found)
	require.True(t, value.Valid)
	require.False(t, value.Emit)
	node.Enrichment["memory_metrics"].Data["CurrentPeriod"].(map[string]any)["BlocksRead"] = json.Number("4")
	value, found = scalarValueByID(client.scalarValues(node, at.Add(time.Second)), "memory_current_period_blocks_read")
	require.True(t, found)
	require.True(t, value.Emit)
	require.Equal(t, float64(8192), value.Value)

}

func TestEverySourceScalarFieldHasRuntimeProducer(t *testing.T) {
	at := time.Now()
	for _, descriptor := range scalarFields {
		if descriptor.ID == managerClockDescriptor.ID {
			continue
		}
		t.Run(descriptor.ID, func(t *testing.T) {
			requireSourceDescriptorRuntimeValue(t, descriptor, at)
		})
	}
}

func TestScalarFallbackRetainsSelectedSourceAndPreferredFailureProvenance(t *testing.T) {
	node := &Resource{
		Kind: "system",
		Key:  "system",
		Data: map[string]any{
			"ProcessorSummary": map[string]any{"Metrics": map[string]any{"BandwidthPercent": json.Number("42")}},
		},
		Enrichment: map[string]Enrichment{
			"processor_summary_metrics": {Data: map[string]any{"BandwidthPercent": "not-a-number"}},
		},
	}

	values := (New("", "", nil)).scalarValues(node, time.Now())
	if len(values) != 1 {
		t.Fatalf("scalarValues() returned %d values, want 1", len(values))
	}
	got := values[0]
	if got.Value != 42 {
		t.Fatalf("fallback value = %+v, want value 42 from Fallback", got)
	}
	if len(got.SourceFailures) != 1 ||
		!strings.Contains(got.SourceFailures[0], "processor_summary_metrics.BandwidthPercent") ||
		!strings.Contains(got.SourceFailures[0], "malformed") {
		t.Fatalf("fallback failure provenance = %v", got.SourceFailures)
	}
}

func TestFindEnrichmentRejectsAmbiguousDocumentKind(t *testing.T) {
	node := &Resource{
		Enrichment: map[string]Enrichment{
			"memory_metrics:0": {Data: map[string]any{"Reading": json.Number("1")}},
			"memory_metrics:1": {Data: map[string]any{"Reading": json.Number("2")}},
		},
	}
	require.Nil(t, findEnrichment(node, "memory_metrics"))

	delete(node.Enrichment, "memory_metrics:1")
	require.Equal(t, json.Number("1"), findEnrichment(node, "memory_metrics")["Reading"])
}

func TestEverySourceStateAndFlagHasRuntimeProducer(t *testing.T) {
	client := New("", "", nil)
	for _, source := range additionalStateSources {
		t.Run("state/"+source.Metric, func(t *testing.T) {
			node := &Resource{
				Kind:       string(source.Kind),
				Key:        source.Metric,
				Data:       make(map[string]any),
				Enrichment: make(map[string]Enrichment),
			}
			document := sourceTestDocument(node, source.Document)
			value := any(source.States[0])
			wantState := source.States[0]
			if source.BooleanFalse != "" || source.BooleanTrue != "" {
				value = false
				wantState = source.BooleanFalse
			}
			setSourceTestPath(document, source.Path, value)
			observation := observationByMetric(client.statusObservations(node), source.Metric)
			if observation == nil {
				t.Fatalf("state source %s.%s has no runtime observation", source.Document, source.Path)
			}
			if observation.State != wantState {
				t.Fatalf("state = %q, want %q", observation.State, wantState)
			}
		})
	}

	for _, set := range sourceFlagSets {
		t.Run("flags/"+set.Metric, func(t *testing.T) {
			node := &Resource{
				Kind:       string(set.Kind),
				Key:        set.Metric,
				Data:       make(map[string]any),
				Enrichment: make(map[string]Enrichment),
			}
			document := sourceTestDocument(node, set.Document)
			for _, member := range set.Members {
				setSourceTestPath(document, member.Path, true)
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

func TestEverySourceReadingSurfaceHasRuntimeNormalizer(t *testing.T) {
	for key, surface := range readingDescriptors {
		if key.Role == "energy_rate" {
			continue
		}
		name := strings.Join(
			[]string{key.Family, key.Basis, key.Role, key.SemanticClass},
			"/",
		)
		t.Run(name, func(t *testing.T) {
			sourceType, sourceUnits, fixed := sourceReadingSource(key.Family)
			raw := rawReading{
				Path:           "Synthetic." + name,
				IdentitySource: "Synthetic." + name,
				Type:           sourceType,
				Units:          sourceUnits,
				Basis:          key.Basis,
				Role:           key.Role,
				Value:          json.Number("10"),
				Primary:        true,
				ReadingScoped:  surface.AlarmMetric != "",
				Health:         "OK",
			}
			if fixed {
				raw.FixedFamily = key.Family
			}
			node := &Resource{
				Kind: "sensor",
				Key:  name,
			}
			switch key.SemanticClass {
			case "fan":
				node.Kind = "fan"
			case "", "direct", "ambient_pressure":
			default:
				t.Fatalf("unrecognized semantic class %q", key.SemanticClass)
			}

			reading := normalizeReading(node, raw)
			if !reading.Valid {
				t.Fatalf("reading is invalid: %+v", reading)
			}
			if reading.Metric != surface.Metric || reading.AlarmMetric != surface.AlarmMetric {
				t.Fatalf(
					"normalized surface = metric %q alarm %q; want %+v",
					reading.Metric,
					reading.AlarmMetric,
					surface,
				)
			}
			observations := (New("", "", nil)).readingObservations(node, reading)
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

func sourceReadingSource(family string) (sourceType, sourceUnits string, fixed bool) {
	for key, source := range readingTypes {
		if source.Family == family {
			return key.SourceType, key.Units, false
		}
	}
	return "Synthetic", "Synthetic", true
}

func requireSourceDescriptorRuntimeValue(
	t *testing.T,
	descriptor sourceField,
	at time.Time,
) {
	t.Helper()
	if len(descriptor.Candidates) == 0 {
		t.Fatal("descriptor has no source candidates")
	}
	source := descriptor.Candidates[0]
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
	setSourceTestPath(document, source.Path, json.Number("10"))
	if source.MultiplierPath != "" {
		multiplierDocument := document
		if source.MultiplierDocument != "" {
			multiplierDocument = sourceTestDocument(node, source.MultiplierDocument)
		}
		setSourceTestPath(multiplierDocument, source.MultiplierPath, json.Number("2"))
	}

	client := New("", "", nil)
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
	if descriptor.Algorithm == algorithmAbsolute {
		if !first.Emit {
			t.Fatalf("absolute scalar = %+v, want emitted", first)
		}
		return
	}
	if first.Emit {
		t.Fatalf("first counter sample = %+v, want baseline only", first)
	}

	setSourceTestPath(document, source.Path, json.Number("11"))
	values = client.scalarValues(node, at.Add(100*time.Second))
	second, ok := scalarValueByID(values, descriptor.ID)
	if !ok || !second.Present || !second.Valid || !second.Emit {
		t.Fatalf("second counter sample = %+v present=%t, want emitted", second, ok)
	}
}

func sourceTestDocument(node *Resource, document string) map[string]any {
	if document == "" {
		return node.Data
	}
	key := string(document)
	value := node.Enrichment[key].Data
	if value == nil {
		value = make(map[string]any)
		node.Enrichment[key] = Enrichment{
			Data: value,
		}
	}
	return value
}

func setSourceTestPath(document map[string]any, path string, value any) {
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

func observationByMetric(values []Observation, metric string) *Observation {
	for index := range values {
		if values[index].Metric == metric {
			return &values[index]
		}
	}
	return nil
}

func TestReadingSourceAlarmDiagnostics(t *testing.T) {
	node := &Resource{
		Kind: "sensor",
		Key:  "sensor-key",
		URI:  "/redfish/v1/Chassis/1/Sensors/1",
	}
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
			reading := normalizeReading(node, raw)
			if reading.SourceAlarm != test.wantAlarm {
				t.Fatalf("source alarm = %q, want %q", reading.SourceAlarm, test.wantAlarm)
			}
			if !strings.Contains(reading.SourceAlarmDiagnostic, test.wantDiagnostic) {
				t.Fatalf(
					"source alarm diagnostic = %q, want substring %q",
					reading.SourceAlarmDiagnostic,
					test.wantDiagnostic,
				)
			}
		})
	}

	notReadingScoped := base
	notReadingScoped.ReadingScoped = false
	reading := normalizeReading(node, notReadingScoped)
	if reading.SourceAlarmDiagnostic != "" {
		t.Fatalf("ineligible resource-level health produced reading diagnostic %q", reading.SourceAlarmDiagnostic)
	}
}

func TestReadingSourceAlarmSurvivesInvalidNumericValue(t *testing.T) {
	node := &Resource{
		Kind: "sensor",
		Key:  "sensor-key",
		URI:  "/redfish/v1/Chassis/1/Sensors/1",
	}
	reading := normalizeReading(node, rawReading{
		Path:          "Reading",
		Type:          "Temperature",
		Units:         "Cel",
		Value:         map[string]any{"invalid": true},
		ReadingScoped: true,
		Health:        "Critical",
	})
	if reading.Valid || reading.SourceAlarm != "critical" {
		t.Fatalf("invalid numeric reading lost source alarm: %+v", reading)
	}

	observations := (New("", "", nil)).readingObservations(node, reading)
	if len(observations) != 1 || observations[0].Metric != reading.AlarmMetric {
		t.Fatalf("observations = %+v, want only source alarm %q", observations, reading.AlarmMetric)
	}
}

func TestReadingAlarmEmitsEverySourceStateWithoutFabricatingMissing(t *testing.T) {
	client := New("", "", nil)
	node := &Resource{
		Kind: "sensor",
		Key:  "sensor",
	}
	for _, state := range alarmStates {
		reading := normalizedReading{
			Key:         "reading",
			Metric:      "reading_temperature_zero_input",
			AlarmMetric: "reading_temperature_zero_input_alarm",
			Valid:       true,
			SourceAlarm: state,
		}
		observation := observationByMetric(client.readingObservations(node, reading), reading.AlarmMetric)
		if observation == nil || observation.State != state {
			t.Fatalf("alarm state %q observation = %#v", state, observation)
		}
	}
	reading := normalizedReading{
		Key:         "reading",
		Metric:      "reading_temperature_zero_input",
		AlarmMetric: "reading_temperature_zero_input_alarm",
		Valid:       true,
	}
	if observation := observationByMetric(client.readingObservations(node, reading), reading.AlarmMetric); observation != nil {
		t.Fatalf("missing effective alarm fabricated observation %#v", observation)
	}
}

func TestDerivedPowerResetsBaselineOnReadingSemanticChange(t *testing.T) {
	client := New("", "", nil)
	node := &Resource{
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
		t.Fatalf(
			"post-change sample derived power count = %d, want 1; readings=%#v baselines=%#v",
			got,
			postChange,
			client.rateBaselines,
		)
	}
}

func TestRateBaselineResetsWhenMultiplierChanges(t *testing.T) {
	client := New("", "", nil)
	t0 := time.Unix(100, 0)

	_, emit := client.rateValue("memory-blocks", "100", 512, t0, algorithmRate, "epoch")
	require.False(t, emit)
	_, emit = client.rateValue(
		"memory-blocks", "110", 4096, t0.Add(time.Second), algorithmRate, "epoch",
	)
	require.False(t, emit)

	value, emit := client.rateValue(
		"memory-blocks", "120", 4096, t0.Add(2*time.Second), algorithmRate, "epoch",
	)
	require.True(t, emit)
	require.Equal(t, 40_960.0, value)
}

func TestOversizedProtocolNumbersFailSoftWithoutRetainedBaselines(t *testing.T) {
	integer := strings.Repeat("1", maxProtocolNumericTokenBytes+1)
	fraction := "0." + strings.Repeat("0", maxProtocolNumericTokenBytes) + "1"
	for name, value := range map[string]any{
		"integer":            json.Number(integer),
		"fraction":           json.Number(fraction),
		"duration component": "PT" + fraction + "S",
		"duration total":     "PT0." + strings.Repeat("0", maxProtocolDurationTokenBytes) + "1S",
	} {
		t.Run(name, func(t *testing.T) {
			client := fixtureClient()
			node := &Resource{
				Kind: "processor",
				Key:  name,
				Enrichment: map[string]Enrichment{
					"processor_metrics": {Data: map[string]any{"PowerLimitThrottleDuration": value}},
				},
			}
			values := client.scalarValues(node, time.Unix(10, 0))
			require.Len(t, values, 1)
			require.False(t, values[0].Valid)
			require.False(t, values[0].Emit)
			require.Len(t, values[0].SourceFailures, 1)

			if _, numeric := value.(json.Number); numeric {
				energy := &Resource{
					Kind: "sensor",
					Key:  "energy",
					Data: map[string]any{
						"ReadingType": "EnergyJoules", "ReadingUnits": "J", "Reading": value,
					},
				}
				readings := client.readingsForNode(energy, time.Unix(10, 0))
				require.Len(t, readings, 1)
				require.False(t, readings[0].Valid)
				require.Empty(t, readings[0].SourceExact)
			}
			require.Empty(t, client.rateBaselines)
		})
	}
}

func TestRateEpochsDigestOversizedBMCValues(t *testing.T) {
	first := strings.Repeat("a", 1<<20)
	second := strings.Repeat("b", 1<<20)

	managerFirst := rateEpoch(map[string]any{"LifetimeStartDateTime": first})
	managerSecond := rateEpoch(map[string]any{"LifetimeStartDateTime": second})
	require.Len(t, managerFirst, 64)
	require.Len(t, managerSecond, 64)
	require.NotEqual(t, managerFirst, managerSecond)

	reading := normalizedReading{
		SourcePath:    "Reading",
		DataSourceURI: &first,
	}
	readingFirst := readingRateEpoch(reading)
	reading.DataSourceURI = &second
	readingSecond := readingRateEpoch(reading)
	require.Len(t, readingFirst, 64)
	require.Len(t, readingSecond, 64)
	require.NotEqual(t, readingFirst, readingSecond)
}

func TestDerivedPowerResetsBaselineOnDecreaseAndEpochChange(t *testing.T) {
	t.Parallel()

	client := New("", "", nil)
	node := &Resource{
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

func TestFlagObservationsDoNotFabricateMissingSiblingValues(t *testing.T) {
	client := New("", "", nil)
	node := &Resource{
		Kind: "memory",
		Enrichment: map[string]Enrichment{
			"memory_metrics:0": {Data: map[string]any{
				"HealthData": map[string]any{
					"DataLossDetected":    true,
					"LastShutdownSuccess": nil,
					"PerformanceDegraded": "false",
				},
			}},
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
	client := New("", "", nil)
	node := &Resource{
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
	var resource Document
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("decode conditions: %v", err)
	}
	counts := conditionCountsFrom(resource.Status.Conditions)
	if counts.OK != 1 || counts.Warning != 0 || counts.Critical != 0 || counts.Unknown != 2 {
		t.Fatalf("condition counts = %+v, want OK=1 Unknown=2", counts)
	}
}

func TestConditionDeduplicationPreservesStructuralFieldBoundaries(t *testing.T) {
	conditions := []Condition{
		{
			MessageID:   "same",
			MessageArgs: []string{"argument"},
			OriginOfCondition: Link{
				ODataID: "origin\x00shifted",
			},
			Timestamp: "timestamp",
			Severity:  json.RawMessage(`"Warning"`),
		},
		{
			MessageID:   "same",
			MessageArgs: []string{"argument"},
			OriginOfCondition: Link{
				ODataID: "origin",
			},
			Timestamp: "shifted\x00timestamp",
			Severity:  json.RawMessage(`"Warning"`),
		},
	}

	counts := conditionCountsFrom(conditions)
	require.Equal(t, 2, counts.Warning)
}

func TestNormalizeReadingRejectsValueOutsideAdvertisedRange(t *testing.T) {
	reading := normalizeReading(
		&Resource{
			Kind: "sensor",
			Key:  "sensor-key",
		},
		rawReading{
			Path:     "Sensor.Reading",
			Type:     "Temperature",
			Units:    "Cel",
			Basis:    "Zero",
			Role:     "input",
			Value:    json.Number("101"),
			Primary:  true,
			RangeMin: json.Number("0"),
			RangeMax: json.Number("100"),
		},
	)
	if reading.Valid {
		t.Fatal("reading outside ReadingRangeMax remained valid")
	}
	if reading.RangeMin == nil || *reading.RangeMin != 0 ||
		reading.RangeMax == nil || *reading.RangeMax != 100 {
		t.Fatalf("normalized reading range = [%v,%v], want [0,100]", reading.RangeMin, reading.RangeMax)
	}
}

func observationLabel(labels []metrix.Label, key string) string {
	for _, label := range labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
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

func TestReadingEpochMetadataPreservesSourcePresence(t *testing.T) {
	const sourceTime = "2026-07-31T00:00:00Z"
	const excerptTime = "2026-08-01T00:00:00Z"
	for _, field := range []string{"SensorResetTime", "LifetimeStartDateTime"} {
		for name, test := range map[string]struct {
			value   any
			present bool
			want    string
		}{
			"missing uses excerpt":     {want: excerptTime},
			"null stays unknown":       {present: true},
			"wrong type stays unknown": {value: true, present: true},
			"source wins":              {value: sourceTime, present: true, want: sourceTime},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				node := &Resource{
					Kind: "sensor",
					Key:  "energy-sensor",
					Data: map[string]any{
						"ReadingType": "EnergyJoules", "ReadingUnits": "J",
						"Reading": json.Number("10"),
					},
					SensorExcerpts: []SensorExcerpt{{
						Path: "EnergyJoules", Type: "EnergyJoules", Units: "J",
						Data: map[string]any{"Reading": json.Number("10"), field: excerptTime},
					}},
				}
				if test.present {
					node.Data[field] = test.value
				}
				client := New("", "", nil)
				readings := client.readingsForNode(node, time.Unix(100, 0))
				require.Len(t, readings, 1)
				got := readings[0].SensorResetTime
				if field == "LifetimeStartDateTime" {
					got = readings[0].LifetimeStartDateTime
				}
				if test.want == "" {
					require.Nil(t, got)
				} else {
					want, err := time.Parse(time.RFC3339, test.want)
					require.NoError(t, err)
					require.NotNil(t, got)
					require.Equal(t, want.UnixMilli(), *got)
				}
			})
		}
	}
}
