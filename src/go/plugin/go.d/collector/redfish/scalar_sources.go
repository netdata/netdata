// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

type scalarAlgorithm uint8

const (
	algorithmAbsolute scalarAlgorithm = iota
	algorithmRate
	algorithmDurationPercent
)

type valueScale struct{ Num, Den int64 }

var identityScale = valueScale{1, 1}

type sourceRequirement struct{ Path, Value string }
type scalarSource struct {
	Document, Path                     string
	Scale                              valueScale
	Requires                           []sourceRequirement
	MultiplierDocument, MultiplierPath string
	MultiplierScale                    valueScale
}
type sourceField struct {
	ID, Kind, Metric string
	Candidates       []scalarSource
	Scale            valueScale
	Algorithm        scalarAlgorithm
}

// Candidates are equivalent Redfish representations, in preference order.
// A source-specific scale and block-size multiplier override the field scale.
var scalarFields = []sourceField{
	{
		ID:        "system_processorsummary_metrics_bandwidthpercent",
		Kind:      "system",
		Metric:    "system_processor_bandwidth",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_summary_metrics", Path: "BandwidthPercent"},
			{Document: "", Path: "ProcessorSummary.Metrics.BandwidthPercent"},
		},
	},
	{
		ID:        "system_processorsummary_metrics_kernelpercent",
		Kind:      "system",
		Metric:    "system_processor_utilization_kernel",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_summary_metrics", Path: "KernelPercent"},
			{Document: "", Path: "ProcessorSummary.Metrics.KernelPercent"},
		},
	},
	{
		ID:        "system_processorsummary_metrics_userpercent",
		Kind:      "system",
		Metric:    "system_processor_utilization_user",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_summary_metrics", Path: "UserPercent"},
			{Document: "", Path: "ProcessorSummary.Metrics.UserPercent"},
		},
	},
	{
		ID:        "system_memorysummary_metrics_bandwidthpercent",
		Kind:      "system",
		Metric:    "system_memory_bandwidth",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_summary_metrics", Path: "BandwidthPercent"},
			{Document: "", Path: "MemorySummary.Metrics.BandwidthPercent"},
		},
	},
	{
		ID:        "system_memorysummary_metrics_capacityutilizationpercent",
		Kind:      "system",
		Metric:    "system_memory_utilization",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_summary_metrics", Path: "CapacityUtilizationPercent"},
			{Document: "", Path: "MemorySummary.Metrics.CapacityUtilizationPercent"},
		},
	},
	{
		ID:        "processor_operating_speed",
		Kind:      "processor",
		Metric:    "processor_clock_speed_operating",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "OperatingSpeedMHz"},
			{Document: "", Path: "OperatingSpeedMHz"},
			{Document: "processor_metrics", Path: "AverageFrequencyMHz"},
		},
	},
	{
		ID:        "processor_processor_metrics_userpercent",
		Kind:      "processor",
		Metric:    "processor_utilization_user",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "UserPercent"},
		},
	},
	{
		ID:        "processor_processor_metrics_kernelpercent",
		Kind:      "processor",
		Metric:    "processor_utilization_kernel",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "KernelPercent"},
		},
	},
	{
		ID:        "processor_processor_metrics_bandwidthpercent",
		Kind:      "processor",
		Metric:    "processor_bandwidth_utilization",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "BandwidthPercent"},
		},
	},
	{
		ID:        "processor_processor_metrics_frequencyratio",
		Kind:      "processor",
		Metric:    "processor_frequency_ratio",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "FrequencyRatio"},
		},
	},
	{
		ID:        "processor_processor_metrics_throttlingcelsius",
		Kind:      "processor",
		Metric:    "processor_thermal_headroom",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "ThrottlingCelsius"},
		},
	},
	{
		ID:        "processor_processor_metrics_correctablecoreerrorcount",
		Kind:      "processor",
		Metric:    "processor_error_rate_correctable_core",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "CorrectableCoreErrorCount"},
		},
	},
	{
		ID:        "processor_processor_metrics_correctableothererrorcount",
		Kind:      "processor",
		Metric:    "processor_error_rate_correctable_other",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "CorrectableOtherErrorCount"},
		},
	},
	{
		ID:        "processor_processor_metrics_uncorrectablecoreerrorcount",
		Kind:      "processor",
		Metric:    "processor_error_rate_uncorrectable_core",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "UncorrectableCoreErrorCount"},
		},
	},
	{
		ID:        "processor_processor_metrics_uncorrectableothererrorcount",
		Kind:      "processor",
		Metric:    "processor_error_rate_uncorrectable_other",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "UncorrectableOtherErrorCount"},
		},
	},
	{
		ID:        "processor_processor_metrics_powerlimitthrottleduration",
		Kind:      "processor",
		Metric:    "processor_throttle_time_power_limit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PowerLimitThrottleDuration"},
		},
	},
	{
		ID:        "processor_processor_metrics_thermallimitthrottleduration",
		Kind:      "processor",
		Metric:    "processor_throttle_time_thermal_limit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "ThermalLimitThrottleDuration"},
		},
	},
	{
		ID:        "memory_memory_metrics_capacityutilizationpercent",
		Kind:      "memory",
		Metric:    "memory_capacity_utilization",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "CapacityUtilizationPercent"},
		},
	},
	{
		ID:        "memory_memory_metrics_bandwidthpercent",
		Kind:      "memory",
		Metric:    "memory_bandwidth_utilization",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "BandwidthPercent"},
		},
	},
	{
		ID:        "memory_memory_metrics_dirtyshutdowncount",
		Kind:      "memory",
		Metric:    "memory_dirty_shutdown_rate",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "DirtyShutdownCount"},
		},
	},
	{
		ID:        "memory_memory_metrics_healthdata_predictedmedialifeleftpercent",
		Kind:      "memory",
		Metric:    "memory_media_health_life_left",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "HealthData.PredictedMediaLifeLeftPercent"},
		},
	},
	{
		ID:        "memory_memory_metrics_healthdata_remainingspareblockpercentage",
		Kind:      "memory",
		Metric:    "memory_media_health_spare_remaining",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "HealthData.RemainingSpareBlockPercentage"},
		},
	},
	{
		ID:        "storage_storage_metrics_statechangecount",
		Kind:      "storage",
		Metric:    "storage_state_change_rate",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "StateChangeCount"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_readiokibytes",
		Kind:      "storage",
		Metric:    "storage_io_read",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.ReadIOKiBytes"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_writeiokibytes",
		Kind:      "storage",
		Metric:    "storage_io_written",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.WriteIOKiBytes"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_readiorequests",
		Kind:      "storage",
		Metric:    "storage_iops_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.ReadIORequests"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_writeiorequests",
		Kind:      "storage",
		Metric:    "storage_iops_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.WriteIORequests"},
		},
	},
	{
		ID:        "storage_controller_pcieinterface_lanesinuse",
		Kind:      "storage_controller",
		Metric:    "storage_controller_pcie_lanes_active",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "PCIeInterface.LanesInUse"},
		},
	},
	{
		ID:        "storage_controller_storage_controller_metrics_statechangecount",
		Kind:      "storage_controller",
		Metric:    "storage_controller_state_change_rate",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "StateChangeCount"},
		},
	},
	{
		ID:        "drive_negotiatedspeedgbs",
		Kind:      "drive",
		Metric:    "drive_link_speed_negotiated",
		Scale:     valueScale{1000000000, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "NegotiatedSpeedGbs"},
		},
	},
	{
		ID:        "drive_predictedmedialifeleftpercent",
		Kind:      "drive",
		Metric:    "drive_media_life",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "PredictedMediaLifeLeftPercent"},
		},
	},
	{
		ID:        "drive_drive_metrics_nativecommandqueuedepth",
		Kind:      "drive",
		Metric:    "drive_queue_depth",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NativeCommandQueueDepth"},
		},
	},
	{
		ID:        "drive_drive_metrics_readiokibytes",
		Kind:      "drive",
		Metric:    "drive_io_read",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "ReadIOKiBytes"},
		},
	},
	{
		ID:        "drive_drive_metrics_writeiokibytes",
		Kind:      "drive",
		Metric:    "drive_io_written",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "WriteIOKiBytes"},
		},
	},
	{
		ID:        "drive_drive_metrics_correctableioreaderrorcount",
		Kind:      "drive",
		Metric:    "drive_error_rate_correctable_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "CorrectableIOReadErrorCount"},
		},
	},
	{
		ID:        "drive_drive_metrics_correctableiowriteerrorcount",
		Kind:      "drive",
		Metric:    "drive_error_rate_correctable_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "CorrectableIOWriteErrorCount"},
		},
	},
	{
		ID:        "drive_drive_metrics_uncorrectableioreaderrorcount",
		Kind:      "drive",
		Metric:    "drive_error_rate_uncorrectable_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "UncorrectableIOReadErrorCount"},
		},
	},
	{
		ID:        "drive_drive_metrics_uncorrectableiowriteerrorcount",
		Kind:      "drive",
		Metric:    "drive_error_rate_uncorrectable_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "UncorrectableIOWriteErrorCount"},
		},
	},
	{
		ID:        "drive_drive_metrics_badblockcount",
		Kind:      "drive",
		Metric:    "drive_error_rate_bad_blocks",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "BadBlockCount"},
		},
	},
	{
		ID:        "volume_remainingcapacitypercent",
		Kind:      "volume",
		Metric:    "volume_remaining_capacity",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "RemainingCapacityPercent"},
		},
	},
	{
		ID:        "volume_volume_metrics_compressionsavingsbytes",
		Kind:      "volume",
		Metric:    "volume_space_savings_compression",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "CompressionSavingsBytes"},
		},
	},
	{
		ID:        "volume_volume_metrics_deduplicationsavingsbytes",
		Kind:      "volume",
		Metric:    "volume_space_savings_deduplication",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "DeduplicationSavingsBytes"},
		},
	},
	{
		ID:        "volume_volume_metrics_thinprovisioningsavingsbytes",
		Kind:      "volume",
		Metric:    "volume_space_savings_thin_provisioning",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "ThinProvisioningSavingsBytes"},
		},
	},
	{
		ID:        "ethernet_interface_speedmbps",
		Kind:      "ethernet_interface",
		Metric:    "ethernet_interface_link_speed",
		Scale:     valueScale{1000000, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "SpeedMbps"},
		},
	},
	{
		ID:        "network_port_currentlinkspeedmbps",
		Kind:      "network_port",
		Metric:    "network_port_link_speed",
		Scale:     valueScale{1000000, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "CurrentLinkSpeedMbps"},
		},
	},
	{
		ID:        "port_currentspeedgbps",
		Kind:      "port",
		Metric:    "port_link_speed_speed",
		Scale:     valueScale{1000000000, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "CurrentSpeedGbps"},
		},
	},
	{
		ID:        "port_activewidth",
		Kind:      "port",
		Metric:    "port_link_width_active",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "ActiveWidth"},
		},
	},
	{
		ID:        "pcie_device_pcieinterface_lanesinuse",
		Kind:      "pcie_device",
		Metric:    "pcie_device_link_width_active",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "PCIeInterface.LanesInUse"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_rxbytes",
		Kind:      "network_adapter",
		Metric:    "network_adapter_traffic_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "RXBytes"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_txbytes",
		Kind:      "network_adapter",
		Metric:    "network_adapter_traffic_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "TXBytes"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxbytes",
		Kind:      "network_device_function",
		Metric:    "network_device_function_traffic_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXBytes"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txbytes",
		Kind:      "network_device_function",
		Metric:    "network_device_function_traffic_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXBytes"},
		},
	},
	{
		ID:        "port_port_metrics_rxbytes",
		Kind:      "port",
		Metric:    "port_traffic_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "RXBytes"},
		},
	},
	{
		ID:        "port_port_metrics_txbytes",
		Kind:      "port",
		Metric:    "port_traffic_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "TXBytes"},
		},
	},
	{
		ID:        "port_port_metrics_rxerrors",
		Kind:      "port",
		Metric:    "port_error_rate_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "RXErrors"},
		},
	},
	{
		ID:        "port_port_metrics_txerrors",
		Kind:      "port",
		Metric:    "port_error_rate_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "TXErrors"},
		},
	},
	{
		ID:        "power_subsystem_allocation_allocatedwatts",
		Kind:      "power_subsystem",
		Metric:    "power_subsystem_capacity_allocated",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "Allocation.AllocatedWatts"},
		},
	},
	{
		ID:        "power_subsystem_allocation_requestedwatts",
		Kind:      "power_subsystem",
		Metric:    "power_subsystem_capacity_requested",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "Allocation.RequestedWatts"},
		},
	},
	{
		ID:        "battery_capacityactualamphours",
		Kind:      "battery",
		Metric:    "battery_charge_capacity_actual",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "CapacityActualAmpHours"},
		},
	},
	{
		ID:        "battery_capacityactualwatthours",
		Kind:      "battery",
		Metric:    "battery_energy_capacity_actual",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "CapacityActualWattHours"},
		},
	},
	{
		ID:        "battery_battery_metrics_crate",
		Kind:      "battery",
		Metric:    "battery_c_rate",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "battery_metrics", Path: "CRate"},
		},
	},
	{
		ID:        "battery_battery_metrics_erate",
		Kind:      "battery",
		Metric:    "battery_e_rate",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "battery_metrics", Path: "ERate"},
		},
	},
	{
		ID:        "manager_datetime_clock_offset",
		Kind:      "manager",
		Metric:    "manager_clock_offset",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "DateTime"},
		},
	},
	{
		ID:        "processor_core_instructions_per_cycle",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_instructions_per_cycle",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "InstructionsPerCycle"},
		},
	},
	{
		ID:        "processor_core_correctable_core_error_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_error_rate_correctable_core",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "CorrectableCoreErrorCount"},
		},
	},
	{
		ID:        "processor_core_correctable_other_error_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_error_rate_correctable_other",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "CorrectableOtherErrorCount"},
		},
	},
	{
		ID:        "processor_core_uncorrectable_core_error_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_error_rate_uncorrectable_core",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "UncorrectableCoreErrorCount"},
		},
	},
	{
		ID:        "processor_core_uncorrectable_other_error_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_error_rate_uncorrectable_other",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "UncorrectableOtherErrorCount"},
		},
	},
	{
		ID:        "processor_core_iostall_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_cycle_rate_io_stall",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "IOStallCount"},
		},
	},
	{
		ID:        "processor_core_memory_stall_count",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_cycle_rate_memory_stall",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "MemoryStallCount"},
		},
	},
	{
		ID:        "processor_core_unhalted_cycles",
		Kind:      "processor_core",
		Metric:    "redfish_processor_core_cycle_rate_unhalted",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "", Path: "UnhaltedCycles"},
		},
	},
	{
		ID:        "memory_current_period_blocks_read",
		Kind:      "memory",
		Metric:    "redfish_memory_io_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{
				Document:           "memory_metrics",
				Path:               "CurrentPeriod.BlocksRead",
				MultiplierDocument: "memory_metrics",
				MultiplierPath:     "BlockSizeBytes",
				MultiplierScale:    valueScale{1, 1},
			},
		},
	},
	{
		ID:        "memory_current_period_blocks_written",
		Kind:      "memory",
		Metric:    "redfish_memory_io_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{
				Document:           "memory_metrics",
				Path:               "CurrentPeriod.BlocksWritten",
				MultiplierDocument: "memory_metrics",
				MultiplierPath:     "BlockSizeBytes",
				MultiplierScale:    valueScale{1, 1},
			},
		},
	},
	{
		ID:        "memory_memory_metrics_current_period_correctable_eccerror_count",
		Kind:      "memory",
		Metric:    "redfish_memory_error_rate_correctable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "CurrentPeriod.CorrectableECCErrorCount"},
		},
	},
	{
		ID:        "memory_memory_metrics_current_period_indeterminate_correctable_error_count",
		Kind:      "memory",
		Metric:    "redfish_memory_error_rate_indeterminate_correctable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "CurrentPeriod.IndeterminateCorrectableErrorCount"},
		},
	},
	{
		ID:        "memory_memory_metrics_current_period_uncorrectable_eccerror_count",
		Kind:      "memory",
		Metric:    "redfish_memory_error_rate_uncorrectable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "CurrentPeriod.UncorrectableECCErrorCount"},
		},
	},
	{
		ID:        "memory_memory_metrics_current_period_indeterminate_uncorrectable_error_count",
		Kind:      "memory",
		Metric:    "redfish_memory_error_rate_indeterminate_uncorrectable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "memory_metrics", Path: "CurrentPeriod.IndeterminateUncorrectableErrorCount"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_read_hit_iorequests",
		Kind:      "storage",
		Metric:    "redfish_storage_iops_read_hit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.ReadHitIORequests"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_write_hit_iorequests",
		Kind:      "storage",
		Metric:    "redfish_storage_iops_write_hit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.WriteHitIORequests"},
		},
	},
	{
		ID:        "storage_storage_metrics_iostatistics_non_iorequests",
		Kind:      "storage",
		Metric:    "redfish_storage_iops_non_io",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_metrics", Path: "IOStatistics.NonIORequests"},
		},
	},
	{
		ID:        "storage_controller_storage_controller_metrics_correctable_eccerror_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_error_rate_correctable_ecc",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "CorrectableECCErrorCount"},
		},
	},
	{
		ID:        "storage_controller_storage_controller_metrics_correctable_parity_error_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_error_rate_correctable_parity",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "CorrectableParityErrorCount"},
		},
	},
	{
		ID:        "storage_controller_storage_controller_metrics_uncorrectable_eccerror_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_error_rate_uncorrectable_ecc",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "UncorrectableECCErrorCount"},
		},
	},
	{
		ID:        "storage_controller_storage_controller_metrics_uncorrectable_parity_error_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_error_rate_uncorrectable_parity",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "UncorrectableParityErrorCount"},
		},
	},
	{
		ID:        "volume_io_statistics_read_ioki_bytes",
		Kind:      "volume",
		Metric:    "redfish_volume_io_read",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.ReadIOKiBytes"},
			{Document: "", Path: "IOStatistics.ReadIOKiBytes"},
		},
	},
	{
		ID:        "volume_io_statistics_write_ioki_bytes",
		Kind:      "volume",
		Metric:    "redfish_volume_io_written",
		Scale:     valueScale{1024, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.WriteIOKiBytes"},
			{Document: "", Path: "IOStatistics.WriteIOKiBytes"},
		},
	},
	{
		ID:        "volume_io_statistics_read_iorequests",
		Kind:      "volume",
		Metric:    "redfish_volume_iops_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.ReadIORequests"},
			{Document: "", Path: "IOStatistics.ReadIORequests"},
		},
	},
	{
		ID:        "volume_io_statistics_write_iorequests",
		Kind:      "volume",
		Metric:    "redfish_volume_iops_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.WriteIORequests"},
			{Document: "", Path: "IOStatistics.WriteIORequests"},
		},
	},
	{
		ID:        "volume_io_statistics_read_hit_iorequests",
		Kind:      "volume",
		Metric:    "redfish_volume_iops_read_hit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.ReadHitIORequests"},
			{Document: "", Path: "IOStatistics.ReadHitIORequests"},
		},
	},
	{
		ID:        "volume_io_statistics_write_hit_iorequests",
		Kind:      "volume",
		Metric:    "redfish_volume_iops_write_hit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.WriteHitIORequests"},
			{Document: "", Path: "IOStatistics.WriteHitIORequests"},
		},
	},
	{
		ID:        "volume_io_statistics_non_iorequests",
		Kind:      "volume",
		Metric:    "redfish_volume_iops_non_io",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.NonIORequests"},
			{Document: "", Path: "IOStatistics.NonIORequests"},
		},
	},
	{
		ID:        "volume_io_statistics_read_iorequest_time",
		Kind:      "volume",
		Metric:    "redfish_volume_io_time_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.ReadIORequestTime"},
			{Document: "", Path: "IOStatistics.ReadIORequestTime"},
		},
	},
	{
		ID:        "volume_io_statistics_write_iorequest_time",
		Kind:      "volume",
		Metric:    "redfish_volume_io_time_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.WriteIORequestTime"},
			{Document: "", Path: "IOStatistics.WriteIORequestTime"},
		},
	},
	{
		ID:        "volume_io_statistics_non_iorequest_time",
		Kind:      "volume",
		Metric:    "redfish_volume_io_time_non_io",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "IOStatistics.NonIORequestTime"},
			{Document: "", Path: "IOStatistics.NonIORequestTime"},
		},
	},
	{
		ID:        "volume_volume_metrics_correctable_ioread_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_correctable_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "CorrectableIOReadErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_correctable_iowrite_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_correctable_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "CorrectableIOWriteErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_uncorrectable_ioread_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_uncorrectable_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "UncorrectableIOReadErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_uncorrectable_iowrite_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_uncorrectable_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "UncorrectableIOWriteErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_consistency_check_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_consistency_check",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "ConsistencyCheckErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_rebuild_error_count",
		Kind:      "volume",
		Metric:    "redfish_volume_error_rate_rebuild",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "RebuildErrorCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_consistency_check_count",
		Kind:      "volume",
		Metric:    "redfish_volume_event_rate_consistency_checks",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "ConsistencyCheckCount"},
		},
	},
	{
		ID:        "volume_volume_metrics_state_change_count",
		Kind:      "volume",
		Metric:    "redfish_volume_event_rate_state_changes",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "volume_metrics", Path: "StateChangeCount"},
		},
	},
	{
		ID:        "control_temperature_sensor",
		Kind:      "control",
		Metric:    "redfish_control_temperature_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Temperature"}, {"SetPointUnits", "Cel"}},
			},
		},
	},
	{
		ID:        "control_temperature_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_temperature_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Temperature"}, {"SetPointUnits", "Cel"}},
			},
		},
	},
	{
		ID:        "control_temperature_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_temperature_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Temperature"}, {"SetPointUnits", "Cel"}},
			},
		},
	},
	{
		ID:        "control_power_sensor",
		Kind:      "control",
		Metric:    "redfish_control_power_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Power"}, {"SetPointUnits", "W"}},
			},
		},
	},
	{
		ID:        "control_power_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_power_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Power"}, {"SetPointUnits", "W"}},
			},
		},
	},
	{
		ID:        "control_power_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_power_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Power"}, {"SetPointUnits", "W"}},
			},
		},
	},
	{
		ID:        "control_frequency_sensor",
		Kind:      "control",
		Metric:    "redfish_control_frequency_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Frequency"}, {"SetPointUnits", "Hz"}},
			},
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1000000, 1},
				Requires: []sourceRequirement{{"ControlType", "FrequencyMHz"}, {"SetPointUnits", "MHz"}},
			},
		},
	},
	{
		ID:        "control_frequency_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_frequency_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Frequency"}, {"SetPointUnits", "Hz"}},
			},
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1000000, 1},
				Requires: []sourceRequirement{{"ControlType", "FrequencyMHz"}, {"SetPointUnits", "MHz"}},
			},
		},
	},
	{
		ID:        "control_frequency_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_frequency_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Frequency"}, {"SetPointUnits", "Hz"}},
			},
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1000000, 1},
				Requires: []sourceRequirement{{"ControlType", "FrequencyMHz"}, {"SetPointUnits", "MHz"}},
			},
		},
	},
	{
		ID:        "control_pressure_sensor",
		Kind:      "control",
		Metric:    "redfish_control_pressure_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "Pressure"}, {"SetPointUnits", "kPa"}},
			},
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "PressurekPa"}, {"SetPointUnits", "kPa"}},
			},
		},
	},
	{
		ID:        "control_pressure_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_pressure_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "Pressure"}, {"SetPointUnits", "kPa"}},
			},
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "PressurekPa"}, {"SetPointUnits", "kPa"}},
			},
		},
	},
	{
		ID:        "control_pressure_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_pressure_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "Pressure"}, {"SetPointUnits", "kPa"}},
			},
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1000, 1},
				Requires: []sourceRequirement{{"ControlType", "PressurekPa"}, {"SetPointUnits", "kPa"}},
			},
		},
	},
	{
		ID:        "control_valve_position_sensor",
		Kind:      "control",
		Metric:    "redfish_control_valve_position_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Valve"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_valve_position_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_valve_position_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Valve"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_valve_position_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_valve_position_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Valve"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_percentage_sensor",
		Kind:      "control",
		Metric:    "redfish_control_percentage_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Percent"}, {"SetPointUnits", "%"}},
			},
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "DutyCycle"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_percentage_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_percentage_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Percent"}, {"SetPointUnits", "%"}},
			},
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "DutyCycle"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_percentage_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_percentage_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "Percent"}, {"SetPointUnits", "%"}},
			},
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "DutyCycle"}, {"SetPointUnits", "%"}},
			},
		},
	},
	{
		ID:        "control_linear_position_sensor",
		Kind:      "control",
		Metric:    "redfish_control_linear_position_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearPosition"}, {"SetPointUnits", "m"}},
			},
		},
	},
	{
		ID:        "control_linear_position_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_linear_position_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearPosition"}, {"SetPointUnits", "m"}},
			},
		},
	},
	{
		ID:        "control_linear_position_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_linear_position_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearPosition"}, {"SetPointUnits", "m"}},
			},
		},
	},
	{
		ID:        "control_linear_velocity_sensor",
		Kind:      "control",
		Metric:    "redfish_control_linear_velocity_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearVelocity"}, {"SetPointUnits", "m/s"}},
			},
		},
	},
	{
		ID:        "control_linear_velocity_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_linear_velocity_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearVelocity"}, {"SetPointUnits", "m/s"}},
			},
		},
	},
	{
		ID:        "control_linear_velocity_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_linear_velocity_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearVelocity"}, {"SetPointUnits", "m/s"}},
			},
		},
	},
	{
		ID:        "control_linear_acceleration_sensor",
		Kind:      "control",
		Metric:    "redfish_control_linear_acceleration_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearAcceleration"}, {"SetPointUnits", "m/s2"}},
			},
		},
	},
	{
		ID:        "control_linear_acceleration_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_linear_acceleration_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearAcceleration"}, {"SetPointUnits", "m/s2"}},
			},
		},
	},
	{
		ID:        "control_linear_acceleration_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_linear_acceleration_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LinearAcceleration"}, {"SetPointUnits", "m/s2"}},
			},
		},
	},
	{
		ID:        "control_rotational_position_sensor",
		Kind:      "control",
		Metric:    "redfish_control_rotational_position_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalPosition"}, {"SetPointUnits", "rad"}},
			},
		},
	},
	{
		ID:        "control_rotational_position_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_rotational_position_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalPosition"}, {"SetPointUnits", "rad"}},
			},
		},
	},
	{
		ID:        "control_rotational_position_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_rotational_position_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalPosition"}, {"SetPointUnits", "rad"}},
			},
		},
	},
	{
		ID:        "control_rotational_velocity_sensor",
		Kind:      "control",
		Metric:    "redfish_control_rotational_velocity_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalVelocity"}, {"SetPointUnits", "rad/s"}},
			},
		},
	},
	{
		ID:        "control_rotational_velocity_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_rotational_velocity_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalVelocity"}, {"SetPointUnits", "rad/s"}},
			},
		},
	},
	{
		ID:        "control_rotational_velocity_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_rotational_velocity_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalVelocity"}, {"SetPointUnits", "rad/s"}},
			},
		},
	},
	{
		ID:        "control_rotational_acceleration_sensor",
		Kind:      "control",
		Metric:    "redfish_control_rotational_acceleration_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalAcceleration"}, {"SetPointUnits", "rad/s2"}},
			},
		},
	},
	{
		ID:        "control_rotational_acceleration_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_rotational_acceleration_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalAcceleration"}, {"SetPointUnits", "rad/s2"}},
			},
		},
	},
	{
		ID:        "control_rotational_acceleration_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_rotational_acceleration_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "RotationalAcceleration"}, {"SetPointUnits", "rad/s2"}},
			},
		},
	},
	{
		ID:        "control_liquid_flow_sensor",
		Kind:      "control",
		Metric:    "redfish_control_liquid_flow_sensor",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "Sensor.Reading",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LiquidFlowLPM"}, {"SetPointUnits", "L/min"}},
			},
		},
	},
	{
		ID:        "control_liquid_flow_setpoint",
		Kind:      "control",
		Metric:    "redfish_control_liquid_flow_setpoint",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPoint",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LiquidFlowLPM"}, {"SetPointUnits", "L/min"}},
			},
		},
	},
	{
		ID:        "control_liquid_flow_setpoint_error",
		Kind:      "control",
		Metric:    "redfish_control_liquid_flow_setpoint_error",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{
				Document: "",
				Path:     "SetPointError",
				Scale:    valueScale{1, 1},
				Requires: []sourceRequirement{{"ControlType", "LiquidFlowLPM"}, {"SetPointUnits", "L/min"}},
			},
		},
	},
	{
		ID:        "storage_controller_nvme_available_spare_percent",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_spare_available",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.AvailableSparePercent"},
		},
	},
	{
		ID:        "storage_controller_nvme_percentage_used",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_wear_used",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.PercentageUsed"},
		},
	},
	{
		ID:        "storage_controller_nvme_composite_temperature_celsius",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_temperature_composite",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.CompositeTemperatureCelsius"},
		},
	},
	{
		ID:        "storage_controller_nvme_data_units_read",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_io_read",
		Scale:     valueScale{512000, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.DataUnitsRead"},
		},
	},
	{
		ID:        "storage_controller_nvme_data_units_written",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_io_written",
		Scale:     valueScale{512000, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.DataUnitsWritten"},
		},
	},
	{
		ID:        "storage_controller_nvme_host_read_commands",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_commands_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.HostReadCommands"},
		},
	},
	{
		ID:        "storage_controller_nvme_host_write_commands",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_commands_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.HostWriteCommands"},
		},
	},
	{
		ID:        "storage_controller_nvme_controller_busy_time_minutes",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_busy_time_busy",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.ControllerBusyTimeMinutes"},
		},
	},
	{
		ID:        "storage_controller_nvme_media_and_data_integrity_errors",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_error_rate_media_integrity",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.MediaAndDataIntegrityErrors"},
		},
	},
	{
		ID:        "storage_controller_nvme_number_of_error_information_log_entries",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_error_rate_error_log_entries",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.NumberOfErrorInformationLogEntries"},
		},
	},
	{
		ID:        "storage_controller_nvme_unsafe_shutdowns",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_event_rate_unsafe_shutdowns",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.UnsafeShutdowns"},
		},
	},
	{
		ID:        "storage_controller_nvme_power_cycles",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_event_rate_power_cycles",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.PowerCycles"},
		},
	},
	{
		ID:        "storage_controller_nvme_warning_composite_temp_time_minutes",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_time_warning",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.WarningCompositeTempTimeMinutes"},
		},
	},
	{
		ID:        "storage_controller_nvme_critical_composite_temp_time_minutes",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_time_critical",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.CriticalCompositeTempTimeMinutes"},
		},
	},
	{
		ID:        "storage_controller_nvme_thermal_mgmt_temp1_total_time_seconds",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_time_management_1",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.ThermalMgmtTemp1TotalTimeSeconds"},
		},
	},
	{
		ID:        "storage_controller_nvme_thermal_mgmt_temp2_total_time_seconds",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_time_management_2",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.ThermalMgmtTemp2TotalTimeSeconds"},
		},
	},
	{
		ID:        "storage_controller_nvme_thermal_mgmt_temp1_transition_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_transition_rate_management_1",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.ThermalMgmtTemp1TransitionCount"},
		},
	},
	{
		ID:        "storage_controller_nvme_thermal_mgmt_temp2_transition_count",
		Kind:      "storage_controller",
		Metric:    "redfish_storage_controller_nvme_thermal_transition_rate_management_2",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "storage_controller_metrics", Path: "NVMeSMART.ThermalMgmtTemp2TransitionCount"},
		},
	},
	{
		ID:        "drive_nvme_available_spare_percent",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_spare_available",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.AvailableSparePercent"},
		},
	},
	{
		ID:        "drive_nvme_percentage_used",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_wear_used",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.PercentageUsed"},
		},
	},
	{
		ID:        "drive_nvme_composite_temperature_celsius",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_temperature_composite",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.CompositeTemperatureCelsius"},
		},
	},
	{
		ID:        "drive_nvme_data_units_read",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_io_read",
		Scale:     valueScale{512000, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.DataUnitsRead"},
		},
	},
	{
		ID:        "drive_nvme_data_units_written",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_io_written",
		Scale:     valueScale{512000, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.DataUnitsWritten"},
		},
	},
	{
		ID:        "drive_nvme_host_read_commands",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_commands_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.HostReadCommands"},
		},
	},
	{
		ID:        "drive_nvme_host_write_commands",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_commands_written",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.HostWriteCommands"},
		},
	},
	{
		ID:        "drive_nvme_controller_busy_time_minutes",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_busy_time_busy",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.ControllerBusyTimeMinutes"},
		},
	},
	{
		ID:        "drive_nvme_media_and_data_integrity_errors",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_error_rate_media_integrity",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.MediaAndDataIntegrityErrors"},
		},
	},
	{
		ID:        "drive_nvme_number_of_error_information_log_entries",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_error_rate_error_log_entries",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.NumberOfErrorInformationLogEntries"},
		},
	},
	{
		ID:        "drive_nvme_unsafe_shutdowns",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_event_rate_unsafe_shutdowns",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.UnsafeShutdowns"},
		},
	},
	{
		ID:        "drive_nvme_power_cycles",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_event_rate_power_cycles",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.PowerCycles"},
		},
	},
	{
		ID:        "drive_nvme_warning_composite_temp_time_minutes",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_time_warning",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.WarningCompositeTempTimeMinutes"},
		},
	},
	{
		ID:        "drive_nvme_critical_composite_temp_time_minutes",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_time_critical",
		Scale:     valueScale{60, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.CriticalCompositeTempTimeMinutes"},
		},
	},
	{
		ID:        "drive_nvme_thermal_mgmt_temp1_total_time_seconds",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_time_management_1",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.ThermalMgmtTemp1TotalTimeSeconds"},
		},
	},
	{
		ID:        "drive_nvme_thermal_mgmt_temp2_total_time_seconds",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_time_management_2",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.ThermalMgmtTemp2TotalTimeSeconds"},
		},
	},
	{
		ID:        "drive_nvme_thermal_mgmt_temp1_transition_count",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_transition_rate_management_1",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.ThermalMgmtTemp1TransitionCount"},
		},
	},
	{
		ID:        "drive_nvme_thermal_mgmt_temp2_transition_count",
		Kind:      "drive",
		Metric:    "redfish_drive_nvme_thermal_transition_rate_management_2",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "drive_metrics", Path: "NVMeSMART.ThermalMgmtTemp2TransitionCount"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_rxunicast_frames",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_frames_received_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "RXUnicastFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_rxmulticast_frames",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_frames_received_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "RXMulticastFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_txunicast_frames",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_frames_sent_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "TXUnicastFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_txmulticast_frames",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_frames_sent_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "TXMulticastFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_ncsirxbytes",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_ncsi_traffic_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "NCSIRXBytes"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_ncsitxbytes",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_ncsi_traffic_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "NCSITXBytes"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_ncsirxframes",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_ncsi_frames_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "NCSIRXFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_ncsitxframes",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_ncsi_frames_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "NCSITXFrames"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_cpucore_percent",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_cpu_utilization",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "CPUCorePercent"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_host_bus_rxpercent",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_host_bus_utilization_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "HostBusRXPercent"},
		},
	},
	{
		ID:        "network_adapter_network_adapter_metrics_host_bus_txpercent",
		Kind:      "network_adapter",
		Metric:    "redfish_network_adapter_host_bus_utilization_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_adapter_metrics", Path: "HostBusTXPercent"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxframes",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txframes",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxunicast_frames",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_received_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXUnicastFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txunicast_frames",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_sent_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXUnicastFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxmulticast_frames",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_received_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXMulticastFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txmulticast_frames",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_frames_sent_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXMulticastFrames"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxavg_queue_depth_percent",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_queue_depth_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXAvgQueueDepthPercent"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txavg_queue_depth_percent",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_queue_depth_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXAvgQueueDepthPercent"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_rxqueues_full",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_queue_full_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "RXQueuesFull"},
		},
	},
	{
		ID:        "network_device_function_network_device_function_metrics_txqueues_full",
		Kind:      "network_device_function",
		Metric:    "redfish_network_device_function_queue_full_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "network_device_function_metrics", Path: "TXQueuesFull"},
		},
	},
	{
		ID:        "port_networking_rxframes",
		Kind:      "port",
		Metric:    "redfish_port_frames_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXFrames"},
		},
	},
	{
		ID:        "port_networking_txframes",
		Kind:      "port",
		Metric:    "redfish_port_frames_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXFrames"},
		},
	},
	{
		ID:        "port_networking_rxunicast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_received_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXUnicastFrames"},
		},
	},
	{
		ID:        "port_networking_txunicast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_sent_unicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXUnicastFrames"},
		},
	},
	{
		ID:        "port_networking_rxmulticast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_received_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXMulticastFrames"},
		},
	},
	{
		ID:        "port_networking_txmulticast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_sent_multicast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXMulticastFrames"},
		},
	},
	{
		ID:        "port_networking_rxbroadcast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_received_broadcast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXBroadcastFrames"},
		},
	},
	{
		ID:        "port_networking_txbroadcast_frames",
		Kind:      "port",
		Metric:    "redfish_port_frames_sent_broadcast",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXBroadcastFrames"},
		},
	},
	{
		ID:        "port_networking_rdmarxbytes",
		Kind:      "port",
		Metric:    "redfish_port_rdma_traffic_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RDMARXBytes"},
		},
	},
	{
		ID:        "port_networking_rdmatxbytes",
		Kind:      "port",
		Metric:    "redfish_port_rdma_traffic_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RDMATXBytes"},
		},
	},
	{
		ID:        "port_networking_rdmaprotection_errors",
		Kind:      "port",
		Metric:    "redfish_port_rdma_error_rate_protection",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RDMAProtectionErrors"},
		},
	},
	{
		ID:        "port_networking_rdmaprotocol_errors",
		Kind:      "port",
		Metric:    "redfish_port_rdma_error_rate_protocol",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RDMAProtocolErrors"},
		},
	},
	{
		ID:        "port_networking_rxdiscards",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_discards",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXDiscards"},
		},
	},
	{
		ID:        "port_networking_txdiscards",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_sent_discards",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXDiscards"},
		},
	},
	{
		ID:        "port_networking_rxfcserrors",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_fcs",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXFCSErrors"},
		},
	},
	{
		ID:        "port_networking_rxfalse_carrier_errors",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_false_carrier",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXFalseCarrierErrors"},
		},
	},
	{
		ID:        "port_networking_rxframe_alignment_errors",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_alignment",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXFrameAlignmentErrors"},
		},
	},
	{
		ID:        "port_networking_rxoversize_frames",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_oversize",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXOversizeFrames"},
		},
	},
	{
		ID:        "port_networking_rxundersize_frames",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_received_undersize",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.RXUndersizeFrames"},
		},
	},
	{
		ID:        "port_networking_txexcessive_collisions",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_sent_excessive_collisions",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXExcessiveCollisions"},
		},
	},
	{
		ID:        "port_networking_txlate_collisions",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_sent_late_collisions",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXLateCollisions"},
		},
	},
	{
		ID:        "port_networking_txmultiple_collisions",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_sent_multiple_collisions",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXMultipleCollisions"},
		},
	},
	{
		ID:        "port_networking_txsingle_collisions",
		Kind:      "port",
		Metric:    "redfish_port_network_error_rate_sent_single_collisions",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "Networking.TXSingleCollisions"},
		},
	},
	{
		ID:        "processor_pcie_errors_bad_dllpcount",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_bad_dllp",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.BadDLLPCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_bad_tlpcount",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_bad_tlp",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.BadTLPCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_correctable_error_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_correctable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.CorrectableErrorCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_fatal_error_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_fatal",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.FatalErrorCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_flow_control_timeout_errors",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_flow_control_timeout",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.FlowControlTimeoutErrors"},
		},
	},
	{
		ID:        "processor_pcie_errors_l0_to_recovery_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_l0_to_recovery",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.L0ToRecoveryCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_nakreceived_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_nak_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.NAKReceivedCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_naksent_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_nak_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.NAKSentCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_non_fatal_error_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_non_fatal",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.NonFatalErrorCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_replay_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_replay",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.ReplayCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_replay_rollover_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_replay_rollover",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.ReplayRolloverCount"},
		},
	},
	{
		ID:        "processor_pcie_errors_unsupported_request_count",
		Kind:      "processor",
		Metric:    "redfish_processor_pcie_error_rate_unsupported_request",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "processor_metrics", Path: "PCIeErrors.UnsupportedRequestCount"},
		},
	},
	{
		ID:        "port_pcie_errors_bad_dllpcount",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_bad_dllp",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.BadDLLPCount"},
		},
	},
	{
		ID:        "port_pcie_errors_bad_tlpcount",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_bad_tlp",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.BadTLPCount"},
		},
	},
	{
		ID:        "port_pcie_errors_correctable_error_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_correctable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.CorrectableErrorCount"},
		},
	},
	{
		ID:        "port_pcie_errors_fatal_error_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_fatal",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.FatalErrorCount"},
		},
	},
	{
		ID:        "port_pcie_errors_flow_control_timeout_errors",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_flow_control_timeout",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.FlowControlTimeoutErrors"},
		},
	},
	{
		ID:        "port_pcie_errors_l0_to_recovery_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_l0_to_recovery",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.L0ToRecoveryCount"},
		},
	},
	{
		ID:        "port_pcie_errors_nakreceived_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_nak_received",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.NAKReceivedCount"},
		},
	},
	{
		ID:        "port_pcie_errors_naksent_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_nak_sent",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.NAKSentCount"},
		},
	},
	{
		ID:        "port_pcie_errors_non_fatal_error_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_non_fatal",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.NonFatalErrorCount"},
		},
	},
	{
		ID:        "port_pcie_errors_replay_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_replay",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.ReplayCount"},
		},
	},
	{
		ID:        "port_pcie_errors_replay_rollover_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_replay_rollover",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.ReplayRolloverCount"},
		},
	},
	{
		ID:        "port_pcie_errors_unsupported_request_count",
		Kind:      "port",
		Metric:    "redfish_port_pcie_error_rate_unsupported_request",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeErrors.UnsupportedRequestCount"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_completion_tlpbytes",
		Kind:      "port",
		Metric:    "redfish_port_pcie_traffic_completion",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundCompletionTLPBytes"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_read_tlpbytes",
		Kind:      "port",
		Metric:    "redfish_port_pcie_traffic_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundReadTLPBytes"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_write_tlpbytes",
		Kind:      "port",
		Metric:    "redfish_port_pcie_traffic_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundWriteTLPBytes"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_completion_tlpcount",
		Kind:      "port",
		Metric:    "redfish_port_pcie_tlp_rate_completion",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundCompletionTLPCount"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_read_tlpcount",
		Kind:      "port",
		Metric:    "redfish_port_pcie_tlp_rate_read",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundReadTLPCount"},
		},
	},
	{
		ID:        "port_pcie_metrics_outbound_write_tlpcount",
		Kind:      "port",
		Metric:    "redfish_port_pcie_tlp_rate_write",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.OutboundWriteTLPCount"},
		},
	},
	{
		ID:        "port_pcie_metrics_completion_credit_exhaustion_drops",
		Kind:      "port",
		Metric:    "redfish_port_pcie_drop_rate_completion_credit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.CompletionCreditExhaustionDrops"},
		},
	},
	{
		ID:        "port_pcie_metrics_npcredit_exhaustion_drops",
		Kind:      "port",
		Metric:    "redfish_port_pcie_drop_rate_non_posted_credit",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.NPCreditExhaustionDrops"},
		},
	},
	{
		ID:        "port_pcie_metrics_tag_unavailability_drops",
		Kind:      "port",
		Metric:    "redfish_port_pcie_drop_rate_tag_unavailable",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmRate,
		Candidates: []scalarSource{
			{Document: "port_metrics", Path: "PCIeMetrics.TagUnavailabilityDrops"},
		},
	},
	{
		ID:        "redundancy_members_active",
		Kind:      "redundancy",
		Metric:    "redfish_redundancy_members_active",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "ActiveRedundancySet.@odata.count"},
			{Document: "", Path: "ActiveRedundancyGroup.@odata.count"},
		},
	},
	{
		ID:        "redundancy_members_total",
		Kind:      "redundancy",
		Metric:    "redfish_redundancy_members_total",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmAbsolute,
		Candidates: []scalarSource{
			{Document: "", Path: "RedundancySet.@odata.count"},
			{Document: "", Path: "RedundancyGroup.@odata.count"},
		},
	},
	{
		ID:        "heater_heater_metrics_pre_power_on_heating_time_seconds",
		Kind:      "heater",
		Metric:    "redfish_heater_heating_time_pre_power_on",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "heater_metrics", Path: "PrePowerOnHeatingTimeSeconds"},
		},
	},
	{
		ID:        "heater_heater_metrics_runtime_heating_time_seconds",
		Kind:      "heater",
		Metric:    "redfish_heater_heating_time_runtime",
		Scale:     valueScale{1, 1},
		Algorithm: algorithmDurationPercent,
		Candidates: []scalarSource{
			{Document: "heater_metrics", Path: "RuntimeHeatingTimeSeconds"},
		},
	},
}

// Index once; collection only examines fields belonging to the current resource.
var scalarFieldsByKind = func() map[string][]sourceField {
	result := make(map[string][]sourceField)
	for _, field := range scalarFields {
		result[field.Kind] = append(result[field.Kind], field)
	}
	return result
}()
