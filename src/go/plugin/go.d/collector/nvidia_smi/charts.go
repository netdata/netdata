// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioGPUPCIBandwidthUsage = module.Priority + iota
	prioGPUPCIBandwidthUtilization
	prioGPUFanSpeed
	prioGPUUtilization
	prioGPUMemUtilization
	prioGPUDecoderUtilization
	prioGPUEncoderUtilization
	prioGPUMIGModeStatus
	prioGPUMIGDevicesCount
	prioGPUFBMemoryUsage
	prioGPUMIGFBMemoryUsage
	prioGPUBAR1MemoryUsage
	prioGPUMIGBAR1MemoryUsage
	prioGPUTemperatureChart
	prioGPUVoltageChart
	prioGPUClockFreq
	prioGPUPowerDraw
	prioGPUPerformanceState
)

var (
	gpuXMLCharts = module.Charts{
		gpuPCIBandwidthUsageChartTmpl.Copy(),
		gpuPCIBandwidthUtilizationChartTmpl.Copy(),
		gpuFanSpeedPercChartTmpl.Copy(),
		gpuUtilizationChartTmpl.Copy(),
		gpuMemUtilizationChartTmpl.Copy(),
		gpuDecoderUtilizationChartTmpl.Copy(),
		gpuEncoderUtilizationChartTmpl.Copy(),
		gpuMIGModeCurrentStatusChartTmpl.Copy(),
		gpuMIGDevicesCountChartTmpl.Copy(),
		gpuFrameBufferMemoryUsageChartTmpl.Copy(),
		gpuBAR1MemoryUsageChartTmpl.Copy(),
		gpuVoltageChartTmpl.Copy(),
		gpuTemperatureChartTmpl.Copy(),
		gpuClockFreqChartTmpl.Copy(),
		gpuPowerDrawChartTmpl.Copy(),
		gpuPerformanceStateChartTmpl.Copy(),
	}
	migDeviceXMLCharts = module.Charts{
		migDeviceFrameBufferMemoryUsageChartTmpl.Copy(),
		migDeviceBAR1MemoryUsageChartTmpl.Copy(),
	}
)

