// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

type statusDescriptor struct{ Status, PowerState, FailurePredicted bool }
type flagMember struct {
	Path, Role string
	Invert     bool
}
type flagSet struct {
	Kind, Document, Metric string
	Members                []flagMember
}

var sourceStatusByKind = map[string]statusDescriptor{
	"service":                 {false, false, false},
	"system":                  {true, true, false},
	"chassis":                 {true, true, false},
	"manager":                 {true, true, false},
	"processor":               {true, true, false},
	"processor_core":          {false, false, false},
	"memory":                  {true, false, false},
	"storage":                 {true, false, false},
	"storage_controller":      {true, false, false},
	"drive":                   {true, false, true},
	"volume":                  {true, false, false},
	"network_adapter":         {true, false, false},
	"network_device_function": {true, false, false},
	"ethernet_interface":      {true, false, false},
	"network_interface":       {true, false, false},
	"network_port":            {true, false, false},
	"port":                    {true, false, false},
	"pcie_device":             {true, false, false},
	"pcie_function":           {true, false, false},
	"fan":                     {true, false, false},
	"pump":                    {true, false, false},
	"power_supply":            {true, false, false},
	"battery":                 {true, false, false},
	"sensor":                  {true, false, false},
	"redundancy":              {true, false, false},
	"thermal_subsystem":       {true, false, false},
	"power_subsystem":         {true, false, false},
	"coolant_connector":       {true, false, false},
	"filter":                  {true, false, false},
	"heater":                  {true, false, false},
	"leak_detection":          {true, false, false},
	"leak_detector_group":     {true, false, false},
	"leak_detector":           {true, false, false},
	"control":                 {true, false, false},
	"firmware":                {true, false, false},
	"software":                {true, false, false},
	"assembly":                {true, false, false},
}

