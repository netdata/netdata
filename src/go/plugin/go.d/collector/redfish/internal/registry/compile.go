// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
)

var (
	compiledOnce sync.Once
	compiled     Contract
	compiledErr  error
)

// Compile validates and returns an isolated copy of the executable contract.
func Compile() (Contract, error) {
	compiledOnce.Do(func() {
		compiled, compiledErr = compile()
	})
	if compiledErr != nil {
		return Contract{}, compiledErr
	}
	return cloneContract(compiled), nil
}

// MustCompile is for consumers whose build cannot continue with an invalid
// checked-in registry.
func MustCompile() Contract {
	value, err := Compile()
	if err != nil {
		panic(err)
	}
	return value
}

func compile() (Contract, error) {
	result := Contract{
		Kinds:         slices.Clone(kindSpecs),
		Relationships: slices.Clone(relationshipSpecs),
		Fields:        cloneFields(fieldSpecs),
		Status:        cloneStatus(statusSpecs),
		States:        cloneStates(stateSpecs),
		Flags:         cloneFlags(flagSetSpecs),
		Inventory:     slices.Clone(inventoryFieldSpecs),
		Operational:   cloneOperational(operationalSpecs),
		ReadingTypes:  cloneReadingTypes(readingTypeSpecs),
		ReadingRoles:  slices.Clone(readingRoleSpecs),
		Histograms:    cloneHistograms(histogramSpecs),
	}
	for index := range result.Fields {
		field := &result.Fields[index]
		if field.Scale.Den == 0 {
			field.Scale = Identity
		}
		if field.Title == "" {
			title, err := fieldTitle(*field)
			if err != nil {
				return Contract{}, err
			}
			field.Title = title
		}
	}
	var err error
	result.Readings, err = compileReadings(result)
	if err != nil {
		return Contract{}, err
	}
	if err := applyPresentationPolicy(&result); err != nil {
		return Contract{}, err
	}
	result.Columns, err = compileColumns(result)
	if err != nil {
		return Contract{}, err
	}
	result.Charts, err = compileCharts(result)
	if err != nil {
		return Contract{}, err
	}
	if err := validate(result); err != nil {
		return Contract{}, err
	}
	return result, nil
}

func compileReadings(contract Contract) ([]ReadingSurfaceSpec, error) {
	unitsByFamily := make(map[string]string)
	var families []string
	for _, item := range contract.ReadingTypes {
		if _, ok := unitsByFamily[item.Family]; ok {
			continue
		}
		unitsByFamily[item.Family] = item.Units
		families = append(families, item.Family)
	}

	var result []ReadingSurfaceSpec
	order := 0
	for _, family := range families {
		for _, basis := range []string{"zero", "delta", "headroom"} {
			for _, role := range contract.ReadingRoles {
				common, ok, err := commonReadingSurface(order, family, basis, role)
				if err != nil {
					return nil, err
				}
				if ok {
					result = append(result, common)
					order++
					if commonReadingExclusive(family, basis, role.ID) {
						continue
					}
				}
				generic, err := genericReadingSurface(
					order,
					basis,
					readingSurfaceTemplate{
						Family: family, Units: unitsByFamily[family], Role: role.ID,
						Exposure: role.Exposure, Primary: role.Primary,
						AggregateKinds: []Kind{"chassis"},
						Histogram:      histogramForReading(family, basis, role.ID),
					},
				)
				if err != nil {
					return nil, err
				}
				result = append(result, generic)
				order++
			}
		}
	}
	for _, fixed := range fixedReadingSurfaces {
		generic, err := genericReadingSurface(
			order,
			"zero",
			fixed,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, generic)
		order++
	}
	result = append(result, ReadingSurfaceSpec{
		Order:             order,
		Family:            "power",
		Basis:             "zero",
		Role:              "energy_rate",
		SemanticClass:     "energy_rate",
		Metric:            "reading_power_zero_energy_rate",
		Context:           "redfish.reading.power.zero.energy_rate",
		Title:             "Power Reading From Energy Rate (Zero basis)",
		Units:             "watts",
		TopFamily:         "Power",
		LeafFamily:        "power",
		AggregateMetric:   "sensor_power_energy_rate",
		AggregateKinds:    []Kind{"power_subsystem", "chassis", "system", "storage", "network_adapter", "network_interface"},
		ComponentClass:    "reading",
		Exposure:          ExposureOperationalReading,
		Primary:           true,
		DerivedFromEnergy: true,
	})
	return uniqueReadingSurfaces(result), nil
}

func commonReadingSurface(
	order int,
	family, basis string,
	roleSpec ReadingRoleSpec,
) (ReadingSurfaceSpec, bool, error) {
	if basis != "zero" {
		return ReadingSurfaceSpec{}, false, nil
	}
	role := roleSpec.ID
	type common struct {
		kind          string
		units         string
		semanticClass string
	}
	var value common
	switch {
	case family == "temperature" && role == "input":
		value = common{"temperature", "Celsius", "direct"}
	case family == "voltage" && role == "input":
		value = common{"voltage", "volts", "direct"}
	case family == "voltage" && role == "average":
		value = common{"voltage", "volts", "direct"}
	case family == "rotational_speed" && role == "input":
		value = common{"fan", "RPM", "fan"}
	case family == "current" && role == "input":
		value = common{"current", "amperes", "direct"}
	case family == "current" && role == "average":
		value = common{"current", "amperes", "direct"}
	case family == "power" && role == "input":
		value = common{"power", "watts", "direct"}
	case family == "power" && role == "average":
		value = common{"power", "watts", "direct"}
	case family == "energy" && role == "input":
		value = common{"energy", "joules", "direct"}
	case family == "humidity" && role == "input":
		value = common{"humidity", "percentage", "direct"}
	case family == "barometric_pressure" && role == "input":
		value = common{"pressure", "pascals", "ambient_pressure"}
	default:
		return ReadingSurfaceSpec{}, false, nil
	}
	title, err := readingTitle(family, basis, role)
	if err != nil {
		return ReadingSurfaceSpec{}, true, err
	}
	metric := "system_hw_sensor_" + value.kind + "_" + role
	alarmMetric := ""
	alarmContext := ""
	if role == "input" {
		alarmMetric = "system_hw_sensor_" + value.kind + "_alarm"
		alarmContext = "system.hw.sensor." + value.kind + ".alarm"
	}
	top, leaf := readingFamilyPlacement(family)
	return ReadingSurfaceSpec{
		Order:           order,
		Family:          family,
		Basis:           basis,
		Role:            role,
		SemanticClass:   value.semanticClass,
		Metric:          metric,
		Context:         "system.hw.sensor." + value.kind + "." + role,
		Title:           title,
		Units:           value.units,
		TopFamily:       top,
		LeafFamily:      leaf,
		Histogram:       histogramForReading(family, basis, role),
		AlarmMetric:     alarmMetric,
		AlarmContext:    alarmContext,
		AggregateMetric: "sensor_" + value.kind + "_" + role,
		AggregateKinds:  []Kind{"chassis"},
		ComponentClass:  "reading",
		CommonContext:   true,
		Exposure:        roleSpec.Exposure,
		Primary:         roleSpec.Primary,
	}, true, nil
}