var (
	gpuPCIBandwidthUsageChartTmpl = module.Chart{
		ID:       "gpu_%s_pcie_bandwidth_usage",
		Title:    "PCI Express Bandwidth Usage",
		Units:    "B/s",
		Fam:      "pcie bandwidth",
		Ctx:      "nvidia_smi.gpu_pcie_bandwidth_usage",
		Type:     module.Area,
		Priority: prioGPUPCIBandwidthUsage,
		Dims: module.Dims{
			{ID: "gpu_%s_pcie_bandwidth_usage_rx", Name: "rx"},
			{ID: "gpu_%s_pcie_bandwidth_usage_tx", Name: "tx", Mul: -1},
		},
	}
	gpuPCIBandwidthUtilizationChartTmpl = module.Chart{
		ID:       "gpu_%s_pcie_bandwidth_utilization",
		Title:    "PCI Express Bandwidth Utilization",
		Units:    "percentage",
		Fam:      "pcie bandwidth",
		Ctx:      "nvidia_smi.gpu_pcie_bandwidth_utilization",
		Priority: prioGPUPCIBandwidthUtilization,
		Dims: module.Dims{
			{ID: "gpu_%s_pcie_bandwidth_utilization_rx", Name: "rx", Div: 100},
			{ID: "gpu_%s_pcie_bandwidth_utilization_tx", Name: "tx", Div: 100},
		},
	}
	gpuFanSpeedPercChartTmpl = module.Chart{
		ID:       "gpu_%s_fan_speed_perc",
		Title:    "Fan speed",
		Units:    "%",
		Fam:      "fan speed",
		Ctx:      "nvidia_smi.gpu_fan_speed_perc",
		Priority: prioGPUFanSpeed,
		Dims: module.Dims{
			{ID: "gpu_%s_fan_speed_perc", Name: "fan_speed"},
		},
	}
	gpuUtilizationChartTmpl = module.Chart{
		ID:       "gpu_%s_gpu_utilization",
		Title:    "GPU utilization",
		Units:    "%",
		Fam:      "gpu utilization",
		Ctx:      "nvidia_smi.gpu_utilization",
		Priority: prioGPUUtilization,
		Dims: module.Dims{
			{ID: "gpu_%s_gpu_utilization", Name: "gpu"},
		},
	}
	gpuMemUtilizationChartTmpl = module.Chart{
		ID:       "gpu_%s_memory_utilization",
		Title:    "Memory utilization",
		Units:    "%",
		Fam:      "mem utilization",
		Ctx:      "nvidia_smi.gpu_memory_utilization",
		Priority: prioGPUMemUtilization,
		Dims: module.Dims{
			{ID: "gpu_%s_mem_utilization", Name: "memory"},
		},
	}
	gpuDecoderUtilizationChartTmpl = module.Chart{
		ID:       "gpu_%s_decoder_utilization",
		Title:    "Decoder utilization",
		Units:    "%",
		Fam:      "dec utilization",
		Ctx:      "nvidia_smi.gpu_decoder_utilization",
		Priority: prioGPUDecoderUtilization,
		Dims: module.Dims{
			{ID: "gpu_%s_decoder_utilization", Name: "decoder"},
		},
	}
	gpuEncoderUtilizationChartTmpl = module.Chart{
		ID:       "gpu_%s_encoder_utilization",
		Title:    "Encoder utilization",
		Units:    "%",
		Fam:      "enc utilization",
		Ctx:      "nvidia_smi.gpu_encoder_utilization",
		Priority: prioGPUEncoderUtilization,
		Dims: module.Dims{
			{ID: "gpu_%s_encoder_utilization", Name: "encoder"},
		},
	}
	gpuMIGModeCurrentStatusChartTmpl = module.Chart{
		ID:       "gpu_%s_mig_mode_current_status",
		Title:    "MIG current mode",
		Units:    "status",
		Fam:      "mig",
		Ctx:      "nvidia_smi.gpu_mig_mode_current_status",
		Priority: prioGPUMIGModeStatus,
		Dims: module.Dims{
			{ID: "gpu_%s_mig_current_mode_enabled", Name: "enabled"},
			{ID: "gpu_%s_mig_current_mode_disabled", Name: "disabled"},
		},
	}
	gpuMIGDevicesCountChartTmpl = module.Chart{
		ID:       "gpu_%s_mig_devices_count",
		Title:    "MIG devices",
		Units:    "devices",
		Fam:      "mig",
		Ctx:      "nvidia_smi.gpu_mig_devices_count",
		Priority: prioGPUMIGDevicesCount,
		Dims: module.Dims{
			{ID: "gpu_%s_mig_devices_count", Name: "mig"},
		},
	}
	gpuFrameBufferMemoryUsageChartTmpl = module.Chart{
		ID:       "gpu_%s_frame_buffer_memory_usage",
		Title:    "Frame buffer memory usage",
		Units:    "B",
		Fam:      "fb mem usage",
		Ctx:      "nvidia_smi.gpu_frame_buffer_memory_usage",
		Type:     module.Stacked,
		Priority: prioGPUFBMemoryUsage,
		Dims: module.Dims{
			{ID: "gpu_%s_frame_buffer_memory_usage_free", Name: "free"},
			{ID: "gpu_%s_frame_buffer_memory_usage_used", Name: "used"},
			{ID: "gpu_%s_frame_buffer_memory_usage_reserved", Name: "reserved"},
		},
	}
	gpuBAR1MemoryUsageChartTmpl = module.Chart{
		ID:       "gpu_%s_bar1_memory_usage",
		Title:    "BAR1 memory usage",
		Units:    "B",
		Fam:      "bar1 mem usage",
		Ctx:      "nvidia_smi.gpu_bar1_memory_usage",
		Type:     module.Stacked,
		Priority: prioGPUBAR1MemoryUsage,
		Dims: module.Dims{
			{ID: "gpu_%s_bar1_memory_usage_free", Name: "free"},
			{ID: "gpu_%s_bar1_memory_usage_used", Name: "used"},
		},
	}
	gpuTemperatureChartTmpl = module.Chart{
		ID:       "gpu_%s_temperature",
		Title:    "Temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "nvidia_smi.gpu_temperature",
		Priority: prioGPUTemperatureChart,
		Dims: module.Dims{
			{ID: "gpu_%s_temperature", Name: "temperature"},
		},
	}
	gpuVoltageChartTmpl = module.Chart{
		ID:       "gpu_%s_voltage",
		Title:    "Voltage",
		Units:    "V",
		Fam:      "voltage",
		Ctx:      "nvidia_smi.gpu_voltage",
		Priority: prioGPUVoltageChart,
		Dims: module.Dims{
			{ID: "gpu_%s_voltage", Name: "voltage", Div: 1000}, // mV => V
		},
	}
	gpuClockFreqChartTmpl = module.Chart{
		ID:       "gpu_%s_clock_freq",
		Title:    "Clock current frequency",
		Units:    "MHz",
		Fam:      "clocks",
		Ctx:      "nvidia_smi.gpu_clock_freq",
		Priority: prioGPUClockFreq,
		Dims: module.Dims{
			{ID: "gpu_%s_graphics_clock", Name: "graphics"},
			{ID: "gpu_%s_video_clock", Name: "video"},
			{ID: "gpu_%s_sm_clock", Name: "sm"},
			{ID: "gpu_%s_mem_clock", Name: "mem"},
		},
	}
	gpuPowerDrawChartTmpl = module.Chart{
		ID:       "gpu_%s_power_draw",
		Title:    "Power draw",
		Units:    "Watts",
		Fam:      "power draw",
		Ctx:      "nvidia_smi.gpu_power_draw",
		Priority: prioGPUPowerDraw,
		Dims: module.Dims{
			{ID: "gpu_%s_power_draw", Name: "power_draw"},
		},
	}
	gpuPerformanceStateChartTmpl = module.Chart{
		ID:       "gpu_%s_performance_state",
		Title:    "Performance state",
		Units:    "state",
		Fam:      "performance state",
		Ctx:      "nvidia_smi.gpu_performance_state",
		Priority: prioGPUPerformanceState,
		Dims: module.Dims{
			{ID: "gpu_%s_performance_state_P0", Name: "P0"},
			{ID: "gpu_%s_performance_state_P1", Name: "P1"},
			{ID: "gpu_%s_performance_state_P2", Name: "P2"},
			{ID: "gpu_%s_performance_state_P3", Name: "P3"},
			{ID: "gpu_%s_performance_state_P4", Name: "P4"},
			{ID: "gpu_%s_performance_state_P5", Name: "P5"},
			{ID: "gpu_%s_performance_state_P6", Name: "P6"},
			{ID: "gpu_%s_performance_state_P7", Name: "P7"},
			{ID: "gpu_%s_performance_state_P8", Name: "P8"},
			{ID: "gpu_%s_performance_state_P9", Name: "P9"},
			{ID: "gpu_%s_performance_state_P10", Name: "P10"},
			{ID: "gpu_%s_performance_state_P11", Name: "P11"},
			{ID: "gpu_%s_performance_state_P12", Name: "P12"},
			{ID: "gpu_%s_performance_state_P13", Name: "P13"},
			{ID: "gpu_%s_performance_state_P14", Name: "P14"},
			{ID: "gpu_%s_performance_state_P15", Name: "P15"},
		},
	}
)

