// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCompareTopFamilyOrdersKnownThenUnknownFamilies(t *testing.T) {
	values := []string{"Zeta", "Compute", "Alpha", "Overview"}
	slices.SortFunc(values, CompareTopFamily)
	want := []string{"Overview", "Compute", "Alpha", "Zeta"}
	if !slices.Equal(values, want) {
		t.Fatalf("top families = %v, want %v", values, want)
	}
}

func TestCompile(t *testing.T) {
	contract, err := Compile()
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Fields) == 0 || len(contract.Charts) == 0 || len(contract.Columns) == 0 {
		t.Fatalf(
			"compiled contract is incomplete: fields=%d charts=%d columns=%d",
			len(contract.Fields),
			len(contract.Charts),
			len(contract.Columns),
		)
	}
}

func TestOperationalChartTypesAreExplicit(t *testing.T) {
	contract := MustCompile()
	charts := make(map[string]ChartSpec, len(contract.Charts))
	for _, chart := range contract.Charts {
		charts[chart.Context] = chart
	}

	for _, source := range contract.Operational {
		if source.Type != ChartLine && source.Type != ChartStacked {
			t.Errorf("operational context %q has chart type %q", source.Context, source.Type)
		}
		if chart := charts[source.Context]; chart.Type != source.Type {
			t.Errorf("operational context %q compiled as %q, want %q", source.Context, chart.Type, source.Type)
		}
	}

	for _, context := range []string{
		"redfish.collection.http_requests",
		"redfish.collection.resources",
		"redfish.collection.log_services",
		"redfish.collection.detail_components",
		"redfish.log_backend.storage",
		"redfish.log_backend.pipeline",
		"redfish.log_service.pipeline",
		"redfish.log_service.reconciliation",
	} {
		if chart := charts[context]; chart.Type != ChartLine {
			t.Errorf("comparison/overlapping context %q compiled as %q, want %q", context, chart.Type, ChartLine)
		}
	}
	for _, context := range []string{
		"redfish.collection.operations",
		"redfish.log_backend.files",
	} {
		if chart := charts[context]; chart.Type != ChartStacked {
			t.Errorf("additive context %q compiled as %q, want %q", context, chart.Type, ChartStacked)
		}
	}
}

func TestCompileChartsRejectsMissingOperationalChartType(t *testing.T) {
	contract := MustCompile()
	contract.Operational[0].Type = ""
	_, err := compileCharts(contract)
	if err == nil || !strings.Contains(err.Error(), "invalid chart type") {
		t.Fatalf("compileCharts() error = %v, want invalid chart type", err)
	}
}

func TestCompileReturnsDeeplyIsolatedColumns(t *testing.T) {
	first := MustCompile()
	var mutatedID string
	for index := range first.Columns {
		if first.Columns[index].Members == nil {
			continue
		}
		mutatedID = first.Columns[index].ID
		first.Columns[index].Members["test_mutation"] = struct{}{}
		break
	}
	if mutatedID == "" {
		t.Fatal("compiled registry has no kind-scoped column")
	}

	second := MustCompile()
	for _, column := range second.Columns {
		if column.ID != mutatedID {
			continue
		}
		if _, ok := column.Members["test_mutation"]; ok {
			t.Fatalf("column %q shared its Members map across Compile calls", mutatedID)
		}
		return
	}
	t.Fatalf("mutated column %q is missing from the second contract", mutatedID)
}

func TestCompileChartsRejectsUnknownKindsBeforeConstructingCharts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Contract)
	}{
		{name: "status", edit: func(contract *Contract) { contract.Status[0].Kind = "unknown" }},
		{name: "state", edit: func(contract *Contract) { contract.States[0].Kind = "unknown" }},
		{name: "flag set", edit: func(contract *Contract) { contract.Flags[0].Kind = "unknown" }},
		{name: "field", edit: func(contract *Contract) {
			for index := range contract.Fields {
				if contract.Fields[index].Exposure == ExposureOperationalScalar {
					contract.Fields[index].Kind = "unknown"
					return
				}
			}
			panic("compiled registry has no operational scalar field")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := MustCompile()
			test.edit(&contract)
			_, err := compileCharts(contract)
			if err == nil || !strings.Contains(err.Error(), `references unknown kind "unknown"`) {
				t.Fatalf("compileCharts() error = %v, want unknown-kind error", err)
			}
		})
	}
}

