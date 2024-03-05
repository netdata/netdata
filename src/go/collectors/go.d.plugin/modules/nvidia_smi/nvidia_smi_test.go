// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataXMLRTX2080Win, _       = os.ReadFile("testdata/rtx-2080-win.xml")
	dataXMLRTX4090Driver535, _ = os.ReadFile("testdata/rtx-4090-driver-535.xml")
	dataXMLRTX3060, _          = os.ReadFile("testdata/rtx-3060.xml")
	dataXMLTeslaP100, _        = os.ReadFile("testdata/tesla-p100.xml")

	dataXMLA100SXM4MIG, _ = os.ReadFile("testdata/a100-sxm4-mig.xml")

	dataHelpQueryGPU, _ = os.ReadFile("testdata/help-query-gpu.txt")
	dataCSVTeslaP100, _ = os.ReadFile("testdata/tesla-p100.csv")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":          dataConfigJSON,
		"dataConfigYAML":          dataConfigYAML,
		"dataXMLRTX2080Win":       dataXMLRTX2080Win,
		"dataXMLRTX4090Driver535": dataXMLRTX4090Driver535,
		"dataXMLRTX3060":          dataXMLRTX3060,
		"dataXMLTeslaP100":        dataXMLTeslaP100,
		"dataXMLA100SXM4MIG":      dataXMLA100SXM4MIG,
		"dataHelpQueryGPU":        dataHelpQueryGPU,
		"dataCSVTeslaP100":        dataCSVTeslaP100,
	} {
		require.NotNil(t, data, name)
	}
}

func TestNvidiaSMI_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &NvidiaSMI{}, dataConfigJSON, dataConfigYAML)
}

func TestNvidiaSMI_Init(t *testing.T) {
	tests := map[string]struct {
		prepare  func(nv *NvidiaSMI)
		wantFail bool
	}{
		"fails if can't local nvidia-smi": {
			wantFail: true,
			prepare: func(nv *NvidiaSMI) {
				nv.binName += "!!!"
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			nv := New()

			test.prepare(nv)

			if test.wantFail {
				assert.Error(t, nv.Init())
			} else {
				assert.NoError(t, nv.Init())
			}
		})
	}
}

func TestNvidiaSMI_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestNvidiaSMI_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(nv *NvidiaSMI)
		wantFail bool
	}{
		"success A100-SXM4 MIG [XML]": {
			wantFail: false,
			prepare:  prepareCaseMIGA100formatXML,
		},
		"success RTX 3060 [XML]": {
			wantFail: false,
			prepare:  prepareCaseRTX3060formatXML,
		},
		"success Tesla P100 [XML]": {
			wantFail: false,
			prepare:  prepareCaseTeslaP100formatXML,
		},
		"success Tesla P100 [CSV]": {
			wantFail: false,
			prepare:  prepareCaseTeslaP100formatCSV,
		},
		"success RTX 2080 Win [XML]": {
			wantFail: false,
			prepare:  prepareCaseRTX2080WinFormatXML,
		},
		"fail on queryGPUInfoXML error": {
			wantFail: true,
			prepare:  prepareCaseErrOnQueryGPUInfoXML,
		},
		"fail on queryGPUInfoCSV error": {
			wantFail: true,
			prepare:  prepareCaseErrOnQueryGPUInfoCSV,
		},
		"fail on queryHelpQueryGPU error": {
			wantFail: true,
			prepare:  prepareCaseErrOnQueryHelpQueryGPU,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			nv := New()

			test.prepare(nv)

			if test.wantFail {
				assert.Error(t, nv.Check())
			} else {
				assert.NoError(t, nv.Check())
			}
		})
	}
}