var additionalStateSources = []stateSource{
	{
		Kind:   "chassis",
		Path:   "PhysicalSecurity.IntrusionSensor",
		Metric: "chassis_intrusion_state",
		States: []string{"normal", "hardware_intrusion", "tampering_detected", "unknown"},
	},
	{
		Kind:         "processor",
		Path:         "Throttled",
		Metric:       "processor_throttling_state",
		BooleanFalse: "clear",
		BooleanTrue:  "throttled",
		States:       []string{"clear", "throttled", "unknown"},
	},
	{
		Kind:   "drive",
		Path:   "StatusIndicator",
		Metric: "drive_status_indicator",
		States: []string{
			"ok",
			"fail",
			"rebuild",
			"predictive_failure_analysis",
			"hotspare",
			"in_a_critical_array",
			"in_a_failed_array",
			"unknown",
		},
	},
	{
		Kind:   "volume",
		Path:   "WriteCacheState",
		Metric: "volume_write_cache_state",
		States: []string{"unprotected", "protected", "degraded", "unknown"},
	},
	{
		Kind:   "ethernet_interface",
		Path:   "LinkStatus",
		Metric: "ethernet_interface_link_status",
		States: []string{"link_up", "no_link", "link_down", "unknown"},
	},
	{
		Kind:   "network_port",
		Path:   "LinkStatus",
		Metric: "network_port_link_status",
		States: []string{"down", "up", "starting", "training", "unknown"},
	},
	{
		Kind:         "network_port",
		Path:         "SignalDetected",
		Metric:       "network_port_signal_detected",
		BooleanFalse: "clear",
		BooleanTrue:  "detected",
		States:       []string{"clear", "detected", "unknown"},
	},
	{Kind: "port", Path: "LinkState", Metric: "port_link_state", States: []string{"enabled", "disabled", "unknown"}},
	{
		Kind:   "port",
		Path:   "LinkStatus",
		Metric: "port_link_status",
		States: []string{"link_up", "starting", "training", "link_down", "no_link", "unknown"},
	},
	{
		Kind:         "port",
		Path:         "SignalDetected",
		Metric:       "port_signal_detected",
		BooleanFalse: "clear",
		BooleanTrue:  "detected",
		States:       []string{"clear", "detected", "unknown"},
	},
	{
		Kind:   "power_supply",
		Path:   "LineInputStatus",
		Metric: "power_supply_line_input_status",
		States: []string{"normal", "loss_of_input", "out_of_range", "unknown"},
	},
	{
		Kind:   "battery",
		Path:   "ChargeState",
		Metric: "battery_charge_state",
		States: []string{"idle", "charging", "discharging", "unknown"},
	},
	{
		Kind:         "redundancy",
		Path:         "RedundancyType",
		FallbackPath: "Mode",
		Metric:       "redundancy_mode",
		States:       []string{"failover", "n_plus_m", "sharing", "sparing", "not_redundant", "unknown"},
	},
	{
		Kind:         "redundancy",
		Path:         "RedundancyEnabled",
		Metric:       "redundancy_enabled",
		BooleanFalse: "disabled",
		BooleanTrue:  "enabled",
		States:       []string{"disabled", "enabled", "unknown"},
	},
	{
		Kind:   "leak_detector",
		Path:   "DetectorState",
		Metric: "leak_detector_detector_state",
		States: []string{"ok", "warning", "critical", "unavailable", "absent", "unknown"},
	},
	{
		Kind:   "system",
		Path:   "ProcessorSummary.Status.Health",
		Metric: "system_processor_summary_health",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "system",
		Path:   "ProcessorSummary.Status.HealthRollup",
		Metric: "system_processor_summary_health_rollup",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "system",
		Path:   "ProcessorSummary.Status.State",
		Metric: "system_processor_summary_state",
		States: []string{
			"enabled",
			"disabled",
			"standby_offline",
			"standby_spare",
			"in_test",
			"starting",
			"absent",
			"unavailable_offline",
			"deferring",
			"quiesced",
			"updating",
			"qualified",
			"degraded",
			"unknown",
		},
	},
	{
		Kind:   "system",
		Path:   "MemorySummary.Status.Health",
		Metric: "system_memory_summary_health",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "system",
		Path:   "MemorySummary.Status.HealthRollup",
		Metric: "system_memory_summary_health_rollup",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "system",
		Path:   "MemorySummary.Status.State",
		Metric: "system_memory_summary_state",
		States: []string{
			"enabled",
			"disabled",
			"standby_offline",
			"standby_spare",
			"in_test",
			"starting",
			"absent",
			"unavailable_offline",
			"deferring",
			"quiesced",
			"updating",
			"qualified",
			"degraded",
			"unknown",
		},
	},
	{
		Kind:   "storage_controller",
		Path:   "CacheSummary.Status.Health",
		Metric: "storage_controller_cache_health",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "storage_controller",
		Path:   "CacheSummary.Status.HealthRollup",
		Metric: "storage_controller_cache_health_rollup",
		States: []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:   "storage_controller",
		Path:   "CacheSummary.Status.State",
		Metric: "storage_controller_cache_state",
		States: []string{
			"enabled",
			"disabled",
			"standby_offline",
			"standby_spare",
			"in_test",
			"starting",
			"absent",
			"unavailable_offline",
			"deferring",
			"quiesced",
			"updating",
			"qualified",
			"degraded",
			"unknown",
		},
	},
	{
		Kind:     "power_supply",
		Document: "power_supply_metrics",
		Path:     "Status.Health",
		Metric:   "power_supply_metrics_health",
		States:   []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:     "power_supply",
		Document: "power_supply_metrics",
		Path:     "Status.HealthRollup",
		Metric:   "power_supply_metrics_health_rollup",
		States:   []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:     "power_supply",
		Document: "power_supply_metrics",
		Path:     "Status.State",
		Metric:   "power_supply_metrics_state",
		States: []string{
			"enabled",
			"disabled",
			"standby_offline",
			"standby_spare",
			"in_test",
			"starting",
			"absent",
			"unavailable_offline",
			"deferring",
			"quiesced",
			"updating",
			"qualified",
			"degraded",
			"unknown",
		},
	},
	{
		Kind:     "battery",
		Document: "battery_metrics",
		Path:     "Status.Health",
		Metric:   "battery_metrics_health",
		States:   []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:     "battery",
		Document: "battery_metrics",
		Path:     "Status.HealthRollup",
		Metric:   "battery_metrics_health_rollup",
		States:   []string{"ok", "warning", "critical", "unknown"},
	},
	{
		Kind:     "battery",
		Document: "battery_metrics",
		Path:     "Status.State",
		Metric:   "battery_metrics_state",
		States: []string{
			"enabled",
			"disabled",
			"standby_offline",
			"standby_spare",
			"in_test",
			"starting",
			"absent",
			"unavailable_offline",
			"deferring",
			"quiesced",
			"updating",
			"qualified",
			"degraded",
			"unknown",
		},
	},
}