func TestCompileCategoricalAggregateChartsReturnsStateConflicts(t *testing.T) {
	tests := []struct {
		name     string
		contract Contract
	}{
		{
			name: "state with state",
			contract: Contract{States: []StateSpec{
				{Metric: "custom_state", States: []string{"first"}, AggregateKinds: []Kind{"system"}},
				{Metric: "custom_state", States: []string{"second"}, AggregateKinds: []Kind{"system"}},
			}},
		},
		{
			name: "state with flag set",
			contract: Contract{
				States: []StateSpec{
					{Metric: "custom_state", States: []string{"first"}, AggregateKinds: []Kind{"system"}},
				},
				Flags: []FlagSetSpec{
					{
						Metric: "custom_state", AggregateKinds: []Kind{"system"},
						Members: []FlagMemberSpec{{Role: "second"}},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileCategoricalAggregateCharts(test.contract, nil)
			if err == nil || !strings.Contains(err.Error(), `categorical aggregate class "custom_state" has incompatible states`) {
				t.Fatalf("compileCategoricalAggregateCharts() error = %v, want state conflict", err)
			}
		})
	}
}

func TestValidateRejectsStatusWithUnknownKind(t *testing.T) {
	contract := MustCompile()
	contract.Status[0].Kind = "unknown"
	err := validate(contract)
	if err == nil || !strings.Contains(err.Error(), `status references unknown kind "unknown"`) {
		t.Fatalf("validate() error = %v, want unknown status kind", err)
	}
}

func TestUniqueReadingSurfacesUsesStructuralKeys(t *testing.T) {
	source := []ReadingSurfaceSpec{
		{Family: "a", Basis: "b\x00c", Role: "d", SemanticClass: "e"},
		{Family: "a\x00b", Basis: "c", Role: "d", SemanticClass: "e"},
	}
	if got := uniqueReadingSurfaces(source); len(got) != 2 {
		t.Fatalf("uniqueReadingSurfaces() returned %d rows, want two", len(got))
	}
}

func TestCompileReadingsReturnsMissingTitleErrors(t *testing.T) {
	tests := map[string]Contract{
		"family": {
			ReadingTypes: []ReadingTypeSpec{{Family: "unreviewed", Units: "units"}},
			ReadingRoles: []ReadingRoleSpec{{ID: "input"}},
		},
		"role": {
			ReadingTypes: []ReadingTypeSpec{{Family: "temperature", Units: "Celsius"}},
			ReadingRoles: []ReadingRoleSpec{{ID: "unreviewed"}},
		},
	}

	for name, contract := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := compileReadings(contract)
			if err == nil || !strings.Contains(err.Error(), "missing reviewed reading title") {
				t.Fatalf("compileReadings() error = %v, want missing-title error", err)
			}
		})
	}
}

