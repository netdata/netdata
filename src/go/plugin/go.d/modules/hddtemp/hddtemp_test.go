// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

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

	dataAllOK, _    = os.ReadFile("testdata/hddtemp-all-ok.txt")
	dataAllSleep, _ = os.ReadFile("testdata/hddtemp-all-sleep.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataAllOK":    dataAllOK,
		"dataAllSleep": dataAllSleep,
	} {
		require.NotNil(t, data, name)
	}
}

func TestHddTemp_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &HddTemp{}, dataConfigJSON, dataConfigYAML)
}

func TestHddTemp_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails if address not set": {
			wantFail: true,
			config: func() Config {
				conf := New().Config
				conf.Address = ""
				return conf
			}(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hdd := New()
			hdd.Config = test.config

			if test.wantFail {
				assert.Error(t, hdd.Init())
			} else {
				assert.NoError(t, hdd.Init())
			}
		})
	}
}

func TestHddTemp_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *HddTemp
	}{
		"not initialized": {
			prepare: func() *HddTemp {
				return New()
			},
		},
		"after check": {
			prepare: func() *HddTemp {
				hdd := New()
				hdd.newHddTempConn = func(config Config) hddtempConn { return prepareMockAllDisksOk() }
				_ = hdd.Check()
				return hdd
			},
		},
		"after collect": {
			prepare: func() *HddTemp {
				hdd := New()
				hdd.newHddTempConn = func(config Config) hddtempConn { return prepareMockAllDisksOk() }
				_ = hdd.Collect()
				return hdd
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hdd := test.prepare()

			assert.NotPanics(t, hdd.Cleanup)
		})
	}
}

func TestHddTemp_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestHddTemp_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockHddTempConn
		wantFail    bool
	}{
		"all disks ok": {
			wantFail:    false,
			prepareMock: prepareMockAllDisksOk,
		},
		"all disks sleep": {
			wantFail:    false,
			prepareMock: prepareMockAllDisksSleep,
		},
		"err on connect": {
			wantFail:    true,
			prepareMock: prepareMockErrOnConnect,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
		"empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hdd := New()
			mock := test.prepareMock()
			hdd.newHddTempConn = func(config Config) hddtempConn { return mock }

			if test.wantFail {
				assert.Error(t, hdd.Check())
			} else {
				assert.NoError(t, hdd.Check())
			}
		})
	}
}

func TestHddTemp_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock    func() *mockHddTempConn
		wantMetrics    map[string]int64
		wantDisconnect bool
		wantCharts     int
	}{
		"all disks ok": {
			prepareMock:    prepareMockAllDisksOk,
			wantDisconnect: true,
			wantCharts:     2 * 4,
			wantMetrics: map[string]int64{
				"disk_sda_temp_sensor_status_err": 0,
				"disk_sda_temp_sensor_status_na":  0,
				"disk_sda_temp_sensor_status_nos": 0,
				"disk_sda_temp_sensor_status_ok":  1,
				"disk_sda_temp_sensor_status_slp": 0,
				"disk_sda_temp_sensor_status_unk": 0,
				"disk_sda_temperature":            50,
				"disk_sdb_temp_sensor_status_err": 0,
				"disk_sdb_temp_sensor_status_na":  0,
				"disk_sdb_temp_sensor_status_nos": 0,
				"disk_sdb_temp_sensor_status_ok":  1,
				"disk_sdb_temp_sensor_status_slp": 0,
				"disk_sdb_temp_sensor_status_unk": 0,
				"disk_sdb_temperature":            49,
				"disk_sdc_temp_sensor_status_err": 0,
				"disk_sdc_temp_sensor_status_na":  0,
				"disk_sdc_temp_sensor_status_nos": 0,
				"disk_sdc_temp_sensor_status_ok":  1,
				"disk_sdc_temp_sensor_status_slp": 0,
				"disk_sdc_temp_sensor_status_unk": 0,
				"disk_sdc_temperature":            27,
				"disk_sdd_temp_sensor_status_err": 0,
				"disk_sdd_temp_sensor_status_na":  0,
				"disk_sdd_temp_sensor_status_nos": 0,
				"disk_sdd_temp_sensor_status_ok":  1,
				"disk_sdd_temp_sensor_status_slp": 0,
				"disk_sdd_temp_sensor_status_unk": 0,
				"disk_sdd_temperature":            29,
			},
		},
		"all disks sleep": {
			prepareMock:    prepareMockAllDisksSleep,
			wantDisconnect: true,
			wantCharts:     3,
			wantMetrics: map[string]int64{
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_err":            0,
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_na":             0,
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_nos":            0,
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_ok":             0,
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_slp":            1,
				"disk_ata-HUP722020APA330_BFGWU7WF_temp_sensor_status_unk":            0,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_err":            0,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_na":             0,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_nos":            0,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_ok":             0,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_slp":            1,
				"disk_ata-HUP722020APA330_BFJ0WS3F_temp_sensor_status_unk":            0,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_err": 0,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_na":  0,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_nos": 0,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_ok":  0,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_slp": 1,
				"disk_ata-WDC_WD10EARS-00Y5B1_WD-WCAV5R693922_temp_sensor_status_unk": 0,
			},
		},
		"err on connect": {
			prepareMock:    prepareMockErrOnConnect,
			wantDisconnect: false,
		},
		"unexpected response": {
			prepareMock:    prepareMockUnexpectedResponse,
			wantDisconnect: true,
		},
		"empty response": {
			prepareMock:    prepareMockEmptyResponse,
			wantDisconnect: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hdd := New()
			mock := test.prepareMock()
			hdd.newHddTempConn = func(config Config) hddtempConn { return mock }

			mx := hdd.Collect()

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *hdd.Charts(), test.wantCharts, "wantCharts")

			assert.Equal(t, test.wantDisconnect, mock.disconnectCalled, "disconnectCalled")

			module.TestMetricsHasAllChartsDims(t, hdd.Charts(), mx)
		})
	}
}

func prepareMockAllDisksOk() *mockHddTempConn {
	return &mockHddTempConn{
		hddTempLine: string(dataAllOK),
	}
}

func prepareMockAllDisksSleep() *mockHddTempConn {
	return &mockHddTempConn{
		hddTempLine: string(dataAllSleep),
	}
}

func prepareMockErrOnConnect() *mockHddTempConn {
	return &mockHddTempConn{
		errOnConnect: true,
	}
}

func prepareMockUnexpectedResponse() *mockHddTempConn {
	return &mockHddTempConn{
		hddTempLine: "Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
	}
}

func prepareMockEmptyResponse() *mockHddTempConn {
	return &mockHddTempConn{
		hddTempLine: "",
	}
}

type mockHddTempConn struct {
	errOnConnect      bool
	errOnQueryHddTemp bool
	hddTempLine       string
	disconnectCalled  bool
}

func (m *mockHddTempConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockHddTempConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockHddTempConn) queryHddTemp() (string, error) {
	if m.errOnQueryHddTemp {
		return "", errors.New("mock.queryHddTemp() error")
	}
	return m.hddTempLine, nil
}
