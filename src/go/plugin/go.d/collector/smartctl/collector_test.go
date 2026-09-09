// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
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
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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

func wantMetricsTypeSata() map[string]int64 {
	return map[string]int64{
		"device_sda_type_sat_ata_smart_error_log_summary_count":       0,
		"device_sda_type_sat_attr_current_pending_sector_decoded":     0,
		"device_sda_type_sat_attr_current_pending_sector_normalized":  100,
		"device_sda_type_sat_attr_load_cycle_count_decoded":           360,
		"device_sda_type_sat_attr_load_cycle_count_normalized":        100,
		"device_sda_type_sat_attr_offline_uncorrectable_decoded":      0,
		"device_sda_type_sat_attr_offline_uncorrectable_normalized":   100,
		"device_sda_type_sat_attr_power-off_retract_count_decoded":    360,
		"device_sda_type_sat_attr_power-off_retract_count_normalized": 100,
		"device_sda_type_sat_attr_power_cycle_count_decoded":          12,
		"device_sda_type_sat_attr_power_cycle_count_normalized":       100,
		"device_sda_type_sat_attr_power_on_hours_decoded":             8244,
		"device_sda_type_sat_attr_power_on_hours_normalized":          99,
		"device_sda_type_sat_attr_raw_read_error_rate_decoded":        0,
		"device_sda_type_sat_attr_raw_read_error_rate_normalized":     100,
		"device_sda_type_sat_attr_reallocated_event_count_decoded":    0,
		"device_sda_type_sat_attr_reallocated_event_count_normalized": 100,
		"device_sda_type_sat_attr_reallocated_sector_ct_decoded":      0,
		"device_sda_type_sat_attr_reallocated_sector_ct_normalized":   100,
		"device_sda_type_sat_attr_seek_error_rate_decoded":            0,
		"device_sda_type_sat_attr_seek_error_rate_normalized":         100,
		"device_sda_type_sat_attr_seek_time_performance_decoded":      15,
		"device_sda_type_sat_attr_seek_time_performance_normalized":   140,
		"device_sda_type_sat_attr_spin_retry_count_decoded":           0,
		"device_sda_type_sat_attr_spin_retry_count_normalized":        100,
		"device_sda_type_sat_attr_spin_up_time_decoded":               281,
		"device_sda_type_sat_attr_spin_up_time_normalized":            86,
		"device_sda_type_sat_attr_start_stop_count_decoded":           12,
		"device_sda_type_sat_attr_start_stop_count_normalized":        100,
		"device_sda_type_sat_attr_temperature_celsius_decoded":        49,
		"device_sda_type_sat_attr_temperature_celsius_normalized":     43,
		"device_sda_type_sat_attr_throughput_performance_decoded":     48,
		"device_sda_type_sat_attr_throughput_performance_normalized":  148,
		"device_sda_type_sat_attr_udma_crc_error_count_decoded":       0,
		"device_sda_type_sat_attr_udma_crc_error_count_normalized":    100,
		"device_sda_type_sat_power_cycle_count":                       12,
		"device_sda_type_sat_power_on_time":                           29678400,
		"device_sda_type_sat_smart_status_failed":                     0,
		"device_sda_type_sat_smart_status_passed":                     1,
		"device_sda_type_sat_temperature":                             49,
		"device_sdc_type_sat_ata_smart_error_log_summary_count":       0,
		"device_sdc_type_sat_attr_available_reservd_space_decoded":    100,
		"device_sdc_type_sat_attr_available_reservd_space_normalized": 100,
		"device_sdc_type_sat_attr_command_timeout_decoded":            0,
		"device_sdc_type_sat_attr_command_timeout_normalized":         100,
		"device_sdc_type_sat_attr_end-to-end_error_decoded":           0,
		"device_sdc_type_sat_attr_end-to-end_error_normalized":        100,
		"device_sdc_type_sat_attr_media_wearout_indicator_decoded":    65406,
		"device_sdc_type_sat_attr_media_wearout_indicator_normalized": 100,
		"device_sdc_type_sat_attr_power_cycle_count_decoded":          13,
		"device_sdc_type_sat_attr_power_cycle_count_normalized":       100,
		"device_sdc_type_sat_attr_power_on_hours_decoded":             8244,
		"device_sdc_type_sat_attr_power_on_hours_normalized":          100,
		"device_sdc_type_sat_attr_reallocated_sector_ct_decoded":      0,
		"device_sdc_type_sat_attr_reallocated_sector_ct_normalized":   100,
		"device_sdc_type_sat_attr_reported_uncorrect_decoded":         0,
		"device_sdc_type_sat_attr_reported_uncorrect_normalized":      100,
		"device_sdc_type_sat_attr_temperature_celsius_decoded":        27,
		"device_sdc_type_sat_attr_temperature_celsius_normalized":     73,
		"device_sdc_type_sat_attr_total_lbas_read_decoded":            76778,
		"device_sdc_type_sat_attr_total_lbas_read_normalized":         253,
		"device_sdc_type_sat_attr_total_lbas_written_decoded":         173833,
		"device_sdc_type_sat_attr_total_lbas_written_normalized":      253,
		"device_sdc_type_sat_attr_udma_crc_error_count_decoded":       0,
		"device_sdc_type_sat_attr_udma_crc_error_count_normalized":    100,
		"device_sdc_type_sat_power_cycle_count":                       13,
		"device_sdc_type_sat_power_on_time":                           29678400,
		"device_sdc_type_sat_smart_status_failed":                     0,
		"device_sdc_type_sat_smart_status_passed":                     1,
		"device_sdc_type_sat_temperature":                             27,
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
			wantMetrics: wantMetricsTypeSata(),
		},
		"success type sata devices concurrent": {
			prepareMock: prepareMockOkTypeSata,
			prepareConfig: func() Config {
				cfg := New().Config
				cfg.ConcurrentScans = 2
				return cfg
			},
			wantCharts:  68,
			wantMetrics: wantMetricsTypeSata(),
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
		"success type sata devices non-fatal exit status": {
			prepareMock: prepareMockOkTypeSataNonFatalExitStatus,
			wantCharts:  68,
			wantMetrics: wantMetricsTypeSata(),
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
			for range 10 {
				mx = collr.Collect(context.Background())
			}

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantCharts, "wantCharts")

			collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func TestCollector_CollectDeviceNameWithWhitespace(t *testing.T) {
	const deviceName = "IOService:/Example Controller(Example)@0/Namespace@1"

	collr := New()
	collr.exec = &mockSmartctlCliExec{
		scanData: nvmeScanData(t, deviceName),
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, deviceName, name)
			require.Equal(t, "nvme", deviceType)
			return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", deviceName), nil
		},
	}

	mx := collr.Collect(context.Background())

	require.Len(t, *collr.Charts(), 4)
	require.Len(t, mx, 5)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

	for _, chart := range *collr.Charts() {
		assert.Equal(t, -1, strings.IndexFunc(chart.ID, unicode.IsSpace), chart.ID)
		assert.Equal(t, deviceName, chartLabelValue(chart, "device_name"), chart.ID)
		for _, dim := range chart.Dims {
			assert.Equal(t, -1, strings.IndexFunc(dim.ID, unicode.IsSpace), dim.ID)
			assert.Contains(t, mx, dim.ID)
		}
	}
}

