// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.exec == nil {
		return nil, errors.New("nvidia-smi exec is not initialized")
	}

	mx := make(map[string]int64)

	if err := c.collectGPUInfo(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectGPUInfo(mx map[string]int64) error {
	bs, err := c.exec.queryGPUInfo()
	if err != nil {
		return fmt.Errorf("error on quering XML GPU info: %v", err)
	}

	info := &gpusInfo{}
	if err := xml.Unmarshal(bs, info); err != nil {
		return fmt.Errorf("error on unmarshaling XML GPU info response: %v", err)
	}

	seenGPU := make(map[string]bool)
	seenMIG := make(map[string]bool)

	for i, gpu := range info.GPUs {
		if !isValidValue(gpu.UUID) {
			continue
		}

		px := "gpu_" + gpu.UUID + "_"

		seenGPU[px] = true

		if !c.gpus[px] {
			c.gpus[px] = true
			c.addGpuCharts(gpu, i)
		}

		addMetric(mx, px+"pcie_bandwidth_usage_rx", gpu.PCI.RxUtil, 1024) // KB => bytes
		addMetric(mx, px+"pcie_bandwidth_usage_tx", gpu.PCI.TxUtil, 1024) // KB => bytes
		if maxBw := calcMaxPCIEBandwidth(gpu); maxBw > 0 {
			rx := parseFloat(gpu.PCI.RxUtil) * 1024 // KB => bytes
			tx := parseFloat(gpu.PCI.TxUtil) * 1024 // KB => bytes
			mx[px+"pcie_bandwidth_utilization_rx"] = int64((rx * 100 / maxBw) * 100)
			mx[px+"pcie_bandwidth_utilization_tx"] = int64((tx * 100 / maxBw) * 100)
		}
		addMetric(mx, px+"fan_speed_perc", gpu.FanSpeed, 0)
		addMetric(mx, px+"gpu_utilization", gpu.Utilization.GpuUtil, 0)
		addMetric(mx, px+"mem_utilization", gpu.Utilization.MemoryUtil, 0)
		addMetric(mx, px+"decoder_utilization", gpu.Utilization.DecoderUtil, 0)
		addMetric(mx, px+"encoder_utilization", gpu.Utilization.EncoderUtil, 0)
		addMetric(mx, px+"frame_buffer_memory_usage_free", gpu.FBMemoryUsage.Free, 1024*1024)         // MiB => bytes
		addMetric(mx, px+"frame_buffer_memory_usage_used", gpu.FBMemoryUsage.Used, 1024*1024)         // MiB => bytes
		addMetric(mx, px+"frame_buffer_memory_usage_reserved", gpu.FBMemoryUsage.Reserved, 1024*1024) // MiB => bytes
		addMetric(mx, px+"bar1_memory_usage_free", gpu.Bar1MemoryUsage.Free, 1024*1024)               // MiB => bytes
		addMetric(mx, px+"bar1_memory_usage_used", gpu.Bar1MemoryUsage.Used, 1024*1024)               // MiB => bytes
		addMetric(mx, px+"temperature", gpu.Temperature.GpuTemp, 0)
		addMetric(mx, px+"graphics_clock", gpu.Clocks.GraphicsClock, 0)
		addMetric(mx, px+"video_clock", gpu.Clocks.VideoClock, 0)
		addMetric(mx, px+"sm_clock", gpu.Clocks.SmClock, 0)
		addMetric(mx, px+"mem_clock", gpu.Clocks.MemClock, 0)
		if gpu.PowerReadings != nil {
			addMetric(mx, px+"power_draw", gpu.PowerReadings.PowerDraw, 0)
		} else if gpu.GPUPowerReadings != nil {
			addMetric(mx, px+"power_draw", gpu.GPUPowerReadings.PowerDraw, 0)
		}
		addMetric(mx, px+"voltage", gpu.Voltage.GraphicsVolt, 0)
		for i := 0; i < 16; i++ {
			s := "P" + strconv.Itoa(i)
			mx[px+"performance_state_"+s] = metrix.Bool(gpu.PerformanceState == s)
		}
		if isValidValue(gpu.MIGMode.CurrentMIG) {
			mode := strings.ToLower(gpu.MIGMode.CurrentMIG)
			mx[px+"mig_current_mode_enabled"] = metrix.Bool(mode == "enabled")
			mx[px+"mig_current_mode_disabled"] = metrix.Bool(mode == "disabled")
			mx[px+"mig_devices_count"] = int64(len(gpu.MIGDevices.MIGDevice))
		}

		for _, mig := range gpu.MIGDevices.MIGDevice {
			if !isValidValue(mig.GPUInstanceID) {
				continue
			}

			px := "mig_instance_" + mig.GPUInstanceID + "_" + px

			seenMIG[px] = true

			if !c.migs[px] {
				c.migs[px] = true
				c.addMIGDeviceCharts(gpu, mig)
			}

			addMetric(mx, px+"ecc_error_sram_uncorrectable", mig.ECCErrorCount.VolatileCount.SRAMUncorrectable, 0)
			addMetric(mx, px+"frame_buffer_memory_usage_free", mig.FBMemoryUsage.Free, 1024*1024)         // MiB => bytes
			addMetric(mx, px+"frame_buffer_memory_usage_used", mig.FBMemoryUsage.Used, 1024*1024)         // MiB => bytes
			addMetric(mx, px+"frame_buffer_memory_usage_reserved", mig.FBMemoryUsage.Reserved, 1024*1024) // MiB => bytes
			addMetric(mx, px+"bar1_memory_usage_free", mig.BAR1MemoryUsage.Free, 1024*1024)               // MiB => bytes
			addMetric(mx, px+"bar1_memory_usage_used", mig.BAR1MemoryUsage.Used, 1024*1024)               // MiB => bytes
		}
	}

	for px := range c.gpus {
		if !seenGPU[px] {
			delete(c.gpus, px)
			c.removeCharts(px)
		}
	}

	for px := range c.migs {
		if !seenMIG[px] {
			delete(c.migs, px)
			c.removeCharts(px)
		}
	}

	return nil
}

func calcMaxPCIEBandwidth(gpu gpuInfo) float64 {
	gen := gpu.PCI.PCIGPULinkInfo.PCIEGen.MaxLinkGen
	width := strings.TrimSuffix(gpu.PCI.PCIGPULinkInfo.LinkWidths.MaxLinkWidth, "x")

	if !isValidValue(gen) || !isValidValue(width) {
		return 0
	}

	// https://enterprise-support.nvidia.com/s/article/understanding-pcie-configuration-for-maximum-performance
	var speed, enc float64
	switch gen {
	case "1":
		speed, enc = 2.5, 1.0/5.0
	case "2":
		speed, enc = 5, 1.0/5.0
	case "3":
		speed, enc = 8, 2.0/130.0
	case "4":
		speed, enc = 16, 2.0/130.0
	case "5":
		speed, enc = 32, 2.0/130.0
	default:
		return 0
	}

	// Maximum PCIe Bandwidth = SPEED * WIDTH * (1 - ENCODING) - 1Gb/s
	return (speed*parseFloat(width)*(1-enc) - 1) * 1e9 / 8 // Gb/s => bytes
}

func addMetric(mx map[string]int64, key, value string, mul int) {
	if !isValidValue(value) {
		return
	}

	value = removeUnits(value)

	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}

	if mul > 0 {
		v *= float64(mul)
	}

	mx[key] = int64(v)
}

func isValidValue(v string) bool {
	return v != "" && v != "N/A" && v != "[N/A]"
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(removeUnits(s), 64)
	return v
}

func removeUnits(s string) string {
	if i := strings.IndexByte(s, ' '); i != -1 {
		s = s[:i]
	}
	return s
}