func (c *Collector) addGpuCharts(gpu gpuInfo, index int) {
	charts := gpuXMLCharts.Copy()

	if !isValidValue(gpu.Utilization.GpuUtil) {
		_ = charts.Remove(gpuUtilizationChartTmpl.ID)
	}
	if !isValidValue(gpu.Utilization.MemoryUtil) {
		_ = charts.Remove(gpuMemUtilizationChartTmpl.ID)
	}
	if !isValidValue(gpu.Utilization.DecoderUtil) {
		_ = charts.Remove(gpuDecoderUtilizationChartTmpl.ID)
	}
	if !isValidValue(gpu.Utilization.EncoderUtil) {
		_ = charts.Remove(gpuEncoderUtilizationChartTmpl.ID)
	}
	if !isValidValue(gpu.MIGMode.CurrentMIG) {
		_ = charts.Remove(gpuMIGModeCurrentStatusChartTmpl.ID)
		_ = charts.Remove(gpuMIGDevicesCountChartTmpl.ID)
	}
	if !isValidValue(gpu.FanSpeed) {
		_ = charts.Remove(gpuFanSpeedPercChartTmpl.ID)
	}
	if (gpu.PowerReadings == nil || !isValidValue(gpu.PowerReadings.PowerDraw)) &&
		(gpu.GPUPowerReadings == nil || !isValidValue(gpu.GPUPowerReadings.PowerDraw)) {
		_ = charts.Remove(gpuPowerDrawChartTmpl.ID)
	}
	if !isValidValue(gpu.Voltage.GraphicsVolt) {
		_ = charts.Remove(gpuVoltageChartTmpl.ID)
	}

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, strings.ToLower(gpu.UUID))
		c.Labels = []module.Label{
			{Key: "index", Value: strconv.Itoa(index)},
			{Key: "uuid", Value: gpu.UUID},
			{Key: "product_name", Value: gpu.ProductName},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, gpu.UUID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

var (
	migDeviceFrameBufferMemoryUsageChartTmpl = module.Chart{
		ID:       "mig_instance_%s_gpu_%s_frame_buffer_memory_usage",
		Title:    "MIG Frame buffer memory usage",
		Units:    "B",
		Fam:      "fb mem usage",
		Ctx:      "nvidia_smi.gpu_mig_frame_buffer_memory_usage",
		Type:     module.Stacked,
		Priority: prioGPUMIGFBMemoryUsage,
		Dims: module.Dims{
			{ID: "mig_instance_%s_gpu_%s_frame_buffer_memory_usage_free", Name: "free"},
			{ID: "mig_instance_%s_gpu_%s_frame_buffer_memory_usage_used", Name: "used"},
			{ID: "mig_instance_%s_gpu_%s_frame_buffer_memory_usage_reserved", Name: "reserved"},
		},
	}
	migDeviceBAR1MemoryUsageChartTmpl = module.Chart{
		ID:       "mig_instance_%s_gpu_%s_bar1_memory_usage",
		Title:    "MIG BAR1 memory usage",
		Units:    "B",
		Fam:      "bar1 mem usage",
		Ctx:      "nvidia_smi.gpu_mig_bar1_memory_usage",
		Type:     module.Stacked,
		Priority: prioGPUMIGBAR1MemoryUsage,
		Dims: module.Dims{
			{ID: "mig_instance_%s_gpu_%s_bar1_memory_usage_free", Name: "free"},
			{ID: "mig_instance_%s_gpu_%s_bar1_memory_usage_used", Name: "used"},
		},
	}
)

func (c *Collector) addMIGDeviceCharts(gpu gpuInfo, mig gpuMIGDeviceInfo) {
	charts := migDeviceXMLCharts.Copy()

	for _, c := range *charts {
		c.ID = fmt.Sprintf(c.ID, strings.ToLower(mig.GPUInstanceID), strings.ToLower(gpu.UUID))
		c.Labels = []module.Label{
			{Key: "gpu_uuid", Value: gpu.UUID},
			{Key: "gpu_product_name", Value: gpu.ProductName},
			{Key: "gpu_instance_id", Value: mig.GPUInstanceID},
		}
		for _, d := range c.Dims {
			d.ID = fmt.Sprintf(d.ID, mig.GPUInstanceID, gpu.UUID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCharts(prefix string) {
	prefix = strings.ToLower(prefix)

	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}