func TestCollector_NormalizedDeviceIdentityDoesNotAlias(t *testing.T) {
	const (
		whitespaceName = "IOService:/Example Controller(Example)@0/Namespace@1"
		underscoreName = "IOService:/Example_Controller(Example)@0/Namespace@1"
	)

	collectIDs := func(t *testing.T, concurrentScans int) map[string]string {
		collr := New()
		collr.ConcurrentScans = concurrentScans
		collr.exec = &mockSmartctlCliExec{
			scanData: nvmeScanData(t, whitespaceName, underscoreName),
			deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
				require.Equal(t, "nvme", deviceType)
				return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", name), nil
			},
		}

		mx := collr.Collect(context.Background())
		require.Len(t, *collr.Charts(), 8)
		require.Len(t, mx, 10)
		collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

		ids := make(map[string]string)
		for _, chart := range *collr.Charts() {
			if chart.Ctx == devicePowerOnTimeChartTmpl.Ctx {
				ids[chartLabelValue(chart, "device_name")] = chart.ID
			}
		}
		require.Len(t, ids, 2)
		require.NotEqual(t, ids[whitespaceName], ids[underscoreName])
		return ids
	}

	sequential := collectIDs(t, 0)
	concurrent := collectIDs(t, 2)
	assert.Equal(t, sequential, concurrent)
}

