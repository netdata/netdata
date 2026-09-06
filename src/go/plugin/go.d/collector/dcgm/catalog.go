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
	entityHost     metricEntity = "host"
)

type sampleKind uint8

const (
	sampleGauge sampleKind = iota
	sampleCounter
	sampleUnsupported
)

type contextSpec struct {
	ID, Title, Units, Family string
	Type                     collectorapi.ChartType
	Priority                 int
}
type metricSpec struct {
	Context contextSpec
	fieldDefinition
}
type groupSpec struct {
	Suffix, Title, Units, Family string
	Type                         collectorapi.ChartType
}

var groupCatalog = []groupSpec{
	{"compute.utilization", "GPU and Memory Busy Time", "percentage", "compute", collectorapi.Line},
	{"compute.engine_activity", "Graphics and Compute Engine Activity", "percentage", "compute", collectorapi.Line},
	{"compute.sm.utilization", "Streaming Multiprocessor Utilization", "percentage", "compute", collectorapi.Line},
	{"compute.resource_activity", "Tensor and Memory Interface Activity", "percentage", "compute", collectorapi.Line},
	{"compute.pipe.activity", "Arithmetic Pipeline Activity", "percentage", "compute", collectorapi.Line},
	{"compute.tensor.activity", "Tensor Instruction Activity", "percentage", "compute", collectorapi.Line},
	{"compute.media.utilization", "Video Engine Utilization", "percentage", "media", collectorapi.Line},
	{"compute.media.activity", "Media Engine Activity", "percentage", "media", collectorapi.Line},
	{"compute.cache.host", "Host Memory Cache Efficiency", "percentage", "cache", collectorapi.Line},
	{"compute.cache.peer", "Peer Memory Cache Efficiency", "percentage", "cache", collectorapi.Line},
	{"memory.utilization", "VRAM Capacity Used", "percentage", "memory", collectorapi.Line},
	{"memory.usage", "VRAM Allocation", "bytes", "memory", collectorapi.Stacked},
	{"memory.capacity", "VRAM Capacity", "bytes", "memory", collectorapi.Line},
	{"memory.bar1_usage", "BAR1 Mapping Usage", "bytes", "memory", collectorapi.Stacked},
	{"memory.bar1_capacity", "BAR1 Mapping Capacity", "bytes", "memory", collectorapi.Line},
	{"memory.ecc_mode", "ECC Mode", "state", "memory/ecc", collectorapi.Line},
	{"memory.ecc_volatile", "Volatile ECC Errors", "errors", "memory/ecc", collectorapi.Line},
	{"memory.ecc_aggregate", "Persistent ECC Errors", "errors", "memory/ecc", collectorapi.Line},
	{"memory.ecc_volatile_rate", "Volatile ECC Error Rate", "errors/s", "memory/ecc", collectorapi.Line},
	{"memory.ecc_aggregate_rate", "Persistent ECC Error Rate", "errors/s", "memory/ecc", collectorapi.Line},
	{"memory.ecc_volatile_detail", "Volatile ECC Errors by Location", "errors", "memory/ecc", collectorapi.Line},
	{"memory.ecc_aggregate_detail", "Persistent ECC Errors by Location", "errors", "memory/ecc", collectorapi.Line},
	{"memory.page_retirements", "Memory Page Retirement Rate", "pages/s", "reliability", collectorapi.Line},
	{"memory.retired_pages", "Retired Memory Pages", "pages", "reliability", collectorapi.Line},
	{"reliability.page_retirement_status", "Memory Page Retirement Pending", "state", "reliability", collectorapi.Line},
	{"reliability.row_remap_status", "Row Remap Status", "state", "reliability", collectorapi.Line},
	{"reliability.row_remap_events", "Row Remap Rate", "rows/s", "reliability", collectorapi.Line},
	{"reliability.remapped_rows", "Remapped Memory Rows", "rows", "reliability", collectorapi.Line},
	{"reliability.remap_banks", "Memory Bank Remap Availability", "banks", "reliability", collectorapi.Stacked},
	{"reliability.memory_health", "Memory Health Flags", "state", "reliability", collectorapi.Line},
	{"reliability.recovery_action", "Recovery Action", "state", "reliability", collectorapi.Line},
	{"clock.sm.frequency", "SM Clock Frequency", "MHz", "clock", collectorapi.Line},
	{"clock.memory.frequency", "Memory Clock Frequency", "MHz", "clock", collectorapi.Line},
	{"clock.video.frequency", "Video Clock Frequency", "MHz", "clock", collectorapi.Line},
	{"clock.frequency", "Clock Frequency", "MHz", "clock", collectorapi.Line},
	{"throttle.reasons", "Clock Event Reasons", "state", "throttle", collectorapi.Line},
	{"throttle.duration", "Clock Limiting Duration", "percentage", "throttle", collectorapi.Line},
	{"throttle.event_samples", "Clock Event Samples", "samples", "throttle", collectorapi.Line},
	{"throttle.event_rate", "Clock Event Rate", "events/s", "throttle", collectorapi.Line},
	{"thermal.temperature", "Temperature and Limits", "Celsius", "thermal", collectorapi.Line},
	{"thermal.headroom", "Thermal Headroom", "Celsius", "thermal", collectorapi.Line},
	{"thermal.fan_speed", "Reported Fan Speed", "percentage", "thermal", collectorapi.Line},
	{"power.usage", "Power Draw and Limit", "Watts", "power", collectorapi.Line},
	{"power.limits", "Configurable Power Limits", "Watts", "power", collectorapi.Line},
	{"power.energy_rate", "Energy-Derived Power", "Watts", "power", collectorapi.Line},
	{"power.smoothing.bounds", "Power Smoothing Bounds", "Watts", "power/smoothing", collectorapi.Line},
	{"power.smoothing.floor", "Power Smoothing Floor", "percentage", "power/smoothing", collectorapi.Line},
	{"power.smoothing.lifetime", "Power Smoothing Circuit Lifetime Remaining", "percentage", "power/smoothing", collectorapi.Line},
	{"power.smoothing.ramp", "Power Smoothing Ramp Rate", "Watts/s", "power/smoothing", collectorapi.Line},
	{"power.smoothing.hysteresis", "Power Smoothing Ramp Hysteresis", "milliseconds", "power/smoothing", collectorapi.Line},
	{"power.smoothing.state", "Power Smoothing Enablement", "state", "power/smoothing", collectorapi.Line},
	{"power.smoothing.privilege", "Power Smoothing Privilege Level", "level", "power/smoothing", collectorapi.Line},
	{"power.smoothing.profile", "Active Power Smoothing Profile", "profile", "power/smoothing", collectorapi.Line},
	{"power.smoothing.profile_capacity", "Power Smoothing Profile Capacity", "profiles", "power/smoothing", collectorapi.Line},
	{"interconnect.total.throughput", "Observed Interconnect Throughput", "bytes/s", "interconnect/overview", collectorapi.Line},
	{"interconnect.pcie.throughput", "PCIe Throughput", "bytes/s", "interconnect/pcie", collectorapi.Area},
	{"interconnect.nvlink.throughput", "NVLink Throughput", "bytes/s", "interconnect/nvlink", collectorapi.Area},
	{"interconnect.combined.throughput", "Combined Link Throughput", "bytes/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.throughput", "Link Throughput", "bytes/s", "interconnect/nvlink", collectorapi.Area},
	{"interconnect.pcie.link.generation", "PCIe Link Generation", "generation", "interconnect/pcie", collectorapi.Line},
	{"interconnect.pcie.link.width", "PCIe Link Width", "lanes", "interconnect/pcie", collectorapi.Line},
	{"interconnect.pcie.error_rate", "PCIe Error and Replay Rate", "errors/s", "interconnect/pcie", collectorapi.Line},
	{"interconnect.nvlink.error_rate", "NVLink Error Rate", "errors/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.error_rate", "Link Error Rate", "errors/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvlink.traffic", "NVLink Packet Rate", "packets/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvlink.ber", "NVLink Bit Error Ratio", "errors per trillion bits", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.ber", "Link Bit Error Ratio", "errors per trillion bits", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvlink.state", "NVLink State", "state", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.fabric", "Fabric Manager State", "state", "interconnect/overview", collectorapi.Line},
	{"interconnect.fabric_error", "Fabric Manager Error", "code", "interconnect/overview", collectorapi.Line},
	{"interconnect.p2p", "Peer Access State", "state", "interconnect/overview", collectorapi.Line},
	{"interconnect.c2c.capacity", "C2C Link Capacity", "bytes/s", "interconnect/overview", collectorapi.Line},
	{"interconnect.c2c.links", "C2C Link Count", "links", "interconnect/overview", collectorapi.Line},
	{"interconnect.c2c.state", "C2C Link State", "state", "interconnect/overview", collectorapi.Line},
	{"interconnect.c2c.throughput", "C2C All Traffic", "bytes/s", "interconnect/overview", collectorapi.Area},
	{"interconnect.c2c.payload", "C2C Payload Traffic", "bytes/s", "interconnect/overview", collectorapi.Area},
	{"interconnect.c2c.error_rate", "C2C Error Rate", "errors/s", "interconnect/overview", collectorapi.Line},
	{"interconnect.nvswitch.sxid", "NVSwitch Error Code", "code", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvswitch.link_sxid", "NVSwitch Link Error Code", "code", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvswitch.ecc_lanes", "NVSwitch ECC Error Rate by Lane", "errors/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvswitch.errors", "NVSwitch Error Rate", "errors/s", "interconnect/nvlink", collectorapi.Line},
	{"interconnect.nvswitch.temperature", "NVSwitch Temperature", "Celsius", "interconnect/nvlink", collectorapi.Line},
	{"reliability.xid", "XID Error Code", "code", "reliability", collectorapi.Line},
	{"reliability.xid_samples", "XID Samples", "samples", "reliability", collectorapi.Line},
	{"reliability.xid_rate", "XID Record Rate", "records/s", "reliability", collectorapi.Line},
	{"health.status", "Health Status by Watch", "state", "health", collectorapi.Line},
	{"state.performance", "Performance State", "state", "state", collectorapi.Line},
	{"state.virtualization", "Virtualization Mode", "state", "state", collectorapi.Line},
	{"state.configuration", "Compute and Persistence Modes", "state", "state", collectorapi.Line},
	{"virtualization.vgpu.license", "vGPU License State", "state", "virtualization", collectorapi.Line},
	{"virtualization.vgpu.memory", "vGPU Memory Usage", "bytes", "virtualization", collectorapi.Line},
	{"virtualization.vgpu.frame_rate", "vGPU Frame Rate Limit", "frames/s", "virtualization", collectorapi.Line},
	{"cpu.utilization", "CPU Busy Time", "percentage", "cpu", collectorapi.Line},
	{"cpu.utilization_modes", "CPU Activity by Mode", "percentage", "cpu", collectorapi.Line},
	{"cpu.temperature", "CPU Temperature", "Celsius", "cpu", collectorapi.Line},
	{"cpu.power", "CPU Power", "Watts", "cpu", collectorapi.Line},
	{"capability.mig_slices", "Maximum MIG Slices", "slices", "capability", collectorapi.Line},
	{"inventory.gpu_count", "GPU Count", "GPUs", "inventory", collectorapi.Line},
}

var contextCatalog = buildContextCatalog()

func buildContextCatalog() map[string]contextSpec {
	catalog := make(map[string]contextSpec)
	for n, entity := range []metricEntity{entityGPU, entityMIG, entityNVLink, entityNVSwitch, entityCPU, entityCPUCore, entityExporter, entityHost} {
		for i, group := range groupCatalog {
			if (entity == entityHost) != (group.Suffix == "inventory.gpu_count") {
				continue
			}
			ctx := fmt.Sprintf("dcgm.%s.%s", entity, group.Suffix)
			title := group.Title
			if !strings.HasPrefix(title, entityDisplayName(entity)+" ") {
				title = entityDisplayName(entity) + " " + title
			}
			catalog[ctx] = contextSpec{
				ID:       ctx,
				Title:    title,
				Units:    group.Units,
				Family:   entityFamilyPrefix(entity) + " " + group.Family,
				Type:     group.Type,
				Priority: collectorapi.Priority + n*10000 + i*10,
			}
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
	case entityHost:
		return "Host"
	default:
		return "Exporter"
	}
}
func entityFamilyPrefix(entity metricEntity) string {
	if entity == entityCPUCore {
		return "cpu core"
	}
	return string(entity)
}
func classifyMetric(entity metricEntity, metricName, _ string, typ sampleKind) metricSpec {
	name := strings.ToUpper(metricName)
	def, known := fieldCatalog[name]
	if !known || def.Raw {
		if known {
			typ = def.Kind
		}
		def = fieldDefinition{
			DimName:   "value",
			Scale:     1,
			Kind:      typ,
			Precision: precision,
			Raw:       true,
		}
		units, title := "value", "Raw Value"
		if typ == sampleCounter {
			units, title = "value/s", "Raw Rate"
		}
		return metricSpec{
			Context: contextSpec{
				ID:       fmt.Sprintf("dcgm.%s.raw.%s", entity, strings.ToLower(name)),
				Title:    metricName + " " + title,
				Units:    units,
				Family:   entityFamilyPrefix(entity) + " raw",
				Type:     collectorapi.Line,
				Priority: collectorapi.Priority + 90000,
			},
			fieldDefinition: def,
		}
	}
	if def.Host {
		entity = entityHost
	}
	group := def.Group
	if entity == entityNVLink {
		switch group {
		case "interconnect.nvlink.throughput":
			group = "interconnect.throughput"
		case "interconnect.nvlink.error_rate":
			group = "interconnect.error_rate"
		case "interconnect.nvlink.ber":
			group = "interconnect.ber"
		}
	}
	return metricSpec{
		Context:         contextCatalog[fmt.Sprintf("dcgm.%s.%s", entity, group)],
		fieldDefinition: def,
	}
}