func commonReadingExclusive(family, basis, role string) bool {
	if basis != "zero" {
		return false
	}
	switch {
	case family == "temperature" && role == "input":
		return true
	case family == "voltage" && (role == "input" || role == "average"):
		return true
	case family == "current" && (role == "input" || role == "average"):
		return true
	case family == "power" && (role == "input" || role == "average"):
		return true
	case family == "energy" && role == "input":
		return true
	case family == "humidity" && role == "input":
		return true
	case family == "barometric_pressure" && role == "input":
		return true
	default:
		return false
	}
}

func genericReadingSurface(
	order int,
	basis string,
	template readingSurfaceTemplate,
) (ReadingSurfaceSpec, error) {
	family, units, role := template.Family, template.Units, template.Role
	primary, histogram := template.Primary, template.Histogram
	if primary && histogram == "" {
		histogram = "range_percentage"
	}
	metric := strings.Join([]string{"reading", family, basis, role}, "_")
	top, leaf := readingFamilyPlacement(family)
	alarmMetric := ""
	alarmContext := ""
	if role == "input" {
		alarmMetric = metric + "_alarm"
		alarmContext = "redfish.reading." + family + "." + basis + ".alarm"
	}
	title, err := readingTitle(family, basis, role)
	if err != nil {
		return ReadingSurfaceSpec{}, err
	}
	return ReadingSurfaceSpec{
		Order:           order,
		Family:          family,
		Basis:           basis,
		Role:            role,
		Metric:          metric,
		Context:         "redfish.reading." + family + "." + basis + "." + role,
		Title:           title,
		Units:           units,
		TopFamily:       top,
		LeafFamily:      leaf,
		Histogram:       histogram,
		AlarmMetric:     alarmMetric,
		AlarmContext:    alarmContext,
		AggregateMetric: "sensor_" + family + "_" + role,
		AggregateKinds:  slices.Clone(template.AggregateKinds),
		ComponentClass:  "reading",
		Exposure:        template.Exposure,
		Primary:         primary,
	}, nil
}

