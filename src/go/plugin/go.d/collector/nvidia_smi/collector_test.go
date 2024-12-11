// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*Collector)
		wantFail bool
	}{
		"fails if can't local nvidia-smi": {
			wantFail: true,
			prepare: func(collr *Collector) {
				collr.binName += "!!!"
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			test.prepare(collr)

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*Collector)
		wantFail bool
	}{
		"success A100-SXM4 MIG": {
			wantFail: false,
			prepare:  prepareCaseMIGA100,
		},
		"success RTX 3060": {
			wantFail: false,
			prepare:  prepareCaseRTX3060,
		},
		"success Tesla P100": {
			wantFail: false,
			prepare:  prepareCaseTeslaP100,
		},
		"success RTX 2080 Win": {
			wantFail: false,
			prepare:  prepareCaseRTX2080Win,
		},
		"fail on queryGPUInfo error": {
			wantFail: true,
			prepare:  prepareCaseErrOnQueryGPUInfo,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			test.prepare(collr)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	type testCaseStep struct {
		prepare func(*Collector)
		check   func(*testing.T, *Collector)
	}
	tests := map[string][]testCaseStep{
		"success A100-SXM4 MIG": {
			{
				prepare: prepareCaseMIGA100,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

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
		"success RTX 4090 Driver 535": {
			{
				prepare: prepareCaseRTX4090Driver535,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

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
		"success RTX 3060": {
			{
				prepare: prepareCaseRTX3060,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

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
		"success Tesla P100": {
			{
				prepare: prepareCaseTeslaP100,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

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
		"success RTX 2080 Win": {
			{
				prepare: prepareCaseRTX2080Win,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

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
		"fails on queryGPUInfo error": {
			{
				prepare: prepareCaseErrOnQueryGPUInfo,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					assert.Equal(t, map[string]int64(nil), mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepare(collr)
					step.check(t, collr)
				})
			}
		})
	}
}

type mockNvidiaSmi struct {
	gpuInfo           []byte
	errOnQueryGPUInfo bool
}

func (m *mockNvidiaSmi) queryGPUInfo() ([]byte, error) {
	if m.errOnQueryGPUInfo {
		return nil, errors.New("error on mock.queryGPUInfo()")
	}
	return m.gpuInfo, nil
}

func (m *mockNvidiaSmi) stop() error {
	return nil
}

func prepareCaseMIGA100(collr *Collector) {
	collr.exec = &mockNvidiaSmi{gpuInfo: dataXMLA100SXM4MIG}
}

func prepareCaseRTX3060(collr *Collector) {
	collr.exec = &mockNvidiaSmi{gpuInfo: dataXMLRTX3060}
}

func prepareCaseRTX4090Driver535(collr *Collector) {
	collr.exec = &mockNvidiaSmi{gpuInfo: dataXMLRTX4090Driver535}
}

func prepareCaseTeslaP100(collr *Collector) {
	collr.exec = &mockNvidiaSmi{gpuInfo: dataXMLTeslaP100}
}

func prepareCaseRTX2080Win(collr *Collector) {
	collr.exec = &mockNvidiaSmi{gpuInfo: dataXMLRTX2080Win}
}

func prepareCaseErrOnQueryGPUInfo(collr *Collector) {
	collr.exec = &mockNvidiaSmi{errOnQueryGPUInfo: true}
}
