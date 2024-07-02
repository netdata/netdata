// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

func (nv *NvidiaSMI) collectGPUInfoXML(mx map[string]int64) error {
	bs, err := nv.exec.queryGPUInfoXML()
	if err != nil {
		return fmt.Errorf("error on quering XML GPU info: %v", err)
	}

	info := &xmlInfo{}
	if err := xml.Unmarshal(bs, info); err != nil {
		return fmt.Errorf("error on unmarshaling XML GPU info response: %v", err)
	}

	seenGPU := make(map[string]bool)
	seenMIG := make(map[string]bool)

	for _, gpu := range info.GPUs {
		if !isValidValue(gpu.UUID) {
			continue
		}

		px := "gpu_" + gpu.UUID + "_"

		seenGPU[px] = true

		if !nv.gpus[px] {
			nv.gpus[px] = true
			nv.addGPUXMLCharts(gpu)
		}

		addMetric(mx, px+"pcie_bandwidth_usage_rx", gpu.PCI.RxUtil, 1024) // KB => bytes
		addMetric(mx, px+"pcie_bandwidth_usage_tx", gpu.PCI.TxUtil, 1024) // KB => bytes
		if max := calcMaxPCIEBandwidth(gpu); max > 0 {
			rx := parseFloat(gpu.PCI.RxUtil) * 1024 // KB => bytes
			tx := parseFloat(gpu.PCI.TxUtil) * 1024 // KB => bytes
			mx[px+"pcie_bandwidth_utilization_rx"] = int64((rx * 100 / max) * 100)
			mx[px+"pcie_bandwidth_utilization_tx"] = int64((tx * 100 / max) * 100)
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
			mx[px+"performance_state_"+s] = boolToInt(gpu.PerformanceState == s)
		}
		if isValidValue(gpu.MIGMode.CurrentMIG) {
			mode := strings.ToLower(gpu.MIGMode.CurrentMIG)
			mx[px+"mig_current_mode_enabled"] = boolToInt(mode == "enabled")
			mx[px+"mig_current_mode_disabled"] = boolToInt(mode == "disabled")
			mx[px+"mig_devices_count"] = int64(len(gpu.MIGDevices.MIGDevice))
		}

		for _, mig := range gpu.MIGDevices.MIGDevice {
			if !isValidValue(mig.GPUInstanceID) {
				continue
			}

			px := "mig_instance_" + mig.GPUInstanceID + "_" + px

			seenMIG[px] = true

			if !nv.migs[px] {
				nv.migs[px] = true
				nv.addMIGDeviceXMLCharts(gpu, mig)
			}

			addMetric(mx, px+"ecc_error_sram_uncorrectable", mig.ECCErrorCount.VolatileCount.SRAMUncorrectable, 0)
			addMetric(mx, px+"frame_buffer_memory_usage_free", mig.FBMemoryUsage.Free, 1024*1024)         // MiB => bytes
			addMetric(mx, px+"frame_buffer_memory_usage_used", mig.FBMemoryUsage.Used, 1024*1024)         // MiB => bytes
			addMetric(mx, px+"frame_buffer_memory_usage_reserved", mig.FBMemoryUsage.Reserved, 1024*1024) // MiB => bytes
			addMetric(mx, px+"bar1_memory_usage_free", mig.BAR1MemoryUsage.Free, 1024*1024)               // MiB => bytes
			addMetric(mx, px+"bar1_memory_usage_used", mig.BAR1MemoryUsage.Used, 1024*1024)               // MiB => bytes
		}
	}

	for px := range nv.gpus {
		if !seenGPU[px] {
			delete(nv.gpus, px)
			nv.removeCharts(px)
		}
	}

	for px := range nv.migs {
		if !seenMIG[px] {
			delete(nv.migs, px)
			nv.removeCharts(px)
		}
	}

	return nil
}

