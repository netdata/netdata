// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/sensors/lmsensors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)

	}
}

func TestSensors_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Sensors{}, dataConfigJSON, dataConfigYAML)
}

func TestSensors_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()
			sensors.Config = test.config

			if test.wantFail {
				assert.Error(t, sensors.Init())
			} else {
				assert.NoError(t, sensors.Init())
			}
		})
	}
}

func TestSensors_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Sensors
	}{
		"not initialized exec": {
			prepare: func() *Sensors {
				return New()
			},
		},
		"after check": {
			prepare: func() *Sensors {
				sensors := New()
				sensors.sc = prepareMockScannerOk()
				_ = sensors.Check()
				return sensors
			},
		},
		"after collect": {
			prepare: func() *Sensors {
				sensors := New()
				sensors.sc = prepareMockScannerOk()
				_ = sensors.Collect()
				return sensors
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := test.prepare()

			assert.NotPanics(t, sensors.Cleanup)
		})
	}
}

func TestSensors_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestSensors_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockScanner
		wantFail    bool
	}{
		"multiple sensors": {
			wantFail:    false,
			prepareMock: prepareMockScannerOk,
		},
		"error on scan": {
			wantFail:    true,
			prepareMock: prepareMockScannerErr,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()
			sensors.sc = test.prepareMock()

			if test.wantFail {
				assert.Error(t, sensors.Check())
			} else {
				assert.NoError(t, sensors.Check())
			}
		})
	}
}

func TestSensors_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareScanner func() *mockScanner
		wantMetrics    map[string]int64
		wantCharts     int
	}{
		"multiple sensors": {
			prepareScanner: prepareMockScannerOk,
			wantCharts:     24,
			wantMetrics: map[string]int64{
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_alarm_clear":          1,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_alarm_triggered":      0,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_average":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_crit":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_highest":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_input":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_lcrit":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_lowest":               42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_max":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_min":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr1_read_time":            0,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_crit":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_highest":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_input":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_lcrit":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_lowest":               42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_max":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_min":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_curr2_read_time":            0,
				"chip_chip0-pci-xxxxxxxx_sensor_energy1_input":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_energy1_read_time":          0,
				"chip_chip0-pci-xxxxxxxx_sensor_energy2_input":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_energy2_read_time":          0,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_alarm_clear":           1,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_alarm_triggered":       0,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_input":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_max":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_min":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_read_time":             0,
				"chip_chip0-pci-xxxxxxxx_sensor_fan1_target":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan2_input":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan2_max":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan2_min":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_fan2_read_time":             0,
				"chip_chip0-pci-xxxxxxxx_sensor_fan2_target":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_humidity1_input":            42000,
				"chip_chip0-pci-xxxxxxxx_sensor_humidity1_read_time":        0,
				"chip_chip0-pci-xxxxxxxx_sensor_humidity2_input":            42000,
				"chip_chip0-pci-xxxxxxxx_sensor_humidity2_read_time":        0,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_alarm_clear":            1,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_alarm_triggered":        0,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_average":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_crit":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_highest":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_input":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_lcrit":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_lowest":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_max":                    42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_min":                    42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in1_read_time":              0,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_crit":                   42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_highest":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_input":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_lcrit":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_lowest":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_max":                    42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_min":                    42000,
				"chip_chip0-pci-xxxxxxxx_sensor_in2_read_time":              0,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion1_alarm_clear":     1,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion1_alarm_triggered": 0,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion1_read_time":       0,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion2_alarm_clear":     0,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion2_alarm_triggered": 1,
				"chip_chip0-pci-xxxxxxxx_sensor_intrusion2_read_time":       0,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_accuracy":            34500,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_alarm_clear":         1,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_alarm_triggered":     0,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_average":             345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_average_highest":     345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_average_lowest":      345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_average_max":         345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_average_min":         345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_cap":                 345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_cap_max":             345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_cap_min":             345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_crit":                345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_input":               345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_input_highest":       345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_input_lowest":        345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_max":                 345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power1_read_time":           0,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_accuracy":            34500,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_average_highest":     345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_average_lowest":      345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_average_max":         345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_average_min":         345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_cap":                 345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_cap_max":             345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_cap_min":             345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_crit":                345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_input":               345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_input_highest":       345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_input_lowest":        345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_max":                 345000,
				"chip_chip0-pci-xxxxxxxx_sensor_power2_read_time":           0,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_alarm_clear":          1,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_alarm_triggered":      0,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_crit":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_emergency":            42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_highest":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_input":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_lcrit":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_lowest":               42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_max":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_min":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp1_read_time":            0,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_crit":                 42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_emergency":            42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_highest":              42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_input":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_lcrit":                42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_lowest":               42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_max":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_min":                  42000,
				"chip_chip0-pci-xxxxxxxx_sensor_temp2_read_time":            0,
			},
		},
		"error on scan": {
			prepareScanner: prepareMockScannerErr,
			wantMetrics:    nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()
			sensors.sc = test.prepareScanner()

			var mx map[string]int64

			for i := 0; i < 10; i++ {
				mx = sensors.Collect()
			}

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *sensors.Charts(), test.wantCharts)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, sensors.Charts(), mx)
			}
		})
	}
}

func prepareMockScannerOk() *mockScanner {
	return &mockScanner{
		scanData: mockChips(),
	}
}

func prepareMockScannerErr() *mockScanner {
	return &mockScanner{
		errOnScan: true,
	}
}

type mockScanner struct {
	errOnScan bool
	scanData  []*lmsensors.Chip
}

