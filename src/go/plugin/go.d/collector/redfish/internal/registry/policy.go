// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"sort"
	"strings"
)

const (
	expectedFieldCount              = 365
	expectedInventoryFieldCount     = 107
	expectedOperationalFieldCount   = 258
	expectedReadingCount            = 940
	expectedInventoryReadingCount   = 450
	expectedPrimaryReadingCount     = 105
	expectedAuxiliaryReadingCount   = 385
	expectedOperationalScalarGroups = 105
)

func applyPresentationPolicy(contract *Contract) error {
	seenFields := make(map[string]struct{}, len(contract.Fields))
	for index := range contract.Fields {
		field := &contract.Fields[index]
		exposure, ok := fieldExposureByID[field.ID]
		if !ok {
			return fmt.Errorf("field %q has no explicit exposure disposition", field.ID)
		}
		field.Exposure = exposure
		seenFields[field.ID] = struct{}{}
		if field.Exposure == ExposureInventoryOnly {
			field.AggregateKinds = nil
			field.AggregateClass = ""
			continue
		}
		field.AggregateKinds = admittedFieldAggregateKinds(*field, contract.Relationships)
		if len(field.AggregateKinds) == 0 {
			field.AggregateClass = ""
			continue
		}
		summary, ok := summaryClassForField(*field)
		if !ok {
			return fmt.Errorf(
				"operational field %q has aggregate owners but no summary class for units %q (additive=%t)",
				field.ID,
				field.Units,
				field.Additive,
			)
		}
		field.AggregateClass = summary.ID
	}
	for id := range fieldExposureByID {
		if _, ok := seenFields[id]; !ok {
			return fmt.Errorf("field exposure disposition %q has no registry row", id)
		}
	}

	for index := range contract.Readings {
		reading := &contract.Readings[index]
		switch reading.Exposure {
		case ExposureInventoryOnly:
			if reading.Primary {
				return fmt.Errorf(
					"inventory reading %s/%s/%s/%s cannot be primary",
					reading.Family,
					reading.Basis,
					reading.Role,
					reading.SemanticClass,
				)
			}
			reading.AggregateKinds = nil
			reading.AggregateClass = ""
			continue
		case ExposureOperationalReading:
		default:
			return fmt.Errorf(
				"reading %s/%s/%s/%s has no explicit exposure disposition",
				reading.Family,
				reading.Basis,
				reading.Role,
				reading.SemanticClass,
			)
		}

		if !reading.CommonContext {
			reading.Metric = "reading_" + reading.Family + "_value"
			reading.Context = "redfish.reading." + reading.Family
			reading.Title = readingFamilyTitles[reading.Family] + " Readings"
			reading.AlarmMetric = "reading_alarm"
			reading.AlarmContext = "redfish.reading.alarm"
		}

		if !reading.Primary || cumulativeReading(*reading) {
			reading.AggregateKinds = nil
			reading.AggregateClass = ""
			continue
		}
		summary, ok := summaryClassForReading(*reading)
		if !ok {
			return fmt.Errorf(
				"operational reading %s/%s/%s has aggregate owners but no summary class for units %q",
				reading.Family,
				reading.Basis,
				reading.Role,
				reading.Units,
			)
		}
		reading.AggregateClass = summary.ID
	}

	contract.SummaryClasses = compileSummaryClasses(*contract)
	return validateExposureCounts(*contract)
}

func admittedFieldAggregateKinds(field FieldSpec, relationships []RelationshipSpec) []Kind {
	if fieldAggregateDirectOnly(field) {
		return nil
	}
	var result []Kind
	for _, parent := range field.AggregateKinds {
		if directAggregateRelationship(field.Kind, parent, relationships) {
			result = append(result, parent)
		}
	}
	return uniqueKinds(result)
}