func TestNvidiaSMI_Collect(t *testing.T) {
	type testCaseStep struct {
		prepare func(nv *NvidiaSMI)
		check   func(t *testing.T, nv *NvidiaSMI)
	}
	tests := map[string][]testCaseStep{
		"success A100-SXM4 MIG [XML]": {
			{
				prepare: prepareCaseMIGA100formatXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_free":                            68718428160,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_used":                            1048576,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_free":                    42273341440,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_reserved":                634388480,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_used":                    39845888,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_graphics_clock":                                    1410,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_mem_clock":                                         1215,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_mig_current_mode_disabled":                         0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_mig_current_mode_enabled":                          1,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_mig_devices_count":                                 2,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_pcie_bandwidth_usage_rx":                           0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_pcie_bandwidth_usage_tx":                           0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_pcie_bandwidth_utilization_rx":                     0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_pcie_bandwidth_utilization_tx":                     0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P0":                              1,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P1":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P10":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P11":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P12":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P13":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P14":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P15":                             0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P2":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P3":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P4":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P5":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P6":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P7":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P8":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_performance_state_P9":                              0,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_power_draw":                                        66,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_sm_clock":                                          1410,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_temperature":                                       36,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_video_clock":                                       1275,
						"gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_voltage":                                           881,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_free":             34358689792,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_used":             0,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_ecc_error_sram_uncorrectable":       0,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_free":     20916994048,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_reserved": 0,
						"mig_instance_1_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_used":     19922944,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_free":             34358689792,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_bar1_memory_usage_used":             0,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_ecc_error_sram_uncorrectable":       0,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_free":     20916994048,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_reserved": 0,
						"mig_instance_2_gpu_GPU-27b94a00-ed54-5c24-b1fd-1054085de32a_frame_buffer_memory_usage_used":     19922944,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success RTX 4090 Driver 535 [XML]": {
			{
				prepare: prepareCaseRTX4090Driver535formatXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_bar1_memory_usage_free":             267386880,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_bar1_memory_usage_used":             1048576,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_decoder_utilization":                0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_encoder_utilization":                0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_fan_speed_perc":                     0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_frame_buffer_memory_usage_free":     25390219264,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_frame_buffer_memory_usage_reserved": 362807296,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_frame_buffer_memory_usage_used":     2097152,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_gpu_utilization":                    0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_graphics_clock":                     210,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_mem_clock":                          405,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_mem_utilization":                    0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_pcie_bandwidth_usage_rx":            0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_pcie_bandwidth_usage_tx":            0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_pcie_bandwidth_utilization_rx":      0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_pcie_bandwidth_utilization_tx":      0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P0":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P1":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P10":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P11":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P12":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P13":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P14":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P15":              0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P2":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P3":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P4":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P5":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P6":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P7":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P8":               1,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_performance_state_P9":               0,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_power_draw":                         26,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_sm_clock":                           210,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_temperature":                        40,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_video_clock":                        1185,
						"gpu_GPU-71d1acc2-662d-2166-bf9f-65272d2fc437_voltage":                            880,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success RTX 3060 [XML]": {
			{
				prepare: prepareCaseRTX3060formatXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_bar1_memory_usage_free":             8586788864,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_bar1_memory_usage_used":             3145728,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_decoder_utilization":                0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_encoder_utilization":                0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_frame_buffer_memory_usage_free":     6228541440,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_frame_buffer_memory_usage_reserved": 206569472,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_frame_buffer_memory_usage_used":     5242880,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_gpu_utilization":                    0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_graphics_clock":                     210,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_mem_clock":                          405,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_mem_utilization":                    0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_pcie_bandwidth_usage_rx":            0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_pcie_bandwidth_usage_tx":            0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_pcie_bandwidth_utilization_rx":      0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_pcie_bandwidth_utilization_tx":      0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P0":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P1":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P10":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P11":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P12":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P13":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P14":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P15":              0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P2":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P3":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P4":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P5":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P6":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P7":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P8":               1,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_performance_state_P9":               0,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_power_draw":                         8,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_sm_clock":                           210,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_temperature":                        45,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_video_clock":                        555,
						"gpu_GPU-473d8d0f-d462-185c-6b36-6fc23e23e571_voltage":                            631,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success Tesla P100 [XML]": {
			{
				prepare: prepareCaseTeslaP100formatXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_bar1_memory_usage_free":             17177772032,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_bar1_memory_usage_used":             2097152,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_decoder_utilization":                0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_encoder_utilization":                0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_frame_buffer_memory_usage_free":     17070817280,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_frame_buffer_memory_usage_reserved": 108003328,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_frame_buffer_memory_usage_used":     0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_gpu_utilization":                    0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_graphics_clock":                     405,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_mem_clock":                          715,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_mem_utilization":                    0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_pcie_bandwidth_usage_rx":            0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_pcie_bandwidth_usage_tx":            0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_pcie_bandwidth_utilization_rx":      0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_pcie_bandwidth_utilization_tx":      0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P0":               1,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P1":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P10":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P11":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P12":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P13":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P14":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P15":              0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P2":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P3":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P4":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P5":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P6":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P7":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P8":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_performance_state_P9":               0,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_power_draw":                         26,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_sm_clock":                           405,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_temperature":                        38,
						"gpu_GPU-d3da8716-eaab-75db-efc1-60e88e1cd55e_video_clock":                        835,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success Tesla P100 [CSV]": {
			{
				prepare: prepareCaseTeslaP100formatCSV,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_frame_buffer_memory_usage_free":     17070817280,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_frame_buffer_memory_usage_reserved": 108003328,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_frame_buffer_memory_usage_used":     0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_gpu_utilization":                    0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_graphics_clock":                     405,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_mem_clock":                          715,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_mem_utilization":                    0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P0":               1,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P1":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P10":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P11":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P12":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P13":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P14":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P15":              0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P2":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P3":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P4":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P5":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P6":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P7":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P8":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_performance_state_P9":               0,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_power_draw":                         28,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_sm_clock":                           405,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_temperature":                        37,
						"gpu_GPU-ef1b2c9b-38d8-2090-2bd1-f567a3eb42a6_video_clock":                        835,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success RTX 2080 Win [XML]": {
			{
				prepare: prepareCaseRTX2080WinFormatXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					expected := map[string]int64{
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_bar1_memory_usage_free":             266338304,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_bar1_memory_usage_used":             2097152,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_decoder_utilization":                0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_encoder_utilization":                0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_fan_speed_perc":                     37,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_frame_buffer_memory_usage_free":     7494172672,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_frame_buffer_memory_usage_reserved": 190840832,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_frame_buffer_memory_usage_used":     903872512,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_gpu_utilization":                    2,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_graphics_clock":                     193,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_mem_clock":                          403,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_mem_utilization":                    7,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_pcie_bandwidth_usage_rx":            93184000,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_pcie_bandwidth_usage_tx":            13312000,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_pcie_bandwidth_utilization_rx":      59,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_pcie_bandwidth_utilization_tx":      8,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P0":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P1":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P10":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P11":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P12":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P13":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P14":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P15":              0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P2":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P3":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P4":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P5":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P6":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P7":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P8":               1,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_performance_state_P9":               0,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_power_draw":                         14,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_sm_clock":                           193,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_temperature":                        29,
						"gpu_GPU-fbd55ed4-1eec-4423-0a47-ad594b4333e3_video_clock":                        539,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"fail on queryGPUInfoXML error [XML]": {
			{
				prepare: prepareCaseErrOnQueryGPUInfoXML,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					assert.Equal(t, map[string]int64(nil), mx)
				},
			},
		},
		"fail on queryGPUInfoCSV error [CSV]": {
			{
				prepare: prepareCaseErrOnQueryGPUInfoCSV,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					assert.Equal(t, map[string]int64(nil), mx)
				},
			},
		},
		"fail on queryHelpQueryGPU error": {
			{
				prepare: prepareCaseErrOnQueryHelpQueryGPU,
				check: func(t *testing.T, nv *NvidiaSMI) {
					mx := nv.Collect()

					assert.Equal(t, map[string]int64(nil), mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			nv := New()

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepare(nv)
					step.check(t, nv)
				})
			}
		})
	}
}

type mockNvidiaSMI struct {
	gpuInfoXML           []byte
	errOnQueryGPUInfoXML bool

	gpuInfoCSV           []byte
	errOnQueryGPUInfoCSV bool

	helpQueryGPU           []byte
	errOnQueryHelpQueryGPU bool
}

func (m *mockNvidiaSMI) queryGPUInfoXML() ([]byte, error) {
	if m.errOnQueryGPUInfoXML {
		return nil, errors.New("error on mock.queryGPUInfoXML()")
	}
	return m.gpuInfoXML, nil
}

func (m *mockNvidiaSMI) queryGPUInfoCSV(_ []string) ([]byte, error) {
	if m.errOnQueryGPUInfoCSV {
		return nil, errors.New("error on mock.queryGPUInfoCSV()")
	}
	return m.gpuInfoCSV, nil
}

func (m *mockNvidiaSMI) queryHelpQueryGPU() ([]byte, error) {
	if m.errOnQueryHelpQueryGPU {
		return nil, errors.New("error on mock.queryHelpQueryGPU()")
	}
	return m.helpQueryGPU, nil
}

func prepareCaseMIGA100formatXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{gpuInfoXML: dataXMLA100SXM4MIG}
}

func prepareCaseRTX3060formatXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{gpuInfoXML: dataXMLRTX3060}
}

func prepareCaseRTX4090Driver535formatXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{gpuInfoXML: dataXMLRTX4090Driver535}
}

func prepareCaseTeslaP100formatXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{gpuInfoXML: dataXMLTeslaP100}
}

func prepareCaseRTX2080WinFormatXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{gpuInfoXML: dataXMLRTX2080Win}
}

func prepareCaseErrOnQueryGPUInfoXML(nv *NvidiaSMI) {
	nv.UseCSVFormat = false
	nv.exec = &mockNvidiaSMI{errOnQueryGPUInfoXML: true}
}

func prepareCaseTeslaP100formatCSV(nv *NvidiaSMI) {
	nv.UseCSVFormat = true
	nv.exec = &mockNvidiaSMI{helpQueryGPU: dataHelpQueryGPU, gpuInfoCSV: dataCSVTeslaP100}
}

func prepareCaseErrOnQueryHelpQueryGPU(nv *NvidiaSMI) {
	nv.UseCSVFormat = true
	nv.exec = &mockNvidiaSMI{errOnQueryHelpQueryGPU: true}
}

func prepareCaseErrOnQueryGPUInfoCSV(nv *NvidiaSMI) {
	nv.UseCSVFormat = true
	nv.exec = &mockNvidiaSMI{helpQueryGPU: dataHelpQueryGPU, errOnQueryGPUInfoCSV: true}
}