func TestCollector_UnchangedRescanDoesNotForceDevicePoll(t *testing.T) {
	const deviceName = "/dev/nvme0"

	devicePolls := 0
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanData: nvmeScanData(t, deviceName),
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, deviceName, name)
			require.Equal(t, "nvme", deviceType)
			devicePolls++
			return dataTypeNvmeDeviceNvme0, nil
		},
	}

	first := collr.Collect(context.Background())
	require.NotEmpty(t, first)
	second := collr.Collect(context.Background())
	require.Equal(t, first, second)
	assert.Equal(t, 1, devicePolls)
}

func TestCollector_AddedDeviceForcesDevicePoll(t *testing.T) {
	scans := [][]byte{
		nvmeScanData(t, "/dev/nvme0"),
		nvmeScanData(t, "/dev/nvme0", "/dev/nvme1"),
	}
	scanIndex := 0
	devicePolls := make(map[string]int)
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanDataFunc: func() ([]byte, error) {
			idx := min(scanIndex, len(scans)-1)
			scanIndex++
			return scans[idx], nil
		},
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, "nvme", deviceType)
			devicePolls[name]++
			switch name {
			case "/dev/nvme0":
				return dataTypeNvmeDeviceNvme0, nil
			case "/dev/nvme1":
				return dataTypeNvmeDeviceNvme1, nil
			default:
				return nil, fmt.Errorf("unexpected device %q", name)
			}
		},
	}

	first := collr.Collect(context.Background())
	require.Len(t, first, 5)
	second := collr.Collect(context.Background())
	require.Len(t, second, 10)
	assert.Equal(t, map[string]int{"/dev/nvme0": 2, "/dev/nvme1": 1}, devicePolls)
}

func TestCollector_RemovesChartsUsingAttachedResponseIdentity(t *testing.T) {
	const (
		scanName     = "IOService:/ScanAlias@0/Namespace@1"
		responseName = "IOService:/Example Controller(Example)@0/Namespace@1"
	)

	scans := [][]byte{
		nvmeScanData(t, scanName),
		nvmeScanData(t),
	}
	scanIndex := 0
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanDataFunc: func() ([]byte, error) {
			idx := min(scanIndex, len(scans)-1)
			scanIndex++
			return scans[idx], nil
		},
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, scanName, name)
			require.Equal(t, "nvme", deviceType)
			return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", responseName), nil
		},
	}

	first := collr.Collect(context.Background())
	require.Len(t, *collr.Charts(), 4)
	require.Len(t, first, 5)

	second := collr.Collect(context.Background())
	require.Empty(t, second)
	for _, chart := range *collr.Charts() {
		assert.True(t, chart.Obsolete, chart.ID)
		assert.True(t, chart.IsRemoved(), chart.ID)
	}
}

func TestCollector_EmitsOnlyChartBackedAttributeMetrics(t *testing.T) {
	collr := New()
	collr.exec = &mockSmartctlCliExec{
		scanData: deviceScanData(t, "sat", "ATA", "/dev/sda"),
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, "/dev/sda", name)
			require.Equal(t, "sat", deviceType)
			return dataTypeSataDeviceHDDSda, nil
		},
	}

	mx := collr.Collect(context.Background())
	require.NotEmpty(t, mx)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
	assert.NotContains(t, mx, "device_sda_type_sat_attr_unknown_attribute_normalized")
	assert.NotContains(t, mx, "device_sda_type_sat_attr_power_on_hours_raw")

	chartDims := make(map[string]bool)
	for _, chart := range *collr.Charts() {
		for _, dim := range chart.Dims {
			chartDims[dim.ID] = true
		}
	}
	for metric := range mx {
		if strings.Contains(metric, "_attr_") {
			assert.True(t, chartDims[metric], "attribute metric has no chart dimension: %s", metric)
		}
	}
}