func fieldAggregateDirectOnly(field FieldSpec) bool {
	if field.Kind == "control" || field.Kind == "battery" || field.Kind == "redundancy" {
		return true
	}
	if field.Kind == "power_subsystem" && (field.Role == "allocated" || field.Role == "requested") {
		return true
	}
	if field.Kind == "volume" && strings.HasPrefix(field.Context, "redfish.volume.space_savings.") {
		return true
	}
	switch field.Context {
	case "redfish.storage_controller.pcie_lanes.active",
		"redfish.drive.link_speed.negotiated",
		"redfish.ethernet_interface.link_speed",
		"redfish.network_port.link_speed",
		"redfish.port.link_speed.speed",
		"redfish.port.link_width.active",
		"redfish.pcie_device.link_width.active":
		return true
	default:
		return false
	}
}

func directAggregateRelationship(child, parent Kind, relationships []RelationshipSpec) bool {
	// Processor cores are embedded standard rows under their processor. The
	// graph constructs this exact edge from ProcessorMetrics.CoreMetrics.
	if child == "processor_core" && parent == "processor" {
		return true
	}
	for _, relationship := range relationships {
		if relationship.Child != child || relationship.Parent != parent || relationship.RollupRank < 0 {
			continue
		}
		if relationship.Mode == RelationshipComponent || relationship.Mode == RelationshipLegacy {
			return true
		}
	}
	return false
}

func cumulativeReading(reading ReadingSurfaceSpec) bool {
	if reading.DerivedFromEnergy {
		return true
	}
	switch reading.Family {
	case "energy", "apparent_energy", "reactive_energy":
		return true
	default:
		return false
	}
}

func summaryClass(units string, additive bool) (SummaryClassSpec, bool) {
	type key struct {
		units    string
		additive bool
	}
	classes := map[key]SummaryClassSpec{
		{"Celsius", false}:             {ID: "temperature", Title: "Temperature", Units: "Celsius", Histogram: "temperature"},
		{"MHz", false}:                 {ID: "clock_speed", Title: "Clock Speed", Units: "MHz"},
		{"bytes/s", true}:              {ID: "data_throughput", Title: "Data Throughput", Units: "bytes/s", Additive: true},
		{"changes/s", true}:            {ID: "change_rate", Title: "Change Rate", Units: "changes/s", Additive: true},
		{"commands", true}:             {ID: "command_depth", Title: "Command Depth", Units: "commands", Additive: true},
		{"commands/s", true}:           {ID: "command_rate", Title: "Command Rate", Units: "commands/s", Additive: true},
		{"cycles/s", true}:             {ID: "cycle_rate", Title: "Cycle Rate", Units: "cycles/s", Additive: true},
		{"drops/s", true}:              {ID: "drop_rate", Title: "Drop Rate", Units: "drops/s", Additive: true},
		{"errors/s", true}:             {ID: "error_rate", Title: "Error Rate", Units: "errors/s", Additive: true},
		{"events/s", true}:             {ID: "event_rate", Title: "Event Rate", Units: "events/s", Additive: true},
		{"frames/s", true}:             {ID: "frame_rate", Title: "Frame Rate", Units: "frames/s", Additive: true},
		{"instructions/cycle", false}:  {ID: "instructions_per_cycle", Title: "Instructions per Cycle", Units: "instructions/cycle"},
		{"packets/s", true}:            {ID: "packet_rate", Title: "Packet Rate", Units: "packets/s", Additive: true},
		{"queues", true}:               {ID: "queue_count", Title: "Queue Count", Units: "queues", Additive: true},
		{"ratio", false}:               {ID: "ratio", Title: "Ratio", Units: "ratio"},
		{"requests/s", true}:           {ID: "request_rate", Title: "Request Rate", Units: "requests/s", Additive: true},
		{"shutdowns/s", true}:          {ID: "shutdown_rate", Title: "Shutdown Rate", Units: "shutdowns/s", Additive: true},
		{"transitions/s", true}:        {ID: "transition_rate", Title: "Transition Rate", Units: "transitions/s", Additive: true},
		{"watts", true}:                {ID: "power_budget", Title: "Power Budget", Units: "watts", Additive: true},
		{"watts", false}:               {ID: "power", Title: "Power", Units: "watts"},
		{"ampere-hours", false}:        {ID: "charge", Title: "Charge", Units: "ampere-hours"},
		{"volts", false}:               {ID: "voltage", Title: "Voltage", Units: "volts"},
		{"amperes", false}:             {ID: "current", Title: "Current", Units: "amperes"},
		{"hertz", false}:               {ID: "frequency", Title: "Frequency", Units: "hertz"},
		{"pascals", false}:             {ID: "pressure", Title: "Pressure", Units: "pascals"},
		{"meters", false}:              {ID: "length", Title: "Length", Units: "meters"},
		{"RPM", false}:                 {ID: "rotational_speed", Title: "Rotational Speed", Units: "RPM"},
		{"cubic-meters/minute", false}: {ID: "air_flow", Title: "Air Flow", Units: "cubic-meters/minute"},
		{"liters/minute", false}:       {ID: "liquid_flow", Title: "Liquid Flow", Units: "liters/minute"},
		{"grams/cubic-meter", false}:   {ID: "absolute_humidity", Title: "Absolute Humidity", Units: "grams/cubic-meter"},
		{"meters/second", false}:       {ID: "linear_velocity", Title: "Linear Velocity", Units: "meters/second"},
		{"meters/second2", false}:      {ID: "linear_acceleration", Title: "Linear Acceleration", Units: "meters/second2"},
		{"radians", false}:             {ID: "rotational_position", Title: "Rotational Position", Units: "radians"},
		{"radians/second", false}:      {ID: "rotational_velocity", Title: "Rotational Velocity", Units: "radians/second"},
		{"radians/second2", false}:     {ID: "rotational_acceleration", Title: "Rotational Acceleration", Units: "radians/second2"},
		{"volt-amperes", false}:        {ID: "apparent_power", Title: "Apparent Power", Units: "volt-amperes"},
		{"vars", false}:                {ID: "reactive_power", Title: "Reactive Power", Units: "vars"},
		{"degrees", false}:             {ID: "phase_angle", Title: "Phase Angle", Units: "degrees"},
		{"watt-hours", false}:          {ID: "stored_energy", Title: "Stored Energy", Units: "watt-hours"},
	}
	result, ok := classes[key{units: units, additive: additive}]
	return result, ok
}

