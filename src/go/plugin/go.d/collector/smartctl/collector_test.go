// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataTypeSataScan, _         = os.ReadFile("testdata/type-sat/scan.json")
	dataTypeSataDeviceHDDSda, _ = os.ReadFile("testdata/type-sat/device-hdd-sda.json")
	dataTypeSataDeviceSSDSdc, _ = os.ReadFile("testdata/type-sat/device-ssd-sdc.json")

	dataTypeNvmeScan, _        = os.ReadFile("testdata/type-nvme/scan.json")
	dataTypeNvmeDeviceNvme0, _ = os.ReadFile("testdata/type-nvme/device-nvme0.json")
	dataTypeNvmeDeviceNvme1, _ = os.ReadFile("testdata/type-nvme/device-nvme1.json")

	dataTypeScsiScan, _      = os.ReadFile("testdata/type-scsi/scan.json")
	dataTypeScsiDeviceSda, _ = os.ReadFile("testdata/type-scsi/device-sda.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataTypeSataScan":         dataTypeSataScan,
		"dataTypeSataDeviceHDDSda": dataTypeSataDeviceHDDSda,
		"dataTypeSataDeviceSSDSdc": dataTypeSataDeviceSSDSdc,

		"dataTypeNvmeScan":        dataTypeNvmeScan,
		"dataTypeNvmeDeviceNvme0": dataTypeNvmeDeviceNvme0,
		"dataTypeNvmeDeviceNvme1": dataTypeNvmeDeviceNvme1,

		"dataTypeScsiScan":      dataTypeScsiScan,
		"dataTypeScsiDeviceSda": dataTypeScsiDeviceSda,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if invalid power mode": {
			wantFail: true,
			config: func() Config {
				cfg := New().Config
				cfg.NoCheckPowerMode = "invalid"
				return cfg
			}(),
		},
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOkTypeSata()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOkTypeSata()
				_ = collr.Collect(context.Background())
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSmartctlCliExec
		wantFail    bool
	}{
		"success type sata devices": {
			wantFail:    false,
			prepareMock: prepareMockOkTypeSata,
		},
		"success type nvme devices": {
			wantFail:    false,
			prepareMock: prepareMockOkTypeNvme,
		},
		"error on scan": {
			wantFail:    true,
			prepareMock: prepareMockErrOnScan,
		},
		"unexpected response on scan": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
		"empty response on scan": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock   func() *mockSmartctlCliExec
		prepareConfig func() Config
		wantMetrics   map[string]int64
		wantCharts    int
	}{
		"success type sata devices": {
			prepareMock: prepareMockOkTypeSata,
			wantCharts:  68,
			wantMetrics: map[string]int64{
				"device_sda_type_sat_ata_smart_error_log_summary_count":       0,
				"device_sda_type_sat_attr_current_pending_sector_decoded":     0,
				"device_sda_type_sat_attr_current_pending_sector_normalized":  100,
				"device_sda_type_sat_attr_current_pending_sector_raw":         0,
				"device_sda_type_sat_attr_load_cycle_count_decoded":           360,
				"device_sda_type_sat_attr_load_cycle_count_normalized":        100,
				"device_sda_type_sat_attr_load_cycle_count_raw":               360,
				"device_sda_type_sat_attr_offline_uncorrectable_decoded":      0,
				"device_sda_type_sat_attr_offline_uncorrectable_normalized":   100,
				"device_sda_type_sat_attr_offline_uncorrectable_raw":          0,
				"device_sda_type_sat_attr_power-off_retract_count_decoded":    360,
				"device_sda_type_sat_attr_power-off_retract_count_normalized": 100,
				"device_sda_type_sat_attr_power-off_retract_count_raw":        360,
				"device_sda_type_sat_attr_power_cycle_count_decoded":          12,
				"device_sda_type_sat_attr_power_cycle_count_normalized":       100,
				"device_sda_type_sat_attr_power_cycle_count_raw":              12,
				"device_sda_type_sat_attr_power_on_hours_decoded":             8244,
				"device_sda_type_sat_attr_power_on_hours_normalized":          99,
				"device_sda_type_sat_attr_power_on_hours_raw":                 8244,
				"device_sda_type_sat_attr_raw_read_error_rate_decoded":        0,
				"device_sda_type_sat_attr_raw_read_error_rate_normalized":     100,
				"device_sda_type_sat_attr_raw_read_error_rate_raw":            0,
				"device_sda_type_sat_attr_reallocated_event_count_decoded":    0,
				"device_sda_type_sat_attr_reallocated_event_count_normalized": 100,
				"device_sda_type_sat_attr_reallocated_event_count_raw":        0,
				"device_sda_type_sat_attr_reallocated_sector_ct_decoded":      0,
				"device_sda_type_sat_attr_reallocated_sector_ct_normalized":   100,
				"device_sda_type_sat_attr_reallocated_sector_ct_raw":          0,
				"device_sda_type_sat_attr_seek_error_rate_decoded":            0,
				"device_sda_type_sat_attr_seek_error_rate_normalized":         100,
				"device_sda_type_sat_attr_seek_error_rate_raw":                0,
				"device_sda_type_sat_attr_seek_time_performance_decoded":      15,
				"device_sda_type_sat_attr_seek_time_performance_normalized":   140,
				"device_sda_type_sat_attr_seek_time_performance_raw":          15,
				"device_sda_type_sat_attr_spin_retry_count_decoded":           0,
				"device_sda_type_sat_attr_spin_retry_count_normalized":        100,
				"device_sda_type_sat_attr_spin_retry_count_raw":               0,
				"device_sda_type_sat_attr_spin_up_time_decoded":               281,
				"device_sda_type_sat_attr_spin_up_time_normalized":            86,
				"device_sda_type_sat_attr_spin_up_time_raw":                   25788088601,
				"device_sda_type_sat_attr_start_stop_count_decoded":           12,
				"device_sda_type_sat_attr_start_stop_count_normalized":        100,
				"device_sda_type_sat_attr_start_stop_count_raw":               12,
				"device_sda_type_sat_attr_temperature_celsius_decoded":        49,
				"device_sda_type_sat_attr_temperature_celsius_normalized":     43,
				"device_sda_type_sat_attr_temperature_celsius_raw":            240519741489,
				"device_sda_type_sat_attr_throughput_performance_decoded":     48,
				"device_sda_type_sat_attr_throughput_performance_normalized":  148,
				"device_sda_type_sat_attr_throughput_performance_raw":         48,
				"device_sda_type_sat_attr_udma_crc_error_count_decoded":       0,
				"device_sda_type_sat_attr_udma_crc_error_count_normalized":    100,
				"device_sda_type_sat_attr_udma_crc_error_count_raw":           0,
				"device_sda_type_sat_attr_unknown_attribute_decoded":          100,
				"device_sda_type_sat_attr_unknown_attribute_normalized":       100,
				"device_sda_type_sat_attr_unknown_attribute_raw":              100,
				"device_sda_type_sat_power_cycle_count":                       12,
				"device_sda_type_sat_power_on_time":                           29678400,
				"device_sda_type_sat_smart_status_failed":                     0,
				"device_sda_type_sat_smart_status_passed":                     1,
				"device_sda_type_sat_temperature":                             49,
				"device_sdc_type_sat_ata_smart_error_log_summary_count":       0,
				"device_sdc_type_sat_attr_available_reservd_space_decoded":    100,
				"device_sdc_type_sat_attr_available_reservd_space_normalized": 100,
				"device_sdc_type_sat_attr_available_reservd_space_raw":        100,
				"device_sdc_type_sat_attr_command_timeout_decoded":            0,
				"device_sdc_type_sat_attr_command_timeout_normalized":         100,
				"device_sdc_type_sat_attr_command_timeout_raw":                0,
				"device_sdc_type_sat_attr_end-to-end_error_decoded":           0,
				"device_sdc_type_sat_attr_end-to-end_error_normalized":        100,
				"device_sdc_type_sat_attr_end-to-end_error_raw":               0,
				"device_sdc_type_sat_attr_media_wearout_indicator_decoded":    65406,
				"device_sdc_type_sat_attr_media_wearout_indicator_normalized": 100,
				"device_sdc_type_sat_attr_media_wearout_indicator_raw":        65406,
				"device_sdc_type_sat_attr_power_cycle_count_decoded":          13,
				"device_sdc_type_sat_attr_power_cycle_count_normalized":       100,
				"device_sdc_type_sat_attr_power_cycle_count_raw":              13,
				"device_sdc_type_sat_attr_power_on_hours_decoded":             8244,
				"device_sdc_type_sat_attr_power_on_hours_normalized":          100,
				"device_sdc_type_sat_attr_power_on_hours_raw":                 8244,
				"device_sdc_type_sat_attr_reallocated_sector_ct_decoded":      0,
				"device_sdc_type_sat_attr_reallocated_sector_ct_normalized":   100,
				"device_sdc_type_sat_attr_reallocated_sector_ct_raw":          0,
				"device_sdc_type_sat_attr_reported_uncorrect_decoded":         0,
				"device_sdc_type_sat_attr_reported_uncorrect_normalized":      100,
				"device_sdc_type_sat_attr_reported_uncorrect_raw":             0,
				"device_sdc_type_sat_attr_temperature_celsius_decoded":        27,
				"device_sdc_type_sat_attr_temperature_celsius_normalized":     73,
				"device_sdc_type_sat_attr_temperature_celsius_raw":            184684970011,
				"device_sdc_type_sat_attr_total_lbas_read_decoded":            76778,
				"device_sdc_type_sat_attr_total_lbas_read_normalized":         253,
				"device_sdc_type_sat_attr_total_lbas_read_raw":                76778,
				"device_sdc_type_sat_attr_total_lbas_written_decoded":         173833,
				"device_sdc_type_sat_attr_total_lbas_written_normalized":      253,
				"device_sdc_type_sat_attr_total_lbas_written_raw":             173833,
				"device_sdc_type_sat_attr_udma_crc_error_count_decoded":       0,
				"device_sdc_type_sat_attr_udma_crc_error_count_normalized":    100,
				"device_sdc_type_sat_attr_udma_crc_error_count_raw":           0,
				"device_sdc_type_sat_attr_unknown_attribute_decoded":          0,
				"device_sdc_type_sat_attr_unknown_attribute_normalized":       0,
				"device_sdc_type_sat_attr_unknown_attribute_raw":              0,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_decoded":      4694419309637,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_normalized":   4,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_raw":          4694419309637,
				"device_sdc_type_sat_power_cycle_count":                       13,
				"device_sdc_type_sat_power_on_time":                           29678400,
				"device_sdc_type_sat_smart_status_failed":                     0,
				"device_sdc_type_sat_smart_status_passed":                     1,
				"device_sdc_type_sat_temperature":                             27,
			},
		},
		"success type sata devices concurrent": {
			prepareMock: prepareMockOkTypeSata,
			prepareConfig: func() Config {
				cfg := New().Config
				cfg.ConcurrentScans = 2
				return cfg
			},
			wantCharts: 68,
			wantMetrics: map[string]int64{
				"device_sda_type_sat_ata_smart_error_log_summary_count":       0,
				"device_sda_type_sat_attr_current_pending_sector_decoded":     0,
				"device_sda_type_sat_attr_current_pending_sector_normalized":  100,
				"device_sda_type_sat_attr_current_pending_sector_raw":         0,
				"device_sda_type_sat_attr_load_cycle_count_decoded":           360,
				"device_sda_type_sat_attr_load_cycle_count_normalized":        100,
				"device_sda_type_sat_attr_load_cycle_count_raw":               360,
				"device_sda_type_sat_attr_offline_uncorrectable_decoded":      0,
				"device_sda_type_sat_attr_offline_uncorrectable_normalized":   100,
				"device_sda_type_sat_attr_offline_uncorrectable_raw":          0,
				"device_sda_type_sat_attr_power-off_retract_count_decoded":    360,
				"device_sda_type_sat_attr_power-off_retract_count_normalized": 100,
				"device_sda_type_sat_attr_power-off_retract_count_raw":        360,
				"device_sda_type_sat_attr_power_cycle_count_decoded":          12,
				"device_sda_type_sat_attr_power_cycle_count_normalized":       100,
				"device_sda_type_sat_attr_power_cycle_count_raw":              12,
				"device_sda_type_sat_attr_power_on_hours_decoded":             8244,
				"device_sda_type_sat_attr_power_on_hours_normalized":          99,
				"device_sda_type_sat_attr_power_on_hours_raw":                 8244,
				"device_sda_type_sat_attr_raw_read_error_rate_decoded":        0,
				"device_sda_type_sat_attr_raw_read_error_rate_normalized":     100,
				"device_sda_type_sat_attr_raw_read_error_rate_raw":            0,
				"device_sda_type_sat_attr_reallocated_event_count_decoded":    0,
				"device_sda_type_sat_attr_reallocated_event_count_normalized": 100,
				"device_sda_type_sat_attr_reallocated_event_count_raw":        0,
				"device_sda_type_sat_attr_reallocated_sector_ct_decoded":      0,
				"device_sda_type_sat_attr_reallocated_sector_ct_normalized":   100,
				"device_sda_type_sat_attr_reallocated_sector_ct_raw":          0,
				"device_sda_type_sat_attr_seek_error_rate_decoded":            0,
				"device_sda_type_sat_attr_seek_error_rate_normalized":         100,
				"device_sda_type_sat_attr_seek_error_rate_raw":                0,
				"device_sda_type_sat_attr_seek_time_performance_decoded":      15,
				"device_sda_type_sat_attr_seek_time_performance_normalized":   140,
				"device_sda_type_sat_attr_seek_time_performance_raw":          15,
				"device_sda_type_sat_attr_spin_retry_count_decoded":           0,
				"device_sda_type_sat_attr_spin_retry_count_normalized":        100,
				"device_sda_type_sat_attr_spin_retry_count_raw":               0,
				"device_sda_type_sat_attr_spin_up_time_decoded":               281,
				"device_sda_type_sat_attr_spin_up_time_normalized":            86,
				"device_sda_type_sat_attr_spin_up_time_raw":                   25788088601,
				"device_sda_type_sat_attr_start_stop_count_decoded":           12,
				"device_sda_type_sat_attr_start_stop_count_normalized":        100,
				"device_sda_type_sat_attr_start_stop_count_raw":               12,
				"device_sda_type_sat_attr_temperature_celsius_decoded":        49,
				"device_sda_type_sat_attr_temperature_celsius_normalized":     43,
				"device_sda_type_sat_attr_temperature_celsius_raw":            240519741489,
				"device_sda_type_sat_attr_throughput_performance_decoded":     48,
				"device_sda_type_sat_attr_throughput_performance_normalized":  148,
				"device_sda_type_sat_attr_throughput_performance_raw":         48,
				"device_sda_type_sat_attr_udma_crc_error_count_decoded":       0,
				"device_sda_type_sat_attr_udma_crc_error_count_normalized":    100,
				"device_sda_type_sat_attr_udma_crc_error_count_raw":           0,
				"device_sda_type_sat_attr_unknown_attribute_decoded":          100,
				"device_sda_type_sat_attr_unknown_attribute_normalized":       100,
				"device_sda_type_sat_attr_unknown_attribute_raw":              100,
				"device_sda_type_sat_power_cycle_count":                       12,
				"device_sda_type_sat_power_on_time":                           29678400,
				"device_sda_type_sat_smart_status_failed":                     0,
				"device_sda_type_sat_smart_status_passed":                     1,
				"device_sda_type_sat_temperature":                             49,
				"device_sdc_type_sat_ata_smart_error_log_summary_count":       0,
				"device_sdc_type_sat_attr_available_reservd_space_decoded":    100,
				"device_sdc_type_sat_attr_available_reservd_space_normalized": 100,
				"device_sdc_type_sat_attr_available_reservd_space_raw":        100,
				"device_sdc_type_sat_attr_command_timeout_decoded":            0,
				"device_sdc_type_sat_attr_command_timeout_normalized":         100,
				"device_sdc_type_sat_attr_command_timeout_raw":                0,
				"device_sdc_type_sat_attr_end-to-end_error_decoded":           0,
				"device_sdc_type_sat_attr_end-to-end_error_normalized":        100,
				"device_sdc_type_sat_attr_end-to-end_error_raw":               0,
				"device_sdc_type_sat_attr_media_wearout_indicator_decoded":    65406,
				"device_sdc_type_sat_attr_media_wearout_indicator_normalized": 100,
				"device_sdc_type_sat_attr_media_wearout_indicator_raw":        65406,
				"device_sdc_type_sat_attr_power_cycle_count_decoded":          13,
				"device_sdc_type_sat_attr_power_cycle_count_normalized":       100,
				"device_sdc_type_sat_attr_power_cycle_count_raw":              13,
				"device_sdc_type_sat_attr_power_on_hours_decoded":             8244,
				"device_sdc_type_sat_attr_power_on_hours_normalized":          100,
				"device_sdc_type_sat_attr_power_on_hours_raw":                 8244,
				"device_sdc_type_sat_attr_reallocated_sector_ct_decoded":      0,
				"device_sdc_type_sat_attr_reallocated_sector_ct_normalized":   100,
				"device_sdc_type_sat_attr_reallocated_sector_ct_raw":          0,
				"device_sdc_type_sat_attr_reported_uncorrect_decoded":         0,
				"device_sdc_type_sat_attr_reported_uncorrect_normalized":      100,
				"device_sdc_type_sat_attr_reported_uncorrect_raw":             0,
				"device_sdc_type_sat_attr_temperature_celsius_decoded":        27,
				"device_sdc_type_sat_attr_temperature_celsius_normalized":     73,
				"device_sdc_type_sat_attr_temperature_celsius_raw":            184684970011,
				"device_sdc_type_sat_attr_total_lbas_read_decoded":            76778,
				"device_sdc_type_sat_attr_total_lbas_read_normalized":         253,
				"device_sdc_type_sat_attr_total_lbas_read_raw":                76778,
				"device_sdc_type_sat_attr_total_lbas_written_decoded":         173833,
				"device_sdc_type_sat_attr_total_lbas_written_normalized":      253,
				"device_sdc_type_sat_attr_total_lbas_written_raw":             173833,
				"device_sdc_type_sat_attr_udma_crc_error_count_decoded":       0,
				"device_sdc_type_sat_attr_udma_crc_error_count_normalized":    100,
				"device_sdc_type_sat_attr_udma_crc_error_count_raw":           0,
				"device_sdc_type_sat_attr_unknown_attribute_decoded":          0,
				"device_sdc_type_sat_attr_unknown_attribute_normalized":       0,
				"device_sdc_type_sat_attr_unknown_attribute_raw":              0,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_decoded":      4694419309637,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_normalized":   4,
				"device_sdc_type_sat_attr_unknown_ssd_attribute_raw":          4694419309637,
				"device_sdc_type_sat_power_cycle_count":                       13,
				"device_sdc_type_sat_power_on_time":                           29678400,
				"device_sdc_type_sat_smart_status_failed":                     0,
				"device_sdc_type_sat_smart_status_passed":                     1,
				"device_sdc_type_sat_temperature":                             27,
			},
		},
		"success type nvme devices": {
			prepareMock: prepareMockOkTypeNvme,
			wantCharts:  4,
			wantMetrics: map[string]int64{
				"device_nvme0_type_nvme_power_cycle_count":   2,
				"device_nvme0_type_nvme_power_on_time":       11206800,
				"device_nvme0_type_nvme_smart_status_failed": 0,
				"device_nvme0_type_nvme_smart_status_passed": 1,
				"device_nvme0_type_nvme_temperature":         39,
			},
		},
		"success type nvme devices with extra": {
			prepareMock: prepareMockOkTypeNvme,
			prepareConfig: func() Config {
				cfg := New().Config
				cfg.ExtraDevices = []ConfigExtraDevice{
					{Name: "/dev/nvme1", Type: "nvme"},
				}
				return cfg
			},
			wantCharts: 8,
			wantMetrics: map[string]int64{
				"device_nvme0_type_nvme_power_cycle_count":   2,
				"device_nvme0_type_nvme_power_on_time":       11206800,
				"device_nvme0_type_nvme_smart_status_failed": 0,
				"device_nvme0_type_nvme_smart_status_passed": 1,
				"device_nvme0_type_nvme_temperature":         39,
				"device_nvme1_type_nvme_power_cycle_count":   5,
				"device_nvme1_type_nvme_power_on_time":       17038800,
				"device_nvme1_type_nvme_smart_status_failed": 0,
				"device_nvme1_type_nvme_smart_status_passed": 1,
				"device_nvme1_type_nvme_temperature":         36,
			},
		},
		"success type scsi devices": {
			prepareMock: prepareMockOkTypeScsi,
			wantCharts:  7,
			wantMetrics: map[string]int64{
				"device_sda_type_scsi_power_cycle_count":                              4,
				"device_sda_type_scsi_power_on_time":                                  5908920,
				"device_sda_type_scsi_scsi_error_log_read_total_errors_corrected":     647736,
				"device_sda_type_scsi_scsi_error_log_read_total_uncorrected_errors":   0,
				"device_sda_type_scsi_scsi_error_log_verify_total_errors_corrected":   0,
				"device_sda_type_scsi_scsi_error_log_verify_total_uncorrected_errors": 0,
				"device_sda_type_scsi_scsi_error_log_write_total_errors_corrected":    0,
				"device_sda_type_scsi_scsi_error_log_write_total_uncorrected_errors":  0,
				"device_sda_type_scsi_smart_status_failed":                            0,
				"device_sda_type_scsi_smart_status_passed":                            1,
				"device_sda_type_scsi_temperature":                                    34,
			},
		},
		"error on scan": {
			prepareMock: prepareMockErrOnScan,
		},
		"unexpected response on scan": {
			prepareMock: prepareMockUnexpectedResponse,
		},
		"empty response on scan": {
			prepareMock: prepareMockEmptyResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if test.prepareConfig != nil {
				collr.Config = test.prepareConfig()
			}
			mock := test.prepareMock()
			collr.exec = mock
			collr.ScanEvery = confopt.Duration(time.Microsecond * 1)
			collr.PollDevicesEvery = confopt.Duration(time.Microsecond * 1)

			var mx map[string]int64
			for i := 0; i < 10; i++ {
				mx = collr.Collect(context.Background())
			}

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantCharts, "wantCharts")

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareMockOkTypeSata() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		errOnScan: false,
		scanData:  dataTypeSataScan,
		deviceDataFunc: func(deviceName, deviceType, powerMode string) ([]byte, error) {
			if deviceType != "sat" {
				return nil, fmt.Errorf("unexpected device type %s", deviceType)
			}
			switch deviceName {
			case "/dev/sda":
				return dataTypeSataDeviceHDDSda, nil
			case "/dev/sdc":
				return dataTypeSataDeviceSSDSdc, nil
			default:
				return nil, fmt.Errorf("unexpected device name %s", deviceName)
			}
		},
	}
}

