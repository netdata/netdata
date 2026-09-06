// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"fmt"
	"strings"
)

// Field meaning comes from DCGM/NVML, not an exporter's configurable TYPE line.
// See METRICS.md for source references, aliases and representations.
type fieldDefinition struct {
	SourceKind                                         sampleKind
	Group, DimName                                     string
	Scale, Precision                                   float64
	Kind                                               sampleKind
	Priority                                           int
	Labels                                             []string
	States                                             []string
	Bitmask, DecodeBER, CounterRate, Host, Raw, Window bool
	Transport, Direction, Link, TotalGroup, RateSource string
}

var fieldCatalog = buildFieldCatalog()

func buildFieldCatalog() map[string]fieldDefinition {
	fields := make(map[string]fieldDefinition)
	add := func(name, group, dim string, kind sampleKind, scale float64) {
		if _, exists := fields[name]; exists {
			panic("duplicate DCGM field: " + name)
		}
		fields[name] = fieldDefinition{
			Group:      group,
			DimName:    dim,
			Kind:       kind,
			SourceKind: kind,
			Scale:      scale,
			Precision:  precision,
			Priority:   10,
		}
	}
	dev := func(name, group, dim string, kind sampleKind, scale float64) {
		add("DCGM_FI_DEV_"+name, group, dim, kind, scale)
	}
	prof := func(name, group, dim string, scale float64) {
		add("DCGM_FI_PROF_"+name, group, dim, sampleGauge, scale)
	}
	update := func(name string, f func(*fieldDefinition)) {
		d, ok := fields[name]
		if !ok {
			panic("missing DCGM field: " + name)
		}
		f(&d)
		fields[name] = d
	}
	alias := func(name, original string) {
		d, ok := fields[original]
		if !ok {
			panic("missing DCGM alias target: " + original)
		}
		d.Priority++
		fields[name] = d
	}

	dev("GPU_UTIL", "compute.utilization", "gpu", sampleGauge, 1)
	dev("MEM_COPY_UTIL", "compute.utilization", "memory_copy", sampleGauge, 1)
	dev("ENC_UTIL", "compute.media.utilization", "encoder", sampleGauge, 1)
	dev("DEC_UTIL", "compute.media.utilization", "decoder", sampleGauge, 1)
	prof("GR_ENGINE_ACTIVE", "compute.engine_activity", "activity", 100)
	prof("SM_ACTIVE", "compute.sm.utilization", "activity", 100)
	prof("SM_OCCUPANCY", "compute.sm.utilization", "occupancy", 100)
	prof("PIPE_TENSOR_ACTIVE", "compute.resource_activity", "tensor", 100)
	prof("DRAM_ACTIVE", "compute.resource_activity", "dram", 100)
	for _, arithmetic := range []string{"FP16", "FP32", "FP64", "INT"} {
		dim := strings.ToLower(arithmetic)
		if arithmetic == "INT" {
			dim = "integer"
		}
		prof("PIPE_"+arithmetic+"_ACTIVE", "compute.pipe.activity", dim, 100)
	}
	for _, tensor := range []string{"HMMA", "IMMA", "DFMA"} {
		prof("PIPE_TENSOR_"+tensor+"_ACTIVE", "compute.tensor.activity", "tensor_"+strings.ToLower(tensor), 100)
	}
	for _, memory := range []string{"HOSTMEM", "PEERMEM"} {
		group := "compute.cache.host"
		if memory == "PEERMEM" {
			group = "compute.cache.peer"
		}
		for _, outcome := range []string{"HIT", "MISS"} {
			prof(memory+"_CACHE_"+outcome, group, strings.ToLower(outcome), 1)
		}
	}
	for _, engine := range []string{"NVDEC", "NVJPG", "NVOFA"} {
		count := 8
		if engine == "NVOFA" {
			count = 2
		}
		for i := 0; i < count; i++ {
			prof(fmt.Sprintf("%s%d_ACTIVE", engine, i), "compute.media.activity", fmt.Sprintf("%s%d", strings.ToLower(engine), i), 100)
			alias(fmt.Sprintf("DCGM_FI_PROF_%s_UTIL_%d_RATIO", engine, i), fmt.Sprintf("DCGM_FI_PROF_%s%d_ACTIVE", engine, i))
		}
	}
	for _, domain := range []string{"SM", "MEM", "VIDEO"} {
		group := map[string]string{"SM": "sm", "MEM": "memory", "VIDEO": "video"}[domain]
		dev(domain+"_CLOCK", "clock."+group+".frequency", "current", sampleGauge, 1)
		dev("MAX_"+domain+"_CLOCK", "clock."+group+".frequency", "maximum", sampleGauge, 1)
		if domain != "VIDEO" {
			dev("APP_"+domain+"_CLOCK", "clock."+group+".frequency", "application", sampleGauge, 1)
		}
	}
	for field, dim := range map[string]string{"GPU_TEMP": "gpu", "MEMORY_TEMP": "memory", "GPU_MAX_OP_TEMP": "gpu_max_operating", "MEM_MAX_OP_TEMP": "memory_max_operating", "SLOWDOWN_TEMP": "gpu_slowdown", "SHUTDOWN_TEMP": "gpu_shutdown"} {
		dev(field, "thermal.temperature", dim, sampleGauge, 1)
	}
	dev("GPU_TEMP_LIMIT", "thermal.headroom", "gpu", sampleGauge, 1)
	dev("FAN_SPEED", "thermal.fan_speed", "fan_speed", sampleGauge, 1)
	for field, dim := range map[string]string{"POWER_USAGE": "draw", "POWER_USAGE_INSTANT": "instantaneous", "ENFORCED_POWER_LIMIT": "enforced_limit", "POWER_MGMT_LIMIT": "configured_limit"} {
		dev(field, "power.usage", dim, sampleGauge, 1)
	}
	for field, dim := range map[string]string{"POWER_MGMT_LIMIT_DEF": "default", "POWER_MGMT_LIMIT_MAX": "maximum", "POWER_MGMT_LIMIT_MIN": "minimum"} {
		dev(field, "power.limits", dim, sampleGauge, 1)
	}
	dev("TOTAL_ENERGY_CONSUMPTION", "power.energy_rate", "power", sampleCounter, 1e-3)
	// NVML duration counters are nanoseconds; old DCGM headers had stale usec comments.
	for field, dim := range map[string]string{"POWER_VIOLATION": "power_violation", "THERMAL_VIOLATION": "thermal_violation", "SYNC_BOOST_VIOLATION": "sync_boost", "BOARD_LIMIT_VIOLATION": "board_limit", "LOW_UTIL_VIOLATION": "low_utilization", "RELIABILITY_VIOLATION": "reliability"} {
		dev(field, "throttle.duration", dim, sampleCounter, 1e-7)
	}
	for _, reason := range []string{"SW_THERM_SLOWDOWN", "HW_THERM_SLOWDOWN", "HW_POWER_BRAKE_SLOWDOWN"} {
		dev("CLOCKS_EVENT_REASON_"+reason+"_NS", "throttle.duration", strings.ToLower(reason), sampleCounter, 1e-7)
	}
	alias("DCGM_FI_DEV_CLOCKS_EVENT_REASON_SW_POWER_CAP_NS", "DCGM_FI_DEV_POWER_VIOLATION")
	alias("DCGM_FI_DEV_CLOCKS_EVENT_REASON_SYNC_BOOST_NS", "DCGM_FI_DEV_SYNC_BOOST_VIOLATION")
	dev("CLOCKS_EVENT_REASONS", "throttle.reasons", "reason", sampleGauge, 1)
	update("DCGM_FI_DEV_CLOCKS_EVENT_REASONS", func(d *fieldDefinition) {
		d.Bitmask = true
		d.States = []string{"gpu_idle", "application_clocks", "power_cap", "hardware_slowdown", "sync_boost", "software_thermal", "hardware_thermal", "hardware_power_brake", "display_clocks"}
	})
	alias("DCGM_FI_DEV_CLOCK_THROTTLE_REASONS", "DCGM_FI_DEV_CLOCKS_EVENT_REASONS")
	update("DCGM_FI_DEV_CLOCK_THROTTLE_REASONS", func(d *fieldDefinition) { d.Priority = 9 })

	for _, prefix := range []string{"FB", "BAR1"} {
		usage, capacity := "memory.usage", "memory.capacity"
		if prefix == "BAR1" {
			usage, capacity = "memory.bar1_usage", "memory.bar1_capacity"
		}
		for _, part := range []string{"USED", "FREE"} {
			dev(prefix+"_"+part, usage, strings.ToLower(part), sampleGauge, 1024*1024)
		}
		dev(prefix+"_TOTAL", capacity, "total", sampleGauge, 1024*1024)
	}
	dev("FB_RESERVED", "memory.usage", "reserved", sampleGauge, 1024*1024)
	dev("FB_USED_PERCENT", "memory.utilization", "used_percent", sampleGauge, 100)
	dev("ECC_CURRENT", "memory.ecc_mode", "current", sampleGauge, 1)
	dev("ECC_PENDING", "memory.ecc_mode", "pending", sampleGauge, 1)
	for lifetime, group := range map[string]string{"VOL": "volatile", "AGG": "aggregate"} {
		for _, severity := range []string{"SBE", "DBE"} {
			field := "DCGM_FI_DEV_ECC_" + severity + "_" + lifetime + "_TOTAL"
			add(field, "memory.ecc_"+group+"_rate", strings.ToLower(severity), sampleCounter, 1)
			update(field, func(d *fieldDefinition) { d.TotalGroup = "memory.ecc_" + group })
			for _, location := range []string{"L1", "L2", "DEV", "REG", "TEX", "SHM", "CBU", "SRM"} {
				// Location counts are kept apart from their rollup totals.
				dev("ECC_"+severity+"_"+lifetime+"_"+location, "memory.ecc_"+group+"_detail", strings.ToLower(severity+"_"+location), sampleGauge, 1)
				update("DCGM_FI_DEV_ECC_"+severity+"_"+lifetime+"_"+location, func(d *fieldDefinition) { d.SourceKind = sampleCounter })
			}
		}
	}
	for _, severity := range []string{"SBE", "DBE"} {
		field := "DCGM_FI_DEV_RETIRED_" + severity
		add(field, "memory.page_retirements", strings.ToLower(severity), sampleCounter, 1)
		update(field, func(d *fieldDefinition) { d.TotalGroup = "memory.retired_pages" })
	}
	dev("RETIRED_PENDING", "reliability.page_retirement_status", "pending", sampleGauge, 1)
	for _, severity := range []string{"CORRECTABLE", "UNCORRECTABLE"} {
		field := "DCGM_FI_DEV_" + severity + "_REMAPPED_ROWS"
		add(field, "reliability.row_remap_events", strings.ToLower(severity)+"_remapped_rows", sampleCounter, 1)
		update(field, func(d *fieldDefinition) { d.TotalGroup = "reliability.remapped_rows" })
	}
	dev("ROW_REMAP_FAILURE", "reliability.row_remap_status", "row_remap_failure", sampleGauge, 1)
	dev("ROW_REMAP_PENDING", "reliability.row_remap_status", "row_remap_pending", sampleGauge, 1)
	dev("THRESHOLD_SRM", "reliability.memory_health", "sram_threshold_exceeded", sampleGauge, 1)
	dev("MEMORY_UNREPAIRABLE_FLAG", "reliability.memory_health", "unrepairable", sampleGauge, 1)
	for _, level := range []string{"MAX", "HIGH", "PARTIAL", "LOW", "NONE"} {
		dev("BANKS_REMAP_ROWS_AVAIL_"+level, "reliability.remap_banks", strings.ToLower(level), sampleGauge, 1)
	}
	dev("GET_GPU_RECOVERY_ACTION", "reliability.recovery_action", "action", sampleGauge, 1)
	dev("XID_ERRORS", "reliability.xid", "xid", sampleGauge, 1)
	dev("PSTATE", "state.performance", "", sampleGauge, 1)
	update("DCGM_FI_DEV_PSTATE", func(d *fieldDefinition) {
		for i := 0; i < 16; i++ {
			d.States = append(d.States, fmt.Sprintf("P%d", i))
		}
	})
	dev("COMPUTE_MODE", "state.configuration", "compute", sampleGauge, 1)
	update("DCGM_FI_DEV_COMPUTE_MODE", func(d *fieldDefinition) {
		d.States = []string{"default", "exclusive_thread", "prohibited", "exclusive_process"}
	})
	dev("PERSISTENCE_MODE", "state.configuration", "persistence_enabled", sampleGauge, 1)
	dev("AUTOBOOST", "state.configuration", "autoboost_enabled", sampleGauge, 1)
	dev("MIG_MODE", "state.virtualization", "mig_enabled", sampleGauge, 1)
	dev("VIRTUAL_MODE", "state.virtualization", "virtualization", sampleGauge, 1)
	update("DCGM_FI_DEV_VIRTUAL_MODE", func(d *fieldDefinition) {
		d.States = []string{"bare_metal", "passthrough", "vgpu", "host_vgpu", "host_vsga"}
	})
	dev("MIG_MAX_SLICES", "capability.mig_slices", "maximum", sampleGauge, 1)
	add("DCGM_FI_DEV_COUNT", "inventory.gpu_count", "gpus", sampleGauge, 1)
	update("DCGM_FI_DEV_COUNT", func(d *fieldDefinition) { d.Host = true })
	dev("VGPU_LICENSE_STATUS", "virtualization.vgpu.license", "licensed", sampleGauge, 1)
	dev("VGPU_MEMORY_USAGE", "virtualization.vgpu.memory", "used", sampleGauge, 1024*1024)
	dev("VGPU_FRAME_RATE_LIMIT", "virtualization.vgpu.frame_rate", "limit", sampleGauge, 1)

	dev("CPU_POWER_UTIL_CURRENT", "cpu.power", "current", sampleGauge, 1)
	dev("CPU_POWER_LIMIT", "cpu.power", "limit", sampleGauge, 1)
	alias("DCGM_FI_DEV_CPU_POWER_WATTS", "DCGM_FI_DEV_CPU_POWER_UTIL_CURRENT")
	alias("DCGM_FI_DEV_CPU_POWER_LIMIT_WATTS", "DCGM_FI_DEV_CPU_POWER_LIMIT")
	dev("CPU_CLOCK_CURRENT", "clock.frequency", "current", sampleGauge, 1e-3)
	dev("CPU_UTIL_TOTAL", "cpu.utilization", "total", sampleGauge, 100)
	for _, mode := range []string{"USER", "NICE", "SYS", "IRQ"} {
		dev("CPU_UTIL_"+mode, "cpu.utilization_modes", strings.ToLower(mode), sampleGauge, 100)
	}
	for _, level := range []string{"CURRENT", "WARNING", "CRITICAL"} {
		dev("CPU_TEMP_"+level, "cpu.temperature", strings.ToLower(level), sampleGauge, 1)
	}

	for _, transport := range []string{"PCIE", "NVLINK"} {
		for _, direction := range []string{"RX", "TX"} {
			name := "DCGM_FI_PROF_" + transport + "_" + direction + "_BYTES"
			add(name, "interconnect."+strings.ToLower(transport)+".throughput", strings.ToLower(transport+"_"+direction), sampleGauge, 1)
			update(name, func(d *fieldDefinition) {
				d.Transport = strings.ToLower(transport)
				d.Direction = strings.ToLower(direction)
				d.Priority = 100
				d.RateSource = "profiling"
			})
			if transport == "PCIE" {
				counter := name + "_TOTAL"
				alias(counter, name)
				update(counter, func(d *fieldDefinition) {
					d.CounterRate = true
					d.SourceKind = sampleCounter
					d.Priority = 30
					d.RateSource = "counter"
				})
			}
		}
	}
	for _, direction := range []string{"RX", "TX"} {
		name := "DCGM_FI_DEV_NVLINK_COUNT_" + direction + "_BYTES"
		alias(name, "DCGM_FI_PROF_NVLINK_"+direction+"_BYTES")
		update(name, func(d *fieldDefinition) {
			d.CounterRate = true
			d.SourceKind = sampleCounter
			d.Priority = 20
			d.RateSource = "counter"
		})
	}
	for _, direction := range []string{"RX", "TX"} {
		alias("DCGM_FI_DEV_NVLINK_"+direction+"_BYTES_TOTAL", "DCGM_FI_DEV_NVLINK_COUNT_"+direction+"_BYTES")
		alias("DCGM_FI_PROF_NVLINK_"+direction+"_BYTES_PER_LINK", "DCGM_FI_PROF_NVLINK_"+direction+"_BYTES")
	}
	// These fields are already rates: NVML KiB delta / seconds / 1000.
	for link := -1; link < 18; link++ {
		suffix, linkID := "TOTAL", ""
		if link >= 0 {
			suffix = fmt.Sprintf("L%d", link)
			linkID = fmt.Sprint(link)
		}
		for _, direction := range []string{"", "RX_", "TX_"} {
			name := "DCGM_FI_DEV_NVLINK_" + direction + "BANDWIDTH_" + suffix
			dir := "total"
			if direction != "" {
				dir = strings.ToLower(strings.TrimSuffix(direction, "_"))
			}
			add(name, "interconnect.nvlink.throughput", "nvlink_"+dir, sampleGauge, 1024000)
			update(name, func(d *fieldDefinition) {
				d.Transport = "nvlink"
				d.Direction = dir
				d.Link = linkID
				d.Priority = 50
				d.RateSource = "legacy"
			})
			alias(strings.Replace(name, "BANDWIDTH", "THROUGHPUT", 1), name)
		}
		if link >= 0 {
			for _, direction := range []string{"RX", "TX"} {
				name := fmt.Sprintf("DCGM_FI_PROF_NVLINK_L%d_%s_BYTES", link, direction)
				alias(name, "DCGM_FI_PROF_NVLINK_"+direction+"_BYTES")
				update(name, func(d *fieldDefinition) { d.Link = linkID })
			}
		}
	}
	for _, direction := range []string{"", "RX_", "TX_"} {
		alias("DCGM_FI_DEV_NVLINK_"+direction+"THROUGHPUT_PER_LINK", "DCGM_FI_DEV_NVLINK_"+direction+"BANDWIDTH_TOTAL")
	}
	for field, dim := range map[string]string{"PCIE_LINK_GEN": "link_gen", "PCIE_MAX_LINK_GEN": "max_link_gen"} {
		dev(field, "interconnect.pcie.link.generation", dim, sampleGauge, 1)
	}
	for field, dim := range map[string]string{"PCIE_LINK_WIDTH": "link_width", "PCIE_MAX_LINK_WIDTH": "max_link_width"} {
		dev(field, "interconnect.pcie.link.width", dim, sampleGauge, 1)
	}
	dev("PCIE_REPLAY_COUNTER", "interconnect.pcie.error_rate", "pcie_replay", sampleCounter, 1)
	for _, name := range []string{"NVLINK_CRC_DATA_ERROR_COUNT", "NVLINK_CRC_FLIT_ERROR_COUNT", "NVLINK_REPLAY_ERROR_COUNT", "NVLINK_RECOVERY_ERROR_COUNT", "NVLINK_ECC_DATA_ERROR_COUNT"} {
		dim := strings.ToLower(strings.TrimSuffix(name, "_COUNT"))
		dev(name+"_TOTAL", "interconnect.nvlink.error_rate", dim, sampleCounter, 1)
		if name == "NVLINK_ECC_DATA_ERROR_COUNT" {
			alias("DCGM_FI_DEV_NVLINK_ECC_ERROR_TOTAL", "DCGM_FI_DEV_"+name+"_TOTAL")
			continue
		}
		alias("DCGM_FI_DEV_"+strings.TrimSuffix(name, "_COUNT")+"_TOTAL", "DCGM_FI_DEV_"+name+"_TOTAL")
		for i := 0; i < 18; i++ {
			field := fmt.Sprintf("DCGM_FI_DEV_%s_L%d", name, i)
			add(field, "interconnect.nvlink.error_rate", dim, sampleCounter, 1)
			update(field, func(d *fieldDefinition) { d.Link = fmt.Sprint(i) })
			alias(fmt.Sprintf("DCGM_FI_DEV_%s_L%d_TOTAL", strings.TrimSuffix(name, "_COUNT"), i), field)
		}
	}
	for _, name := range []string{"NVLINK_ERROR_DL_CRC", "NVLINK_ERROR_DL_REPLAY", "NVLINK_ERROR_DL_RECOVERY"} {
		dev(name, "interconnect.nvlink.error_rate", strings.ToLower(name), sampleCounter, 1)
	}
	for _, direction := range []string{"RX", "TX"} {
		dev("NVLINK_COUNT_"+direction+"_PACKETS", "interconnect.nvlink.traffic", strings.ToLower(direction), sampleCounter, 1)
	}
	for _, kind := range []string{"EFFECTIVE", "SYMBOL"} {
		name := "DCGM_FI_DEV_NVLINK_COUNT_" + kind + "_BER"
		add(name, "interconnect.nvlink.ber", strings.ToLower(kind), sampleGauge, 1e12)
		// Packed values preserve precision that exporter %f formatting loses for doubles.
		update(name, func(d *fieldDefinition) { d.DecodeBER = true; d.Precision = 1e6; d.Priority = 20 })
		alias(name+"_FLOAT", name)
		update(name+"_FLOAT", func(d *fieldDefinition) { d.DecodeBER = false; d.Priority = 10 })
		alias("DCGM_FI_DEV_NVLINK_"+kind+"_BER_RATIO", name+"_FLOAT")
		alias("DCGM_FI_DEV_NVLINK_"+kind+"_BER_RAW", name)
	}
	dev("FABRIC_MANAGER_STATUS", "interconnect.fabric", "", sampleGauge, 1)
	update("DCGM_FI_DEV_FABRIC_MANAGER_STATUS", func(d *fieldDefinition) {
		d.States = []string{"not_supported", "not_started", "in_progress", "success", "failure", "unrecognized", "nvml_too_old"}
	})
	dev("FABRIC_MANAGER_ERROR_CODE", "interconnect.fabric_error", "error", sampleGauge, 1)
	dev("C2C_MAX_BANDWIDTH", "interconnect.c2c.capacity", "maximum", sampleGauge, 1e6)
	dev("C2C_LINK_COUNT", "interconnect.c2c.links", "links", sampleGauge, 1)
	for _, state := range []string{"STATUS", "POWER_STATE"} {
		dev("C2C_LINK_"+state, "interconnect.c2c.state", strings.ToLower(state), sampleGauge, 1)
	}
	for _, kind := range []string{"ALL", "DATA"} {
		group := "interconnect.c2c.throughput"
		if kind == "DATA" {
			group = "interconnect.c2c.payload"
		}
		for _, direction := range []string{"RX", "TX"} {
			prof("C2C_"+direction+"_"+kind+"_BYTES", group, strings.ToLower(direction), 1)
		}
	}
	for _, kind := range []string{"INTR", "REPLAY", "REPLAY_B2B"} {
		dev("C2C_LINK_ERROR_"+kind, "interconnect.c2c.error_rate", strings.ToLower(kind), sampleCounter, 1)
	}
	for _, prefix := range []string{"", "LINK_"} {
		group := "interconnect.nvswitch.sxid"
		if prefix != "" {
			group = "interconnect.nvswitch.link_sxid"
		}
		for _, severity := range []string{"FATAL", "NON_FATAL"} {
			dev("NVSWITCH_"+prefix+severity+"_ERRORS", group, strings.ToLower(severity), sampleGauge, 1)
		}
	}
	for _, kind := range []string{"REPLAY", "RECOVERY", "CRC", "ECC"} {
		dev("NVSWITCH_LINK_"+kind+"_ERRORS", "interconnect.nvswitch.errors", strings.ToLower(kind), sampleCounter, 1)
		alias("DCGM_FI_DEV_NVSWITCH_LINK_"+kind+"_ERROR_TOTAL", "DCGM_FI_DEV_NVSWITCH_LINK_"+kind+"_ERRORS")
	}
	for i := 0; i < 8; i++ {
		dev(fmt.Sprintf("NVSWITCH_LINK_ECC_ERRORS_LANE%d", i), "interconnect.nvswitch.ecc_lanes", fmt.Sprintf("lane%d", i), sampleCounter, 1)
		alias(fmt.Sprintf("DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERROR_L%d_TOTAL", i), fmt.Sprintf("DCGM_FI_DEV_NVSWITCH_LINK_ECC_ERRORS_LANE%d", i))
	}
	dev("NVSWITCH_TEMPERATURE_CURRENT", "interconnect.nvswitch.temperature", "current", sampleGauge, 1)

	// TMP is Total Module Power: its ceiling and floor are Watts, not temperatures.
	for suffix, group := range map[string]string{
		"APPLIED_TMP_CEIL": "bounds", "APPLIED_TMP_FLOOR": "bounds",
		"MAX_PERCENT_TMP_FLOOR_SETTING": "floor", "MIN_PERCENT_TMP_FLOOR_SETTING": "floor", "PROFILE_PERCENT_TMP_FLOOR": "floor", "ADMIN_OVERRIDE_PERCENT_TMP_FLOOR": "floor",
		"HW_CIRCUITRY_PERCENT_LIFETIME_REMAINING": "lifetime",
		"PROFILE_RAMP_UP_RATE":                    "ramp", "PROFILE_RAMP_DOWN_RATE": "ramp", "ADMIN_OVERRIDE_RAMP_UP_RATE": "ramp", "ADMIN_OVERRIDE_RAMP_DOWN_RATE": "ramp",
		"PROFILE_RAMP_DOWN_HYST_VAL": "hysteresis", "ADMIN_OVERRIDE_RAMP_DOWN_HYST_VAL": "hysteresis",
		"ENABLED": "state", "IMM_RAMP_DOWN_ENABLED": "state", "PRIV_LVL": "privilege", "ACTIVE_PRESET_PROFILE": "profile", "MAX_NUM_PRESET_PROFILES": "profile_capacity",
	} {
		scale := 1.0
		if group == "ramp" {
			scale = 1e-3
		}
		dev("PWR_SMOOTHING_"+suffix, "power.smoothing."+group, strings.ToLower(suffix), sampleGauge, scale)
	}
	for name, group := range map[string]string{"CLOCK_EVENTS_COUNT": "throttle.event_samples", "CLOCK_EVENTS_TOTAL": "throttle.event_rate", "XID_ERRORS_COUNT": "reliability.xid_samples", "XID_ERRORS_TOTAL": "reliability.xid_rate", "GPU_HEALTH_STATUS": "health.status", "P2P_STATUS": "interconnect.p2p"} {
		kind := sampleGauge
		if strings.HasSuffix(name, "_TOTAL") {
			kind = sampleCounter
		}
		field := "DCGM_EXP_" + name
		add(field, group, "value", kind, 1)
		update(field, func(d *fieldDefinition) {
			switch name {
			case "GPU_HEALTH_STATUS":
				d.Labels = []string{"health_watch"}
			case "P2P_STATUS":
				d.Labels = []string{"peer_gpu"}
			default:
				if strings.HasPrefix(name, "CLOCK_") {
					d.Labels = []string{"clock_event"}
				} else {
					d.Labels = []string{"xid"}
				}
			}
			d.Window = strings.HasSuffix(name, "_COUNT")
		})
	}
	// Exact aliases in the v4.6.1 field header share the same sample semantics.
	for name, original := range map[string]string{
		"BANK_REMAP_AVAIL_HIGH": "BANKS_REMAP_ROWS_AVAIL_HIGH", "BANK_REMAP_AVAIL_LOW": "BANKS_REMAP_ROWS_AVAIL_LOW",
		"BANK_REMAP_AVAIL_MAX": "BANKS_REMAP_ROWS_AVAIL_MAX", "BANK_REMAP_AVAIL_NONE": "BANKS_REMAP_ROWS_AVAIL_NONE", "BANK_REMAP_AVAIL_PARTIAL": "BANKS_REMAP_ROWS_AVAIL_PARTIAL",
		"BOARD_POWER_LIMIT_DEFAULT_WATTS": "POWER_MGMT_LIMIT_DEF", "BOARD_POWER_LIMIT_ENFORCED_WATTS": "ENFORCED_POWER_LIMIT", "BOARD_POWER_LIMIT_MAX_WATTS": "POWER_MGMT_LIMIT_MAX", "BOARD_POWER_LIMIT_MIN_WATTS": "POWER_MGMT_LIMIT_MIN", "BOARD_POWER_LIMIT_REQUESTED_WATTS": "POWER_MGMT_LIMIT",
		"C2C_LINK_QUANTITY": "C2C_LINK_COUNT", "CLOCKS_AUTOBOOST_MODE": "AUTOBOOST", "FB_USED_RATIO": "FB_USED_PERCENT", "GPU_RECOVERY_ACTION": "GET_GPU_RECOVERY_ACTION", "GPU_VIRTUAL_MODE": "VIRTUAL_MODE", "MEMORY_UNREPAIRABLE": "MEMORY_UNREPAIRABLE_FLAG",
		"NVLINK_CRC_ERROR_TOTAL": "NVLINK_ERROR_DL_CRC", "NVLINK_RECOVERY_TOTAL": "NVLINK_ERROR_DL_RECOVERY", "NVLINK_REPLAY_TOTAL": "NVLINK_ERROR_DL_REPLAY",
		"NVLINK_RX_PACKET_TOTAL": "NVLINK_COUNT_RX_PACKETS", "NVLINK_TX_PACKET_TOTAL": "NVLINK_COUNT_TX_PACKETS",
		"NVSWITCH_TEMP_CELSIUS": "NVSWITCH_TEMPERATURE_CURRENT", "SXID_FATAL_ERROR": "NVSWITCH_FATAL_ERRORS", "SXID_NON_FATAL_ERROR": "NVSWITCH_NON_FATAL_ERRORS",
	} {
		alias("DCGM_FI_DEV_"+name, "DCGM_FI_DEV_"+original)
	}
	// Explicit aliases retain the old field's units even when the new name says RATIO.
	for name, original := range map[string]string{
		"CPU_TEMP_CELSIUS": "CPU_TEMP_CURRENT", "CPU_TEMP_WARNING_CELSIUS": "CPU_TEMP_WARNING", "CPU_TEMP_CRITICAL_CELSIUS": "CPU_TEMP_CRITICAL",
		"GPU_UTIL_RATIO": "GPU_UTIL", "GPU_TEMP_CELSIUS": "GPU_TEMP", "MEMORY_TEMP_CELSIUS": "MEMORY_TEMP", "GPU_TEMP_MARGIN_CELSIUS": "GPU_TEMP_LIMIT",
		"GPU_MAX_OP_TEMP_CELSIUS": "GPU_MAX_OP_TEMP", "MEMORY_MAX_OP_TEMP_CELSIUS": "MEM_MAX_OP_TEMP", "GPU_TEMP_SLOWDOWN_CELSIUS": "SLOWDOWN_TEMP", "GPU_TEMP_SHUTDOWN_CELSIUS": "SHUTDOWN_TEMP",
		"BOARD_POWER_WATTS": "POWER_USAGE", "BOARD_POWER_RAW_WATTS": "POWER_USAGE_INSTANT", "GPU_PSTATE": "PSTATE", "GPU_COMPUTE_MODE": "COMPUTE_MODE", "GPU_PERSISTENCE_MODE": "PERSISTENCE_MODE",
		"ECC_MODE": "ECC_CURRENT", "PAGE_RETIRED_PENDING": "RETIRED_PENDING", "PAGE_RETIRED_SBE_TOTAL": "RETIRED_SBE", "PAGE_RETIRED_DBE_TOTAL": "RETIRED_DBE",
		"ROW_REMAP_FAILED": "ROW_REMAP_FAILURE", "ROW_REMAP_CORRECTABLE_TOTAL": "CORRECTABLE_REMAPPED_ROWS", "ROW_REMAP_UNCORRECTABLE_TOTAL": "UNCORRECTABLE_REMAPPED_ROWS", "SRAM_EXCEEDED": "THRESHOLD_SRM",
		"PCIE_REPLAY_TOTAL": "PCIE_REPLAY_COUNTER", "XID_ERROR": "XID_ERRORS", "FABRIC_MANAGER_ERROR": "FABRIC_MANAGER_ERROR_CODE",
	} {
		alias("DCGM_FI_DEV_"+name, "DCGM_FI_DEV_"+original)
	}
	alias("DCGM_FI_SYSTEM_GPU_QUANTITY", "DCGM_FI_DEV_COUNT")
	for name, original := range map[string]string{"GR_ENGINE_UTIL_RATIO": "GR_ENGINE_ACTIVE", "SM_UTIL_RATIO": "SM_ACTIVE", "SM_OCCUPANCY_RATIO": "SM_OCCUPANCY", "TENSOR_UTIL_RATIO": "PIPE_TENSOR_ACTIVE", "DRAM_UTIL_RATIO": "DRAM_ACTIVE", "FP16_UTIL_RATIO": "PIPE_FP16_ACTIVE", "FP32_UTIL_RATIO": "PIPE_FP32_ACTIVE", "FP64_UTIL_RATIO": "PIPE_FP64_ACTIVE", "INT_UTIL_RATIO": "PIPE_INT_ACTIVE", "HMMA_UTIL_RATIO": "PIPE_TENSOR_HMMA_ACTIVE", "IMMA_UTIL_RATIO": "PIPE_TENSOR_IMMA_ACTIVE", "DFMA_UTIL_RATIO": "PIPE_TENSOR_DFMA_ACTIVE"} {
		alias("DCGM_FI_PROF_"+name, "DCGM_FI_PROF_"+original)
	}
	// Event payloads and backend-specific counters have no established common unit.
	for _, name := range []string{"DCGM_FI_DEV_GPU_NVLINK_ERRORS", "DCGM_FI_DEV_NVLINK_ERROR"} {
		fields[name] = fieldDefinition{
			Kind: sampleGauge,
			Raw:  true,
		}
	}
	return fields
}