func TestCollector_RetriesUnresolvedScsiTypeThroughForcedScan(t *testing.T) {
	const deviceName = "/dev/sda"

	satCalls := 0
	scsiCalls := 0
	collr := New()
	collr.ScanEvery = 0
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanData: deviceScanData(t, "scsi", "SCSI", deviceName),
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, deviceName, name)
			switch deviceType {
			case "sat":
				satCalls++
				if satCalls == 1 {
					return nil, fmt.Errorf("transient SAT probe failure")
				}
				return dataTypeSataDeviceHDDSda, nil
			case "scsi":
				scsiCalls++
				return []byte(`{"smartctl":{"exit_status":2,"messages":[{"string":"open failed: No such device"}]}}`), fmt.Errorf("exit status 2")
			default:
				return nil, fmt.Errorf("unexpected device type %q", deviceType)
			}
		},
	}

	assert.Empty(t, collr.Collect(context.Background()))
	assert.True(t, collr.forceScan)
	require.Contains(t, collr.scannedDevices, deviceName+"|scsi")
	assert.True(t, collr.scannedDevices[deviceName+"|scsi"].typeUnresolved)

	mx := collr.Collect(context.Background())
	require.NotEmpty(t, mx)
	assert.Equal(t, 3, satCalls)
	assert.Equal(t, 1, scsiCalls)
	assert.Contains(t, collr.scannedDevices, deviceName+"|sat")
	assert.NotContains(t, collr.scannedDevices, deviceName+"|scsi")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_RetriesUnresolvedScsiTypeWithScanEveryDisabled(t *testing.T) {
	const deviceName = "/dev/sda"

	for name, firstProbe := range map[string]func() ([]byte, error){
		"transport failure": func() ([]byte, error) {
			return nil, fmt.Errorf("transient SAT probe failure")
		},
		"device open failure": func() ([]byte, error) {
			data := bytes.Replace(dataTypeSataDeviceHDDSda, []byte(`"exit_status": 0`), []byte(`"exit_status": 2`), 1)
			return data, fmt.Errorf("exit status 2")
		},
		"ATA command failure": func() ([]byte, error) {
			data := bytes.Replace(dataTypeSataDeviceHDDSda, []byte(`"exit_status": 0`), []byte(`"exit_status": 4`), 1)
			return data, fmt.Errorf("exit status 4")
		},
	} {
		t.Run(name, func(t *testing.T) {
			satProbes := 0
			collr := New()
			collr.ScanEvery = 0
			collr.exec = &mockSmartctlCliExec{
				scanData: deviceScanData(t, "scsi", "SCSI", deviceName),
				deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
					require.Equal(t, deviceName, name)
					require.Equal(t, "sat", deviceType)
					satProbes++
					if satProbes == 1 {
						return firstProbe()
					}
					return dataTypeSataDeviceHDDSda, nil
				},
			}

			first, err := collr.scanDevices()
			require.NoError(t, err)
			require.Contains(t, first, deviceName+"|scsi")
			assert.True(t, first[deviceName+"|scsi"].typeUnresolved)
			collr.scannedDevices = first

			second, err := collr.scanDevices()
			require.NoError(t, err)
			require.Contains(t, second, deviceName+"|sat")
			assert.Equal(t, 2, satProbes)
			collr.scannedDevices = second

			third, err := collr.scanDevices()
			require.NoError(t, err)
			require.Contains(t, third, deviceName+"|sat")
			assert.Equal(t, 2, satProbes, "confirmed SAT devices must use the cached fast path")
		})
	}
}