func TestValidateRejectsUnreachableStateAndFlagAggregateOwners(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Contract)
		want string
	}{
		{
			name: "state",
			edit: func(contract *Contract) {
				contract.States[0].AggregateKinds = []Kind{"manager"}
			},
			want: `state "redfish.chassis.intrusion_state" has unreachable aggregate owner "manager" for kind "chassis"`,
		},
		{
			name: "flag set",
			edit: func(contract *Contract) {
				contract.Flags[0].AggregateKinds = []Kind{"manager"}
			},
			want: `flag set "redfish.memory.health_flags" has unreachable aggregate owner "manager" for kind "memory"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := MustCompile()
			test.edit(&contract)
			err := validate(contract)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsUnreachableReadingAggregateOwner(t *testing.T) {
	contract := MustCompile()
	for index := range contract.Readings {
		if len(contract.Readings[index].AggregateKinds) == 0 {
			continue
		}
		contract.Readings[index].AggregateKinds = []Kind{"manager"}
		err := validate(contract)
		if err == nil || !strings.Contains(err.Error(), "has unreachable aggregate owner \"manager\"") {
			t.Fatalf("validate() error = %v, want unreachable reading owner", err)
		}
		return
	}
	t.Fatal("compiled registry has no aggregate reading to mutate")
}

func TestCompiledColumnTypes(t *testing.T) {
	contract := MustCompile()
	columns := make(map[string]ColumnType, len(contract.Columns))
	for _, column := range contract.Columns {
		columns[column.ID] = column.Type
	}

	for id, want := range map[string]ColumnType{
		"row_type":                            ColumnEnum,
		"health":                              ColumnEnum,
		"reading_type":                        ColumnEnum,
		"threshold_upper_critical_activation": ColumnEnum,
		"failure_predicted":                   ColumnBoolean,
		"control_setpoint_type":               ColumnEnum,
		"control_setting_min":                 ColumnFloat,
		"software_lowest_supported_version":   ColumnString,
		"assembly_country_of_origin":          ColumnString,
		"log_service_overflow":                ColumnBoolean,
	} {
		if got := columns[id]; got != want {
			t.Errorf("column %q type = %q, want %q", id, got, want)
		}
	}
}

func TestLogicalOwnerCandidatesColumnIsStructured(t *testing.T) {
	contract := MustCompile()
	for _, column := range contract.Columns {
		if column.ID != "logical_owner_candidates" {
			continue
		}
		if !column.Structured || column.Facet || column.Sortable {
			t.Fatalf("logical_owner_candidates presentation = structured:%t facet:%t sortable:%t", column.Structured, column.Facet, column.Sortable)
		}
		return
	}
	t.Fatal("logical_owner_candidates column is missing")
}

func TestCompileColumnsRejectsIncompatibleDuplicateTypes(t *testing.T) {
	_, err := compileColumns(Contract{Inventory: []InventoryFieldSpec{{
		Kind:   "chassis",
		Path:   "Status.Health",
		Column: "health",
		Type:   ColumnString,
	}}})
	if err == nil || !strings.Contains(err.Error(), `column "health" has conflicting types`) {
		t.Fatalf("compileColumns() error = %v, want incompatible-type error", err)
	}
}

func TestCompileColumnsPromotesCompatibleNumericTypes(t *testing.T) {
	columns, err := compileColumns(Contract{
		Inventory: []InventoryFieldSpec{{
			Kind:   "processor",
			Path:   "BaseSpeedMHz",
			Column: "test_numeric",
			Type:   ColumnInteger,
		}},
		Fields: []FieldSpec{{
			Kind:      "processor",
			Column:    "test_numeric",
			Algorithm: AlgorithmAbsolute,
			Float:     true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if column.ID == "test_numeric" {
			if column.Type != ColumnFloat {
				t.Fatalf("numeric column type = %q, want %q", column.Type, ColumnFloat)
			}
			return
		}
	}
	t.Fatal("numeric column not compiled")
}

func TestCompileColumnsKeepCommonColumnsApplicableToEveryKind(t *testing.T) {
	contract := MustCompile()
	for _, id := range []string{
		"health",
		"state",
		"power_state",
		"failure_predicted",
		"resource_kind",
		"host_uri",
	} {
		var found bool
		for _, column := range contract.Columns {
			if column.ID != id {
				continue
			}
			found = true
			if len(column.Members) != 0 {
				t.Errorf("common column %q members = %v, want all kinds", id, column.Members)
			}
			break
		}
		if !found {
			t.Errorf("common column %q is missing", id)
		}
	}
}

func TestCompileGroupsOperationalScalarRolesAndOmitsInventoryOnlyScalars(t *testing.T) {
	contract := MustCompile()
	charts := make(map[string]ChartSpec, len(contract.Charts))
	for _, chart := range contract.Charts {
		charts[chart.Context] = chart
	}
	if _, ok := charts["redfish.system.memory_capacity"]; ok {
		t.Fatal("inventory-only system memory capacity unexpectedly has a chart")
	}
	chart, ok := charts["redfish.processor.utilization"]
	if !ok {
		t.Fatal("grouped processor utilization chart is missing")
	}
	if len(chart.Dimensions) != 2 ||
		chart.Dimensions[0].ID != "user" ||
		chart.Dimensions[1].ID != "kernel" {
		t.Fatalf("processor utilization dimensions = %#v, want [user kernel]", chart.Dimensions)
	}
}

func TestEveryOperationalRegistryRowReachesExactlyOneDirectChart(t *testing.T) {
	contract := MustCompile()
	metricCharts := make(map[string]map[string]ChartSpec)
	for _, chart := range contract.Charts {
		if chart.Class != ClassOperational &&
			chart.Class != ClassResourceScalar &&
			chart.Class != ClassReadingScalar &&
			chart.Class != ClassReadingAuxiliary {
			continue
		}
		for _, dimension := range chart.Dimensions {
			if metricCharts[dimension.Metric] == nil {
				metricCharts[dimension.Metric] = make(map[string]ChartSpec)
			}
			metricCharts[dimension.Metric][chart.Context] = chart
		}
	}

	assertOne := func(row, metric, context string) {
		t.Helper()
		byContext := metricCharts[metric]
		charts := make([]ChartSpec, 0, len(byContext))
		for _, chart := range byContext {
			charts = append(charts, chart)
		}
		if len(charts) != 1 {
			t.Errorf("%s metric %q reaches %d direct charts, want exactly one", row, metric, len(charts))
			return
		}
		if charts[0].Context != context {
			t.Errorf("%s metric %q reaches context %q, want %q", row, metric, charts[0].Context, context)
		}
	}

	for _, source := range contract.Operational {
		for _, dimension := range source.Dimensions {
			metric := source.Metric
			if strings.Contains(metric, "%s") {
				metric = fmt.Sprintf(metric, dimension)
			} else if len(source.Dimensions) > 1 && source.Units != "state" {
				metric += "_" + dimension
			}
			assertOne("operational row "+source.Context+"/"+dimension, metric, source.Context)
		}
	}
	for _, field := range contract.Fields {
		if field.Exposure != ExposureOperationalScalar {
			continue
		}
		assertOne("field "+field.ID, field.Metric, scalarBaseRowContext(field.Context, field.Role))
	}
	for _, reading := range contract.Readings {
		if reading.Exposure != ExposureOperationalReading {
			continue
		}
		assertOne(
			"reading "+strings.Join([]string{reading.Family, reading.Basis, reading.Role}, "/"),
			reading.Metric,
			reading.Context,
		)
	}
}

func TestFallbackCandidatesRequireExplicitEquivalenceProof(t *testing.T) {
	contract := MustCompile()
	for index := range contract.Fields {
		if len(contract.Fields[index].Candidates) < 2 {
			continue
		}
		contract.Fields[index].EquivalenceProof = ""
		err := validate(contract)
		if err == nil || !strings.Contains(err.Error(), "has fallback sources without an equivalence proof") {
			t.Fatalf("validate() error = %v, want missing equivalence-proof error", err)
		}
		return
	}
	t.Fatal("compiled registry has no fallback to mutate")
}

func TestCompiledExposureAndAggregateContract(t *testing.T) {
	contract := MustCompile()
	chartMetrics := make(map[string]struct{})
	for _, chart := range contract.Charts {
		for _, dimension := range chart.Dimensions {
			chartMetrics[dimension.Metric] = struct{}{}
		}
	}

	var inventoryFields, operationalFields, aggregatePairs int
	for _, field := range contract.Fields {
		_, charted := chartMetrics[field.Metric]
		switch field.Exposure {
		case ExposureInventoryOnly:
			inventoryFields++
			if charted {
				t.Errorf("inventory-only field %q reaches a chart", field.ID)
			}
		case ExposureOperationalScalar:
			operationalFields++
			if !charted {
				t.Errorf("operational field %q does not reach a direct chart", field.ID)
			}
		default:
			t.Errorf("field %q has exposure %q", field.ID, field.Exposure)
		}
		for _, parent := range field.AggregateKinds {
			aggregatePairs++
			if !directAggregateRelationship(field.Kind, parent, contract.Relationships) {
				t.Errorf("field %q has unreachable aggregate owner %q", field.ID, parent)
			}
			if field.AggregateClass == "" {
				t.Errorf("field %q has aggregate owner %q without a summary class", field.ID, parent)
			}
		}
	}
	if inventoryFields != expectedInventoryFieldCount ||
		operationalFields != expectedOperationalFieldCount ||
		aggregatePairs != 413 {
		t.Fatalf(
			"field contract counts inventory=%d operational=%d aggregate_pairs=%d",
			inventoryFields,
			operationalFields,
			aggregatePairs,
		)
	}

	var inventoryReadings, primaryReadings, auxiliaryReadings int
	for _, reading := range contract.Readings {
		_, charted := chartMetrics[reading.Metric]
		switch reading.Exposure {
		case ExposureInventoryOnly:
			inventoryReadings++
			if charted {
				t.Errorf(
					"inventory-only reading %s/%s/%s reaches a chart",
					reading.Family,
					reading.Basis,
					reading.Role,
				)
			}
		case ExposureOperationalReading:
			if reading.Primary {
				primaryReadings++
			} else {
				auxiliaryReadings++
			}
			if !charted {
				t.Errorf(
					"operational reading %s/%s/%s does not reach a direct chart",
					reading.Family,
					reading.Basis,
					reading.Role,
				)
			}
			if !reading.Primary && len(reading.AggregateKinds) != 0 {
				t.Errorf(
					"auxiliary reading %s/%s/%s has aggregate owners %v",
					reading.Family,
					reading.Basis,
					reading.Role,
					reading.AggregateKinds,
				)
			}
		default:
			t.Errorf(
				"reading %s/%s/%s has exposure %q",
				reading.Family,
				reading.Basis,
				reading.Role,
				reading.Exposure,
			)
		}
	}
	if inventoryReadings != expectedInventoryReadingCount ||
		primaryReadings != expectedPrimaryReadingCount ||
		auxiliaryReadings != expectedAuxiliaryReadingCount {
		t.Fatalf(
			"reading contract counts inventory=%d primary=%d auxiliary=%d",
			inventoryReadings,
			primaryReadings,
			auxiliaryReadings,
		)
	}
}

func TestCompiledChartsStayBoundedAndUseGenericAggregateContexts(t *testing.T) {
	contract := MustCompile()
	if len(contract.SummaryClasses) != 46 {
		t.Fatalf("compiled summary class count = %d, want reviewed closed set of 46", len(contract.SummaryClasses))
	}
	if len(contract.Charts) < 450 || len(contract.Charts) > 550 {
		t.Fatalf("compiled chart count = %d, want the reviewed bounded range [450,550]", len(contract.Charts))
	}
	contexts := make([]string, 0, len(contract.Charts))
	for _, chart := range contract.Charts {
		contexts = append(contexts, chart.Context)
		if strings.Contains(chart.Context, ".rollup.") {
			t.Errorf("chart %q retains an owner-specific rollup context", chart.Context)
		}
	}
	for _, required := range []string{
		"redfish.aggregate.population",
		"redfish.aggregate.completeness",
		"redfish.aggregate.temperature",
		"redfish.aggregate.range_percentage_distribution",
		"redfish.aggregate.health",
		"redfish.aggregate.reading_alarm",
	} {
		if !slices.Contains(contexts, required) {
			t.Errorf("generic aggregate context %q is missing", required)
		}
	}
}

func TestPercentageSummaryClassesKeepDifferentSemanticsSeparate(t *testing.T) {
	contract := MustCompile()
	want := map[string]string{
		"redfish.processor.utilization": "utilization_percentage",
		"redfish.drive.media_life":      "remaining_percentage",
		"redfish.drive.nvme.wear":       "wear_percentage",
		"redfish.volume.io_time":        "time_percentage",
	}
	for _, field := range contract.Fields {
		context := scalarBaseRowContext(field.Context, field.Role)
		class, ok := want[context]
		if !ok {
			continue
		}
		if field.AggregateClass != class {
			t.Errorf("field %q aggregate class = %q, want %q", field.ID, field.AggregateClass, class)
		}
		delete(want, context)
	}
	for context := range want {
		t.Errorf("reviewed percentage field context %q is missing", context)
	}
}

func TestEveryChartSelectorHasDeclaredProducer(t *testing.T) {
	contract := MustCompile()
	producers := declaredProducerMetrics(contract)
	for _, chart := range contract.Charts {
		for _, dimension := range chart.Dimensions {
			if _, ok := producers[dimension.Metric]; !ok {
				t.Errorf(
					"chart %q dimension %q selects undeclared producer metric %q",
					chart.Context,
					dimension.ID,
					dimension.Metric,
				)
			}
		}
	}
}
