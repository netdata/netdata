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

func TestCollector_ValidateConfigBounds(t *testing.T) {
	tests := map[string]struct {
		prepare func(*Collector)
		wantErr bool
	}{
		"minimum timeout": {
			prepare: func(c *Collector) { c.Timeout = confopt.Duration(500 * time.Millisecond) },
		},
		"timeout below minimum": {
			prepare: func(c *Collector) { c.Timeout = confopt.Duration(499 * time.Millisecond) },
			wantErr: true,
		},
		"zero scan interval": {
			prepare: func(c *Collector) { c.ScanEvery = 0 },
		},
		"negative scan interval": {
			prepare: func(c *Collector) { c.ScanEvery = -1 },
			wantErr: true,
		},
		"zero device poll interval": {
			prepare: func(c *Collector) { c.PollDevicesEvery = 0 },
			wantErr: true,
		},
		"negative concurrency": {
			prepare: func(c *Collector) { c.ConcurrentScans = -1 },
			wantErr: true,
		},
		"incomplete extra device": {
			prepare: func(c *Collector) {
				c.ExtraDevices = []ConfigExtraDevice{{Name: "/dev/sda"}}
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			test.prepare(collr)
			if test.wantErr {
				assert.Error(t, collr.validateConfig())
			} else {
				assert.NoError(t, collr.validateConfig())
			}
		})
	}
}

