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

	dataSensors, _ = os.ReadFile("testdata/sensors.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataSensors": dataSensors,
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
		"success if 'binary_path' is not set": {
			wantFail: false,
			config: Config{
				BinaryPath: "",
			},
		},
		"success if failed to find binary": {
			wantFail: false,
			config: Config{
				BinaryPath: "sensors!!!",
			},
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
				sensors.exec = prepareMockExecOk()
				_ = sensors.Check()
				return sensors
			},
		},
		"after collect": {
			prepare: func() *Sensors {
				sensors := New()
				sensors.exec = prepareMockExecOk()
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
		prepareMock func() *mockSensorsBinary
		wantFail    bool
	}{
		"exec: only temperature": {
			wantFail:    false,
			prepareMock: prepareMockExecOk,
		},
		"exec: error on sensors info call": {
			wantFail:    true,
			prepareMock: prepareMockExecErr,
		},
		"exec: empty response": {
			wantFail:    true,
			prepareMock: prepareMockExecEmptyResponse,
		},
		"exec: unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockExecUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()
			mock := test.prepareMock()
			sensors.exec = mock

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
		prepareExecMock  func() *mockSensorsBinary
		prepareSysfsMock func() *mockSysfsScanner
		wantMetrics      map[string]int64
		wantCharts       int
	}{
		"exec: multiple sensors": {
			prepareExecMock: prepareMockExecOk,
			wantCharts:      22,
			wantMetrics: map[string]int64{
				"sensor_chip_acpitz-acpi-0_feature_temp1_subfeature_temp1_input":                        88000,
				"sensor_chip_amdgpu-pci-0300_feature_edge_subfeature_temp1_input":                       53000,
				"sensor_chip_amdgpu-pci-0300_feature_fan1_subfeature_fan1_input":                        0,
				"sensor_chip_amdgpu-pci-0300_feature_junction_subfeature_temp2_input":                   58000,
				"sensor_chip_amdgpu-pci-0300_feature_mem_subfeature_temp3_input":                        57000,
				"sensor_chip_amdgpu-pci-0300_feature_ppt_subfeature_power1_average":                     29000,
				"sensor_chip_amdgpu-pci-0300_feature_vddgfx_subfeature_in0_input":                       787,
				"sensor_chip_amdgpu-pci-6700_feature_edge_subfeature_temp1_input":                       60000,
				"sensor_chip_amdgpu-pci-6700_feature_ppt_subfeature_power1_average":                     5088,
				"sensor_chip_amdgpu-pci-6700_feature_vddgfx_subfeature_in0_input":                       1335,
				"sensor_chip_amdgpu-pci-6700_feature_vddnb_subfeature_in1_input":                        973,
				"sensor_chip_asus-isa-0000_feature_cpu_fan_subfeature_fan1_input":                       5700000,
				"sensor_chip_asus-isa-0000_feature_gpu_fan_subfeature_fan2_input":                       6600000,
				"sensor_chip_bat0-acpi-0_feature_in0_subfeature_in0_input":                              17365,
				"sensor_chip_k10temp-pci-00c3_feature_tctl_subfeature_temp1_input":                      90000,
				"sensor_chip_nct6779-isa-0290_feature_intrusion0_subfeature_intrusion0_alarm_clear":     0,
				"sensor_chip_nct6779-isa-0290_feature_intrusion0_subfeature_intrusion0_alarm_triggered": 1,
				"sensor_chip_nct6779-isa-0290_feature_intrusion1_subfeature_intrusion1_alarm_clear":     0,
				"sensor_chip_nct6779-isa-0290_feature_intrusion1_subfeature_intrusion1_alarm_triggered": 1,
				"sensor_chip_nvme-pci-0600_feature_composite_subfeature_temp1_input":                    33850,
				"sensor_chip_nvme-pci-0600_feature_sensor_1_subfeature_temp2_input":                     48850,
				"sensor_chip_nvme-pci-0600_feature_sensor_2_subfeature_temp3_input":                     33850,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_curr1_subfeature_curr1_input": 0,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_in0_subfeature_in0_input":     0,
			},
		},
		"exec: error on sensors info call": {
			prepareExecMock: prepareMockExecErr,
			wantMetrics:     nil,
		},
		"exec: empty response": {
			prepareExecMock: prepareMockExecEmptyResponse,
			wantMetrics:     nil,
		},
		"exec: unexpected response": {
			prepareExecMock: prepareMockExecUnexpectedResponse,
			wantMetrics:     nil,
		},

		"sysfs: multiple sensors": {
			prepareSysfsMock: prepareMockSysfsScannerOk,
			wantCharts:       21,
			wantMetrics: map[string]int64{
				"sensor_chip_acpitz-acpi-0_feature_temp1_subfeature_temp1_input":                        88000,
				"sensor_chip_amdgpu-pci-0300_feature_edge_subfeature_temp1_input":                       53000,
				"sensor_chip_amdgpu-pci-0300_feature_fan1_subfeature_fan1_input":                        0,
				"sensor_chip_amdgpu-pci-0300_feature_junction_subfeature_temp2_input":                   58000,
				"sensor_chip_amdgpu-pci-0300_feature_mem_subfeature_temp3_input":                        57000,
				"sensor_chip_amdgpu-pci-0300_feature_ppt_subfeature_power1_average":                     29000,
				"sensor_chip_amdgpu-pci-0300_feature_vddgfx_subfeature_in0_input":                       787,
				"sensor_chip_amdgpu-pci-6700_feature_edge_subfeature_temp1_input":                       60000,
				"sensor_chip_amdgpu-pci-6700_feature_ppt_subfeature_power1_average":                     5088,
				"sensor_chip_amdgpu-pci-6700_feature_vddgfx_subfeature_in0_input":                       1335,
				"sensor_chip_amdgpu-pci-6700_feature_vddnb_subfeature_in1_input":                        973,
				"sensor_chip_asus-isa-0000_feature_cpu_fan_subfeature_fan1_input":                       5700000,
				"sensor_chip_asus-isa-0000_feature_gpu_fan_subfeature_fan2_input":                       6600000,
				"sensor_chip_asus-isa-0000_feature_intrusion0_subfeature_intrusion0_alarm_clear":        0,
				"sensor_chip_asus-isa-0000_feature_intrusion0_subfeature_intrusion0_alarm_triggered":    1,
				"sensor_chip_bat0-acpi-0_feature_in0_subfeature_in0_input":                              17365,
				"sensor_chip_k10temp-pci-00c3_feature_tctl_subfeature_temp1_input":                      90000,
				"sensor_chip_nvme-pci-0600_feature_composite_subfeature_temp1_input":                    33850,
				"sensor_chip_nvme-pci-0600_feature_sensor_1_subfeature_temp2_input":                     48850,
				"sensor_chip_nvme-pci-0600_feature_sensor_2_subfeature_temp3_input":                     33850,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_curr1_subfeature_curr1_input": 0,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_in0_subfeature_in0_input":     0,
			},
		},
		"sysfs: error on scan": {
			prepareSysfsMock: prepareMockSysfsScannerErr,
			wantMetrics:      nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()

			if test.prepareExecMock != nil {
				sensors.exec = test.prepareExecMock()
			} else if test.prepareSysfsMock != nil {
				sensors.sc = test.prepareSysfsMock()
			} else {
				t.Fail()
			}

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

func prepareMockExecOk() *mockSensorsBinary {
	return &mockSensorsBinary{
		sensorsInfoData: dataSensors,
	}
}

func prepareMockExecErr() *mockSensorsBinary {
	return &mockSensorsBinary{
		errOnSensorsInfo: true,
	}
}

func prepareMockExecUnexpectedResponse() *mockSensorsBinary {
	return &mockSensorsBinary{
		sensorsInfoData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

func prepareMockExecEmptyResponse() *mockSensorsBinary {
	return &mockSensorsBinary{}
}

type mockSensorsBinary struct {
	errOnSensorsInfo bool
	sensorsInfoData  []byte
}

func (m *mockSensorsBinary) sensorsInfo() ([]byte, error) {
	if m.errOnSensorsInfo {
		return nil, errors.New("mock.sensorsInfo() error")
	}

	return m.sensorsInfoData, nil
}

func prepareMockSysfsScannerOk() *mockSysfsScanner {
	return &mockSysfsScanner{
		scanData: []*lmsensors.Device{
			{Name: "asus-isa-0000", Sensors: []lmsensors.Sensor{
				&lmsensors.FanSensor{
					Name:  "fan1",
					Label: "cpu_fan",
					Input: 5700,
				},
				&lmsensors.FanSensor{
					Name:  "fan2",
					Label: "gpu_fan",
					Input: 6600,
				},
				&lmsensors.IntrusionSensor{
					Name:  "intrusion0",
					Label: "intrusion0",
					Alarm: true,
				},
			}},
			{Name: "nvme-pci-0600", Sensors: []lmsensors.Sensor{
				&lmsensors.TemperatureSensor{
					Name:     "temp1",
					Label:    "Composite",
					Input:    33.85,
					Maximum:  83.85,
					Minimum:  -40.15,
					Critical: 87.85,
					Alarm:    false,
				},
				&lmsensors.TemperatureSensor{
					Name:    "temp2",
					Label:   "Sensor 1",
					Input:   48.85,
					Maximum: 65261.85,
					Minimum: -273.15,
				},
				&lmsensors.TemperatureSensor{
					Name:    "temp3",
					Label:   "Sensor 2",
					Input:   33.85,
					Maximum: 65261.85,
					Minimum: -273.15,
				},
			}},
			{Name: "amdgpu-pci-6700", Sensors: []lmsensors.Sensor{
				&lmsensors.VoltageSensor{
					Name:  "in0",
					Label: "vddgfx",
					Input: 1.335,
				},
				&lmsensors.VoltageSensor{
					Name:  "in1",
					Label: "vddnb",
					Input: 0.973,
				},
				&lmsensors.TemperatureSensor{
					Name:  "temp1",
					Label: "edge",
					Input: 60.000,
				},
				&lmsensors.PowerSensor{
					Name:    "power1",
					Label:   "PPT",
					Average: 5.088,
				},
			}},
			{Name: "BAT0-acpi-0", Sensors: []lmsensors.Sensor{
				&lmsensors.VoltageSensor{
					Name:  "in0",
					Label: "in0",
					Input: 17.365,
				},
			}},
			{Name: "ucsi_source_psy_USBC000:001-isa-0000", Sensors: []lmsensors.Sensor{
				&lmsensors.VoltageSensor{
					Name:  "in0",
					Label: "in0",
					Input: 0.000,
				},
				&lmsensors.CurrentSensor{
					Name:  "curr1",
					Label: "curr1",
					Input: 0.000,
				},
			}},
			{Name: "k10temp-pci-00c3", Sensors: []lmsensors.Sensor{
				&lmsensors.TemperatureSensor{
					Name:  "temp1",
					Label: "Tctl",
					Input: 90,
				},
			}},
			{Name: "amdgpu-pci-0300", Sensors: []lmsensors.Sensor{
				&lmsensors.VoltageSensor{
					Name:  "in0",
					Label: "vddgfx",
					Input: 0.787,
				},
				&lmsensors.FanSensor{
					Name:    "fan1",
					Label:   "fan1",
					Maximum: 4900,
				},
				&lmsensors.TemperatureSensor{
					Name:      "temp1",
					Label:     "edge",
					Input:     53,
					Critical:  100,
					Emergency: 105,
				},
				&lmsensors.TemperatureSensor{
					Name:      "temp2",
					Label:     "junction",
					Input:     58,
					Critical:  100,
					Emergency: 105,
				},
				&lmsensors.TemperatureSensor{
					Name:      "temp3",
					Label:     "mem",
					Input:     57,
					Critical:  106,
					Emergency: 110,
				},
				&lmsensors.PowerSensor{
					Name:    "power1",
					Label:   "PPT",
					Average: 29,
				},
			}},
			{Name: "acpitz-acpi-0", Sensors: []lmsensors.Sensor{
				&lmsensors.FanSensor{
					Name:  "temp1",
					Label: "temp1",
					Input: 88,
				},
			}},
		},
	}
}

func prepareMockSysfsScannerErr() *mockSysfsScanner {
	return &mockSysfsScanner{
		errOnScan: true,
	}
}

type mockSysfsScanner struct {
	errOnScan bool
	scanData  []*lmsensors.Device
}

func (m *mockSysfsScanner) Scan() ([]*lmsensors.Device, error) {
	if m.errOnScan {
		return nil, errors.New("mock.scan() error")
	}
	return m.scanData, nil
}
