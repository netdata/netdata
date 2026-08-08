// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import "strings"

// extendedFieldSpecs contains the mechanically expanded standard scalar rows
// that are easier to review as compact families than as repeated literals.
// The checked-in chart manifest remains the generated review surface.
func extendedFieldSpecs(firstOrder int) []FieldSpec {
	var result []FieldSpec
	add := func(spec FieldSpec) {
		spec.Order = firstOrder + len(result)
		if spec.ID == "" {
			spec.ID = strings.ReplaceAll(spec.Metric, "_", "-")
		}
		if spec.Scale.Den == 0 {
			spec.Scale = Identity
		}
		result = append(result, spec)
	}
	addFields := func(
		document Document,
		group string,
		base FieldSpec,
		fields ...fieldAtom,
	) {
		for _, atom := range fields {
			context := "redfish." + string(base.Kind) + "." + group
			if len(fields) > 1 {
				context += "." + atom.Role
			}
			metric := strings.ReplaceAll(context, ".", "_")
			column := atom.Column
			if column == "" {
				column = string(base.Kind) + "_" + snakePath(atom.Path)
			}
			spec := base
			spec.ID = string(base.Kind) + "_" + snakePath(string(document)+"_"+atom.Path)
			spec.Candidates = []SourceCandidate{{Document: document, Path: atom.Path, Unit: atom.SourceUnit}}
			spec.Metric = metric
			spec.Context = context
			spec.Role = atom.Role
			spec.Column = column
			spec.Title = atom.Title
			spec.Float = atom.Float
			spec.AggregateKinds = append([]Kind(nil), base.AggregateKinds...)
			add(spec)
		}
	}

	resource := "resource"
	replaceable := "replaceable"
	network := "network"

	// Processor core metrics.
	addFields("", "instructions_per_cycle", FieldSpec{
		Kind: "processor_core", Units: "instructions/cycle", Algorithm: AlgorithmAbsolute, Scale: Identity,
		AggregateKinds: []Kind{"processor"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "InstructionsPerCycle", Role: "instructions", Column: "processor_core_instructions_per_cycle", Float: true,
			Title: "Processor Core Instructions per Cycle"})
	addFields("", "error_rate", FieldSpec{
		Kind: "processor_core", Units: "errors/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"processor"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "CorrectableCoreErrorCount", Role: "correctable_core", Column: "processor_core_correctable_core_error_total", Float: true, Title: "Processor Core Correctable Core Error Rate"},
		fieldAtom{Path: "CorrectableOtherErrorCount", Role: "correctable_other", Column: "processor_core_correctable_other_error_total", Float: true, Title: "Processor Core Correctable Other Error Rate"},
		fieldAtom{Path: "UncorrectableCoreErrorCount", Role: "uncorrectable_core", Column: "processor_core_uncorrectable_core_error_total", Float: true, Title: "Processor Core Uncorrectable Core Error Rate"},
		fieldAtom{Path: "UncorrectableOtherErrorCount", Role: "uncorrectable_other", Column: "processor_core_uncorrectable_other_error_total", Float: true, Title: "Processor Core Uncorrectable Other Error Rate"})
	addFields("", "cycle_rate", FieldSpec{
		Kind: "processor_core", Units: "cycles/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"processor"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "IOStallCount", Role: "io_stall", Column: "processor_core_io_stall_cycles_total", Float: true, Title: "Processor Core I/O Stall Cycle Rate"},
		fieldAtom{Path: "MemoryStallCount", Role: "memory_stall", Column: "processor_core_memory_stall_cycles_total", Float: true, Title: "Processor Core Memory Stall Cycle Rate"},
		fieldAtom{Path: "UnhaltedCycles", Role: "unhalted", Column: "processor_core_unhalted_cycles_total", Float: true, Title: "Processor Core Unhalted Cycle Rate"})

	// Memory current-period activity and errors.
	for _, atom := range []fieldAtom{
		{Path: "CurrentPeriod.BlocksRead", Role: "read", Column: "memory_blocks_read_total", Float: true, Title: "Memory Read Throughput"},
		{Path: "CurrentPeriod.BlocksWritten", Role: "written", Column: "memory_blocks_written_total", Float: true, Title: "Memory Write Throughput"},
	} {
		context := "redfish.memory.io." + atom.Role
		add(FieldSpec{
			ID: "memory_" + snakePath(atom.Path), Kind: "memory",
			Candidates: []SourceCandidate{{
				Document: "memory_metrics", Path: atom.Path, Unit: "blocks",
				MultiplierDocument: "memory_metrics", MultiplierPath: "BlockSizeBytes",
				MultiplierScale: Identity, MultiplierColumn: "memory_block_size_bytes",
			}},
			Metric: strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.Role,
			Column: atom.Column, Title: atom.Title, Units: "bytes/s", Scale: Identity,
			Algorithm: AlgorithmRate, Float: true, Additive: true,
			AggregateKinds: []Kind{"system", "chassis"}, ComponentClass: replaceable,
		})
	}
	addFields("memory_metrics", "error_rate", FieldSpec{
		Kind: "memory", Units: "errors/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"system", "chassis"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "CurrentPeriod.CorrectableECCErrorCount", Role: "correctable", Column: "memory_current_correctable_ecc_error_total", Float: true, Title: "Memory Correctable ECC Error Rate"},
		fieldAtom{Path: "CurrentPeriod.IndeterminateCorrectableErrorCount", Role: "indeterminate_correctable", Column: "memory_current_indeterminate_correctable_error_total", Float: true, Title: "Memory Indeterminate Correctable Error Rate"},
		fieldAtom{Path: "CurrentPeriod.UncorrectableECCErrorCount", Role: "uncorrectable", Column: "memory_current_uncorrectable_ecc_error_total", Float: true, Title: "Memory Uncorrectable ECC Error Rate"},
		fieldAtom{Path: "CurrentPeriod.IndeterminateUncorrectableErrorCount", Role: "indeterminate_uncorrectable", Column: "memory_current_indeterminate_uncorrectable_error_total", Float: true, Title: "Memory Indeterminate Uncorrectable Error Rate"})

	// Storage and controller activity.
	addFields("storage_metrics", "iops", FieldSpec{
		Kind: "storage", Units: "requests/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"system", "chassis"}, ComponentClass: resource,
	},
		fieldAtom{Path: "IOStatistics.ReadHitIORequests", Role: "read_hit", Column: "storage_io_statistics_read_hit_io_requests", Float: true, Title: "Storage Read Hit IOPS"},
		fieldAtom{Path: "IOStatistics.WriteHitIORequests", Role: "write_hit", Column: "storage_io_statistics_write_hit_io_requests", Float: true, Title: "Storage Write Hit IOPS"},
		fieldAtom{Path: "IOStatistics.NonIORequests", Role: "non_io", Column: "storage_io_statistics_non_io_requests", Float: true, Title: "Storage Non-I/O Request Rate"})
	addFields("storage_controller_metrics", "error_rate", FieldSpec{
		Kind: "storage_controller", Units: "errors/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"storage", "chassis"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "CorrectableECCErrorCount", Role: "correctable_ecc", Column: "storage_controller_correctable_ecc_error_total", Float: true, Title: "Storage Controller Correctable ECC Error Rate"},
		fieldAtom{Path: "CorrectableParityErrorCount", Role: "correctable_parity", Column: "storage_controller_correctable_parity_error_total", Float: true, Title: "Storage Controller Correctable Parity Error Rate"},
		fieldAtom{Path: "UncorrectableECCErrorCount", Role: "uncorrectable_ecc", Column: "storage_controller_uncorrectable_ecc_error_total", Float: true, Title: "Storage Controller Uncorrectable ECC Error Rate"},
		fieldAtom{Path: "UncorrectableParityErrorCount", Role: "uncorrectable_parity", Column: "storage_controller_uncorrectable_parity_error_total", Float: true, Title: "Storage Controller Uncorrectable Parity Error Rate"})

	// Volume activity uses the linked Metrics representation first and the
	// legacy inline representation only as the declared fallback.
	volumeIO := []struct {
		path, group, role, units, column, title string
		algorithm                               Algorithm
		scale                                   Rational
	}{
		{"ReadIOKiBytes", "io", "read", "bytes/s", "volume_io_statistics_read_io_ki_bytes", "Volume Read Throughput", AlgorithmRate, Rational{Num: 1024, Den: 1}},
		{"WriteIOKiBytes", "io", "written", "bytes/s", "volume_io_statistics_write_io_ki_bytes", "Volume Write Throughput", AlgorithmRate, Rational{Num: 1024, Den: 1}},
		{"ReadIORequests", "iops", "read", "requests/s", "volume_io_statistics_read_io_requests", "Volume Read IOPS", AlgorithmRate, Identity},
		{"WriteIORequests", "iops", "written", "requests/s", "volume_io_statistics_write_io_requests", "Volume Write IOPS", AlgorithmRate, Identity},
		{"ReadHitIORequests", "iops", "read_hit", "requests/s", "volume_io_statistics_read_hit_io_requests", "Volume Read Hit IOPS", AlgorithmRate, Identity},
		{"WriteHitIORequests", "iops", "write_hit", "requests/s", "volume_io_statistics_write_hit_io_requests", "Volume Write Hit IOPS", AlgorithmRate, Identity},
		{"NonIORequests", "iops", "non_io", "requests/s", "volume_io_statistics_non_io_requests", "Volume Non-I/O Request Rate", AlgorithmRate, Identity},
		{"ReadIORequestTime", "io_time", "read", "percentage", "volume_io_statistics_read_io_request_time", "Volume Read I/O Time", AlgorithmDurationPercent, Identity},
		{"WriteIORequestTime", "io_time", "written", "percentage", "volume_io_statistics_write_io_request_time", "Volume Write I/O Time", AlgorithmDurationPercent, Identity},
		{"NonIORequestTime", "io_time", "non_io", "percentage", "volume_io_statistics_non_io_request_time", "Volume Non-I/O Time", AlgorithmDurationPercent, Identity},
	}
	for _, atom := range volumeIO {
		context := "redfish.volume." + atom.group + "." + atom.role
		add(FieldSpec{
			ID: "volume_io_statistics_" + snakePath(atom.path), Kind: "volume",
			Candidates: []SourceCandidate{
				{Document: "volume_metrics", Path: "IOStatistics." + atom.path},
				{Path: "IOStatistics." + atom.path},
			},
			EquivalenceProof: "volume_metrics_preferred_inline_fallback",
			Metric:           strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.role,
			Column: atom.column, Title: atom.title, Units: atom.units, Scale: atom.scale,
			Algorithm: atom.algorithm, Float: true, Additive: atom.algorithm == AlgorithmRate,
			Histogram:      map[bool]string{true: "percentage"}[atom.algorithm == AlgorithmDurationPercent],
			AggregateKinds: []Kind{"storage"}, ComponentClass: resource,
		})
	}
	addFields("volume_metrics", "error_rate", FieldSpec{
		Kind: "volume", Units: "errors/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"storage"}, ComponentClass: resource,
	},
		fieldAtom{Path: "CorrectableIOReadErrorCount", Role: "correctable_read", Column: "volume_correctable_io_read_error_total", Float: true, Title: "Volume Correctable Read Error Rate"},
		fieldAtom{Path: "CorrectableIOWriteErrorCount", Role: "correctable_write", Column: "volume_correctable_io_write_error_total", Float: true, Title: "Volume Correctable Write Error Rate"},
		fieldAtom{Path: "UncorrectableIOReadErrorCount", Role: "uncorrectable_read", Column: "volume_uncorrectable_io_read_error_total", Float: true, Title: "Volume Uncorrectable Read Error Rate"},
		fieldAtom{Path: "UncorrectableIOWriteErrorCount", Role: "uncorrectable_write", Column: "volume_uncorrectable_io_write_error_total", Float: true, Title: "Volume Uncorrectable Write Error Rate"},
		fieldAtom{Path: "ConsistencyCheckErrorCount", Role: "consistency_check", Column: "volume_consistency_check_error_total", Float: true, Title: "Volume Consistency Check Error Rate"},
		fieldAtom{Path: "RebuildErrorCount", Role: "rebuild", Column: "volume_rebuild_error_total", Float: true, Title: "Volume Rebuild Error Rate"})
	addFields("volume_metrics", "event_rate", FieldSpec{
		Kind: "volume", Units: "events/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"storage"}, ComponentClass: resource,
	},
		fieldAtom{Path: "ConsistencyCheckCount", Role: "consistency_checks", Column: "volume_consistency_check_total", Float: true, Title: "Volume Consistency Check Rate"},
		fieldAtom{Path: "StateChangeCount", Role: "state_changes", Column: "volume_state_change_total", Float: true, Title: "Volume State Change Rate"})

	// Control values are operational only when the standard ControlType and
	// SetPointUnits pair proves the semantic and conversion.
	type controlVariant struct {
		controlType string
		sourceUnits string
		scale       Rational
	}
	type controlFamily struct {
		family, units, title, histogram string
		variants                        []controlVariant
	}
	controlFamilies := []controlFamily{
		{"temperature", "Celsius", "Temperature", "temperature", []controlVariant{{"Temperature", "Cel", Identity}}},
		{"power", "watts", "Power", "", []controlVariant{{"Power", "W", Identity}}},
		{"frequency", "hertz", "Frequency", "", []controlVariant{
			{"Frequency", "Hz", Identity},
			{"FrequencyMHz", "MHz", Rational{Num: 1_000_000, Den: 1}},
		}},
		{"pressure", "pascals", "Pressure", "", []controlVariant{
			{"Pressure", "kPa", Rational{Num: 1_000, Den: 1}},
			{"PressurekPa", "kPa", Rational{Num: 1_000, Den: 1}},
		}},
		{"valve_position", "percentage", "Valve Position", "percentage", []controlVariant{{"Valve", "%", Identity}}},
		{"percentage", "percentage", "Percentage", "percentage", []controlVariant{
			{"Percent", "%", Identity},
			{"DutyCycle", "%", Identity},
		}},
		{"linear_position", "meters", "Linear Position", "", []controlVariant{{"LinearPosition", "m", Identity}}},
		{"linear_velocity", "meters/second", "Linear Velocity", "", []controlVariant{{"LinearVelocity", "m/s", Identity}}},
		{"linear_acceleration", "meters/second2", "Linear Acceleration", "", []controlVariant{{"LinearAcceleration", "m/s2", Identity}}},
		{"rotational_position", "radians", "Rotational Position", "", []controlVariant{{"RotationalPosition", "rad", Identity}}},
		{"rotational_velocity", "radians/second", "Rotational Velocity", "", []controlVariant{{"RotationalVelocity", "rad/s", Identity}}},
		{"rotational_acceleration", "radians/second2", "Rotational Acceleration", "", []controlVariant{{"RotationalAcceleration", "rad/s2", Identity}}},
		{"liquid_flow", "liters/minute", "Liquid Flow", "", []controlVariant{{"LiquidFlowLPM", "L/min", Identity}}},
	}
	controlRoles := []struct {
		path, role, column, title string
	}{
		{"Sensor.Reading", "sensor", "control_sensor_reading", "Sensor Reading"},
		{"SetPoint", "setpoint", "control_setpoint", "Set Point"},
		{"DefaultSetPoint", "default_setpoint", "control_default_setpoint", "Default Set Point"},
		{"SetPointError", "setpoint_error", "control_setpoint_error", "Set Point Error"},
		{"AllowableMin", "allowable_minimum", "control_allowable_min", "Allowable Minimum"},
		{"AllowableMax", "allowable_maximum", "control_allowable_max", "Allowable Maximum"},
		{"SettingMin", "setting_minimum", "control_setting_min", "Setting Minimum"},
		{"SettingMax", "setting_maximum", "control_setting_max", "Setting Maximum"},
	}
	for _, family := range controlFamilies {
		for _, role := range controlRoles {
			candidates := make([]SourceCandidate, 0, len(family.variants))
			for _, variant := range family.variants {
				candidates = append(candidates, SourceCandidate{
					Path:  role.path,
					Unit:  variant.sourceUnits,
					Scale: variant.scale,
					Requires: []SourceRequirement{
						{Path: "ControlType", Value: variant.controlType},
						{Path: "SetPointUnits", Value: variant.sourceUnits},
					},
				})
			}
			context := "redfish.control." + family.family + "." + role.role
			equivalence := ""
			if len(candidates) > 1 {
				equivalence = "control_type_unit_pair"
			}
			add(FieldSpec{
				ID:               "control_" + family.family + "_" + role.role,
				Kind:             "control",
				Candidates:       candidates,
				EquivalenceProof: equivalence,
				Metric:           strings.ReplaceAll(context, ".", "_"),
				Context:          context,
				Role:             role.role,
				Column:           role.column,
				Title:            "Control " + family.title + " " + role.title,
				Units:            family.units,
				Scale:            Identity,
				Algorithm:        AlgorithmAbsolute,
				Float:            true,
				MixedColumnUnits: true,
				Histogram:        family.histogram,
				AggregateKinds:   []Kind{"chassis"},
				ComponentClass:   resource,
			})
		}
	}

	// The same closed NVMe SMART adapter is applied to drives and storage
	// controllers. Lifetime PowerOnHours remains inventory-only.
	addNVMe := func(kind Kind, document Document, parents []Kind, componentClass string) {
		kindTitle := map[Kind]string{"drive": "Drive", "storage_controller": "Storage Controller"}[kind]
		type nvmeAtom struct {
			path, group, role, units, title string
			algorithm                       Algorithm
			scale                           Rational
			additive                        bool
			histogram                       string
		}
		atoms := []nvmeAtom{
			{"AvailableSparePercent", "nvme.spare", "available", "percentage", "NVMe Available Spare", AlgorithmAbsolute, Identity, false, "percentage"},
			{"AvailableSpareThresholdPercent", "nvme.spare", "threshold", "percentage", "NVMe Available Spare Threshold", AlgorithmAbsolute, Identity, false, "percentage"},
			{"PercentageUsed", "nvme.wear", "used", "percentage", "NVMe Percentage Used", AlgorithmAbsolute, Identity, false, "percentage"},
			{"CompositeTemperatureCelsius", "nvme.temperature", "composite", "Celsius", "NVMe Composite Temperature", AlgorithmAbsolute, Identity, false, "temperature"},
			{"DataUnitsRead", "nvme.io", "read", "bytes/s", "NVMe Read Throughput", AlgorithmRate, Rational{Num: 512_000, Den: 1}, true, ""},
			{"DataUnitsWritten", "nvme.io", "written", "bytes/s", "NVMe Write Throughput", AlgorithmRate, Rational{Num: 512_000, Den: 1}, true, ""},
			{"HostReadCommands", "nvme.commands", "read", "commands/s", "NVMe Host Read Command Rate", AlgorithmRate, Identity, true, ""},
			{"HostWriteCommands", "nvme.commands", "written", "commands/s", "NVMe Host Write Command Rate", AlgorithmRate, Identity, true, ""},
			{"ControllerBusyTimeMinutes", "nvme.busy_time", "busy", "percentage", "NVMe Controller Busy Time", AlgorithmDurationPercent, Rational{Num: 60, Den: 1}, false, "percentage"},
			{"MediaAndDataIntegrityErrors", "nvme.error_rate", "media_integrity", "errors/s", "NVMe Media and Data Integrity Error Rate", AlgorithmRate, Identity, true, ""},
			{"NumberOfErrorInformationLogEntries", "nvme.error_rate", "error_log_entries", "errors/s", "NVMe Error Information Log Entry Rate", AlgorithmRate, Identity, true, ""},
			{"UnsafeShutdowns", "nvme.event_rate", "unsafe_shutdowns", "events/s", "NVMe Unsafe Shutdown Rate", AlgorithmRate, Identity, true, ""},
			{"PowerCycles", "nvme.event_rate", "power_cycles", "events/s", "NVMe Power Cycle Rate", AlgorithmRate, Identity, true, ""},
			{"WarningCompositeTempTimeMinutes", "nvme.thermal_time", "warning", "percentage", "NVMe Warning Temperature Time", AlgorithmDurationPercent, Rational{Num: 60, Den: 1}, false, "percentage"},
			{"CriticalCompositeTempTimeMinutes", "nvme.thermal_time", "critical", "percentage", "NVMe Critical Temperature Time", AlgorithmDurationPercent, Rational{Num: 60, Den: 1}, false, "percentage"},
			{"ThermalMgmtTemp1TotalTimeSeconds", "nvme.thermal_time", "management_1", "percentage", "NVMe Thermal Management 1 Time", AlgorithmDurationPercent, Identity, false, "percentage"},
			{"ThermalMgmtTemp2TotalTimeSeconds", "nvme.thermal_time", "management_2", "percentage", "NVMe Thermal Management 2 Time", AlgorithmDurationPercent, Identity, false, "percentage"},
			{"ThermalMgmtTemp1TransitionCount", "nvme.thermal_transition_rate", "management_1", "transitions/s", "NVMe Thermal Management 1 Transition Rate", AlgorithmRate, Identity, true, ""},
			{"ThermalMgmtTemp2TransitionCount", "nvme.thermal_transition_rate", "management_2", "transitions/s", "NVMe Thermal Management 2 Transition Rate", AlgorithmRate, Identity, true, ""},
		}
		for _, atom := range atoms {
			context := "redfish." + string(kind) + "." + atom.group + "." + atom.role
			add(FieldSpec{
				ID: string(kind) + "_nvme_" + snakePath(atom.path), Kind: kind,
				Candidates: []SourceCandidate{{Document: document, Path: "NVMeSMART." + atom.path}},
				Metric:     strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.role,
				Column: string(kind) + "_nvme_" + snakePath(atom.path), Title: kindTitle + " " + atom.title,
				Units: atom.units, Scale: atom.scale, Algorithm: atom.algorithm, Float: true,
				Additive: atom.additive, Histogram: atom.histogram,
				AggregateKinds: append([]Kind(nil), parents...), ComponentClass: componentClass,
			})
		}
	}
	addNVMe("storage_controller", "storage_controller_metrics", []Kind{"storage", "chassis"}, replaceable)
	addNVMe("drive", "drive_metrics", []Kind{"storage", "chassis"}, replaceable)

	// Network adapter and network-device-function metrics.
	addFields("network_adapter_metrics", "frames", FieldSpec{
		Kind: "network_adapter", Units: "frames/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"chassis"}, ComponentClass: network,
	},
		fieldAtom{Path: "RXUnicastFrames", Role: "received_unicast", Float: true, Title: "Network Adapter RX Unicast Frame Rate"},
		fieldAtom{Path: "RXMulticastFrames", Role: "received_multicast", Float: true, Title: "Network Adapter RX Multicast Frame Rate"},
		fieldAtom{Path: "TXUnicastFrames", Role: "sent_unicast", Float: true, Title: "Network Adapter TX Unicast Frame Rate"},
		fieldAtom{Path: "TXMulticastFrames", Role: "sent_multicast", Float: true, Title: "Network Adapter TX Multicast Frame Rate"})
	addFields("network_adapter_metrics", "ncsi_traffic", FieldSpec{
		Kind: "network_adapter", Units: "bytes/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"chassis"}, ComponentClass: network,
	},
		fieldAtom{Path: "NCSIRXBytes", Role: "received", Float: true, Title: "Network Adapter NCSI Received Traffic"},
		fieldAtom{Path: "NCSITXBytes", Role: "sent", Float: true, Title: "Network Adapter NCSI Sent Traffic"})
	addFields("network_adapter_metrics", "ncsi_frames", FieldSpec{
		Kind: "network_adapter", Units: "frames/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"chassis"}, ComponentClass: network,
	},
		fieldAtom{Path: "NCSIRXFrames", Role: "received", Float: true, Title: "Network Adapter NCSI Received Frame Rate"},
		fieldAtom{Path: "NCSITXFrames", Role: "sent", Float: true, Title: "Network Adapter NCSI Sent Frame Rate"})
	addFields("network_adapter_metrics", "cpu_utilization", FieldSpec{
		Kind: "network_adapter", Units: "percentage", Algorithm: AlgorithmAbsolute, Scale: Identity,
		Histogram: "percentage", AggregateKinds: []Kind{"chassis"}, ComponentClass: network,
	},
		fieldAtom{Path: "CPUCorePercent", Role: "cpu", Float: true, Title: "Network Adapter CPU Utilization"})
	addFields("network_adapter_metrics", "host_bus_utilization", FieldSpec{
		Kind: "network_adapter", Units: "percentage", Algorithm: AlgorithmAbsolute, Scale: Identity,
		Histogram: "percentage", AggregateKinds: []Kind{"chassis"}, ComponentClass: network,
	},
		fieldAtom{Path: "HostBusRXPercent", Role: "received", Float: true, Title: "Network Adapter Host Bus RX Utilization"},
		fieldAtom{Path: "HostBusTXPercent", Role: "sent", Float: true, Title: "Network Adapter Host Bus TX Utilization"})

	addFields("network_device_function_metrics", "frames", FieldSpec{
		Kind: "network_device_function", Units: "frames/s", Algorithm: AlgorithmRate, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"network_adapter", "network_interface"}, ComponentClass: network,
	},
		fieldAtom{Path: "RXFrames", Role: "received", Float: true, Title: "Network Device Function RX Frame Rate"},
		fieldAtom{Path: "TXFrames", Role: "sent", Float: true, Title: "Network Device Function TX Frame Rate"},
		fieldAtom{Path: "RXUnicastFrames", Role: "received_unicast", Float: true, Title: "Network Device Function RX Unicast Frame Rate"},
		fieldAtom{Path: "TXUnicastFrames", Role: "sent_unicast", Float: true, Title: "Network Device Function TX Unicast Frame Rate"},
		fieldAtom{Path: "RXMulticastFrames", Role: "received_multicast", Float: true, Title: "Network Device Function RX Multicast Frame Rate"},
		fieldAtom{Path: "TXMulticastFrames", Role: "sent_multicast", Float: true, Title: "Network Device Function TX Multicast Frame Rate"})
	addFields("network_device_function_metrics", "queue_depth", FieldSpec{
		Kind: "network_device_function", Units: "percentage", Algorithm: AlgorithmAbsolute, Scale: Identity,
		Histogram: "percentage", AggregateKinds: []Kind{"network_adapter", "network_interface"}, ComponentClass: network,
	},
		fieldAtom{Path: "RXAvgQueueDepthPercent", Role: "received", Float: true, Title: "Network Device Function RX Queue Depth"},
		fieldAtom{Path: "TXAvgQueueDepthPercent", Role: "sent", Float: true, Title: "Network Device Function TX Queue Depth"})
	addFields("network_device_function_metrics", "queue_full", FieldSpec{
		Kind: "network_device_function", Units: "queues", Algorithm: AlgorithmAbsolute, Scale: Identity, Additive: true,
		AggregateKinds: []Kind{"network_adapter", "network_interface"}, ComponentClass: network,
	},
		fieldAtom{Path: "RXQueuesFull", Role: "received", Float: false, Title: "Network Device Function RX Full Queues"},
		fieldAtom{Path: "TXQueuesFull", Role: "sent", Float: false, Title: "Network Device Function TX Full Queues"})

	// Port frame, error, RDMA, and PCIe activity.
	portMetrics := []struct {
		path, group, role, units, title string
		scale                           Rational
	}{
		{"Networking.RXFrames", "frames", "received", "frames/s", "Port RX Frame Rate", Identity},
		{"Networking.TXFrames", "frames", "sent", "frames/s", "Port TX Frame Rate", Identity},
		{"Networking.RXUnicastFrames", "frames", "received_unicast", "frames/s", "Port RX Unicast Frame Rate", Identity},
		{"Networking.TXUnicastFrames", "frames", "sent_unicast", "frames/s", "Port TX Unicast Frame Rate", Identity},
		{"Networking.RXMulticastFrames", "frames", "received_multicast", "frames/s", "Port RX Multicast Frame Rate", Identity},
		{"Networking.TXMulticastFrames", "frames", "sent_multicast", "frames/s", "Port TX Multicast Frame Rate", Identity},
		{"Networking.RXBroadcastFrames", "frames", "received_broadcast", "frames/s", "Port RX Broadcast Frame Rate", Identity},
		{"Networking.TXBroadcastFrames", "frames", "sent_broadcast", "frames/s", "Port TX Broadcast Frame Rate", Identity},
		{"Networking.RDMARXBytes", "rdma_traffic", "received", "bytes/s", "Port RDMA Received Traffic", Identity},
		{"Networking.RDMATXBytes", "rdma_traffic", "sent", "bytes/s", "Port RDMA Sent Traffic", Identity},
		{"Networking.RDMAProtectionErrors", "rdma_error_rate", "protection", "errors/s", "Port RDMA Protection Error Rate", Identity},
		{"Networking.RDMAProtocolErrors", "rdma_error_rate", "protocol", "errors/s", "Port RDMA Protocol Error Rate", Identity},
	}
	for _, property := range []struct{ path, role, title string }{
		{"RXDiscards", "received_discards", "Port RX Discard Rate"},
		{"TXDiscards", "sent_discards", "Port TX Discard Rate"},
		{"RXFCSErrors", "received_fcs", "Port RX FCS Error Rate"},
		{"RXFalseCarrierErrors", "received_false_carrier", "Port RX False Carrier Error Rate"},
		{"RXFrameAlignmentErrors", "received_alignment", "Port RX Frame Alignment Error Rate"},
		{"RXOversizeFrames", "received_oversize", "Port RX Oversize Frame Rate"},
		{"RXUndersizeFrames", "received_undersize", "Port RX Undersize Frame Rate"},
		{"TXExcessiveCollisions", "sent_excessive_collisions", "Port TX Excessive Collision Rate"},
		{"TXLateCollisions", "sent_late_collisions", "Port TX Late Collision Rate"},
		{"TXMultipleCollisions", "sent_multiple_collisions", "Port TX Multiple Collision Rate"},
		{"TXSingleCollisions", "sent_single_collisions", "Port TX Single Collision Rate"},
	} {
		portMetrics = append(portMetrics, struct {
			path, group, role, units, title string
			scale                           Rational
		}{"Networking." + property.path, "network_error_rate", property.role, "errors/s", property.title, Identity})
	}
	for _, atom := range portMetrics {
		context := "redfish.port." + atom.group + "." + atom.role
		add(FieldSpec{
			ID: "port_" + snakePath(atom.path), Kind: "port",
			Candidates: []SourceCandidate{{Document: "port_metrics", Path: atom.path}},
			Metric:     strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.role,
			Column: "port_" + snakePath(atom.path), Title: atom.title, Units: atom.units, Scale: atom.scale,
			Algorithm: AlgorithmRate, Float: true, Additive: true,
			AggregateKinds: []Kind{"processor", "storage_controller", "network_adapter", "network_interface"},
			ComponentClass: network,
		})
	}
	addPCIeFields := func(kind Kind, document Document, prefix string, parents []Kind) {
		for _, atom := range []fieldAtom{
			{Path: prefix + "BadDLLPCount", Role: "bad_dllp", Title: "Bad DLLP Error Rate"},
			{Path: prefix + "BadTLPCount", Role: "bad_tlp", Title: "Bad TLP Error Rate"},
			{Path: prefix + "CorrectableErrorCount", Role: "correctable", Title: "Correctable PCIe Error Rate"},
			{Path: prefix + "FatalErrorCount", Role: "fatal", Title: "Fatal PCIe Error Rate"},
			{Path: prefix + "FlowControlTimeoutErrors", Role: "flow_control_timeout", Title: "PCIe Flow Control Timeout Rate"},
			{Path: prefix + "L0ToRecoveryCount", Role: "l0_to_recovery", Title: "PCIe L0-to-Recovery Rate"},
			{Path: prefix + "NAKReceivedCount", Role: "nak_received", Title: "PCIe NAK Received Rate"},
			{Path: prefix + "NAKSentCount", Role: "nak_sent", Title: "PCIe NAK Sent Rate"},
			{Path: prefix + "NonFatalErrorCount", Role: "non_fatal", Title: "Non-Fatal PCIe Error Rate"},
			{Path: prefix + "ReplayCount", Role: "replay", Title: "PCIe Replay Rate"},
			{Path: prefix + "ReplayRolloverCount", Role: "replay_rollover", Title: "PCIe Replay Rollover Rate"},
			{Path: prefix + "UnsupportedRequestCount", Role: "unsupported_request", Title: "PCIe Unsupported Request Rate"},
		} {
			context := "redfish." + string(kind) + ".pcie_error_rate." + atom.Role
			add(FieldSpec{
				ID: string(kind) + "_" + snakePath(atom.Path), Kind: kind,
				Candidates: []SourceCandidate{{Document: document, Path: atom.Path}},
				Metric:     strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.Role,
				Column: string(kind) + "_" + snakePath(atom.Path), Title: atom.Title, Units: "errors/s",
				Scale: Identity, Algorithm: AlgorithmRate, Float: true, Additive: true,
				AggregateKinds: parents, ComponentClass: network,
			})
		}
	}
	addPCIeFields("processor", "processor_metrics", "PCIeErrors.", []Kind{"system", "chassis"})
	addPCIeFields("port", "port_metrics", "PCIeErrors.", []Kind{"processor", "storage_controller", "network_adapter", "network_interface"})
	for _, atom := range []struct{ path, group, role, units, title string }{
		{"PCIeMetrics.OutboundCompletionTLPBytes", "pcie_traffic", "completion", "bytes/s", "Port PCIe Completion Traffic"},
		{"PCIeMetrics.OutboundReadTLPBytes", "pcie_traffic", "read", "bytes/s", "Port PCIe Read Traffic"},
		{"PCIeMetrics.OutboundWriteTLPBytes", "pcie_traffic", "write", "bytes/s", "Port PCIe Write Traffic"},
		{"PCIeMetrics.OutboundCompletionTLPCount", "pcie_tlp_rate", "completion", "packets/s", "Port PCIe Completion TLP Rate"},
		{"PCIeMetrics.OutboundReadTLPCount", "pcie_tlp_rate", "read", "packets/s", "Port PCIe Read TLP Rate"},
		{"PCIeMetrics.OutboundWriteTLPCount", "pcie_tlp_rate", "write", "packets/s", "Port PCIe Write TLP Rate"},
		{"PCIeMetrics.CompletionCreditExhaustionDrops", "pcie_drop_rate", "completion_credit", "drops/s", "Port PCIe Completion Credit Drop Rate"},
		{"PCIeMetrics.NPCreditExhaustionDrops", "pcie_drop_rate", "non_posted_credit", "drops/s", "Port PCIe Non-Posted Credit Drop Rate"},
		{"PCIeMetrics.TagUnavailabilityDrops", "pcie_drop_rate", "tag_unavailable", "drops/s", "Port PCIe Tag Unavailable Drop Rate"},
	} {
		context := "redfish.port." + atom.group + "." + atom.role
		add(FieldSpec{
			ID: "port_" + snakePath(atom.path), Kind: "port",
			Candidates: []SourceCandidate{{Document: "port_metrics", Path: atom.path}},
			Metric:     strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.role,
			Column: "port_" + snakePath(atom.path), Title: atom.title, Units: atom.units,
			Scale: Identity, Algorithm: AlgorithmRate, Float: true, Additive: true,
			AggregateKinds: []Kind{"processor", "storage_controller", "network_adapter", "network_interface"},
			ComponentClass: network,
		})
	}

	// Redundancy and heater metrics.
	for _, atom := range []struct {
		candidates          []SourceCandidate
		role, column, title string
	}{
		{[]SourceCandidate{{Path: `ActiveRedundancySet.@odata.count`}, {Path: `ActiveRedundancyGroup.@odata.count`}}, "active", "redundancy_active_count", "Redundancy Active Members"},
		{[]SourceCandidate{{Path: `RedundancySet.@odata.count`}, {Path: `RedundancyGroup.@odata.count`}}, "total", "redundancy_member_count", "Redundancy Total Members"},
		{[]SourceCandidate{{Path: "MinNumNeeded"}, {Path: "MinNeededInGroup"}}, "minimum", "redundancy_min_needed", "Redundancy Minimum Members"},
		{[]SourceCandidate{{Path: "MinNumNeededForFaultTolerance"}, {Path: "MinNeededForFaultTolerance"}}, "fault_tolerance_minimum", "redundancy_min_fault_tolerance", "Redundancy Fault-Tolerance Minimum"},
		{[]SourceCandidate{{Path: "MaxNumSupported"}, {Path: "MaxSupportedInGroup"}}, "maximum", "redundancy_max_supported", "Redundancy Maximum Members"},
	} {
		context := "redfish.redundancy.members." + atom.role
		add(FieldSpec{
			ID: "redundancy_members_" + atom.role, Kind: "redundancy",
			Candidates: atom.candidates, EquivalenceProof: "redundancy_model_equivalence",
			Metric: strings.ReplaceAll(context, ".", "_"), Context: context, Role: atom.role,
			Column: atom.column, Title: atom.title, Units: "members", Scale: Identity,
			Algorithm: AlgorithmAbsolute, Float: false, Additive: false,
			AggregateKinds: []Kind{"system", "manager", "storage", "thermal_subsystem", "power_subsystem"},
			ComponentClass: resource,
		})
	}
	addFields("heater_metrics", "heating_time", FieldSpec{
		Kind: "heater", Units: "percentage", Algorithm: AlgorithmDurationPercent, Scale: Identity,
		Histogram: "percentage", AggregateKinds: []Kind{"thermal_subsystem", "chassis"}, ComponentClass: replaceable,
	},
		fieldAtom{Path: "PrePowerOnHeatingTimeSeconds", Role: "pre_power_on", Column: "heater_pre_power_on_heating_seconds_total", Float: true, Title: "Heater Pre-Power-On Heating Time"},
		fieldAtom{Path: "RuntimeHeatingTimeSeconds", Role: "runtime", Column: "heater_runtime_heating_seconds_total", Float: true, Title: "Heater Runtime Heating Time"})

	return result
}

type fieldAtom struct {
	Path       string
	Role       string
	Column     string
	SourceUnit string
	Title      string
	Float      bool
}

func snakePath(value string) string {
	value = strings.Trim(value, "_.")
	var out []byte
	var priorLower bool
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case ch == '.':
			if len(out) > 0 && out[len(out)-1] != '_' {
				out = append(out, '_')
			}
			priorLower = false
		case ch >= 'A' && ch <= 'Z':
			if len(out) > 0 && priorLower && out[len(out)-1] != '_' {
				out = append(out, '_')
			}
			out = append(out, ch+('a'-'A'))
			priorLower = false
		default:
			out = append(out, ch)
			priorLower = ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9'
		}
	}
	return strings.Trim(string(out), "_")
}
