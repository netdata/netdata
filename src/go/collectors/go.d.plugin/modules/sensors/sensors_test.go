// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

import (
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataSensorsTemp, _               = os.ReadFile("testdata/sensors-temp.txt")
	dataSensorsTempInCurrPowerFan, _ = os.ReadFile("testdata/sensors-temp-in-curr-power-fan.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataSensorsTemp":               dataSensorsTemp,
		"dataSensorsTempInCurrPowerFan": dataSensorsTempInCurrPowerFan,
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
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
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
				sensors.exec = prepareMockOkOnlyTemp()
				_ = sensors.Check()
				return sensors
			},
		},
		"after collect": {
			prepare: func() *Sensors {
				sensors := New()
				sensors.exec = prepareMockOkTempInCurrPowerFan()
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
		prepareMock func() *mockSensorsCLIExec
		wantFail    bool
	}{
		"only temperature": {
			wantFail:    false,
			prepareMock: prepareMockOkOnlyTemp,
		},
		"temperature and voltage": {
			wantFail:    false,
			prepareMock: prepareMockOkTempInCurrPowerFan,
		},
		"error on sensors info call": {
			wantFail:    true,
			prepareMock: prepareMockErr,
		},
		"empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
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
		prepareMock func() *mockSensorsCLIExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"only temperature": {
			prepareMock: prepareMockOkOnlyTemp,
			wantCharts:  24,
			wantMetrics: map[string]int64{
				"sensor_chip_bnxt_en-pci-6200_feature_temp1_subfeature_temp1_input":  80000,
				"sensor_chip_bnxt_en-pci-6201_feature_temp1_subfeature_temp1_input":  81000,
				"sensor_chip_k10temp-pci-00c3_feature_tccd1_subfeature_temp3_input":  58250,
				"sensor_chip_k10temp-pci-00c3_feature_tccd2_subfeature_temp4_input":  60250,
				"sensor_chip_k10temp-pci-00c3_feature_tccd3_subfeature_temp5_input":  57000,
				"sensor_chip_k10temp-pci-00c3_feature_tccd4_subfeature_temp6_input":  57250,
				"sensor_chip_k10temp-pci-00c3_feature_tccd5_subfeature_temp7_input":  57750,
				"sensor_chip_k10temp-pci-00c3_feature_tccd6_subfeature_temp8_input":  59500,
				"sensor_chip_k10temp-pci-00c3_feature_tccd7_subfeature_temp9_input":  58500,
				"sensor_chip_k10temp-pci-00c3_feature_tccd8_subfeature_temp10_input": 61250,
				"sensor_chip_k10temp-pci-00c3_feature_tctl_subfeature_temp1_input":   62000,
				"sensor_chip_k10temp-pci-00cb_feature_tccd1_subfeature_temp3_input":  54000,
				"sensor_chip_k10temp-pci-00cb_feature_tccd2_subfeature_temp4_input":  55500,
				"sensor_chip_k10temp-pci-00cb_feature_tccd3_subfeature_temp5_input":  56000,
				"sensor_chip_k10temp-pci-00cb_feature_tccd4_subfeature_temp6_input":  52750,
				"sensor_chip_k10temp-pci-00cb_feature_tccd5_subfeature_temp7_input":  53500,
				"sensor_chip_k10temp-pci-00cb_feature_tccd6_subfeature_temp8_input":  55250,
				"sensor_chip_k10temp-pci-00cb_feature_tccd7_subfeature_temp9_input":  53000,
				"sensor_chip_k10temp-pci-00cb_feature_tccd8_subfeature_temp10_input": 53750,
				"sensor_chip_k10temp-pci-00cb_feature_tctl_subfeature_temp1_input":   57500,
				"sensor_chip_nouveau-pci-4100_feature_temp1_subfeature_temp1_input":  51000,
				"sensor_chip_nvme-pci-0100_feature_composite_subfeature_temp1_input": 39850,
				"sensor_chip_nvme-pci-6100_feature_composite_subfeature_temp1_input": 48850,
				"sensor_chip_nvme-pci-8100_feature_composite_subfeature_temp1_input": 39850,
			},
		},
		"multiple sensors": {
			prepareMock: prepareMockOkTempInCurrPowerFan,
			wantCharts:  19,
			wantMetrics: map[string]int64{
				"sensor_chip_acpitz-acpi-0_feature_temp1_subfeature_temp1_input":                        88000,
				"sensor_chip_amdgpu-pci-0300_feature_edge_subfeature_temp1_input":                       53000,
				"sensor_chip_amdgpu-pci-0300_feature_fan1_subfeature_fan1_input":                        0,
				"sensor_chip_amdgpu-pci-0300_feature_junction_subfeature_temp2_input":                   58000,
				"sensor_chip_amdgpu-pci-0300_feature_mem_subfeature_temp3_input":                        57000,
				"sensor_chip_amdgpu-pci-0300_feature_vddgfx_subfeature_in0_input":                       787,
				"sensor_chip_amdgpu-pci-6700_feature_edge_subfeature_temp1_input":                       60000,
				"sensor_chip_amdgpu-pci-6700_feature_ppt_subfeature_power1_input":                       8144,
				"sensor_chip_amdgpu-pci-6700_feature_vddgfx_subfeature_in0_input":                       1335,
				"sensor_chip_amdgpu-pci-6700_feature_vddnb_subfeature_in1_input":                        973,
				"sensor_chip_asus-isa-0000_feature_cpu_fan_subfeature_fan1_input":                       5700000,
				"sensor_chip_asus-isa-0000_feature_gpu_fan_subfeature_fan2_input":                       6600000,
				"sensor_chip_bat0-acpi-0_feature_in0_subfeature_in0_input":                              17365,
				"sensor_chip_k10temp-pci-00c3_feature_tctl_subfeature_temp1_input":                      90000,
				"sensor_chip_nvme-pci-0600_feature_composite_subfeature_temp1_input":                    33850,
				"sensor_chip_nvme-pci-0600_feature_sensor_1_subfeature_temp2_input":                     48850,
				"sensor_chip_nvme-pci-0600_feature_sensor_2_subfeature_temp3_input":                     33850,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_curr1_subfeature_curr1_input": 0,
				"sensor_chip_ucsi_source_psy_usbc000:001-isa-0000_feature_in0_subfeature_in0_input":     0,
			},
		},
		"error on sensors info call": {
			prepareMock: prepareMockErr,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sensors := New()
			mock := test.prepareMock()
			sensors.exec = mock

			var mx map[string]int64
			for i := 0; i < 10; i++ {
				mx = sensors.Collect()
			}

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *sensors.Charts(), test.wantCharts)
			testMetricsHasAllChartsDims(t, sensors, mx)
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, sensors *Sensors, mx map[string]int64) {
	for _, chart := range *sensors.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareMockOkOnlyTemp() *mockSensorsCLIExec {
	return &mockSensorsCLIExec{
		sensorsInfoData: dataSensorsTemp,
	}
}

func prepareMockOkTempInCurrPowerFan() *mockSensorsCLIExec {
	return &mockSensorsCLIExec{
		sensorsInfoData: dataSensorsTempInCurrPowerFan,
	}
}

func prepareMockErr() *mockSensorsCLIExec {
	return &mockSensorsCLIExec{
		errOnSensorsInfo: true,
	}
}

func prepareMockUnexpectedResponse() *mockSensorsCLIExec {
	return &mockSensorsCLIExec{
		sensorsInfoData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

func prepareMockEmptyResponse() *mockSensorsCLIExec {
	return &mockSensorsCLIExec{}
}

type mockSensorsCLIExec struct {
	errOnSensorsInfo bool
	sensorsInfoData  []byte
}

func (m *mockSensorsCLIExec) sensorsInfo() ([]byte, error) {
	if m.errOnSensorsInfo {
		return nil, errors.New("mock.sensorsInfo() error")
	}

	return m.sensorsInfoData, nil
}
