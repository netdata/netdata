// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type metricEntity string

const (
	entityGPU      metricEntity = "gpu"
	entityMIG      metricEntity = "mig"
	entityNVLink   metricEntity = "nvlink"
	entityNVSwitch metricEntity = "nvswitch"
	entityCPU      metricEntity = "cpu"
	entityCPUCore  metricEntity = "cpu_core"
	entityExporter metricEntity = "exporter"
)

type sampleKind uint8

const (
	sampleGauge sampleKind = iota
	sampleCounter
	sampleUnsupported
)

type contextSpec struct {
	ID       string
	Title    string
	Units    string
	Family   string
	Type     collectorapi.ChartType
	Priority int
}

type metricSpec struct {
	Context contextSpec
	DimName string
	Scale   float64
}

type groupSpec struct {
	Suffix string
	Title  string
	Units  string
	Family string
	Type   collectorapi.ChartType
}

var groupCatalog = []groupSpec{
	{Suffix: "compute.utilization", Title: "Compute Utilization", Units: "percentage", Family: "compute", Type: collectorapi.Line},
	{Suffix: "compute.activity", Title: "Compute Pipeline Activity", Units: "percentage", Family: "compute", Type: collectorapi.Line},
	{Suffix: "compute.tensor.activity", Title: "Tensor Core Activity by Precision", Units: "percentage", Family: "compute", Type: collectorapi.Line},
	{Suffix: "compute.media.activity", Title: "Media Engine Activity", Units: "percentage", Family: "compute", Type: collectorapi.Line},
	{Suffix: "compute.cache.activity", Title: "Memory Cache Hit/Miss", Units: "events/s", Family: "compute", Type: collectorapi.Line},
	{Suffix: "memory.utilization", Title: "Memory Utilization", Units: "percentage", Family: "memory", Type: collectorapi.Line},
	{Suffix: "memory.usage", Title: "Memory Usage", Units: "bytes", Family: "memory", Type: collectorapi.Stacked},
	{Suffix: "memory.capacity", Title: "Memory Capacity", Units: "bytes", Family: "memory", Type: collectorapi.Line},
	{Suffix: "memory.bar1_usage", Title: "BAR1 Memory Usage", Units: "bytes", Family: "memory", Type: collectorapi.Stacked},
	{Suffix: "memory.bar1_capacity", Title: "BAR1 Memory Capacity", Units: "bytes", Family: "memory", Type: collectorapi.Line},
	{Suffix: "memory.ecc_errors", Title: "ECC Errors", Units: "errors", Family: "memory", Type: collectorapi.Line},
	{Suffix: "memory.ecc_error_rate", Title: "ECC Error Rate", Units: "errors/s", Family: "memory", Type: collectorapi.Line},
	{Suffix: "memory.page_retirements", Title: "Retired Memory Pages", Units: "pages/s", Family: "memory", Type: collectorapi.Line},
	{Suffix: "reliability.row_remap_status", Title: "Row Remap Status", Units: "state", Family: "reliability", Type: collectorapi.Line},
	{Suffix: "reliability.row_remap_events", Title: "Row Remap Events", Units: "rows/s", Family: "reliability", Type: collectorapi.Line},
	{Suffix: "reliability.memory_health", Title: "Memory Health", Units: "state", Family: "reliability", Type: collectorapi.Line},
	{Suffix: "reliability.recovery_action", Title: "Recovery Action", Units: "state", Family: "reliability", Type: collectorapi.Line},
	{Suffix: "clock.frequency", Title: "Clock Frequency", Units: "MHz", Family: "clock", Type: collectorapi.Line},
	{Suffix: "throttle.reasons", Title: "Throttle Reasons", Units: "bitmask", Family: "throttle", Type: collectorapi.Line},
	{Suffix: "thermal.temperature", Title: "Temperature", Units: "Celsius", Family: "thermal", Type: collectorapi.Line},
	{Suffix: "thermal.fan_speed", Title: "Fan Speed", Units: "percentage", Family: "thermal", Type: collectorapi.Line},
	{Suffix: "power.usage", Title: "Power Usage", Units: "Watts", Family: "power", Type: collectorapi.Line},
	{Suffix: "power.energy", Title: "Energy Consumption Rate", Units: "mJ/s", Family: "power", Type: collectorapi.Line},
	{Suffix: "power.profiles", Title: "Power Profiles", Units: "state", Family: "power", Type: collectorapi.Line},
	{Suffix: "power.smoothing", Title: "Power Smoothing", Units: "value", Family: "power", Type: collectorapi.Line},
	{Suffix: "throttle.violations", Title: "Throttle Violation Duration", Units: "milliseconds/s", Family: "throttle", Type: collectorapi.Line},
	{Suffix: "interconnect.total.throughput", Title: "Interconnect Total Throughput", Units: "bytes/s", Family: "interconnect/overview", Type: collectorapi.Area},
	{Suffix: "interconnect.pcie.throughput", Title: "PCIe Throughput", Units: "bytes/s", Family: "interconnect/pcie", Type: collectorapi.Area},
	{Suffix: "interconnect.nvlink.throughput", Title: "NVLink Throughput", Units: "bytes/s", Family: "interconnect/nvlink", Type: collectorapi.Area},
	{Suffix: "interconnect.throughput", Title: "Interconnect Throughput", Units: "bytes/s", Family: "interconnect/overview", Type: collectorapi.Area},
	{Suffix: "interconnect.pcie.traffic", Title: "PCIe Traffic", Units: "events/s", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.traffic", Title: "NVLink Traffic", Units: "events/s", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.traffic", Title: "Interconnect Traffic", Units: "events/s", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.ber", Title: "PCIe Bit Error Rate", Units: "ratio", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.ber", Title: "NVLink Bit Error Rate", Units: "ratio", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.ber", Title: "Interconnect Bit Error Rate", Units: "ratio", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.link.generation", Title: "PCIe Link Generation", Units: "generation", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.link.width", Title: "PCIe Link Width", Units: "lanes", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.state", Title: "PCIe State", Units: "state", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.state", Title: "NVLink State", Units: "state", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.state", Title: "Interconnect State", Units: "state", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.congestion", Title: "NVLink Congestion", Units: "events/s", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.congestion", Title: "Interconnect Congestion", Units: "events/s", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.fabric", Title: "Fabric State", Units: "state", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.errors", Title: "PCIe Errors", Units: "errors", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.pcie.error_rate", Title: "PCIe Error Rate", Units: "errors/s", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.errors", Title: "NVLink Errors", Units: "errors", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvlink.error_rate", Title: "NVLink Error Rate", Units: "errors/s", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.errors", Title: "Interconnect Errors", Units: "errors", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.error_rate", Title: "Interconnect Error Rate", Units: "errors/s", Family: "interconnect/overview", Type: collectorapi.Line},
	{Suffix: "interconnect.connectx.status", Title: "ConnectX Status", Units: "state", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.connectx.link", Title: "ConnectX Link", Units: "value", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.connectx.temperature", Title: "ConnectX Temperature", Units: "Celsius", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.connectx.errors", Title: "ConnectX Errors", Units: "errors/s", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.status", Title: "NVSwitch Status", Units: "state", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.topology", Title: "NVSwitch Topology", Units: "value", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.throughput", Title: "NVSwitch Throughput", Units: "bytes/s", Family: "interconnect/nvlink", Type: collectorapi.Area},
	{Suffix: "interconnect.nvswitch.latency", Title: "NVSwitch Link Latency", Units: "events/s", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.errors", Title: "NVSwitch Errors", Units: "errors/s", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.temperature", Title: "NVSwitch Temperature", Units: "Celsius", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.power", Title: "NVSwitch Power", Units: "Watts", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.current", Title: "NVSwitch Current", Units: "value", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.nvswitch.voltage", Title: "NVSwitch Voltage", Units: "mV", Family: "interconnect/nvlink", Type: collectorapi.Line},
	{Suffix: "interconnect.connectx.error_status", Title: "ConnectX Error Status", Units: "state", Family: "interconnect/pcie", Type: collectorapi.Line},
	{Suffix: "reliability.xid", Title: "XID Errors", Units: "code", Family: "reliability", Type: collectorapi.Line},
	{Suffix: "health.status", Title: "Health Status", Units: "state", Family: "health", Type: collectorapi.Line},
	{Suffix: "state.performance", Title: "Performance State", Units: "state", Family: "state", Type: collectorapi.Line},
	{Suffix: "state.virtualization", Title: "Virtualization State", Units: "state", Family: "state", Type: collectorapi.Line},
	{Suffix: "state.configuration", Title: "Configuration State", Units: "state", Family: "state", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.license", Title: "vGPU License", Units: "state", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.type", Title: "vGPU Type", Units: "value", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.instance", Title: "vGPU Instance", Units: "value", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.vm", Title: "vGPU VM", Units: "value", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.memory", Title: "vGPU Memory", Units: "bytes", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.frame_rate", Title: "vGPU Frame Rate", Units: "fps", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.utilization", Title: "vGPU Utilization", Units: "percentage", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.sessions", Title: "vGPU Sessions", Units: "value", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "virtualization.vgpu.software", Title: "vGPU Software", Units: "value", Family: "virtualization", Type: collectorapi.Line},
	{Suffix: "workload.sessions", Title: "Workload Sessions", Units: "value", Family: "workload", Type: collectorapi.Line},
	{Suffix: "cpu.utilization", Title: "CPU Utilization", Units: "percentage", Family: "cpu", Type: collectorapi.Line},
	{Suffix: "cpu.temperature", Title: "CPU Temperature", Units: "Celsius", Family: "cpu", Type: collectorapi.Line},
	{Suffix: "cpu.power", Title: "CPU Power", Units: "Watts", Family: "cpu", Type: collectorapi.Line},
	{Suffix: "cpu.info", Title: "CPU Information", Units: "value", Family: "cpu", Type: collectorapi.Line},
	{Suffix: "diagnostics.status", Title: "Diagnostics Status", Units: "state", Family: "diagnostics", Type: collectorapi.Line},
	{Suffix: "diagnostics.results", Title: "Diagnostics Results", Units: "state", Family: "diagnostics", Type: collectorapi.Line},
	{Suffix: "inventory.identity", Title: "Inventory Identity", Units: "value", Family: "inventory", Type: collectorapi.Line},
	{Suffix: "inventory.software", Title: "Software and Firmware", Units: "value", Family: "inventory", Type: collectorapi.Line},
	{Suffix: "inventory.platform", Title: "Platform Inventory", Units: "value", Family: "inventory", Type: collectorapi.Line},
	{Suffix: "topology.affinity", Title: "Topology and Affinity", Units: "value", Family: "topology", Type: collectorapi.Line},
	{Suffix: "capability.support", Title: "Capability Support", Units: "state", Family: "capability", Type: collectorapi.Line},
	{Suffix: "internal.boundary", Title: "Internal Boundary Fields", Units: "state", Family: "internal", Type: collectorapi.Line},
	{Suffix: "state", Title: "Device State", Units: "state", Family: "state", Type: collectorapi.Line},
	{Suffix: "other.gauge", Title: "Other Metrics", Units: "value", Family: "other", Type: collectorapi.Line},
	{Suffix: "other.counter", Title: "Other Metric Rate", Units: "events/s", Family: "other", Type: collectorapi.Line},
}

var contextCatalog = buildContextCatalog()

func buildContextCatalog() map[string]contextSpec {
	entities := []metricEntity{entityGPU, entityMIG, entityNVLink, entityNVSwitch, entityCPU, entityCPUCore, entityExporter}
	catalog := make(map[string]contextSpec, len(entities)*len(groupCatalog))

	prio := collectorapi.Priority
	for _, entity := range entities {
		for _, group := range groupCatalog {
			ctx := fmt.Sprintf("dcgm.%s.%s", entity, group.Suffix)
			catalog[ctx] = contextSpec{
				ID:       ctx,
				Title:    fmt.Sprintf("%s %s", entityDisplayName(entity), group.Title),
				Units:    group.Units,
				Family:   fmt.Sprintf("%s %s", entityFamilyPrefix(entity), group.Family),
				Type:     group.Type,
				Priority: prio,
			}
			prio += 10
		}
	}

	return catalog
}

func entityDisplayName(entity metricEntity) string {
	switch entity {
	case entityGPU:
		return "GPU"
	case entityMIG:
		return "MIG"
	case entityNVLink:
		return "NVLink"
	case entityNVSwitch:
		return "NVSwitch"
	case entityCPU:
		return "CPU"
	case entityCPUCore:
		return "CPU Core"
	default:
		return "Exporter"
	}
}

func entityFamilyPrefix(entity metricEntity) string {
	switch entity {
	case entityCPUCore:
		return "cpu core"
	default:
		return string(entity)
	}
}

func classifyMetric(entity metricEntity, metricName, help string, typ sampleKind) metricSpec {
	group := classifyMetricGroup(entity, metricName, typ)
	ctxID := fmt.Sprintf("dcgm.%s.%s", entity, group)
	spec, ok := contextCatalog[ctxID]
	if !ok {
		fallback := "other.gauge"
		if typ == sampleCounter {
			fallback = "other.counter"
		}
		spec = contextCatalog[fmt.Sprintf("dcgm.%s.%s", entity, fallback)]
	}

	return metricSpec{
		Context: spec,
		DimName: metricDimensionName(metricName),
		Scale:   metricScale(metricName, help, spec),
	}
}

func classifyMetricGroup(entity metricEntity, metricName string, typ sampleKind) string {
	name := strings.ToUpper(metricName)

	switch {
	case strings.HasPrefix(name, "DCGM_FI_INTERNAL_FIELDS_"),
		strings.Contains(name, "FIRST_"),
		strings.Contains(name, "LAST_"):
		return "internal.boundary"
	case !strings.Contains(name, "VGPU_") && containsAny(name, "GPU_UTIL", "MEM_COPY_UTIL", "ENC_UTIL", "DEC_UTIL"):
		return "compute.utilization"
	case containsAny(name, "TENSOR_HMMA", "TENSOR_IMMA", "TENSOR_DFMA"):
		return "compute.tensor.activity"
	case containsAny(name, "NVDEC", "NVJPG", "NVOFA"):
		return "compute.media.activity"
	case containsAny(name, "HOSTMEM_CACHE", "PEERMEM_CACHE"):
		return "compute.cache.activity"
	case containsAny(name,
		"SM_ACTIVE",
		"SM_OCCUPANCY",
		"GR_ENGINE_ACTIVE",
		"PIPE_",
		"DRAM_ACTIVE",
		"TENSOR",
		"FP16",
		"FP32",
		"FP64",
		"INTEGER_ACTIVE",
	):
		return "compute.activity"
	case strings.Contains(name, "FB_USED_PERCENT"):
		return "memory.utilization"
	case strings.Contains(name, "BAR1_TOTAL"):
		return "memory.bar1_capacity"
	case strings.Contains(name, "BAR1"):
		return "memory.bar1_usage"
	case strings.Contains(name, "FB_TOTAL"):
		return "memory.capacity"
	case containsAny(name, "FB_FREE", "FB_USED", "FB_RESERVED", "FRAME_BUFFER"):
		return "memory.usage"
	case strings.Contains(name, "ECC_"):
		if typ == sampleCounter {
			return "memory.ecc_error_rate"
		}
		return "memory.ecc_errors"
	case strings.Contains(name, "RETIRED_"):
		return "memory.page_retirements"
	case strings.Contains(name, "ROW_REMAP_FAILURE"):
		return "reliability.row_remap_status"
	case strings.Contains(name, "ROW_REMAP_PENDING"):
		return "reliability.row_remap_status"
	case strings.Contains(name, "REMAPPED_ROWS"):
		return "reliability.row_remap_events"
	case containsAny(name, "BANKS_REMAP", "MEMORY_UNREPAIRABLE_FLAG", "THRESHOLD_SRM"):
		return "reliability.memory_health"
	case strings.Contains(name, "GET_GPU_RECOVERY_ACTION"):
		return "reliability.recovery_action"
	case containsAny(name, "HEALTH_STATUS", "P2P_STATUS", "CLOCK_EVENTS_COUNT", "IMEX_DOMAIN_STATUS", "IMEX_DAEMON_STATUS", "BIND_UNBIND_EVENT"):
		return "health.status"
	case strings.Contains(name, "CLOCKS_EVENT_REASONS"):
		return "throttle.reasons"
	case strings.Contains(name, "CLOCKS_EVENT_REASON"):
		return "throttle.violations"
	case strings.Contains(name, "CLOCK_THROTTLE_REASONS"):
		return "throttle.reasons"
	case strings.Contains(name, "PSTATE"):
		return "state.performance"
	case containsAny(name, "VIRTUAL_MODE", "MIG_MODE"):
		return "state.virtualization"
	case containsAny(name, "COMPUTE_MODE", "PERSISTENCE_MODE", "AUTOBOOST", "SYNC_BOOST"):
		return "state.configuration"
	case containsAny(name, "GPU_TEMP", "MEMORY_TEMP", "SLOWDOWN_TEMP", "MAX_OP_TEMP", "SHUTDOWN_TEMP", "TEMPERATURE"):
		return "thermal.temperature"
	case strings.Contains(name, "FAN_SPEED"):
		return "thermal.fan_speed"
	case strings.Contains(name, "TOTAL_ENERGY"):
		return "power.energy"
	case containsAny(name, "POWER_PROFILE_MASK"):
		return "power.profiles"
	case containsAny(name, "PWR_SMOOTHING"):
		return "power.smoothing"
	case containsAny(name, "POWER_USAGE", "POWER_MGMT_LIMIT", "POWER_MANAGEMENT_LIMIT", "ENFORCED_POWER_LIMIT"):
		return "power.usage"
	case strings.Contains(name, "VIOLATION"):
		return "throttle.violations"
	case strings.Contains(name, "PCIE") && strings.Contains(name, "LINK_GEN"):
		return "interconnect.pcie.link.generation"
	case strings.Contains(name, "PCIE") && strings.Contains(name, "LINK_WIDTH"):
		return "interconnect.pcie.link.width"
	case strings.Contains(name, "NVSWITCH"):
		return classifyNVSwitchGroup(name, typ)
	case strings.Contains(name, "CONNECTX"):
		return classifyConnectXGroup(name, typ)
	case strings.Contains(name, "C2C_"):
		return classifyC2CGroup(name, typ)
	case strings.Contains(name, "PCIE") || strings.Contains(name, "NVLINK") || strings.Contains(name, "P2P_") || strings.Contains(name, "FABRIC_"):
		return classifyInterconnectGroup(entity, name, typ)
	case strings.Contains(name, "XID"):
		return "reliability.xid"
	case strings.Contains(name, "DIAG_"):
		if strings.HasSuffix(name, "_STATUS") {
			return "diagnostics.status"
		}
		return "diagnostics.results"
	case strings.Contains(name, "CPU_UTIL"):
		return "cpu.utilization"
	case strings.Contains(name, "CPU_TEMP"):
		return "cpu.temperature"
	case containsAny(name, "CPU_POWER", "MODULE_POWER", "SYSIO_POWER"):
		return "cpu.power"
	case containsAny(name, "CPU_VENDOR", "CPU_MODEL"):
		return "cpu.info"
	case containsAny(name, "CPU_AFFINITY", "MEM_AFFINITY", "GPU_TOPOLOGY", "PCI_BUSID", "PCI_COMBINED_ID", "PCI_SUBSYS_ID"):
		return "topology.affinity"
	case strings.Contains(name, "VGPU_"):
		return classifyVGPUGroup(name)
	case containsAny(name, "ACCOUNTING_DATA", "FBC_", "ENC_STATS"):
		return "workload.sessions"
	case containsAny(name, "SUPPORTED_", "CREATABLE_", "CUDA_COMPUTE_CAPABILITY", "CC_MODE", "GPM_SUPPORT", "MIG_ATTRIBUTES", "MIG_GI_INFO", "MIG_CI_INFO", "MIG_MAX_SLICES"):
		return "capability.support"
	case containsAny(name, "DRIVER_VERSION", "NVML_VERSION", "VBIOS_VERSION", "INFOROM", "OEM_INFOROM", "PROCESS_NAME"):
		return "inventory.software"
	case containsAny(name, "PLATFORM_", "CHASSIS", "HOST_ID", "TRAY_INDEX", "MODULE_ID", "INFINIBAND_GUID"):
		return "inventory.platform"
	case containsAny(name, "DEV_NAME", "BRAND", "SERIAL", "UUID", "MINOR_NUMBER", "NVML_INDEX", "CUDA_VISIBLE_DEVICES_STR", "DEV_COUNT"):
		return "inventory.identity"
	case strings.Contains(name, "CLOCK"):
		return "clock.frequency"
	default:
		if typ == sampleCounter {
			return "other.counter"
		}
		return "other.gauge"
	}
}

func classifyInterconnectGroup(entity metricEntity, name string, typ sampleKind) string {
	switch {
	case containsAny(name, "P2P_STATUS"):
		return "health.status"
	case containsAny(name, "FABRIC_"):
		return "interconnect.fabric"
	case strings.Contains(name, "PCIE") && containsAny(name, "BYTES", "THROUGHPUT", "BANDWIDTH"):
		return "interconnect.pcie.throughput"
	case strings.Contains(name, "NVLINK") && containsAny(name, "BYTES", "THROUGHPUT", "BANDWIDTH"):
		if entity == entityNVLink {
			return "interconnect.throughput"
		}
		return "interconnect.nvlink.throughput"
	case containsAny(name, "XMIT_WAIT"):
		if strings.Contains(name, "NVLINK") {
			if entity == entityNVLink {
				return "interconnect.congestion"
			}
			return "interconnect.nvlink.congestion"
		}
		return "interconnect.congestion"
	case containsAny(name, "BER"):
		if strings.Contains(name, "PCIE") {
			return "interconnect.pcie.ber"
		}
		if strings.Contains(name, "NVLINK") {
			if entity == entityNVLink {
				return "interconnect.ber"
			}
			return "interconnect.nvlink.ber"
		}
		return "interconnect.ber"
	case containsAny(name, "PACKETS", "CODES"):
		if strings.Contains(name, "PCIE") {
			return "interconnect.pcie.traffic"
		}
		if strings.Contains(name, "NVLINK") {
			if entity == entityNVLink {
				return "interconnect.traffic"
			}
			return "interconnect.nvlink.traffic"
		}
		return "interconnect.traffic"
	case containsAny(name, "BYTES", "THROUGHPUT", "BANDWIDTH"):
		return "interconnect.throughput"
	case containsAny(name, "ERROR", "CRC", "REPLAY", "RECOVERY", "DISCARD", "FEC", "UNCORRECTABLE", "INTEGRITY"):
		if strings.Contains(name, "PCIE") {
			if typ == sampleCounter {
				return "interconnect.pcie.error_rate"
			}
			return "interconnect.pcie.errors"
		}
		if strings.Contains(name, "NVLINK") {
			if entity == entityNVLink {
				if typ == sampleCounter {
					return "interconnect.error_rate"
				}
				return "interconnect.errors"
			}
			if typ == sampleCounter {
				return "interconnect.nvlink.error_rate"
			}
			return "interconnect.nvlink.errors"
		}
		if typ == sampleCounter {
			return "interconnect.error_rate"
		}
		return "interconnect.errors"
	case strings.Contains(name, "PCIE") && strings.Contains(name, "RESULT"):
		return "interconnect.pcie.state"
	case containsAny(name, "STATE", "STATUS", "POWER_STATE", "LINK_COUNT"):
		if strings.Contains(name, "PCIE") {
			return "interconnect.pcie.state"
		}
		if strings.Contains(name, "NVLINK") {
			if entity == entityNVLink {
				return "interconnect.state"
			}
			return "interconnect.nvlink.state"
		}
		return "interconnect.state"
	default:
		return "interconnect.state"
	}
}

func classifyNVSwitchGroup(name string, typ sampleKind) string {
	switch {
	case containsAny(name, "LINK_LATENCY_"):
		return "interconnect.nvswitch.latency"
	case containsAny(name, "THROUGHPUT"):
		return "interconnect.nvswitch.throughput"
	case containsAny(name, "ERROR", "FATAL", "NON_FATAL", "CRC", "REPLAY", "RECOVERY", "FLIT"):
		if typ == sampleCounter {
			return "interconnect.nvswitch.errors"
		}
		return "interconnect.nvswitch.errors"
	case containsAny(name, "TEMPERATURE"):
		return "interconnect.nvswitch.temperature"
	case containsAny(name, "POWER_"):
		return "interconnect.nvswitch.power"
	case containsAny(name, "CURRENT_"):
		return "interconnect.nvswitch.current"
	case containsAny(name, "VOLTAGE"):
		return "interconnect.nvswitch.voltage"
	case containsAny(name, "PCIE_", "PHYS_ID", "DEVICE_UUID", "LINK_ID", "LINK_SID", "DEVICE_LINK_ID", "REMOTE_PCIE_"):
		return "interconnect.nvswitch.topology"
	default:
		return "interconnect.nvswitch.status"
	}
}

func classifyConnectXGroup(name string, typ sampleKind) string {
	switch {
	case containsAny(name, "TEMPERATURE"):
		return "interconnect.connectx.temperature"
	case containsAny(name, "ERR", "ERROR"):
		if typ == sampleCounter {
			return "interconnect.connectx.errors"
		}
		return "interconnect.connectx.error_status"
	case containsAny(name, "LINK_SPEED", "LINK_WIDTH", "PCIE"):
		return "interconnect.connectx.link"
	default:
		return "interconnect.connectx.status"
	}
}

func classifyC2CGroup(name string, typ sampleKind) string {
	switch {
	case containsAny(name, "RX_", "TX_", "BANDWIDTH"):
		return "interconnect.throughput"
	case containsAny(name, "ERROR", "REPLAY", "FEC", "BER", "INTR", "DISCARD", "RECOVERY"):
		if typ == sampleCounter {
			return "interconnect.error_rate"
		}
		return "interconnect.errors"
	default:
		return "interconnect.state"
	}
}

func classifyVGPUGroup(name string) string {
	switch {
	case containsAny(name, "VGPU_LICENSE_STATUS", "TYPE_LICENSE", "INSTANCE_LICENSE_STATE"):
		return "virtualization.vgpu.license"
	case containsAny(name, "VGPU_TYPE", "TYPE_CLASS", "TYPE_INFO", "TYPE_NAME"):
		return "virtualization.vgpu.type"
	case containsAny(name, "VGPU_INSTANCE_IDS", "VGPU_UUID", "VGPU_PCI_ID"):
		return "virtualization.vgpu.instance"
	case containsAny(name, "VGPU_VM_"):
		return "virtualization.vgpu.vm"
	case containsAny(name, "VGPU_MEMORY_USAGE"):
		return "virtualization.vgpu.memory"
	case containsAny(name, "VGPU_FRAME_RATE_LIMIT"):
		return "virtualization.vgpu.frame_rate"
	case containsAny(name, "VGPU_UTILIZATIONS", "PER_PROCESS_UTILIZATION"):
		return "virtualization.vgpu.utilization"
	case containsAny(name, "VGPU_ENC_", "VGPU_FBC_"):
		return "virtualization.vgpu.sessions"
	case containsAny(name, "VGPU_DRIVER_VERSION"):
		return "virtualization.vgpu.software"
	default:
		return "virtualization.vgpu.instance"
	}
}

func metricScale(metricName, help string, spec contextSpec) float64 {
	name := strings.ToUpper(metricName)
	h := strings.ToLower(help)

	if spec.ID == "" {
		return 1
	}

	if strings.HasSuffix(spec.ID, ".compute.activity") ||
		strings.HasSuffix(spec.ID, ".compute.tensor.activity") ||
		strings.HasSuffix(spec.ID, ".compute.media.activity") {
		return 100
	}

	if strings.HasSuffix(spec.ID, ".memory.utilization") {
		return 100
	}

	if (strings.HasSuffix(spec.ID, ".memory.usage") ||
		strings.HasSuffix(spec.ID, ".memory.capacity") ||
		strings.HasSuffix(spec.ID, ".memory.bar1_usage") ||
		strings.HasSuffix(spec.ID, ".memory.bar1_capacity")) &&
		(containsAny(name, "FB_", "BAR1") || strings.Contains(h, "mib") || strings.Contains(h, " mb")) {
		return 1024 * 1024
	}

	if strings.HasSuffix(spec.ID, ".virtualization.vgpu.memory") &&
		(strings.Contains(h, "mib") || strings.Contains(h, " mb")) {
		return 1024 * 1024
	}

	if strings.HasSuffix(spec.ID, ".virtualization.vgpu.utilization") && strings.Contains(h, "ratio") {
		return 100
	}

	if strings.HasSuffix(spec.ID, ".throttle.violations") && strings.Contains(name, "VIOLATION") {
		return 1.0 / 1e6 // ns => ms
	}
	if strings.HasSuffix(spec.ID, ".throttle.violations") && strings.Contains(name, "CLOCKS_EVENT_REASON") {
		return 1.0 / 1e6 // ns => ms
	}

	return 1
}

func metricDimensionName(metricName string) string {
	name := strings.ToUpper(metricName)
	if v, ok := metricDimensionAliases[name]; ok {
		return v
	}

	for _, pfx := range []string{
		"DCGM_FI_DEV_",
		"DCGM_FI_PROF_",
		"DCGM_FI_",
		"DCGM_EXP_",
		"DCGM_",
	} {
		name = strings.TrimPrefix(name, pfx)
	}

	name = strings.TrimSuffix(name, "_TOTAL")
	name = strings.TrimSuffix(name, "_COUNT")
	name = strings.TrimSuffix(name, "_VALUE")
	name = strings.ToLower(name)

	return sanitizeID(name)
}

func containsAny(s string, values ...string) bool {
	for _, v := range values {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

var metricDimensionAliases = map[string]string{
	"DCGM_FI_DEV_GPU_UTIL":                                       "gpu",
	"DCGM_FI_DEV_MEM_COPY_UTIL":                                  "memory_copy",
	"DCGM_FI_DEV_ENC_UTIL":                                       "encoder",
	"DCGM_FI_DEV_DEC_UTIL":                                       "decoder",
	"DCGM_FI_DEV_GPU_TEMP":                                       "gpu",
	"DCGM_FI_DEV_MEMORY_TEMP":                                    "memory",
	"DCGM_FI_DEV_SM_CLOCK":                                       "sm",
	"DCGM_FI_DEV_MEM_CLOCK":                                      "memory",
	"DCGM_FI_DEV_POWER_USAGE":                                    "draw",
	"DCGM_FI_DEV_TOTAL_ENERGY_CONSUMPTION":                       "total",
	"DCGM_FI_DEV_PCIE_REPLAY_COUNTER":                            "pcie_replay",
	"DCGM_FI_DEV_XID_ERRORS":                                     "xid",
	"DCGM_FI_DEV_POWER_VIOLATION":                                "power_violation",
	"DCGM_FI_DEV_THERMAL_VIOLATION":                              "thermal_violation",
	"DCGM_FI_DEV_SYNC_BOOST_VIOLATION":                           "sync_boost_violation",
	"DCGM_FI_DEV_BOARD_LIMIT_VIOLATION":                          "board_limit_violation",
	"DCGM_FI_DEV_LOW_UTIL_VIOLATION":                             "low_utilization_violation",
	"DCGM_FI_DEV_RELIABILITY_VIOLATION":                          "reliability_violation",
	"DCGM_FI_DEV_FB_FREE":                                        "free",
	"DCGM_FI_DEV_FB_TOTAL":                                       "total",
	"DCGM_FI_DEV_FB_USED":                                        "used",
	"DCGM_FI_DEV_FB_RESERVED":                                    "reserved",
	"DCGM_FI_DEV_FB_USED_PERCENT":                                "used_percent",
	"DCGM_FI_DEV_BAR1_TOTAL":                                     "total",
	"DCGM_FI_DEV_BAR1_USED":                                      "used",
	"DCGM_FI_DEV_BAR1_FREE":                                      "free",
	"DCGM_FI_DEV_FAN_SPEED":                                      "fan_speed",
	"DCGM_FI_DEV_ENFORCED_POWER_LIMIT":                           "enforced_limit",
	"DCGM_FI_DEV_PCIE_LINK_GEN":                                  "link_gen",
	"DCGM_FI_DEV_PCIE_MAX_LINK_GEN":                              "max_link_gen",
	"DCGM_FI_DEV_PCIE_LINK_WIDTH":                                "link_width",
	"DCGM_FI_DEV_PCIE_MAX_LINK_WIDTH":                            "max_link_width",
	"DCGM_FI_DEV_CLOCK_THROTTLE_REASONS":                         "reasons",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS":            "sw_power_cap",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_THERM_SLOWDOWN_NS":       "hw_therm_slowdown",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_THERM_SLOWDOWN_NS":       "sw_therm_slowdown",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_HW_POWER_BRAKE_SLOWDOWN_NS": "hw_power_brake_slowdown",
	"DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS":              "sync_boost",
	"DCGM_FI_DEV_UNCORRECTABLE_REMAPPED_ROWS":                    "uncorrectable_remapped_rows",
	"DCGM_FI_DEV_CORRECTABLE_REMAPPED_ROWS":                      "correctable_remapped_rows",
	"DCGM_FI_DEV_ROW_REMAP_FAILURE":                              "row_remap_failure",
	"DCGM_FI_DEV_ROW_REMAP_PENDING":                              "row_remap_pending",
	"DCGM_FI_PROF_SM_ACTIVE":                                     "sm_active",
	"DCGM_FI_PROF_SM_OCCUPANCY":                                  "sm_occupancy",
	"DCGM_FI_PROF_GR_ENGINE_ACTIVE":                              "graphics_engine_active",
	"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE":                            "tensor",
	"DCGM_FI_PROF_DRAM_ACTIVE":                                   "dram",
	"DCGM_FI_PROF_PIPE_FP64_ACTIVE":                              "fp64",
	"DCGM_FI_PROF_PIPE_FP32_ACTIVE":                              "fp32",
	"DCGM_FI_PROF_PIPE_FP16_ACTIVE":                              "fp16",
	"DCGM_FI_PROF_PCIE_TX_BYTES":                                 "pcie_tx",
	"DCGM_FI_PROF_PCIE_RX_BYTES":                                 "pcie_rx",
	"DCGM_FI_PROF_PIPE_INT_ACTIVE":                               "integer",
	"DCGM_FI_PROF_PIPE_TENSOR_DFMA_ACTIVE":                       "tensor_dfma",
	"DCGM_FI_PROF_PIPE_TENSOR_HMMA_ACTIVE":                       "tensor_hmma",
	"DCGM_FI_PROF_PIPE_TENSOR_IMMA_ACTIVE":                       "tensor_imma",
}