func summaryClassForField(field FieldSpec) (SummaryClassSpec, bool) {
	if field.Units != "percentage" {
		return summaryClass(field.Units, field.Additive)
	}
	context := scalarBaseRowContext(field.Context, field.Role)
	var id, title string
	switch context {
	case "redfish.drive.media_life",
		"redfish.drive.nvme.spare",
		"redfish.memory.media_health",
		"redfish.storage_controller.nvme.spare",
		"redfish.volume.remaining_capacity":
		id, title = "remaining_percentage", "Remaining Percentage"
	case "redfish.drive.nvme.wear",
		"redfish.storage_controller.nvme.wear":
		id, title = "wear_percentage", "Wear Percentage"
	case "redfish.drive.nvme.busy_time",
		"redfish.drive.nvme.thermal_time",
		"redfish.heater.heating_time",
		"redfish.processor.throttle_time",
		"redfish.storage_controller.nvme.busy_time",
		"redfish.storage_controller.nvme.thermal_time",
		"redfish.volume.io_time":
		id, title = "time_percentage", "Time Percentage"
	case "redfish.memory.bandwidth_utilization",
		"redfish.memory.capacity_utilization",
		"redfish.network_adapter.cpu_utilization",
		"redfish.network_adapter.host_bus_utilization",
		"redfish.network_device_function.queue_depth",
		"redfish.processor.bandwidth_utilization",
		"redfish.processor.utilization":
		id, title = "utilization_percentage", "Utilization Percentage"
	default:
		return SummaryClassSpec{}, false
	}
	return SummaryClassSpec{
		ID: id, Title: title, Units: field.Units, Histogram: "percentage",
	}, true
}