func TestCollector_RemovesOnlyChartsOwnedByDisappearedDevice(t *testing.T) {
	const (
		firstScanName  = "/dev/alias0"
		secondScanName = "/dev/alias1"
		firstName      = "foo"
		secondName     = "foo_type_nvme_bar"
	)

	scans := [][]byte{
		nvmeScanData(t, firstScanName, secondScanName),
		nvmeScanData(t, secondScanName),
	}
	scanIndex := 0
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Nanosecond)
	collr.exec = &mockSmartctlCliExec{
		scanDataFunc: func() ([]byte, error) {
			idx := min(scanIndex, len(scans)-1)
			scanIndex++
			return scans[idx], nil
		},
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, "nvme", deviceType)
			switch name {
			case firstScanName:
				return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", firstName), nil
			case secondScanName:
				return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", secondName), nil
			default:
				return nil, fmt.Errorf("unexpected device %q", name)
			}
		},
	}

	first := collr.Collect(context.Background())
	require.Len(t, *collr.Charts(), 8)
	require.Len(t, first, 10)

	second := collr.Collect(context.Background())
	require.Len(t, second, 5)

	var removed, active int
	for _, chart := range *collr.Charts() {
		switch chartLabelValue(chart, "device_name") {
		case firstName:
			assert.True(t, chart.IsRemoved(), chart.ID)
			removed++
		case secondName:
			assert.False(t, chart.IsRemoved(), chart.ID)
			active++
		default:
			t.Errorf("unexpected device label on chart %q", chart.ID)
		}
	}
	assert.Equal(t, 4, removed)
	assert.Equal(t, 4, active)
}

func TestCollector_ReplacesChangedResponseIdentity(t *testing.T) {
	const (
		scanName   = "IOService:/ScanAlias@0/Namespace@1"
		firstName  = "IOService:/First Controller(Example)@0/Namespace@1"
		secondName = "IOService:/Second Controller(Example)@0/Namespace@1"
	)

	responses := []string{firstName, secondName}
	responseIndex := 0
	collr := New()
	collr.PollDevicesEvery = confopt.Duration(time.Nanosecond)
	collr.exec = &mockSmartctlCliExec{
		scanData: nvmeScanData(t, scanName),
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, scanName, name)
			require.Equal(t, "nvme", deviceType)
			idx := min(responseIndex, len(responses)-1)
			responseIndex++
			return replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", responses[idx]), nil
		},
	}

	first := collr.Collect(context.Background())
	require.Len(t, first, 5)
	require.Len(t, *collr.Charts(), 4)

	second := collr.Collect(context.Background())
	require.Len(t, second, 5)
	require.Len(t, *collr.Charts(), 8)

	var removed, active int
	for _, chart := range *collr.Charts() {
		switch chartLabelValue(chart, "device_name") {
		case firstName:
			assert.True(t, chart.Obsolete, chart.ID)
			assert.True(t, chart.IsRemoved(), chart.ID)
			removed++
		case secondName:
			assert.False(t, chart.Obsolete, chart.ID)
			assert.False(t, chart.IsRemoved(), chart.ID)
			active++
		default:
			t.Errorf("unexpected device label on chart %q", chart.ID)
		}
	}
	assert.Equal(t, 4, removed)
	assert.Equal(t, 4, active)
	assert.Equal(t, newDeviceIdentity(secondName, "nvme"), collr.attachedDevices[scanName+"|nvme"].id)
}