func (m *mockScanner) Scan() ([]*lmsensors.Chip, error) {
	if m.errOnScan {
		return nil, errors.New("mock.scan() error")
	}
	return m.scanData, nil
}

func ptr[T any](v T) *T { return &v }

func mockChips() []*lmsensors.Chip {
	return []*lmsensors.Chip{
		{
			Name:       "chip0",
			UniqueName: "chip0-pci-xxxxxxxx",
			SysDevice:  "pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0",
			Sensors: lmsensors.Sensors{
				Voltage: []*lmsensors.VoltageSensor{
					{
						Name:    "in1",
						Label:   "some_label1",
						Alarm:   ptr(false),
						Input:   ptr(42.0),
						Average: ptr(42.0),
						Min:     ptr(42.0),
						Max:     ptr(42.0),
						CritMin: ptr(42.0),
						CritMax: ptr(42.0),
						Lowest:  ptr(42.0),
						Highest: ptr(42.0),
					},
					{
						Name:    "in2",
						Label:   "some_label2",
						Input:   ptr(42.0),
						Min:     ptr(42.0),
						Max:     ptr(42.0),
						CritMin: ptr(42.0),
						CritMax: ptr(42.0),
						Lowest:  ptr(42.0),
						Highest: ptr(42.0),
					},
				},
				Fan: []*lmsensors.FanSensor{
					{
						Name:   "fan1",
						Label:  "some_label1",
						Alarm:  ptr(false),
						Input:  ptr(42.0),
						Min:    ptr(42.0),
						Max:    ptr(42.0),
						Target: ptr(42.0),
					},
					{
						Name:   "fan2",
						Label:  "some_label2",
						Input:  ptr(42.0),
						Min:    ptr(42.0),
						Max:    ptr(42.0),
						Target: ptr(42.0),
					},
				},
				Temperature: []*lmsensors.TemperatureSensor{
					{
						Name:        "temp1",
						Label:       "some_label1",
						Alarm:       ptr(false),
						TempTypeRaw: 1,
						Input:       ptr(42.0),
						Max:         ptr(42.0),
						Min:         ptr(42.0),
						CritMin:     ptr(42.0),
						CritMax:     ptr(42.0),
						Emergency:   ptr(42.0),
						Lowest:      ptr(42.0),
						Highest:     ptr(42.0),
					},
					{
						Name:        "temp2",
						Label:       "some_label2",
						TempTypeRaw: 1,
						Input:       ptr(42.0),
						Max:         ptr(42.0),
						Min:         ptr(42.0),
						CritMin:     ptr(42.0),
						CritMax:     ptr(42.0),
						Emergency:   ptr(42.0),
						Lowest:      ptr(42.0),
						Highest:     ptr(42.0),
					},
				},
				Current: []*lmsensors.CurrentSensor{
					{
						Name:    "curr1",
						Label:   "some_label1",
						Alarm:   ptr(false),
						Max:     ptr(42.0),
						Min:     ptr(42.0),
						CritMin: ptr(42.0),
						CritMax: ptr(42.0),
						Input:   ptr(42.0),
						Average: ptr(42.0),
						Lowest:  ptr(42.0),
						Highest: ptr(42.0),
					},
					{
						Name:    "curr2",
						Label:   "some_label2",
						Max:     ptr(42.0),
						Min:     ptr(42.0),
						CritMin: ptr(42.0),
						CritMax: ptr(42.0),
						Input:   ptr(42.0),
						Lowest:  ptr(42.0),
						Highest: ptr(42.0),
					},
				},
				Power: []*lmsensors.PowerSensor{
					{
						Name:           "power1",
						Label:          "some_label1",
						Alarm:          ptr(false),
						Average:        ptr(345.0),
						AverageHighest: ptr(345.0),
						AverageLowest:  ptr(345.0),
						AverageMax:     ptr(345.0),
						AverageMin:     ptr(345.0),
						Input:          ptr(345.0),
						InputHighest:   ptr(345.0),
						InputLowest:    ptr(345.0),
						Accuracy:       ptr(34.5),
						Cap:            ptr(345.0),
						CapMax:         ptr(345.0),
						CapMin:         ptr(345.0),
						Max:            ptr(345.0),
						CritMax:        ptr(345.0),
					},
					{
						Name:           "power2",
						Label:          "some_label2",
						AverageHighest: ptr(345.0),
						AverageLowest:  ptr(345.0),
						AverageMax:     ptr(345.0),
						AverageMin:     ptr(345.0),
						Input:          ptr(345.0),
						InputHighest:   ptr(345.0),
						InputLowest:    ptr(345.0),
						Accuracy:       ptr(34.5),
						Cap:            ptr(345.0),
						CapMax:         ptr(345.0),
						CapMin:         ptr(345.0),
						Max:            ptr(345.0),
						CritMax:        ptr(345.0),
					},
				},
				Energy: []*lmsensors.EnergySensor{
					{
						Name:  "energy1",
						Label: "some_label1",
						Input: ptr(42.0),
					},
					{
						Name:  "energy2",
						Label: "some_label2",
						Input: ptr(42.0),
					},
				},
				Humidity: []*lmsensors.HumiditySensor{
					{
						Name:  "humidity1",
						Label: "some_label1",
						Input: ptr(42.0),
					},
					{
						Name:  "humidity2",
						Label: "some_label2",
						Input: ptr(42.0),
					},
				},
				Intrusion: []*lmsensors.IntrusionSensor{
					{
						Name:  "intrusion1",
						Label: "some_label1",
						Alarm: ptr(false),
					},
					{
						Name:  "intrusion2",
						Label: "some_label2",
						Alarm: ptr(true),
					},
				},
			},
		},
	}
}