func calcMaxPCIEBandwidth(gpu xmlGPUInfo) float64 {
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

type (
	xmlInfo struct {
		GPUs []xmlGPUInfo `xml:"gpu"`
	}
	xmlGPUInfo struct {
		ID                  string `xml:"id,attr"`
		ProductName         string `xml:"product_name"`
		ProductBrand        string `xml:"product_brand"`
		ProductArchitecture string `xml:"product_architecture"`
		UUID                string `xml:"uuid"`
		FanSpeed            string `xml:"fan_speed"`
		PerformanceState    string `xml:"performance_state"`
		MIGMode             struct {
			CurrentMIG string `xml:"current_mig"`
		} `xml:"mig_mode"`
		MIGDevices struct {
			MIGDevice []xmlMIGDeviceInfo `xml:"mig_device"`
		} `xml:"mig_devices"`
		PCI struct {
			TxUtil         string `xml:"tx_util"`
			RxUtil         string `xml:"rx_util"`
			PCIGPULinkInfo struct {
				PCIEGen struct {
					MaxLinkGen string `xml:"max_link_gen"`
				} `xml:"pcie_gen"`
				LinkWidths struct {
					MaxLinkWidth string `xml:"max_link_width"`
				} `xml:"link_widths"`
			} `xml:"pci_gpu_link_info"`
		} `xml:"pci"`
		Utilization struct {
			GpuUtil     string `xml:"gpu_util"`
			MemoryUtil  string `xml:"memory_util"`
			EncoderUtil string `xml:"encoder_util"`
			DecoderUtil string `xml:"decoder_util"`
		} `xml:"utilization"`
		FBMemoryUsage struct {
			Total    string `xml:"total"`
			Reserved string `xml:"reserved"`
			Used     string `xml:"used"`
			Free     string `xml:"free"`
		} `xml:"fb_memory_usage"`
		Bar1MemoryUsage struct {
			Total string `xml:"total"`
			Used  string `xml:"used"`
			Free  string `xml:"free"`
		} `xml:"bar1_memory_usage"`
		Temperature struct {
			GpuTemp                string `xml:"gpu_temp"`
			GpuTempMaxThreshold    string `xml:"gpu_temp_max_threshold"`
			GpuTempSlowThreshold   string `xml:"gpu_temp_slow_threshold"`
			GpuTempMaxGpuThreshold string `xml:"gpu_temp_max_gpu_threshold"`
			GpuTargetTemperature   string `xml:"gpu_target_temperature"`
			MemoryTemp             string `xml:"memory_temp"`
			GpuTempMaxMemThreshold string `xml:"gpu_temp_max_mem_threshold"`
		} `xml:"temperature"`
		Clocks struct {
			GraphicsClock string `xml:"graphics_clock"`
			SmClock       string `xml:"sm_clock"`
			MemClock      string `xml:"mem_clock"`
			VideoClock    string `xml:"video_clock"`
		} `xml:"clocks"`
		PowerReadings    *xmlPowerReadings `xml:"power_readings"`
		GPUPowerReadings *xmlPowerReadings `xml:"gpu_power_readings"`
		Voltage          struct {
			GraphicsVolt string `xml:"graphics_volt"`
		} `xml:"voltage"`
		Processes struct {
			ProcessInfo []struct {
				PID         string `xml:"pid"`
				ProcessName string `xml:"process_name"`
				UsedMemory  string `xml:"used_memory"`
			} `sml:"process_info"`
		} `xml:"processes"`
	}

	xmlPowerReadings struct {
		//PowerState         string `xml:"power_state"`
		//PowerManagement    string `xml:"power_management"`
		PowerDraw string `xml:"power_draw"`
		//PowerLimit         string `xml:"power_limit"`
		//DefaultPowerLimit  string `xml:"default_power_limit"`
		//EnforcedPowerLimit string `xml:"enforced_power_limit"`
		//MinPowerLimit      string `xml:"min_power_limit"`
		//MaxPowerLimit      string `xml:"max_power_limit"`
	}

	xmlMIGDeviceInfo struct {
		Index             string `xml:"index"`
		GPUInstanceID     string `xml:"gpu_instance_id"`
		ComputeInstanceID string `xml:"compute_instance_id"`
		DeviceAttributes  struct {
			Shared struct {
				MultiprocessorCount string `xml:"multiprocessor_count"`
				CopyEngineCount     string `xml:"copy_engine_count"`
				EncoderCount        string `xml:"encoder_count"`
				DecoderCount        string `xml:"decoder_count"`
				OFACount            string `xml:"ofa_count"`
				JPGCount            string `xml:"jpg_count"`
			} `xml:"shared"`
		} `xml:"device_attributes"`
		ECCErrorCount struct {
			VolatileCount struct {
				SRAMUncorrectable string `xml:"sram_uncorrectable"`
			} `xml:"volatile_count"`
		} `xml:"ecc_error_count"`
		FBMemoryUsage struct {
			Free     string `xml:"free"`
			Used     string `xml:"used"`
			Reserved string `xml:"reserved"`
		} `xml:"fb_memory_usage"`
		BAR1MemoryUsage struct {
			Free string `xml:"free"`
			Used string `xml:"used"`
		} `xml:"bar1_memory_usage"`
	}
)