func TestCollector_ConfigSchemaMatchesSupportedBounds(t *testing.T) {
	schema := gjson.Parse(configSchema)

	assert.Equal(t, int64(0), schema.Get("jsonSchema.properties.scan_every.minimum").Int())
	assert.False(t, schema.Get("jsonSchema.properties.device_selector.minimum").Exists())
	assert.Equal(t, "object", schema.Get("jsonSchema.properties.extra_devices.items.type").String())
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
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

func TestParseOutputRejectsInvalidResponses(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":               nil,
		"invalid JSON":        []byte(`{`),
		"missing exit status": []byte(`{"devices": []}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseOutput("smartctl test", data, New().Logger)
			assert.Error(t, err)
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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if test.prepareConfig != nil {
				collr.Config = test.prepareConfig()
			}
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantCharts, "wantCharts")

			collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func TestCollector_CollectionVariantsMatchCanonicalSataResult(t *testing.T) {
	canonical := New()
	canonical.exec = prepareMockOkTypeSata()
	wantMetrics := canonical.Collect(context.Background())
	wantCharts := len(*canonical.Charts())
	require.NotEmpty(t, wantMetrics)

	for name, prepare := range map[string]func(*Collector){
		"concurrent device polling": func(c *Collector) {
			c.ConcurrentScans = 2
			c.exec = prepareMockOkTypeSata()
		},
		"non-fatal smartctl status": func(c *Collector) {
			c.exec = prepareMockOkTypeSataNonFatalExitStatus()
		},
	} {
		t.Run(name, func(t *testing.T) {
			collr := New()
			prepare(collr)
			mx := collr.Collect(context.Background())
			assert.Equal(t, wantMetrics, mx)
			assert.Len(t, *collr.Charts(), wantCharts)
			collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func TestCollector_ScanModeFollowsPowerMode(t *testing.T) {
	for _, test := range []struct {
		powerMode string
		wantOpen  bool
	}{
		{powerMode: "standby", wantOpen: false},
		{powerMode: "never", wantOpen: true},
	} {
		t.Run(test.powerMode, func(t *testing.T) {
			var gotOpen bool
			collr := New()
			collr.NoCheckPowerMode = test.powerMode
			collr.exec = &mockSmartctlCliExec{
				scanData:     deviceScanData(t, "nvme", "NVMe"),
				scanOpenFunc: func(open bool) { gotOpen = open },
			}
			_ = collr.Collect(context.Background())
			assert.Equal(t, test.wantOpen, gotOpen)
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
		responses := map[string][]byte{
			whitespaceName: replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", whitespaceName),
			underscoreName: replaceFixtureValue(t, dataTypeNvmeDeviceNvme0, "/dev/nvme0", underscoreName),
		}
		collr := New()
		collr.ConcurrentScans = concurrentScans
		collr.exec = &mockSmartctlCliExec{
			scanData: nvmeScanData(t, whitespaceName, underscoreName),
			deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
				if deviceType != "nvme" {
					return nil, fmt.Errorf("unexpected device type %q", deviceType)
				}
				response, ok := responses[name]
				if !ok {
					return nil, fmt.Errorf("unexpected device name %q", name)
				}
				return response, nil
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

func TestCollector_UnchangedRescanDoesNotForceDevicePoll(t *testing.T) {
	deviceInfoCalls := 0
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanDataFunc: func() ([]byte, error) {
			return nvmeScanData(t, "/dev/nvme0"), nil
		},
		deviceDataFunc: func(name, deviceType, _ string) ([]byte, error) {
			require.Equal(t, "/dev/nvme0", name)
			require.Equal(t, "nvme", deviceType)
			deviceInfoCalls++
			return dataTypeNvmeDeviceNvme0, nil
		},
	}

	first := collr.Collect(context.Background())
	require.NotEmpty(t, first)
	require.Equal(t, 1, deviceInfoCalls)

	second := collr.Collect(context.Background())
	assert.Equal(t, first, second)
	assert.Equal(t, 1, deviceInfoCalls)
}

func TestCollector_RetriesUnresolvedScsiType(t *testing.T) {
	lowPower := []byte(`{
		"smartctl": {
			"exit_status": 2,
			"messages": [{"severity": "error", "string": "Device is in STANDBY mode"}]
		}
	}`)

	var calls []string
	satCalls := 0
	collr := New()
	collr.ScanEvery = confopt.Duration(time.Nanosecond)
	collr.PollDevicesEvery = confopt.Duration(time.Hour)
	collr.exec = &mockSmartctlCliExec{
		scanData: deviceScanData(t, "scsi", "SCSI", "/dev/sda"),
		deviceDataFunc: func(deviceName, deviceType, _ string) ([]byte, error) {
			calls = append(calls, deviceType)
			require.Equal(t, "/dev/sda", deviceName)
			if deviceType == "scsi" {
				return lowPower, fmt.Errorf("device is in standby mode")
			}
			require.Equal(t, "sat", deviceType)
			satCalls++
			if satCalls == 1 {
				return lowPower, fmt.Errorf("device is in standby mode")
			}
			return dataTypeSataDeviceHDDSda, nil
		},
	}

	assert.Empty(t, collr.Collect(context.Background()))
	second := collr.Collect(context.Background())
	require.NotEmpty(t, second)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), second)
	assert.Equal(t, []string{"sat", "scsi", "sat", "sat"}, calls)

	third := collr.Collect(context.Background())
	assert.Equal(t, second, third)
	assert.Equal(t, []string{"sat", "scsi", "sat", "sat"}, calls)
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
}

func TestCollector_ReconcilesSamePathDeviceReplacement(t *testing.T) {
	secondResponse := replaceFixtureValue(t, dataTypeSataDeviceHDDSda, "WDC WD181KRYZ-01AGBB0", "REPLACEMENT MODEL")
	secondResponse = replaceFixtureValue(t, secondResponse, `"serial_number": "REDACTED"`, `"serial_number": "REPLACEMENT-SERIAL"`)
	secondResponse = replaceFixtureValue(t, secondResponse, "Raw_Read_Error_Rate", "Replacement_Read_Error")
	responses := [][]byte{dataTypeSataDeviceHDDSda, secondResponse}
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
	baseChart := collr.Charts().Get("device_sda_type_sat_power_on_time")
	require.NotNil(t, baseChart)
	require.Equal(t, "WDC WD181KRYZ-01AGBB0", chartLabelValue(baseChart, "model_name"))

	second := collr.Collect(context.Background())
	require.NotEmpty(t, second)
	assert.Same(t, baseChart, collr.Charts().Get("device_sda_type_sat_power_on_time"))
	for _, chart := range *collr.Charts() {
		if chart.IsRemoved() {
			continue
		}
		assert.Equal(t, "REPLACEMENT MODEL", chartLabelValue(chart, "model_name"), chart.ID)
		assert.Equal(t, "REPLACEMENT-SERIAL", chartLabelValue(chart, "serial_number"), chart.ID)
	}

	for _, chartID := range []string{
		"device_sda_type_sat_smart_attr_raw_read_error_rate",
		"device_sda_type_sat_smart_attr_raw_read_error_rate_normalized",
	} {
		chart := collr.Charts().Get(chartID)
		require.NotNil(t, chart)
		assert.True(t, chart.IsRemoved(), chartID)
	}
	for _, chartID := range []string{
		"device_sda_type_sat_smart_attr_replacement_read_error",
		"device_sda_type_sat_smart_attr_replacement_read_error_normalized",
	} {
		chart := collr.Charts().Get(chartID)
		require.NotNil(t, chart)
		assert.False(t, chart.IsRemoved(), chartID)
	}
	assert.NotContains(t, second, "device_sda_type_sat_attr_raw_read_error_rate_decoded")
	assert.Contains(t, second, "device_sda_type_sat_attr_replacement_read_error_decoded")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), second)
}

func TestCollector_AttachmentCollisionDoesNotRecordOrphanState(t *testing.T) {
	for _, concurrentScans := range []int{0, 2} {
		t.Run(fmt.Sprintf("concurrent_scans=%d", concurrentScans), func(t *testing.T) {
			collr := New()
			collr.ConcurrentScans = concurrentScans
			collr.exec = &mockSmartctlCliExec{
				scanData: nvmeScanData(t, "/dev/alias0", "/dev/alias1"),
				deviceDataFunc: func(_, deviceType, _ string) ([]byte, error) {
					if deviceType != "nvme" {
						return nil, fmt.Errorf("unexpected device type %q", deviceType)
					}
					return dataTypeNvmeDeviceNvme0, nil
				},
			}

			mx := collr.Collect(context.Background())
			require.Len(t, *collr.Charts(), 4)
			require.Len(t, mx, 5)
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
			require.NotEmpty(t, *collr.Charts())
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
			assert.NotNil(t, collr.Charts().Get("device_sda_type_sat_smart_attr_duplicate_name_id_1"))
			assert.NotNil(t, collr.Charts().Get("device_sda_type_sat_smart_attr_duplicate_name_id_2"))
		})
	}
}

func TestCollector_CollectsOnlyChartBackedSmartAttributes(t *testing.T) {
	collr := New()
	collr.exec = &mockSmartctlCliExec{
		scanData: deviceScanData(t, "sat", "ATA", "/dev/sda"),
		deviceDataFunc: func(_, _, _ string) ([]byte, error) {
			return dataTypeSataDeviceHDDSda, nil
		},
	}

	mx := collr.Collect(context.Background())
	require.NotEmpty(t, mx)
	for key := range mx {
		assert.False(t, strings.HasSuffix(key, "_raw"), key)
		assert.NotContains(t, key, "unknown_attribute")
		assert.NotContains(t, key, "not_in_use")
	}
	assert.Contains(t, mx, "device_sda_type_sat_attr_power_on_hours_normalized")
	assert.Contains(t, mx, "device_sda_type_sat_attr_power_on_hours_decoded")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_ReconcilesConflictingAttributeIdentityAcrossPolls(t *testing.T) {
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
	assert.Contains(t, second, "device_sda_type_sat_attr_duplicate_name_normalized")
	assert.Contains(t, second, "device_sda_type_sat_attr_throughput_performance_normalized")
	assert.NotContains(t, second, "device_sda_type_sat_attr_duplicate_name_id_1_normalized")
	assert.NotContains(t, second, "device_sda_type_sat_attr_duplicate_name_id_2_normalized")
	for _, chartID := range []string{
		"device_sda_type_sat_smart_attr_duplicate_name_id_1",
		"device_sda_type_sat_smart_attr_duplicate_name_id_2",
	} {
		chart := collr.Charts().Get(chartID)
		require.NotNil(t, chart)
		assert.True(t, chart.IsRemoved(), chartID)
	}
	assert.NotNil(t, collr.Charts().Get("device_sda_type_sat_smart_attr_duplicate_name"))
	assert.NotNil(t, collr.Charts().Get("device_sda_type_sat_smart_attr_throughput_performance"))
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

type mockSmartctlCliExec struct {
	errOnScan      bool
	scanData       []byte
	scanDataFunc   func() ([]byte, error)
	scanOpenFunc   func(bool)
	deviceDataFunc func(deviceName, deviceType, powerMode string) ([]byte, error)
}

func (m *mockSmartctlCliExec) scan(open bool) (*gjson.Result, error) {
	if m.scanOpenFunc != nil {
		m.scanOpenFunc(open)
	}
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
