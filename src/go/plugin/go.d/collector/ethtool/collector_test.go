// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataModuleDdm, _        = os.ReadFile("testdata/ddm.txt")
	dataModuleDdmNoFiber, _ = os.ReadFile("testdata/ddm-no-fiber.txt")
	dataModuleNoDdm, _      = os.ReadFile("testdata/no-ddm.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataModuleDdm":        dataModuleDdm,
		"dataModuleDdmNoFiber": dataModuleDdmNoFiber,
		"dataModuleNoDdm":      dataModuleNoDdm,
	} {
		require.NotNil(t, data, name)

	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if failed to locate ndsudo": {
			wantFail: true,
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
				collr.exec = prepareMockEepromDdm()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockEepromDdm()
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockEthtoolExec
		wantFail    bool
	}{
		"module with ddm": {
			wantFail:    false,
			prepareMock: prepareMockEepromDdm,
		},
		"module with ddm no fiber": {
			wantFail:    false,
			prepareMock: prepareMockEepromDdmNoFiber,
		},
		"module without ddm": {
			wantFail:    true,
			prepareMock: prepareMockEepromNoDdm,
		},
		"moduleEeprom() error": {
			wantFail:    true,
			prepareMock: prepareMockEepromError,
		},
		"moduleEeprom() unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockEepromUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock
			collr.OpticInterfaces = "eth1 eth2"

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
		prepareMock func() *mockEthtoolExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"module with ddm": {
			prepareMock: prepareMockEepromDdm,
			wantCharts:  len(ifaceModuleEepromCharts) * 2,
			wantMetrics: map[string]int64{
				"iface_eth1_laser_bias_current_ma":                     13088,
				"iface_eth1_laser_output_power_dbm":                    5580,
				"iface_eth1_laser_output_power_mw":                     3611,
				"iface_eth1_module_temperature_c":                      38000,
				"iface_eth1_module_temperature_f":                      100400,
				"iface_eth1_module_voltage_v":                          3484,
				"iface_eth1_receiver_signal_average_optical_power_dbm": -23570,
				"iface_eth1_receiver_signal_average_optical_power_mw":  4,
				"iface_eth2_laser_bias_current_ma":                     13088,
				"iface_eth2_laser_output_power_dbm":                    5580,
				"iface_eth2_laser_output_power_mw":                     3611,
				"iface_eth2_module_temperature_c":                      38000,
				"iface_eth2_module_temperature_f":                      100400,
				"iface_eth2_module_voltage_v":                          3484,
				"iface_eth2_receiver_signal_average_optical_power_dbm": -23570,
				"iface_eth2_receiver_signal_average_optical_power_mw":  4,
			},
		},
		"module with ddm no fiber": {
			prepareMock: prepareMockEepromDdmNoFiber,
			wantCharts:  len(ifaceModuleEepromCharts) * 2,
			wantMetrics: map[string]int64{
				"iface_eth1_laser_bias_current_ma":                     12768,
				"iface_eth1_laser_output_power_dbm":                    4840,
				"iface_eth1_laser_output_power_mw":                     3048,
				"iface_eth1_module_temperature_c":                      36750,
				"iface_eth1_module_temperature_f":                      98150,
				"iface_eth1_module_voltage_v":                          3486,
				"iface_eth1_receiver_signal_average_optical_power_dbm": -40000,
				"iface_eth1_receiver_signal_average_optical_power_mw":  0,
				"iface_eth2_laser_bias_current_ma":                     12768,
				"iface_eth2_laser_output_power_dbm":                    4840,
				"iface_eth2_laser_output_power_mw":                     3048,
				"iface_eth2_module_temperature_c":                      36750,
				"iface_eth2_module_temperature_f":                      98150,
				"iface_eth2_module_voltage_v":                          3486,
				"iface_eth2_receiver_signal_average_optical_power_dbm": -40000,
				"iface_eth2_receiver_signal_average_optical_power_mw":  0,
			},
		},
		"module without ddm": {
			prepareMock: prepareMockEepromNoDdm,
			wantMetrics: nil,
		},
		"moduleEeprom() error": {
			prepareMock: prepareMockEepromError,
			wantMetrics: nil,
		},
		"moduleEeprom() unexpected response": {
			prepareMock: prepareMockEepromUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock
			collr.OpticInterfaces = "eth1 eth2"

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, test.wantCharts, "want charts")

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockEepromDdm() *mockEthtoolExec {
	return &mockEthtoolExec{
		moduleEepromData: dataModuleDdm,
	}
}

func prepareMockEepromDdmNoFiber() *mockEthtoolExec {
	return &mockEthtoolExec{
		moduleEepromData: dataModuleDdmNoFiber,
	}
}

func prepareMockEepromNoDdm() *mockEthtoolExec {
	return &mockEthtoolExec{
		moduleEepromData: dataModuleNoDdm,
	}
}

func prepareMockEepromError() *mockEthtoolExec {
	return &mockEthtoolExec{
		errOnModuleEeprom: true,
	}
}

func prepareMockEepromUnexpectedResponse() *mockEthtoolExec {
	return &mockEthtoolExec{
		moduleEepromData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockEthtoolExec struct {
	errOnModuleEeprom bool
	moduleEepromData  []byte
}

func (m *mockEthtoolExec) moduleEeprom(_ string) ([]byte, error) {
	if m.errOnModuleEeprom {
		return nil, errors.New("mock.moduleEeprom() error")
	}
	return m.moduleEepromData, nil
}