var sourceFlagSets = []flagSet{
	{Kind: "memory", Document: "memory_metrics", Metric: "memory_health_flags", Members: []flagMember{
		{"HealthData.DataLossDetected", "data_loss", false},
		{"HealthData.LastShutdownSuccess", "last_shutdown_failed", true},
		{"HealthData.PerformanceDegraded", "performance_degraded", false},
		{"HealthData.AlarmTrips.AddressParityError", "address_parity", false},
		{"HealthData.AlarmTrips.CorrectableECCError", "correctable_ecc", false},
		{"HealthData.AlarmTrips.SpareBlock", "spare_block", false},
		{"HealthData.AlarmTrips.Temperature", "temperature", false},
		{"HealthData.AlarmTrips.UncorrectableECCError", "uncorrectable_ecc", false},
	}},
	{
		Kind:     "network_device_function",
		Document: "network_device_function_metrics",
		Metric:   "network_device_function_queue_empty_state",
		Members: []flagMember{
			{"RXQueuesEmpty", "received", false},
			{"TXQueuesEmpty", "sent", false},
		},
	},
	{
		Kind:     "storage_controller",
		Document: "storage_controller_metrics",
		Metric:   "storage_controller_nvme_critical_warnings",
		Members: []flagMember{
			{"NVMeSMART.CriticalWarnings.MediaInReadOnly", "media_read_only", false},
			{"NVMeSMART.CriticalWarnings.OverallSubsystemDegraded", "subsystem_degraded", false},
			{"NVMeSMART.CriticalWarnings.PMRUnreliable", "pmr_unreliable", false},
			{"NVMeSMART.CriticalWarnings.PowerBackupFailed", "power_backup_failed", false},
			{"NVMeSMART.CriticalWarnings.SpareCapacityWornOut", "spare_capacity_worn_out", false},
			{"NVMeSMART.EGCriticalWarningSummary.NamespacesInReadOnlyMode", "namespaces_read_only", false},
			{"NVMeSMART.EGCriticalWarningSummary.ReliabilityDegraded", "reliability_degraded", false},
			{"NVMeSMART.EGCriticalWarningSummary.SpareCapacityUnderThreshold", "spare_capacity_under_threshold", false},
		},
	},
	{Kind: "drive", Document: "drive_metrics", Metric: "drive_nvme_critical_warnings", Members: []flagMember{
		{"NVMeSMART.CriticalWarnings.MediaInReadOnly", "media_read_only", false},
		{"NVMeSMART.CriticalWarnings.OverallSubsystemDegraded", "subsystem_degraded", false},
		{"NVMeSMART.CriticalWarnings.PMRUnreliable", "pmr_unreliable", false},
		{"NVMeSMART.CriticalWarnings.PowerBackupFailed", "power_backup_failed", false},
		{"NVMeSMART.CriticalWarnings.SpareCapacityWornOut", "spare_capacity_worn_out", false},
		{"NVMeSMART.EGCriticalWarningSummary.NamespacesInReadOnlyMode", "namespaces_read_only", false},
		{"NVMeSMART.EGCriticalWarningSummary.ReliabilityDegraded", "reliability_degraded", false},
		{"NVMeSMART.EGCriticalWarningSummary.SpareCapacityUnderThreshold", "spare_capacity_under_threshold", false},
	}},
}

var (
	healthStates   = []string{"ok", "warning", "critical", "unknown"}
	resourceStates = []string{
		"enabled", "disabled", "standby_offline", "standby_spare", "in_test", "starting",
		"absent", "unavailable_offline", "deferring", "quiesced", "updating", "qualified",
		"degraded", "unknown",
	}
	powerStates       = []string{"on", "off", "powering_on", "powering_off", "paused", "unknown"}
	failureStates     = []string{"clear", "predicted", "unknown"}
	acquisitionStates = []string{"readable", "unreadable", "unknown"}
	alarmStates       = []string{"clear", "warning", "critical"}
)

type stateSource struct {
	Kind         string
	Document     string
	Path         string
	FallbackPath string
	Metric       string
	States       []string
	BooleanFalse string
	BooleanTrue  string
}
