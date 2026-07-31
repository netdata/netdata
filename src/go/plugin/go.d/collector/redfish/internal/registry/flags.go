// SPDX-License-Identifier: GPL-3.0-or-later

package registry

var flagSetSpecs = []FlagSetSpec{
	{
		Order: 0, Kind: "memory", Document: "memory_metrics",
		Metric: "memory_health_flags", Context: "redfish.memory.health_flags",
		Title: "Memory Health Flags", ComponentClass: "replaceable",
		AggregateKinds: []Kind{"system", "chassis"},
		Members: []FlagMemberSpec{
			{Path: "HealthData.DataLossDetected", Role: "data_loss", Column: "memory_data_loss_detected"},
			{Path: "HealthData.LastShutdownSuccess", Role: "last_shutdown_failed", Column: "memory_last_shutdown_failed", Invert: true},
			{Path: "HealthData.PerformanceDegraded", Role: "performance_degraded", Column: "memory_performance_degraded"},
			{Path: "HealthData.AlarmTrips.AddressParityError", Role: "address_parity", Column: "memory_alarm_address_parity"},
			{Path: "HealthData.AlarmTrips.CorrectableECCError", Role: "correctable_ecc", Column: "memory_alarm_correctable_ecc"},
			{Path: "HealthData.AlarmTrips.SpareBlock", Role: "spare_block", Column: "memory_alarm_spare_block"},
			{Path: "HealthData.AlarmTrips.Temperature", Role: "temperature", Column: "memory_alarm_temperature"},
			{Path: "HealthData.AlarmTrips.UncorrectableECCError", Role: "uncorrectable_ecc", Column: "memory_alarm_uncorrectable_ecc"},
		},
	},
	{
		Order: 1, Kind: "network_device_function", Document: "network_device_function_metrics",
		Metric:  "network_device_function_queue_empty_state",
		Context: "redfish.network_device_function.queue_empty_state",
		Title:   "Network Device Function Empty Queue State", ComponentClass: "network",
		AggregateKinds: []Kind{"network_adapter", "network_interface"},
		Members: []FlagMemberSpec{
			{Path: "RXQueuesEmpty", Role: "received", Column: "network_device_function_rx_queues_empty"},
			{Path: "TXQueuesEmpty", Role: "sent", Column: "network_device_function_tx_queues_empty"},
		},
	},
	{
		Order: 2, Kind: "storage_controller", Document: "storage_controller_metrics",
		Metric:  "storage_controller_nvme_critical_warnings",
		Context: "redfish.storage_controller.nvme.critical_warnings",
		Title:   "Storage Controller NVMe Critical Warnings", ComponentClass: "replaceable",
		AggregateKinds: []Kind{"storage"},
		Members:        nvmeWarningMembers("storage_controller"),
	},
	{
		Order: 3, Kind: "drive", Document: "drive_metrics",
		Metric:  "drive_nvme_critical_warnings",
		Context: "redfish.drive.nvme.critical_warnings",
		Title:   "Drive NVMe Critical Warnings", ComponentClass: "replaceable",
		AggregateKinds: []Kind{"storage", "chassis"},
		Members:        nvmeWarningMembers("drive"),
	},
}

func nvmeWarningMembers(prefix string) []FlagMemberSpec {
	return []FlagMemberSpec{
		{Path: "NVMeSMART.CriticalWarnings.MediaInReadOnly", Role: "media_read_only", Column: prefix + "_nvme_warning_media_read_only"},
		{Path: "NVMeSMART.CriticalWarnings.OverallSubsystemDegraded", Role: "subsystem_degraded", Column: prefix + "_nvme_warning_subsystem_degraded"},
		{Path: "NVMeSMART.CriticalWarnings.PMRUnreliable", Role: "pmr_unreliable", Column: prefix + "_nvme_warning_pmr_unreliable"},
		{Path: "NVMeSMART.CriticalWarnings.PowerBackupFailed", Role: "power_backup_failed", Column: prefix + "_nvme_warning_power_backup_failed"},
		{Path: "NVMeSMART.CriticalWarnings.SpareCapacityWornOut", Role: "spare_capacity_worn_out", Column: prefix + "_nvme_warning_spare_capacity_worn_out"},
		{Path: "NVMeSMART.EGCriticalWarningSummary.NamespacesInReadOnlyMode", Role: "namespaces_read_only", Column: prefix + "_nvme_warning_namespaces_read_only"},
		{Path: "NVMeSMART.EGCriticalWarningSummary.ReliabilityDegraded", Role: "reliability_degraded", Column: prefix + "_nvme_warning_reliability_degraded"},
		{Path: "NVMeSMART.EGCriticalWarningSummary.SpareCapacityUnderThreshold", Role: "spare_capacity_under_threshold", Column: prefix + "_nvme_warning_spare_capacity_under_threshold"},
	}
}
