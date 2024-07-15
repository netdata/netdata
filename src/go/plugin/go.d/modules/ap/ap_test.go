// SPDX-License-Identifier: GPL-3.0-or-later

package ap

import (
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

	dataIWDevAP, _       = os.ReadFile("testdata/iw_dev_ap.txt")
	dataIWDevManaged, _  = os.ReadFile("testdata/iw_dev_managed.txt")
	dataIWStationDump, _ = os.ReadFile("testdata/station_dump.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":    dataConfigJSON,
		"dataConfigYAML":    dataConfigYAML,
		"dataIWDev":         dataIWDevAP,
		"dataIWStationDump": dataIWStationDump,
	} {
		require.NotNil(t, data, name)
	}
}

func TestAP_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &AP{}, dataConfigJSON, dataConfigYAML)
}

func TestAP_Init(t *testing.T) {
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
				BinaryPath: "IW!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			pf.Config = test.config

			if test.wantFail {
				assert.Error(t, pf.Init())
			} else {
				assert.NoError(t, pf.Init())
			}
		})
	}
}

func TestAP_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *AP
	}{
		"not initialized exec": {
			prepare: func() *AP {
				return New()
			},
		},
		"after check": {
			prepare: func() *AP {
				a := New()
				a.execIWDev = prepareMockOKIWDev()
				a.execIWStationDump = prepareMockOKIWStationDump()
				_ = a.Check()
				return a
			},
		},
		"after collect": {
			prepare: func() *AP {
				a := New()
				a.execIWDev = prepareMockOKIWDev()
				a.execIWStationDump = prepareMockOKIWStationDump()
				_ = a.Collect()
				return a
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := test.prepare()

			assert.NotPanics(t, pf.Cleanup)
		})
	}
}

func TestAP_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestAP_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMockIWDev         func() *mockExecIWDev
		prepareMockIWStationDump func() *mockExecIWStationDump
		wantFail                 bool
	}{
		"success case": {
			wantFail:                 false,
			prepareMockIWDev:         prepareMockOKIWDev,
			prepareMockIWStationDump: prepareMockOKIWStationDump,
		},
		"no ap devices": {
			prepareMockIWDev:         prepareMockNoAPDevicesIWDev,
			prepareMockIWStationDump: prepareMockNoAPDevicesIWStationDump,
			wantFail:                 true,
		},
		"error on list call": {
			prepareMockIWDev:         prepareMockErrOnListIWDev,
			prepareMockIWStationDump: prepareMockErrOnListIWStationDump,
			wantFail:                 true,
		},
		"unexpected response": {
			prepareMockIWDev:         prepareMockUnexpectedResponseIWDev,
			prepareMockIWStationDump: prepareMockUnexpectedResponseIWStationDump,
			wantFail:                 true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			mock := test.prepareMockIWDev()
			pf.execIWDev = mock
			mockb := test.prepareMockIWStationDump()
			pf.execIWStationDump = mockb

			if test.wantFail {
				assert.Error(t, pf.Check())
			} else {
				assert.NoError(t, pf.Check())
			}
		})
	}
}

func TestAP_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMockIWDev         func() *mockExecIWDev
		prepareMockIWStationDump func() *mockExecIWStationDump
		wantMetrics              map[string]int64
	}{
		"success case": {
			prepareMockIWDev:         prepareMockOKIWDev,
			prepareMockIWStationDump: prepareMockOKIWStationDump,
			wantMetrics: map[string]int64{
				"ap_wlp1s0_average_signal":   -34000,
				"ap_wlp1s0_bitrate_receive":  65500,
				"ap_wlp1s0_bitrate_transmit": 65000,
				"ap_wlp1s0_bw_received":      95117,
				"ap_wlp1s0_bw_sent":          8270,
				"ap_wlp1s0_clients":          2,
				"ap_wlp1s0_issues_failures":  0,
				"ap_wlp1s0_issues_retries":   0,
				"ap_wlp1s0_packets_received": 2531,
				"ap_wlp1s0_packets_sent":     38,
			},
		},
		"no ap devices": {
			prepareMockIWDev:         prepareMockNoAPDevicesIWDev,
			prepareMockIWStationDump: prepareMockNoAPDevicesIWStationDump,
			wantMetrics:              nil,
		},
		"error on list call": {
			prepareMockIWDev:         prepareMockErrOnListIWDev,
			prepareMockIWStationDump: prepareMockErrOnListIWStationDump,
			wantMetrics:              nil,
		},
		"unexpected response": {
			prepareMockIWDev:         prepareMockUnexpectedResponseIWDev,
			prepareMockIWStationDump: prepareMockUnexpectedResponseIWStationDump,
			wantMetrics:              nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			a := New()
			mock := test.prepareMockIWDev()
			a.execIWDev = mock
			mockb := test.prepareMockIWStationDump()
			a.execIWStationDump = mockb

			mx := a.Collect()

			assert.Equal(t, test.wantMetrics, mx)
		})
	}
}

func prepareMockOKIWDev() *mockExecIWDev {
	return &mockExecIWDev{
		listData: dataIWDevAP,
	}
}

func prepareMockOKIWStationDump() *mockExecIWStationDump {
	return &mockExecIWStationDump{
		listData: dataIWStationDump,
	}
}

func prepareMockNoAPDevicesIWDev() *mockExecIWDev {
	return &mockExecIWDev{
		listData: dataIWDevManaged,
	}
}

func prepareMockNoAPDevicesIWStationDump() *mockExecIWStationDump {
	return &mockExecIWStationDump{
		listData: []byte(""),
	}
}

func prepareMockErrOnListIWDev() *mockExecIWDev {
	return &mockExecIWDev{
		errOnList: true,
	}
}

func prepareMockErrOnListIWStationDump() *mockExecIWStationDump {
	return &mockExecIWStationDump{
		errOnList: true,
	}
}

func prepareMockUnexpectedResponseIWDev() *mockExecIWDev {
	return &mockExecIWDev{
		listData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

func prepareMockUnexpectedResponseIWStationDump() *mockExecIWStationDump {
	return &mockExecIWStationDump{
		listData: []byte(``),
	}
}

type mockExecIWDev struct {
	errOnList bool
	listData  []byte
}

type mockExecIWStationDump struct {
	errOnList bool
	listData  []byte
}

func (m *mockExecIWDev) list() ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	return m.listData, nil
}

func (m *mockExecIWStationDump) list(ifaceName string) ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}
	return m.listData, nil
}