func uniqueReadingSurfaces(source []ReadingSurfaceSpec) []ReadingSurfaceSpec {
	type surfaceKey struct {
		family        string
		basis         string
		role          string
		semanticClass string
	}
	result := make([]ReadingSurfaceSpec, 0, len(source))
	seen := make(map[surfaceKey]int)
	for _, item := range source {
		key := surfaceKey{
			family: item.Family, basis: item.Basis, role: item.Role, semanticClass: item.SemanticClass,
		}
		if index, ok := seen[key]; ok {
			merged := &result[index]
			merged.AggregateKinds = uniqueKinds(append(merged.AggregateKinds, item.AggregateKinds...))
			if merged.Histogram == "" {
				merged.Histogram = item.Histogram
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, item)
	}
	for index := range result {
		result[index].Order = index
	}
	return result
}

func uniqueKinds(source []Kind) []Kind {
	seen := make(map[Kind]struct{}, len(source))
	result := make([]Kind, 0, len(source))
	for _, item := range source {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func histogramForReading(family, basis, role string) string {
	if role != "input" {
		return ""
	}
	switch {
	case basis == "zero" && family == "temperature":
		return "temperature"
	case basis == "zero" && (family == "percentage" || family == "humidity" || family == "valve_position"):
		return "percentage"
	default:
		return "range_percentage"
	}
}

func readingFamilyPlacement(family string) (string, string) {
	switch family {
	case "power", "energy", "charge", "voltage", "current", "frequency",
		"apparent_power", "reactive_power", "apparent_energy", "reactive_energy",
		"crest_factor", "phase_angle", "power_factor", "harmonic_distortion", "stored_energy":
		return "Power", family
	case "percentage":
		return "Overview", family
	default:
		return "Thermal", family
	}
}

var readingFamilyTitles = map[string]string{
	"temperature":             "Temperature",
	"humidity":                "Relative Humidity",
	"power":                   "Power",
	"energy":                  "Energy",
	"charge":                  "Charge",
	"voltage":                 "Voltage",
	"current":                 "Current",
	"frequency":               "Frequency",
	"pressure":                "Pressure",
	"liquid_level":            "Liquid Level",
	"rotational_speed":        "Rotational Speed",
	"air_flow":                "Air Flow",
	"liquid_flow":             "Liquid Flow",
	"barometric_pressure":     "Barometric Pressure",
	"altitude":                "Altitude",
	"percentage":              "Percentage",
	"absolute_humidity":       "Absolute Humidity",
	"heat":                    "Heat",
	"linear_position":         "Linear Position",
	"linear_velocity":         "Linear Velocity",
	"linear_acceleration":     "Linear Acceleration",
	"rotational_position":     "Rotational Position",
	"rotational_velocity":     "Rotational Velocity",
	"rotational_acceleration": "Rotational Acceleration",
	"valve_position":          "Valve Position",
	"apparent_power":          "Apparent Power",
	"reactive_power":          "Reactive Power",
	"apparent_energy":         "Apparent Energy",
	"reactive_energy":         "Reactive Energy",
	"crest_factor":            "Crest Factor",
	"phase_angle":             "Phase Angle",
	"power_factor":            "Power Factor",
	"harmonic_distortion":     "Harmonic Distortion",
	"stored_energy":           "Stored Energy",
}

var readingRoleTitles = map[string]string{
	"input":                      "Reading",
	"average":                    "Average",
	"lowest_interval":            "Lowest Interval",
	"peak_interval":              "Peak Interval",
	"lowest_since_reset":         "Lowest Since Reset",
	"peak_since_reset":           "Peak Since Reset",
	"reading_range_min":          "Reading Range Minimum",
	"reading_range_max":          "Reading Range Maximum",
	"minimum_allowable":          "Minimum Allowable",
	"maximum_allowable":          "Maximum Allowable",
	"adjusted_minimum_allowable": "Adjusted Minimum Allowable",
	"adjusted_maximum_allowable": "Adjusted Maximum Allowable",
	"speed_rpm":                  "Speed RPM",
	"apparent_va":                "Apparent VA",
	"reactive_var":               "Reactive VAR",
	"apparent_kvah":              "Apparent kVAh",
	"reactive_kvarh":             "Reactive kVARh",
	"crest_factor":               "Crest Factor",
	"phase_angle_degrees":        "Phase Angle",
	"power_factor":               "Power Factor",
	"thd_percent":                "THD",
	"load_percent":               "Load",
	"speed":                      "Speed",
	"power":                      "Power",
	"pressure":                   "Pressure",
	"flow":                       "Flow",
	"position":                   "Position",
	"heat_removed":               "Heat Removed",
	"core_voltage":               "Core Voltage",
	"input_power":                "Input Power",
	"output_power":               "Output Power",
	"input_current":              "Input Current",
	"input_voltage":              "Input Voltage",
	"frequency":                  "Frequency",
	"temperature":                "Temperature",
	"fan_speed":                  "Fan Speed",
	"energy":                     "Energy",
	"charge":                     "Charge",
	"state_of_health":            "State of Health",
	"stored_charge":              "Stored Charge",
	"stored_energy":              "Stored Energy",
	"current":                    "Current",
	"voltage":                    "Voltage",
	"ambient_temperature":        "Ambient Temperature",
	"dew_point":                  "Dew Point",
	"humidity":                   "Humidity",
	"power_load":                 "Power Load",
	"airflow":                    "Airflow",
	"absolute_humidity":          "Absolute Humidity",
}

func readingTitle(family, basis, role string) (string, error) {
	familyTitle := readingFamilyTitles[family]
	roleTitle := readingRoleTitles[role]
	if familyTitle == "" || roleTitle == "" {
		return "", fmt.Errorf("missing reviewed reading title for %s/%s", family, role)
	}
	basisLabel, err := basisTitle(basis)
	if err != nil {
		return "", err
	}
	if role == "input" {
		return fmt.Sprintf("%s Reading (%s basis)", familyTitle, basisLabel), nil
	}
	return fmt.Sprintf("%s %s (%s basis)", familyTitle, roleTitle, basisLabel), nil
}

func basisTitle(value string) (string, error) {
	switch value {
	case "zero":
		return "Zero", nil
	case "delta":
		return "Delta", nil
	case "headroom":
		return "Headroom", nil
	default:
		return "", fmt.Errorf("unknown reading basis %q", value)
	}
}

func compileColumns(contract Contract) ([]ColumnSpec, error) {
	result := commonColumns()
	commonCount := len(result)
	byID := make(map[string]int, len(result))
	for index := range result {
		byID[result[index].ID] = index
	}
	add := func(candidate ColumnSpec) error {
		if index, ok := byID[candidate.ID]; ok {
			current := &result[index]
			if current.Type != candidate.Type {
				if isNumericColumnType(current.Type) && isNumericColumnType(candidate.Type) {
					current.Type = ColumnFloat
				} else {
					return fmt.Errorf(
						"column %q has conflicting types %q and %q",
						candidate.ID,
						current.Type,
						candidate.Type,
					)
				}
			}
			if current.Units != "" && candidate.Units != "" && current.Units != candidate.Units {
				return fmt.Errorf(
					"column %q has conflicting units %q and %q",
					candidate.ID,
					current.Units,
					candidate.Units,
				)
			}
			if current.Structured != candidate.Structured {
				return fmt.Errorf(
					"column %q has conflicting structured values %t and %t",
					candidate.ID,
					current.Structured,
					candidate.Structured,
				)
			}
			current.Visible = current.Visible || candidate.Visible
			current.Facet = current.Facet || candidate.Facet
			current.Structured = current.Structured || candidate.Structured
			current.Additive = current.Additive || candidate.Additive
			// Common columns describe every inventory row. A kind-specific field
			// may enrich their presentation, but must not narrow their membership.
			if index >= commonCount && len(candidate.Members) > 0 {
				if current.Members == nil {
					current.Members = make(map[string]struct{})
				}
				for member := range candidate.Members {
					current.Members[member] = struct{}{}
				}
			}
			if current.Units == "" {
				current.Units = candidate.Units
			}
			return nil
		}
		candidate.Order = len(result)
		result = append(result, candidate)
		byID[candidate.ID] = len(result) - 1
		return nil
	}
	for _, inventory := range contract.Inventory {
		if err := add(ColumnSpec{
			ID:         inventory.Column,
			Name:       inventory.Column,
			Tooltip:    inventory.Expression,
			Type:       inventory.Type,
			Units:      inventory.Units,
			Visible:    inventory.Visible,
			Facet:      inventory.Facet,
			Sortable:   !inventory.Structured,
			Structured: inventory.Structured,
			Members:    map[string]struct{}{string(inventory.Kind): {}},
		}); err != nil {
			return nil, fmt.Errorf("inventory field %s.%s: %w", inventory.Kind, inventory.Path, err)
		}
	}
	for _, field := range contract.Fields {
		typ := ColumnInteger
		if field.Float || field.Algorithm != AlgorithmAbsolute {
			typ = ColumnFloat
		}
		columnUnits := field.Units
		if field.MixedColumnUnits {
			columnUnits = ""
		}
		if err := add(ColumnSpec{
			ID:       field.Column,
			Name:     field.Column,
			Tooltip:  field.Title,
			Type:     typ,
			Units:    columnUnits,
			Visible:  true,
			Facet:    true,
			Sortable: true,
			Additive: field.Additive,
			Members:  map[string]struct{}{string(field.Kind): {}},
		}); err != nil {
			return nil, fmt.Errorf("metric field %s.%s: %w", field.Kind, field.Column, err)
		}
		if field.Algorithm == AlgorithmRate || field.Algorithm == AlgorithmDurationPercent {
			if err := add(ColumnSpec{
				ID:       field.Column + "_exact",
				Name:     field.Column + "_exact",
				Tooltip:  field.Title + " exact source total",
				Type:     ColumnString,
				Visible:  false,
				Sortable: true,
				Members:  map[string]struct{}{string(field.Kind): {}},
			}); err != nil {
				return nil, fmt.Errorf("exact metric field %s.%s: %w", field.Kind, field.Column, err)
			}
		}
		if len(field.Candidates) > 1 {
			if err := add(ColumnSpec{
				ID:       field.Column + "_source",
				Name:     field.Column + "_source",
				Tooltip:  field.Title + " selected source",
				Type:     ColumnString,
				Visible:  false,
				Facet:    true,
				Sortable: true,
				Members:  map[string]struct{}{string(field.Kind): {}},
			}); err != nil {
				return nil, fmt.Errorf("metric source field %s.%s: %w", field.Kind, field.Column, err)
			}
		}
		for _, candidate := range field.Candidates {
			if candidate.MultiplierColumn == "" {
				continue
			}
			if err := add(ColumnSpec{
				ID:       candidate.MultiplierColumn,
				Name:     candidate.MultiplierColumn,
				Tooltip:  "Multiplier used to normalize " + field.Title,
				Type:     ColumnInteger,
				Units:    "bytes",
				Visible:  false,
				Facet:    true,
				Sortable: true,
				Members:  map[string]struct{}{string(field.Kind): {}},
			}); err != nil {
				return nil, fmt.Errorf("metric multiplier field %s.%s: %w", field.Kind, field.Column, err)
			}
		}
	}
	for _, state := range contract.States {
		typ := ColumnEnum
		if state.BooleanFalse != "" || state.BooleanTrue != "" {
			typ = ColumnBoolean
		}
		if err := add(ColumnSpec{
			ID:       state.Column,
			Name:     state.Column,
			Tooltip:  state.Title,
			Type:     typ,
			Visible:  true,
			Facet:    true,
			Sortable: true,
			Members:  map[string]struct{}{string(state.Kind): {}},
		}); err != nil {
			return nil, fmt.Errorf("state field %s.%s: %w", state.Kind, state.Column, err)
		}
	}
	for _, flags := range contract.Flags {
		for _, member := range flags.Members {
			if err := add(ColumnSpec{
				ID:       member.Column,
				Name:     member.Column,
				Tooltip:  flags.Title + " " + member.Role,
				Type:     ColumnBoolean,
				Visible:  true,
				Facet:    true,
				Sortable: true,
				Members:  map[string]struct{}{string(flags.Kind): {}},
			}); err != nil {
				return nil, fmt.Errorf("flag field %s.%s: %w", flags.Kind, member.Column, err)
			}
		}
	}
	for _, reading := range readingColumns(len(result)) {
		if err := add(reading); err != nil {
			return nil, fmt.Errorf("reading field %s: %w", reading.ID, err)
		}
	}
	return result, nil
}

func isNumericColumnType(value ColumnType) bool {
	return value == ColumnInteger || value == ColumnFloat
}

func compileCharts(contract Contract) ([]ChartSpec, error) {
	var result []ChartSpec
	for _, source := range contract.Operational {
		if source.Type != ChartLine && source.Type != ChartStacked {
			return nil, fmt.Errorf("operational context %q has invalid chart type %q", source.Context, source.Type)
		}
		dimensions := make([]DimensionSpec, 0, len(source.Dimensions))
		for _, dimension := range source.Dimensions {
			selector := source.Metric
			if strings.Contains(selector, "%s") {
				selector = fmt.Sprintf(selector, dimension)
			} else if len(source.Dimensions) > 1 && source.Units != "state" {
				selector += "_" + dimension
			}
			metric := selector
			if source.Units == "state" {
				metric = source.Metric
				selector = fmt.Sprintf(`%s{%s=%q}`, source.Metric, source.Metric, dimension)
			}
			dimensions = append(dimensions, DimensionSpec{
				ID:        dimension,
				Name:      dimension,
				Metric:    metric,
				Selector:  selector,
				Algorithm: map[bool]string{false: "absolute", true: "incremental"}[source.Incremental],
				Float:     source.Float,
			})
		}
		module := "redfish"
		if source.LeafFamily == "log_backend" {
			module = "redfish_logs"
		}
		result = append(result, ChartSpec{
			Order:          source.Order,
			Module:         module,
			ID:             chartID(source.Context),
			Context:        source.Context,
			Title:          source.Title,
			Units:          source.Units,
			Type:           source.Type,
			Class:          ClassOperational,
			TopFamily:      source.TopFamily,
			LeafFamily:     source.LeafFamily,
			InstanceLabels: slices.Clone(source.InstanceLabels),
			PromotedLabels: operationalPromotedLabels(source),
			Dimensions:     dimensions,
			ExpireAfter:    5,
		})
	}

	kindByID := make(map[Kind]KindSpec, len(contract.Kinds))
	for _, kind := range contract.Kinds {
		kindByID[kind.ID] = kind
	}
	for _, status := range contract.Status {
		kind, err := compiledKind(kindByID, "status", status.Kind)
		if err != nil {
			return nil, err
		}
		if status.Status {
			result = append(result,
				stateChart(kind, "health", "Health", HealthStates),
				stateChart(kind, "health_rollup", "Health Rollup", HealthStates),
				stateChart(kind, "state", "State", ResourceStates),
				fixedChart(kind, "conditions", "Conditions", "conditions", []string{"ok", "warning", "critical", "unknown"}),
			)
		}
		if status.PowerState {
			result = append(result, stateChart(kind, "power_state", "Power State", PowerStates))
		}
		if status.FailurePredicted {
			result = append(result, stateChart(kind, "failure_predicted", "Failure Prediction", FailureStates))
		}
		result = append(result, stateChart(kind, "acquisition_state", "Acquisition State", AcquisitionStates))
	}
	for _, state := range contract.States {
		kind, err := compiledKind(kindByID, "state "+state.Context, state.Kind)
		if err != nil {
			return nil, err
		}
		result = append(result, stateChartExplicit(kind, state))
	}
	for _, flags := range contract.Flags {
		kind, err := compiledKind(kindByID, "flag set "+flags.Context, flags.Kind)
		if err != nil {
			return nil, err
		}
		dimensions := make([]DimensionSpec, 0, len(flags.Members))
		for _, member := range flags.Members {
			dimensions = append(dimensions, DimensionSpec{
				ID:        member.Role,
				Name:      member.Role,
				Metric:    flags.Metric + "_" + member.Role,
				Selector:  flags.Metric + "_" + member.Role,
				Algorithm: "absolute",
			})
		}
		result = append(result, ChartSpec{
			Module:         "redfish",
			ID:             chartID(flags.Context),
			Context:        flags.Context,
			Title:          flags.Title,
			Units:          "flags",
			Type:           ChartStacked,
			Class:          ClassResourceCategorical,
			TopFamily:      kind.TopFamily,
			LeafFamily:     kind.LeafFamily,
			InstanceLabels: []string{"endpoint_key", "resource_key"},
			PromotedLabels: promotedLabels(flags.ComponentClass),
			Dimensions:     dimensions,
			ExpireAfter:    5,
		})
	}
	fieldCharts, err := compileFieldCharts(contract, kindByID)
	if err != nil {
		return nil, err
	}
	result = append(result, fieldCharts...)
	result = append(result, compileReadingCharts(contract)...)
	result = append(result, compileAggregateCharts(contract, kindByID)...)
	categoricalCharts, err := compileCategoricalAggregateCharts(contract, kindByID)
	if err != nil {
		return nil, err
	}
	result = append(result, categoricalCharts...)
	sortCharts(result)
	for index := range result {
		result[index].Order = index
		result[index].Priority = 1000 + index*10
	}
	return result, nil
}

func compiledKind(kinds map[Kind]KindSpec, surface string, id Kind) (KindSpec, error) {
	kind, ok := kinds[id]
	if !ok {
		return KindSpec{}, fmt.Errorf("%s references unknown kind %q", surface, id)
	}
	return kind, nil
}

func stateChart(kind KindSpec, suffix, title string, states []string) ChartSpec {
	metric := string(kind.ID) + "_" + suffix
	return ChartSpec{
		Module:         "redfish",
		ID:             chartID("redfish." + string(kind.ID) + "." + suffix),
		Context:        "redfish." + string(kind.ID) + "." + suffix,
		Title:          kind.Display + " " + title,
		Units:          "state",
		Type:           ChartStacked,
		Class:          ClassResourceCategorical,
		TopFamily:      kind.TopFamily,
		LeafFamily:     kind.LeafFamily,
		InstanceLabels: []string{"endpoint_key", "resource_key"},
		PromotedLabels: promotedLabels(kind.ComponentClass),
		Dimensions:     stateDimensions(metric, states),
		ExpireAfter:    5,
	}
}

func stateChartExplicit(kind KindSpec, state StateSpec) ChartSpec {
	return ChartSpec{
		Module:         "redfish",
		ID:             chartID(state.Context),
		Context:        state.Context,
		Title:          state.Title,
		Units:          "state",
		Type:           ChartStacked,
		Class:          ClassResourceCategorical,
		TopFamily:      kind.TopFamily,
		LeafFamily:     kind.LeafFamily,
		InstanceLabels: []string{"endpoint_key", "resource_key"},
		PromotedLabels: promotedLabels(state.ComponentClass),
		Dimensions:     stateDimensions(state.Metric, state.States),
		ExpireAfter:    5,
	}
}

func fixedChart(kind KindSpec, suffix, title, units string, dimensions []string) ChartSpec {
	metric := string(kind.ID) + "_" + suffix
	result := ChartSpec{
		Module:         "redfish",
		ID:             chartID("redfish." + string(kind.ID) + "." + suffix),
		Context:        "redfish." + string(kind.ID) + "." + suffix,
		Title:          kind.Display + " " + title,
		Units:          units,
		Type:           ChartStacked,
		Class:          ClassResourceCategorical,
		TopFamily:      kind.TopFamily,
		LeafFamily:     kind.LeafFamily,
		InstanceLabels: []string{"endpoint_key", "resource_key"},
		PromotedLabels: promotedLabels(kind.ComponentClass),
		ExpireAfter:    5,
	}
	for _, dimension := range dimensions {
		selector := metric + "_" + dimension
		result.Dimensions = append(result.Dimensions, DimensionSpec{
			ID: dimension, Name: dimension, Metric: selector, Selector: selector, Algorithm: "absolute",
		})
	}
	return result
}

func stateDimensions(metric string, states []string) []DimensionSpec {
	result := make([]DimensionSpec, 0, len(states))
	for _, state := range states {
		selector := fmt.Sprintf(`%s{%s=%q}`, metric, metric, state)
		result = append(result, DimensionSpec{
			ID: state, Name: state, Metric: metric, Selector: selector, Algorithm: "absolute",
		})
	}
	return result
}

func chartID(context string) string {
	return strings.ReplaceAll(context, ".", "_")
}

func promotedLabels(class string) []string {
	result := []string{"endpoint_job", "endpoint_key"}
	resource := []string{
		"system_key", "system_name", "resource_kind", "resource_name",
		"logical_owner_key", "logical_owner_name", "rollup_owner_kind",
		"rollup_owner_key", "rollup_owner_name", "component_family", "source_model",
	}
	switch class {
	case "system":
		result = append(result,
			"system_key", "system_name", "resource_kind", "resource_name",
			"manufacturer", "model", "serial_number", "asset_tag", "bios_version", "source_model",
		)
	case "replaceable":
		result = append(result, resource...)
		result = append(result, "slot", "location", "manufacturer", "model", "serial_number", "part_number", "spare_part_number", "firmware_version")
	case "network":
		result = append(result, resource...)
		result = append(result, "mac_address", "wwn", "link_type")
	case "reading":
		result = append(result, resource...)
		result = append(result,
			"reading_key", "physical_context", "physical_subcontext", "reading_type",
			"reading_basis", "reading_role", "reading_source", "semantic_source_class",
			"implementation_type",
		)
	case "log_service":
		result = append(result, resource...)
		result = append(result, "log_service_id")
	case "aggregate":
		result = append(result,
			"system_key", "system_name", "rollup_owner_kind", "rollup_owner_key",
			"rollup_owner_name", "component_family", "aggregate_key",
			"aggregate_class", "aggregate_semantic", "aggregate_role", "aggregate_units",
			"child_resource_kind", "physical_context", "reading_basis", "reading_source",
			"semantic_source_class",
		)
	default:
		result = append(result, resource...)
	}
	return uniqueStrings(result)
}

func operationalPromotedLabels(source OperationalSpec) []string {
	switch source.LeafFamily {
	case "log_backend":
		return []string{"backend_name", "backend_key"}
	case "log_service":
		return promotedLabels("log_service")
	default:
		result := []string{"endpoint_job"}
		if slices.Contains(source.InstanceLabels, "logical_owner_key") {
			result = append(result, "logical_owner_name", "component_family")
		}
		return result
	}
}

func sortCharts(charts []ChartSpec) {
	classRank := map[ChartClass]int{
		ClassOperational: 0, ClassResourceScalar: 1, ClassResourceCategorical: 2,
		ClassReadingScalar: 3, ClassReadingAuxiliary: 4, ClassReadingAlarm: 5,
		ClassNumericParent: 6, ClassCategoricalParent: 7, ClassHistogramParent: 8,
	}
	parentOrder := []string{
		"service", "system", "chassis", "manager", "processor", "processor_core", "memory",
		"storage", "storage_controller", "drive", "volume", "network_adapter", "network_device_function",
		"ethernet_interface", "network_interface", "network_port", "port", "pcie_device", "pcie_function",
		"fan", "pump", "power_supply", "battery", "sensor", "redundancy", "thermal_subsystem",
		"power_subsystem", "coolant_connector", "filter", "heater", "leak_detection",
		"leak_detector_group", "leak_detector", "control", "firmware", "software", "assembly", "log_service",
	}
	parentRank := make(map[string]int, len(parentOrder))
	for i, value := range parentOrder {
		parentRank[value] = i + 1
	}
	statisticRank := map[string]int{
		"total": 1, "minimum": 2, "average": 3, "maximum": 4,
		"population": 5, "completeness": 6, "histogram": 7,
	}
	key := func(chart ChartSpec) (base string, role, parent, statistic int) {
		base = chart.BaseRowContext
		if base == "" {
			base = chart.Context
		}
		role = chart.ScalarRoleRank
		parts := strings.Split(chart.Context, ".")
		if len(parts) > 0 {
			if rank := statisticRank[parts[len(parts)-1]]; rank != 0 {
				statistic = rank
				base = strings.Join(parts[:len(parts)-1], ".")
			}
		}
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "rollup" {
				parent = parentRank[parts[i+1]]
				break
			}
		}
		return base, role, parent, statistic
	}
	sort.SliceStable(charts, func(i, j int) bool {
		left, right := charts[i], charts[j]
		if compared := CompareTopFamily(left.TopFamily, right.TopFamily); compared != 0 {
			return compared < 0
		}
		if left.LeafFamily != right.LeafFamily {
			return left.LeafFamily < right.LeafFamily
		}
		leftBase, leftRole, leftParent, leftStatistic := key(left)
		rightBase, rightRole, rightParent, rightStatistic := key(right)
		if leftBase != rightBase {
			return leftBase < rightBase
		}
		if classRank[left.Class] != classRank[right.Class] {
			return classRank[left.Class] < classRank[right.Class]
		}
		if leftRole != rightRole {
			return leftRole < rightRole
		}
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		if leftStatistic != rightStatistic {
			return leftStatistic < rightStatistic
		}
		return left.Context < right.Context
	})
}

var topFamilyRanks = map[string]int{
	"Overview": 0, "Compute": 1, "Memory": 2, "Thermal": 3, "Power": 4,
	"Storage": 5, "Network": 6, "Management": 7, "Collection": 8,
}

// CompareTopFamily orders established dashboard families before profile-added families.
func CompareTopFamily(left, right string) int {
	leftRank, leftKnown := topFamilyRanks[left]
	rightRank, rightKnown := topFamilyRanks[right]
	switch {
	case leftKnown && rightKnown:
		if leftRank < rightRank {
			return -1
		}
		if leftRank > rightRank {
			return 1
		}
		return strings.Compare(left, right)
	case leftKnown:
		return -1
	case rightKnown:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func scalarBaseRowContext(context, role string) string {
	suffix := "." + role
	if role != "" && strings.HasSuffix(context, suffix) {
		return strings.TrimSuffix(context, suffix)
	}
	return context
}

func fieldTitle(field FieldSpec) (string, error) {
	kind := findKind(field.Kind)
	if kind.ID == "" {
		return "", fmt.Errorf("field %q references unknown kind %q", field.ID, field.Kind)
	}
	parts := strings.Split(strings.TrimPrefix(field.Context, "redfish."+string(field.Kind)+"."), ".")
	words := []string{kind.Display}
	for _, part := range parts {
		display, ok := displayTokens[part]
		if !ok {
			return "", fmt.Errorf("field %q context contains unknown title token %q", field.ID, part)
		}
		words = append(words, display)
	}
	if field.Role != "" {
		display, ok := displayTokens[field.Role]
		if !ok {
			return "", fmt.Errorf("field %q role contains unknown title token %q", field.ID, field.Role)
		}
		words = append(words, display)
	}
	return strings.Join(removeAdjacentDuplicates(words), " "), nil
}

var displayTokens = map[string]string{
	"memory_capacity":         "Memory Capacity",
	"processor_count":         "Processor Count",
	"processor_core_count":    "Processor Core Count",
	"processor_bandwidth":     "Processor Bandwidth",
	"processor_utilization":   "Processor Utilization",
	"memory_bandwidth":        "Memory Bandwidth",
	"memory_utilization":      "Memory Utilization",
	"core_count":              "Core Count",
	"thread_count":            "Thread Count",
	"clock_speed":             "Clock Speed",
	"clock_limits":            "Clock Limits",
	"clock_offset":            "Clock Offset",
	"tdp":                     "TDP",
	"utilization":             "Utilization",
	"bandwidth_utilization":   "Bandwidth Utilization",
	"frequency_ratio":         "Frequency Ratio",
	"thermal_headroom":        "Thermal Headroom",
	"error_rate":              "Error Rate",
	"throttle_time":           "Throttle Time",
	"capacity":                "Capacity",
	"offset":                  "Offset",
	"capacity_utilization":    "Capacity Utilization",
	"dirty_shutdown_rate":     "Dirty Shutdown Rate",
	"media_health":            "Media Health",
	"state_change_rate":       "State Change Rate",
	"io":                      "I/O",
	"iops":                    "IOPS",
	"speed":                   "Speed",
	"pcie_lanes":              "PCIe Lanes",
	"cache_capacity":          "Cache Capacity",
	"link_speed":              "Link Speed",
	"rotation_speed":          "Rotation Speed",
	"media_life":              "Media Life",
	"queue_depth":             "Queue Depth",
	"remaining_capacity":      "Remaining Capacity",
	"space_savings":           "Space Savings",
	"link_width":              "Link Width",
	"traffic":                 "Traffic",
	"charge_capacity":         "Charge Capacity",
	"energy_capacity":         "Energy Capacity",
	"c_rate":                  "C-rate",
	"e_rate":                  "E-rate",
	"busy_time":               "Busy Time",
	"cpu_utilization":         "CPU Utilization",
	"cycle_rate":              "Cycle Rate",
	"event_rate":              "Event Rate",
	"frames":                  "Frame Rate",
	"frequency":               "Frequency",
	"heating_time":            "Heating Time",
	"host_bus_utilization":    "Host Bus Utilization",
	"instructions_per_cycle":  "Instructions per Cycle",
	"io_time":                 "I/O Time",
	"linear_acceleration":     "Linear Acceleration",
	"linear_position":         "Linear Position",
	"linear_velocity":         "Linear Velocity",
	"liquid_flow":             "Liquid Flow",
	"members":                 "Members",
	"ncsi_frames":             "NCSI Frame Rate",
	"ncsi_traffic":            "NCSI Traffic",
	"network_error_rate":      "Network Error Rate",
	"nvme":                    "NVMe",
	"pcie_drop_rate":          "PCIe Drop Rate",
	"pcie_error_rate":         "PCIe Error Rate",
	"pcie_tlp_rate":           "PCIe TLP Rate",
	"pcie_traffic":            "PCIe Traffic",
	"percentage":              "Percentage",
	"power":                   "Power",
	"pressure":                "Pressure",
	"queue_full":              "Full Queues",
	"rdma_error_rate":         "RDMA Error Rate",
	"rdma_traffic":            "RDMA Traffic",
	"rotational_acceleration": "Rotational Acceleration",
	"rotational_position":     "Rotational Position",
	"rotational_velocity":     "Rotational Velocity",
	"spare":                   "Spare",
	"temperature":             "Temperature",
	"thermal_time":            "Thermal Time",
	"thermal_transition_rate": "Thermal Transition Rate",
	"valve_position":          "Valve Position",
	"wear":                    "Wear",

	"volatile": "Volatile", "persistent": "Persistent", "processors": "Processors",
	"physical": "Physical", "logical": "Logical", "bandwidth": "Bandwidth",
	"kernel": "Kernel", "user": "User", "total": "Total", "enabled": "Enabled",
	"operating": "Operating", "base": "Base", "minimum": "Minimum", "maximum": "Maximum",
	"limit": "Limit", "configured": "Configured", "ratio": "Ratio", "headroom": "Headroom",
	"correctable_core": "Correctable Core", "correctable_other": "Correctable Other",
	"uncorrectable_core": "Uncorrectable Core", "uncorrectable_other": "Uncorrectable Other",
	"power_limit": "Power Limit", "thermal_limit": "Thermal Limit", "cache": "Cache",
	"nonvolatile": "Nonvolatile", "life_left": "Life Left", "spare_remaining": "Spare Remaining",
	"shutdowns": "Shutdowns", "changes": "Changes", "read": "Read", "written": "Written",
	"active": "Active", "negotiated": "Negotiated", "capable": "Capable",
	"remaining": "Remaining", "commands": "Commands", "correctable_read": "Correctable Read",
	"correctable_write": "Correctable Write", "uncorrectable_read": "Uncorrectable Read",
	"uncorrectable_write": "Uncorrectable Write", "bad_blocks": "Bad Blocks",
	"compression": "Compression", "deduplication": "Deduplication",
	"thin_provisioning": "Thin Provisioning", "received": "Received", "sent": "Sent",
	"allocated": "Allocated", "requested": "Requested", "actual": "Actual", "rated": "Rated",
	"rate": "Rate",
}

func validate(contract Contract) error {
	var errs []error
	producerMetrics := declaredProducerMetrics(contract)
	kinds := make(map[Kind]KindSpec, len(contract.Kinds))
	ranks := make(map[int]Kind)
	for _, kind := range contract.Kinds {
		if kind.ID == "" || kind.Display == "" || kind.TopFamily == "" || kind.LeafFamily == "" {
			errs = append(errs, fmt.Errorf("invalid kind declaration %#v", kind))
			continue
		}
		if _, ok := kinds[kind.ID]; ok {
			errs = append(errs, fmt.Errorf("duplicate kind %q", kind.ID))
		}
		if previous, ok := ranks[kind.ParentPresentationRank]; ok {
			errs = append(errs, fmt.Errorf("kind presentation rank %d is shared by %q and %q", kind.ParentPresentationRank, previous, kind.ID))
		}
		kinds[kind.ID] = kind
		ranks[kind.ParentPresentationRank] = kind.ID
	}
	aggregateOwners := make(map[Kind]map[Kind]struct{}, len(contract.Status))
	for _, status := range contract.Status {
		if _, ok := kinds[status.Kind]; !ok {
			errs = append(errs, fmt.Errorf("status references unknown kind %q", status.Kind))
		}
		owners := make(map[Kind]struct{}, len(status.AggregateKinds))
		for _, owner := range status.AggregateKinds {
			owners[owner] = struct{}{}
		}
		aggregateOwners[status.Kind] = owners
	}
	validateAggregateOwners := func(surface, context string, kind Kind, owners []Kind) {
		for _, owner := range owners {
			if _, ok := aggregateOwners[kind][owner]; !ok {
				errs = append(errs, fmt.Errorf(
					"%s %q has unreachable aggregate owner %q for kind %q",
					surface,
					context,
					owner,
					kind,
				))
			}
		}
	}
	fieldIDs := make(map[string]struct{})
	contexts := make(map[string]string)
	metricContract := make(map[string]FieldSpec)
	for _, inventory := range contract.Inventory {
		if _, ok := kinds[inventory.Kind]; !ok {
			errs = append(errs, fmt.Errorf("inventory field %q references unknown kind %q", inventory.Path, inventory.Kind))
		}
		if inventory.Path == "" || inventory.Column == "" || inventory.Type == "" {
			errs = append(errs, fmt.Errorf("invalid inventory field declaration %#v", inventory))
		}
		if inventory.Scale.Num != 0 && inventory.Scale.Den <= 0 {
			errs = append(errs, fmt.Errorf("inventory field %s.%s has invalid scale", inventory.Kind, inventory.Path))
		}
		if inventory.SourceType != "" &&
			(inventory.SourceType != ColumnFloat || inventory.Type != ColumnInteger || inventory.Structured) {
			errs = append(errs, fmt.Errorf(
				"inventory field %s.%s has unsupported source/final type conversion %q -> %q",
				inventory.Kind,
				inventory.Path,
				inventory.SourceType,
				inventory.Type,
			))
		}
	}
	for _, field := range contract.Fields {
		if _, ok := kinds[field.Kind]; !ok {
			errs = append(errs, fmt.Errorf("field %q references unknown kind %q", field.ID, field.Kind))
		}
		if _, ok := fieldIDs[field.ID]; ok {
			errs = append(errs, fmt.Errorf("duplicate field ID %q", field.ID))
		}
		fieldIDs[field.ID] = struct{}{}
		if previous, ok := contexts[field.Context]; ok {
			errs = append(errs, fmt.Errorf("context %q is shared by fields %q and %q", field.Context, previous, field.ID))
		}
		contexts[field.Context] = field.ID
		if len(field.Candidates) == 0 {
			errs = append(errs, fmt.Errorf("field %q has no source candidate", field.ID))
		}
		if len(field.Candidates) > 1 && field.EquivalenceProof == "" {
			errs = append(errs, fmt.Errorf("field %q has fallback sources without an equivalence proof", field.ID))
		}
		if field.Scale.Den <= 0 {
			errs = append(errs, fmt.Errorf("field %q has invalid scale", field.ID))
		}
		if previous, ok := metricContract[field.Metric]; ok &&
			(previous.Units != field.Units || previous.Algorithm != field.Algorithm || previous.Float != field.Float) {
			errs = append(errs, fmt.Errorf("metric %q has conflicting descriptors", field.Metric))
		}
		metricContract[field.Metric] = field
		if field.Additive && field.Algorithm == AlgorithmDurationPercent {
			errs = append(errs, fmt.Errorf("duration percentage field %q cannot be additive", field.ID))
		}
	}
	flagContexts := make(map[string]string)
	for _, state := range contract.States {
		if _, ok := kinds[state.Kind]; !ok {
			errs = append(errs, fmt.Errorf("state %q references unknown kind %q", state.Context, state.Kind))
		}
		if state.Column == "" {
			errs = append(errs, fmt.Errorf("state %q has no Function column", state.Context))
		}
		validateAggregateOwners("state", state.Context, state.Kind, state.AggregateKinds)
	}
	readingOwners := map[Kind]struct{}{
		"system": {}, "chassis": {}, "storage": {}, "network_adapter": {},
		"network_interface": {}, "thermal_subsystem": {}, "power_subsystem": {},
	}
	for _, reading := range contract.Readings {
		for _, owner := range reading.AggregateKinds {
			if _, ok := kinds[owner]; !ok {
				errs = append(errs, fmt.Errorf(
					"reading %s/%s/%s references unknown aggregate owner %q",
					reading.Family, reading.Basis, reading.Role, owner,
				))
				continue
			}
			if _, ok := readingOwners[owner]; !ok {
				errs = append(errs, fmt.Errorf(
					"reading %s/%s/%s has unreachable aggregate owner %q",
					reading.Family, reading.Basis, reading.Role, owner,
				))
			}
		}
	}
	for _, flags := range contract.Flags {
		if _, ok := kinds[flags.Kind]; !ok {
			errs = append(errs, fmt.Errorf("flag set %q references unknown kind %q", flags.Context, flags.Kind))
		}
		if previous, ok := contexts[flags.Context]; ok {
			errs = append(errs, fmt.Errorf("flag context %q conflicts with field %q", flags.Context, previous))
		}
		if previous, ok := flagContexts[flags.Context]; ok {
			errs = append(errs, fmt.Errorf("duplicate flag context %q (%s)", flags.Context, previous))
		}
		validateAggregateOwners("flag set", flags.Context, flags.Kind, flags.AggregateKinds)
		flagContexts[flags.Context] = flags.Metric
		roles := make(map[string]struct{}, len(flags.Members))
		for _, member := range flags.Members {
			if member.Path == "" || member.Role == "" || member.Column == "" {
				errs = append(errs, fmt.Errorf("invalid member in flag set %q", flags.Context))
			}
			if _, ok := roles[member.Role]; ok {
				errs = append(errs, fmt.Errorf("flag set %q has duplicate role %q", flags.Context, member.Role))
			}
			roles[member.Role] = struct{}{}
		}
	}
	columnContract := make(map[string]ColumnSpec)
	for _, column := range contract.Columns {
		if previous, ok := columnContract[column.ID]; ok {
			errs = append(errs, fmt.Errorf("duplicate compiled column %q (%#v, %#v)", column.ID, previous, column))
		}
		columnContract[column.ID] = column
	}
	chartIDs := make(map[string]string)
	chartContexts := make(map[string]string)
	priorities := make(map[int]string)
	for _, chart := range contract.Charts {
		if previous, ok := chartIDs[chart.ID]; ok {
			errs = append(errs, fmt.Errorf("chart ID %q is shared by contexts %q and %q", chart.ID, previous, chart.Context))
		}
		if previous, ok := chartContexts[chart.Context]; ok {
			errs = append(errs, fmt.Errorf("chart context %q is shared by IDs %q and %q", chart.Context, previous, chart.ID))
		}
		if previous, ok := priorities[chart.Priority]; ok {
			errs = append(errs, fmt.Errorf("chart priority %d is shared by %q and %q", chart.Priority, previous, chart.ID))
		}
		chartIDs[chart.ID] = chart.Context
		chartContexts[chart.Context] = chart.ID
		priorities[chart.Priority] = chart.ID
		dimensions := make(map[string]struct{}, len(chart.Dimensions))
		for _, dimension := range chart.Dimensions {
			if _, ok := dimensions[dimension.ID]; ok {
				errs = append(errs, fmt.Errorf("chart %q has duplicate dimension %q", chart.ID, dimension.ID))
			}
			dimensions[dimension.ID] = struct{}{}
			if _, ok := producerMetrics[dimension.Metric]; !ok {
				errs = append(errs, fmt.Errorf(
					"chart %q dimension %q selects undeclared producer metric %q",
					chart.ID,
					dimension.ID,
					dimension.Metric,
				))
			}
		}
	}
	return errors.Join(errs...)
}

func declaredProducerMetrics(contract Contract) map[string]struct{} {
	result := make(map[string]struct{})
	add := func(metric string) {
		if metric != "" {
			result[metric] = struct{}{}
		}
	}
	for _, source := range contract.Operational {
		if source.Units == "state" {
			add(source.Metric)
			continue
		}
		for _, dimension := range source.Dimensions {
			metric := source.Metric
			if strings.Contains(metric, "%s") {
				metric = fmt.Sprintf(metric, dimension)
			} else if len(source.Dimensions) > 1 {
				metric += "_" + dimension
			}
			add(metric)
		}
	}
	for _, status := range contract.Status {
		prefix := string(status.Kind)
		add(prefix + "_acquisition_state")
		if status.Status {
			add(prefix + "_health")
			add(prefix + "_health_rollup")
			add(prefix + "_state")
			for _, state := range []string{"ok", "warning", "critical", "unknown"} {
				add(prefix + "_conditions_" + state)
			}
		}
		if status.PowerState {
			add(prefix + "_power_state")
		}
		if status.FailurePredicted {
			add(prefix + "_failure_predicted")
		}
	}
	for _, state := range contract.States {
		add(state.Metric)
	}
	for _, flags := range contract.Flags {
		for _, member := range flags.Members {
			add(flags.Metric + "_" + member.Role)
		}
	}
	for _, field := range contract.Fields {
		if field.Exposure == ExposureOperationalScalar {
			add(field.Metric)
		}
	}
	for _, reading := range contract.Readings {
		if reading.Exposure != ExposureOperationalReading {
			continue
		}
		add(reading.Metric)
		add(reading.AlarmMetric)
	}
	for _, summary := range contract.SummaryClasses {
		for _, statistic := range []string{"minimum", "average", "maximum"} {
			add("aggregate_" + summary.ID + "_" + statistic)
		}
		if summary.Additive {
			add("aggregate_" + summary.ID + "_total")
		}
	}
	for _, metric := range []string{
		"aggregate_population_total",
		"aggregate_population_readable",
		"aggregate_population_unreadable",
		"aggregate_population_unknown",
		"aggregate_population_histogram_eligible",
		"aggregate_population_histogram_ineligible",
		"aggregate_completeness_complete",
		"aggregate_completeness_incomplete",
		"aggregate_completeness_histogram_available",
		"aggregate_completeness_histogram_unavailable",
	} {
		add(metric)
	}
	for _, histogram := range contract.Histograms {
		for _, bucket := range histogram.Buckets {
			add("aggregate_" + histogram.ID + "_distribution_" + bucket.ID)
		}
	}
	categories := map[string][]string{
		"health":            HealthStates,
		"health_rollup":     HealthStates,
		"resource_state":    ResourceStates,
		"power_state":       PowerStates,
		"failure_predicted": FailureStates,
		"acquisition_state": AcquisitionStates,
		"conditions":        {"ok", "warning", "critical", "unknown"},
		"reading_alarm":     AlarmStates,
	}
	for _, state := range contract.States {
		if len(state.AggregateKinds) != 0 {
			categories[aggregateCategoricalClass(state.Metric)] = state.States
		}
	}
	for _, flags := range contract.Flags {
		if len(flags.AggregateKinds) == 0 {
			continue
		}
		states := make([]string, 0, len(flags.Members))
		for _, member := range flags.Members {
			states = append(states, member.Role)
		}
		categories[aggregateCategoricalClass(flags.Metric)] = states
	}
	for class, states := range categories {
		for _, state := range states {
			add("aggregate_" + class + "_" + state)
		}
	}
	return result
}

func findKind(id Kind) KindSpec {
	for _, value := range kindSpecs {
		if value.ID == id {
			return value
		}
	}
	return KindSpec{}
}

func removeAdjacentDuplicates(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneContract(source Contract) Contract {
	source.Kinds = slices.Clone(source.Kinds)
	source.Relationships = slices.Clone(source.Relationships)
	source.Fields = cloneFields(source.Fields)
	source.Status = cloneStatus(source.Status)
	source.States = cloneStates(source.States)
	source.Flags = cloneFlags(source.Flags)
	source.Charts = cloneCharts(source.Charts)
	source.Columns = cloneColumns(source.Columns)
	source.Inventory = slices.Clone(source.Inventory)
	source.Operational = cloneOperational(source.Operational)
	source.ReadingTypes = cloneReadingTypes(source.ReadingTypes)
	source.ReadingRoles = slices.Clone(source.ReadingRoles)
	source.Readings = cloneReadings(source.Readings)
	source.Histograms = cloneHistograms(source.Histograms)
	source.SummaryClasses = slices.Clone(source.SummaryClasses)
	return source
}

func cloneColumns(source []ColumnSpec) []ColumnSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].Members = maps.Clone(result[index].Members)
	}
	return result
}

func cloneFields(source []FieldSpec) []FieldSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].Candidates = slices.Clone(result[index].Candidates)
		for candidate := range result[index].Candidates {
			result[index].Candidates[candidate].Requires = slices.Clone(
				result[index].Candidates[candidate].Requires,
			)
		}
		result[index].AggregateKinds = slices.Clone(result[index].AggregateKinds)
	}
	return result
}