func TestCollector_AttachmentCollisionDoesNotRecordOrphanState(t *testing.T) {
	for _, concurrentScans := range []int{0, 2} {
		t.Run(fmt.Sprintf("concurrent_scans=%d", concurrentScans), func(t *testing.T) {
			collr := New()
			collr.ConcurrentScans = concurrentScans
			collr.exec = &mockSmartctlCliExec{
				scanData: nvmeScanData(t, "/dev/alias0", "/dev/alias1"),
				deviceDataFunc: func(_, deviceType, _ string) ([]byte, error) {
					require.Equal(t, "nvme", deviceType)
					return dataTypeNvmeDeviceNvme0, nil
				},
			}

			mx := collr.Collect(context.Background())
			require.Len(t, *collr.Charts(), 4)
			require.Len(t, mx, 5)
			assert.Len(t, collr.attachedDevices, 1)
			collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func TestCollector_DisambiguatesConflictingAttributeNames(t *testing.T) {
	data := replaceFixtureValue(t, dataTypeSataDeviceHDDSda, "Raw_Read_Error_Rate", "Duplicate Name")
	data = replaceFixtureValue(t, data, "Throughput_Performance", "Duplicate/Name")

	for name, response := range map[string][]byte{
		"original order": data,
		"reverse order":  reverseSmartAttributes(t, data),
	} {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.exec = &mockSmartctlCliExec{
				scanData: deviceScanData(t, "sat", "ATA", "/dev/sda"),
				deviceDataFunc: func(deviceName, deviceType, _ string) ([]byte, error) {
					require.Equal(t, "/dev/sda", deviceName)
					require.Equal(t, "sat", deviceType)
					return response, nil
				},
			}

			mx := collr.Collect(context.Background())
			require.NotEmpty(t, mx)
			require.Len(t, *collr.Charts(), 39)
			require.Len(t, collr.attachedDevices, 1)
			collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

			for key, want := range map[string]int64{
				"device_sda_type_sat_attr_duplicate_name_id_1_normalized": 100,
				"device_sda_type_sat_attr_duplicate_name_id_1_decoded":    0,
				"device_sda_type_sat_attr_duplicate_name_id_2_normalized": 148,
				"device_sda_type_sat_attr_duplicate_name_id_2_decoded":    48,
			} {
				if assert.Contains(t, mx, key) {
					assert.Equal(t, want, mx[key])
				}
			}
			assert.NotContains(t, mx, "device_sda_type_sat_attr_duplicate_name_normalized")
			for _, chartID := range []string{
				"device_sda_type_sat_smart_attr_duplicate_name_id_1",
				"device_sda_type_sat_smart_attr_duplicate_name_id_1_normalized",
				"device_sda_type_sat_smart_attr_duplicate_name_id_2",
				"device_sda_type_sat_smart_attr_duplicate_name_id_2_normalized",
				"device_sda_type_sat_smart_attr_spin_up_time",
			} {
				assert.NotNil(t, collr.Charts().Get(chartID), chartID)
			}
		})
	}
}

func TestCollector_KeepsConflictingAttributeIdentityAcrossPolls(t *testing.T) {
	firstResponse := replaceFixtureValue(t, dataTypeSataDeviceHDDSda, "Raw_Read_Error_Rate", "Duplicate Name")
	firstResponse = replaceFixtureValue(t, firstResponse, "Throughput_Performance", "Duplicate/Name")
	secondResponse := replaceFixtureValue(t, firstResponse, "Duplicate/Name", "Throughput_Performance")
	responses := [][]byte{firstResponse, secondResponse}
	responseIndex := 0

	collr := New()
	collr.PollDevicesEvery = confopt.Duration(time.Nanosecond)
	collr.exec = &mockSmartctlCliExec{
		scanData: deviceScanData(t, "sat", "ATA", "/dev/sda"),
		deviceDataFunc: func(deviceName, deviceType, _ string) ([]byte, error) {
			require.Equal(t, "/dev/sda", deviceName)
			require.Equal(t, "sat", deviceType)
			idx := min(responseIndex, len(responses)-1)
			responseIndex++
			return responses[idx], nil
		},
	}

	first := collr.Collect(context.Background())
	require.NotEmpty(t, first)
	second := collr.Collect(context.Background())
	require.NotEmpty(t, second)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), second)
	assert.Contains(t, second, "device_sda_type_sat_attr_duplicate_name_id_1_normalized")
	assert.Contains(t, second, "device_sda_type_sat_attr_duplicate_name_id_2_normalized")
	assert.NotContains(t, second, "device_sda_type_sat_attr_duplicate_name_normalized")
	assert.NotContains(t, second, "device_sda_type_sat_attr_throughput_performance_normalized")
}

func TestCollector_AddDeviceChartsIsAtomic(t *testing.T) {
	result := gjson.ParseBytes(dataTypeNvmeDeviceNvme0)
	dev := newSmartDevice(&result)
	id := newDeviceIdentity(dev.deviceName(), dev.deviceType())
	collr := New()
	smartAttrs := newSmartAttributeIdentities(dev)

	charts := collr.newDeviceCharts(dev, id)
	existing := charts.Get(id.prefix + "smart_status")
	require.NotNil(t, existing)
	require.NoError(t, collr.Charts().Add(existing))

	_, err := collr.addDeviceCharts(dev, id, smartAttrs)
	require.Error(t, err)
	require.Len(t, *collr.Charts(), 1)
	assert.Equal(t, existing, (*collr.Charts())[0])
}

