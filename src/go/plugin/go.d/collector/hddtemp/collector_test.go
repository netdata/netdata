// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
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
		"not initialized": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				collr := New()
				collr.conn = prepareMockAllDisksOk()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.conn = prepareMockAllDisksOk()
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
			collr := New()
			collr.conn = test.prepareMock()

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
			collr := New()
			collr.conn = test.prepareMock()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantCharts, "wantCharts")

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
	errOnQueryHddTemp bool
	hddTempLine       string
}

func (m *mockHddTempConn) queryHddTemp() (string, error) {
	if m.errOnQueryHddTemp {
		return "", errors.New("mock.queryHddTemp() error")
	}
	return m.hddTempLine, nil
}