func cloneStatus(source []StatusSpec) []StatusSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].AggregateKinds = slices.Clone(result[index].AggregateKinds)
	}
	return result
}

func cloneStates(source []StateSpec) []StateSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].States = slices.Clone(result[index].States)
		result[index].AggregateKinds = slices.Clone(result[index].AggregateKinds)
	}
	return result
}

func cloneFlags(source []FlagSetSpec) []FlagSetSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].Members = slices.Clone(result[index].Members)
		result[index].AggregateKinds = slices.Clone(result[index].AggregateKinds)
	}
	return result
}

func cloneCharts(source []ChartSpec) []ChartSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].InstanceLabels = slices.Clone(result[index].InstanceLabels)
		result[index].PromotedLabels = slices.Clone(result[index].PromotedLabels)
		result[index].Dimensions = slices.Clone(result[index].Dimensions)
	}
	return result
}

func cloneOperational(source []OperationalSpec) []OperationalSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].InstanceLabels = slices.Clone(result[index].InstanceLabels)
		result[index].Dimensions = slices.Clone(result[index].Dimensions)
	}
	return result
}

func cloneReadingTypes(source []ReadingTypeSpec) []ReadingTypeSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].SourceUnits = slices.Clone(result[index].SourceUnits)
	}
	return result
}

func cloneReadings(source []ReadingSurfaceSpec) []ReadingSurfaceSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].AggregateKinds = slices.Clone(result[index].AggregateKinds)
	}
	return result
}

func cloneHistograms(source []HistogramSpec) []HistogramSpec {
	result := slices.Clone(source)
	for index := range result {
		result[index].Buckets = slices.Clone(result[index].Buckets)
		for bucket := range result[index].Buckets {
			if value := result[index].Buckets[bucket].UpperExclusive; value != nil {
				copy := *value
				result[index].Buckets[bucket].UpperExclusive = &copy
			}
		}
	}
	return result
}