func summaryClassForReading(reading ReadingSurfaceSpec) (SummaryClassSpec, bool) {
	if reading.Units != "percentage" {
		return summaryClass(reading.Units, false)
	}
	var id, title string
	switch reading.Family {
	case "humidity":
		id, title = "humidity_percentage", "Humidity"
	case "valve_position":
		id, title = "valve_position_percentage", "Valve Position"
	case "percentage":
		switch reading.Role {
		case "speed", "fan_speed":
			id, title = "speed_percentage", "Speed Percentage"
		case "charge":
			id, title = "charge_percentage", "Charge Percentage"
		case "state_of_health":
			id, title = "health_percentage", "Health Percentage"
		case "power_load":
			id, title = "load_percentage", "Load Percentage"
		case "input":
			id, title = "percentage", "Percentage"
		default:
			return SummaryClassSpec{}, false
		}
	default:
		return SummaryClassSpec{}, false
	}
	return SummaryClassSpec{
		ID: id, Title: title, Units: reading.Units, Histogram: "percentage",
	}, true
}

func compileSummaryClasses(contract Contract) []SummaryClassSpec {
	byID := make(map[string]SummaryClassSpec)
	for _, field := range contract.Fields {
		if field.AggregateClass == "" {
			continue
		}
		summary, _ := summaryClassForField(field)
		byID[summary.ID] = summary
	}
	for _, reading := range contract.Readings {
		if reading.AggregateClass == "" {
			continue
		}
		summary, _ := summaryClassForReading(reading)
		byID[summary.ID] = summary
	}
	result := make([]SummaryClassSpec, 0, len(byID))
	for _, summary := range byID {
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func validateExposureCounts(contract Contract) error {
	type fieldGroupKey struct {
		kind    Kind
		context string
	}
	var inventoryFields, operationalFields int
	fieldGroups := make(map[fieldGroupKey]struct{})
	for _, field := range contract.Fields {
		switch field.Exposure {
		case ExposureInventoryOnly:
			inventoryFields++
		case ExposureOperationalScalar:
			operationalFields++
			fieldGroups[fieldGroupKey{
				kind: field.Kind, context: scalarBaseRowContext(field.Context, field.Role),
			}] = struct{}{}
		default:
			return fmt.Errorf("field %q has unspecified exposure %q", field.ID, field.Exposure)
		}
	}
	var inventoryReadings, primaryReadings, auxiliaryReadings int
	for _, reading := range contract.Readings {
		switch reading.Exposure {
		case ExposureInventoryOnly:
			inventoryReadings++
		case ExposureOperationalReading:
			if reading.Primary {
				primaryReadings++
			} else {
				auxiliaryReadings++
			}
		default:
			return fmt.Errorf(
				"reading %s/%s/%s/%s has unspecified exposure %q",
				reading.Family,
				reading.Basis,
				reading.Role,
				reading.SemanticClass,
				reading.Exposure,
			)
		}
	}
	if len(contract.Fields) != expectedFieldCount ||
		inventoryFields != expectedInventoryFieldCount ||
		operationalFields != expectedOperationalFieldCount ||
		len(fieldGroups) != expectedOperationalScalarGroups {
		return fmt.Errorf(
			"field exposure contract changed: total=%d inventory=%d operational=%d groups=%d",
			len(contract.Fields),
			inventoryFields,
			operationalFields,
			len(fieldGroups),
		)
	}
	if len(contract.Readings) != expectedReadingCount ||
		inventoryReadings != expectedInventoryReadingCount ||
		primaryReadings != expectedPrimaryReadingCount ||
		auxiliaryReadings != expectedAuxiliaryReadingCount {
		return fmt.Errorf(
			"reading exposure contract changed: total=%d inventory=%d primary=%d auxiliary=%d",
			len(contract.Readings),
			inventoryReadings,
			primaryReadings,
			auxiliaryReadings,
		)
	}
	return nil
}
