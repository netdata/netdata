// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

type gpusInfo struct {
	GPUs []gpuInfo `xml:"gpu"`
}

type (
	gpuInfo struct {
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
			MIGDevice []gpuMIGDeviceInfo `xml:"mig_device"`
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
		PowerReadings    *gpuPowerReadings `xml:"power_readings"`
		GPUPowerReadings *gpuPowerReadings `xml:"gpu_power_readings"`
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
	gpuPowerReadings struct {
		//PowerState         string `xml:"power_state"`
		//PowerManagement    string `xml:"power_management"`
		PowerDraw        *string `xml:"power_draw"`
		AveragePowerDraw *string `xml:"average_power_draw"`
		InstantPowerDraw *string `xml:"instant_power_draw"`
		//PowerLimit         string `xml:"power_limit"`
		//DefaultPowerLimit  string `xml:"default_power_limit"`
		//EnforcedPowerLimit string `xml:"enforced_power_limit"`
		//MinPowerLimit      string `xml:"min_power_limit"`
		//MaxPowerLimit      string `xml:"max_power_limit"`
	}

	gpuMIGDeviceInfo struct {
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
