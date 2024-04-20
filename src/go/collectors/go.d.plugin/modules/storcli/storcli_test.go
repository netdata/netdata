// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

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

	dataMegaControllerInfo, _ = os.ReadFile("testdata/megaraid-controllers-info.json")
	dataMegaDrivesInfo, _     = os.ReadFile("testdata/megaraid-drives-info.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataMegaControllerInfo": dataMegaControllerInfo,
		"dataMegaDrivesInfo":     dataMegaDrivesInfo,
	} {
		require.NotNil(t, data, name)
	}
}

func TestStorCli_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &StorCli{}, dataConfigJSON, dataConfigYAML)
}

func TestStorCli_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'ndsudo' not found": {
			wantFail: true,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stor := New()

			if test.wantFail {
				assert.Error(t, stor.Init())
			} else {
				assert.NoError(t, stor.Init())
			}
		})
	}
}

func TestStorCli_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *StorCli
	}{
		"not initialized exec": {
			prepare: func() *StorCli {
				return New()
			},
		},
		"after check": {
			prepare: func() *StorCli {
				stor := New()
				stor.exec = prepareMockMegaRaidOK()
				_ = stor.Check()
				return stor
			},
		},
		"after collect": {
			prepare: func() *StorCli {
				stor := New()
				stor.exec = prepareMockMegaRaidOK()
				_ = stor.Collect()
				return stor
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stor := test.prepare()

			assert.NotPanics(t, stor.Cleanup)
		})
	}
}

func TestStorCli_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestStorCli_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockStorCliExec
		wantFail    bool
	}{
		"success MegaRAID controller": {
			wantFail:    false,
			prepareMock: prepareMockMegaRaidOK,
		},
		"err on exec": {
			wantFail:    true,
			prepareMock: prepareMockErr,
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
			stor := New()
			mock := test.prepareMock()
			stor.exec = mock

			if test.wantFail {
				assert.Error(t, stor.Check())
			} else {
				assert.NoError(t, stor.Check())
			}
		})
	}
}

func TestStorCli_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockStorCliExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success MegaRAID controller": {
			prepareMock: prepareMockMegaRaidOK,
			wantCharts:  len(controllerChartsTmpl)*1 + len(physDriveChartsTmpl)*6 + len(bbuChartsTmpl)*1,
			wantMetrics: map[string]int64{
				"bbu_0_cntrl_0_temperature":                                       34,
				"cntrl_0_bbu_status_healthy":                                      1,
				"cntrl_0_bbu_status_na":                                           0,
				"cntrl_0_bbu_status_unhealthy":                                    0,
				"cntrl_0_status_degraded":                                         0,
				"cntrl_0_status_failed":                                           0,
				"cntrl_0_status_optimal":                                          1,
				"cntrl_0_status_partially_degraded":                               0,
				"phys_drive_5000C500C36C8BCD_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500C36C8BCD_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500C36C8BCD_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500C36C8BCD_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500C36C8BCD_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500C36C8BCD_cntrl_0_temperature":                 28,
				"phys_drive_5000C500D59840FE_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500D59840FE_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500D59840FE_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500D59840FE_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500D59840FE_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500D59840FE_cntrl_0_temperature":                 28,
				"phys_drive_5000C500D6061539_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500D6061539_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500D6061539_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500D6061539_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500D6061539_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500D6061539_cntrl_0_temperature":                 28,
				"phys_drive_5000C500DC79B194_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500DC79B194_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500DC79B194_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500DC79B194_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500DC79B194_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500DC79B194_cntrl_0_temperature":                 28,
				"phys_drive_5000C500E54F4EBB_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500E54F4EBB_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500E54F4EBB_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500E54F4EBB_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500E54F4EBB_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500E54F4EBB_cntrl_0_temperature":                 28,
				"phys_drive_5000C500E5659BA7_cntrl_0_media_error_count":           0,
				"phys_drive_5000C500E5659BA7_cntrl_0_other_error_count":           0,
				"phys_drive_5000C500E5659BA7_cntrl_0_predictive_failure_count":    0,
				"phys_drive_5000C500E5659BA7_cntrl_0_smart_alert_status_active":   0,
				"phys_drive_5000C500E5659BA7_cntrl_0_smart_alert_status_inactive": 1,
				"phys_drive_5000C500E5659BA7_cntrl_0_temperature":                 27,
			},
		},
		"err on exec": {
			prepareMock: prepareMockErr,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stor := New()
			mock := test.prepareMock()
			stor.exec = mock

			mx := stor.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *stor.Charts(), test.wantCharts)
			testMetricsHasAllChartsDims(t, stor, mx)
		})
	}
}

func prepareMockMegaRaidOK() *mockStorCliExec {
	return &mockStorCliExec{
		controllersInfoData: dataMegaControllerInfo,
		drivesInfoData:      dataMegaDrivesInfo,
	}
}

func prepareMockErr() *mockStorCliExec {
	return &mockStorCliExec{
		errOnInfo: true,
	}
}

func prepareMockUnexpectedResponse() *mockStorCliExec {
	resp := []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`)
	return &mockStorCliExec{
		controllersInfoData: resp,
		drivesInfoData:      resp,
	}
}

func prepareMockEmptyResponse() *mockStorCliExec {
	return &mockStorCliExec{}
}

type mockStorCliExec struct {
	errOnInfo           bool
	controllersInfoData []byte
	drivesInfoData      []byte
}

func (m *mockStorCliExec) controllersInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.controllerInfo() error")
	}
	return m.controllersInfoData, nil
}

func (m *mockStorCliExec) drivesInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.drivesInfo() error")
	}
	return m.drivesInfoData, nil
}

func testMetricsHasAllChartsDims(t *testing.T, stor *StorCli, mx map[string]int64) {
	for _, chart := range *stor.Charts() {
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