func TestCollector_AttributeMetricIdentityMatchesChart(t *testing.T) {
	data := replaceFixtureValue(t, dataTypeSataDeviceHDDSda, "Power_On_Hours", "Power/On_Hours")
	result := gjson.ParseBytes(data)
	dev := newSmartDevice(&result)
	collr := New()

	id := newDeviceIdentity(dev.deviceName(), dev.deviceType())
	smartAttrs := newSmartAttributeIdentities(dev)
	charts, err := collr.newDeviceSmartAttrCharts(dev, id, smartAttrs)
	require.NoError(t, err)
	mx := make(map[string]int64)
	collr.collectSmartDevice(mx, dev, id, smartAttrs)

	chart := charts.Get("device_sda_type_sat_smart_attr_power_on_hours")
	require.NotNil(t, chart)
	require.Len(t, chart.Dims, 1)
	assert.Contains(t, mx, chart.Dims[0].ID)
}

func nvmeScanData(t *testing.T, deviceNames ...string) []byte {
	return deviceScanData(t, "nvme", "NVMe", deviceNames...)
}

func deviceScanData(t *testing.T, deviceType, protocol string, deviceNames ...string) []byte {
	t.Helper()

	devices := make([]map[string]string, 0, len(deviceNames))
	for _, name := range deviceNames {
		devices = append(devices, map[string]string{
			"name":      name,
			"info_name": name,
			"type":      deviceType,
			"protocol":  protocol,
		})
	}
	data, err := json.Marshal(map[string]any{"devices": devices})
	require.NoError(t, err)
	return data
}

func replaceFixtureValue(t *testing.T, data []byte, old, new string) []byte {
	t.Helper()
	require.Positive(t, bytes.Count(data, []byte(old)), "fixture does not contain %q", old)
	return bytes.ReplaceAll(data, []byte(old), []byte(new))
}

func reverseSmartAttributes(t *testing.T, data []byte) []byte {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	attrs := payload["ata_smart_attributes"].(map[string]any)["table"].([]any)
	for left, right := 0, len(attrs)-1; left < right; left, right = left+1, right-1 {
		attrs[left], attrs[right] = attrs[right], attrs[left]
	}
	result, err := json.Marshal(payload)
	require.NoError(t, err)
	return result
}

func chartLabelValue(chart *collectorapi.Chart, key string) string {
	for _, label := range chart.Labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
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

func prepareMockOkTypeSataNonFatalExitStatus() *mockSmartctlCliExec {
	return &mockSmartctlCliExec{
		errOnScan: false,
		scanData:  dataTypeSataScan,
		deviceDataFunc: func(deviceName, deviceType, powerMode string) ([]byte, error) {
			if deviceType != "sat" {
				return nil, fmt.Errorf("unexpected device type %s", deviceType)
			}
			// Simulate smartctl exit status 32 (bit 5): some attributes were <= threshold in the past.
			// The data is still valid and should be processed.
			var data []byte
			switch deviceName {
			case "/dev/sda":
				data = bytes.Replace(dataTypeSataDeviceHDDSda, []byte(`"exit_status": 0`), []byte(`"exit_status": 32`), 1)
			case "/dev/sdc":
				data = bytes.Replace(dataTypeSataDeviceSSDSdc, []byte(`"exit_status": 0`), []byte(`"exit_status": 32`), 1)
			default:
				return nil, fmt.Errorf("unexpected device name %s", deviceName)
			}

			// Verify that the modified payload actually encodes smartctl.exit_status == 32.
			v := gjson.GetBytes(data, "smartctl.exit_status")
			if !v.Exists() || v.Int() != 32 {
				panic("prepareMockOkTypeSataNonFatalExitStatus: failed to construct payload with smartctl.exit_status == 32")
			}
			return data, fmt.Errorf("exit status 32")
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
	scanDataFunc   func() ([]byte, error)
	deviceDataFunc func(deviceName, deviceType, powerMode string) ([]byte, error)
}

func (m *mockSmartctlCliExec) scan(_ bool) (*gjson.Result, error) {
	if m.scanDataFunc != nil {
		data, err := m.scanDataFunc()
		if err != nil {
			return nil, err
		}
		res := gjson.ParseBytes(data)
		return &res, nil
	}
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
	if len(bs) == 0 {
		return nil, err
	}
	res := gjson.ParseBytes(bs)
	return &res, err
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