func prepareMockOkTypeNvme() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		errOnScan: false,
		scanData:  dataTypeNvmeScan,
		deviceDataFunc: func(deviceName, deviceType, powerMode string) ([]byte, error) {
			if deviceType != "nvme" {
				return nil, fmt.Errorf("unexpected device type %s", deviceType)
			}
			switch deviceName {
			case "/dev/nvme0":
				return dataTypeNvmeDeviceNvme0, nil
			case "/dev/nvme1":
				return dataTypeNvmeDeviceNvme1, nil
			default:
				return nil, fmt.Errorf("unexpected device name %s", deviceName)
			}
		},
	}
}

func prepareMockOkTypeScsi() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		errOnScan: false,
		scanData:  dataTypeScsiScan,
		deviceDataFunc: func(deviceName, deviceType, powerMode string) ([]byte, error) {
			if deviceType != "scsi" {
				return nil, fmt.Errorf("unexpected device type %s", deviceType)
			}
			switch deviceName {
			case "/dev/sda":
				return dataTypeScsiDeviceSda, nil
			default:
				return nil, fmt.Errorf("unexpected device name %s", deviceName)
			}
		},
	}
}

func prepareMockErrOnScan() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		errOnScan: true,
	}
}

func prepareMockUnexpectedResponse() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		scanData:       []byte(randomJsonData),
		deviceDataFunc: func(_, _, _ string) ([]byte, error) { return []byte(randomJsonData), nil },
	}
}

func prepareMockEmptyResponse() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{}
}

type mockSmartctlCliExec struct {
	errOnScan      bool
	scanData       []byte
	deviceDataFunc func(deviceName, deviceType, powerMode string) ([]byte, error)
}

func (m *mockSmartctlCliExec) scan(_ bool) (*gjson.Result, error) {
	if m.errOnScan {
		return nil, fmt.Errorf("mock.scan() error")
	}
	res := gjson.ParseBytes(m.scanData)
	return &res, nil
}

func (m *mockSmartctlCliExec) deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error) {
	if m.deviceDataFunc == nil {
		return nil, nil
	}
	bs, err := m.deviceDataFunc(deviceName, deviceType, powerMode)
	if err != nil {
		return nil, err
	}
	res := gjson.ParseBytes(bs)
	return &res, nil
}

var randomJsonData = `
{
    "elephant": {
        "burn": false,
        "mountain": true,
        "fog": false,
        "skin": -1561907625,
        "burst": "anyway",
        "shadow": 1558616893
    },
    "start": "ever",
    "base": 2093056027,
    "mission": -2007590351,
    "victory": 999053756,
    "die": false
}
`
