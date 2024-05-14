// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

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

	dataP212andP410i, _    = os.ReadFile("testdata/ssacli-P212_P410i.txt")
	dataP400ar, _          = os.ReadFile("testdata/ssacli-P400ar.txt")
	dataP400iUnassigned, _ = os.ReadFile("testdata/ssacli-P400i-unassigned.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataP212andP410i":    dataP212andP410i,
		"dataP400ar":          dataP400ar,
		"dataP400iUnassigned": dataP400iUnassigned,
	} {
		require.NotNil(t, data, name)
	}
}

func TestHpssa_Init(t *testing.T) {
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
			hpe := New()

			if test.wantFail {
				assert.Error(t, hpe.Init())
			} else {
				assert.NoError(t, hpe.Init())
			}
		})
	}
}

func TestHpssa_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Hpssa
	}{
		"not initialized exec": {
			prepare: func() *Hpssa {
				return New()
			},
		},
		"after check": {
			prepare: func() *Hpssa {
				hpe := New()
				hpe.exec = prepareMockOkP212andP410i()
				_ = hpe.Check()
				return hpe
			},
		},
		"after collect": {
			prepare: func() *Hpssa {
				hpe := New()
				hpe.exec = prepareMockOkP212andP410i()
				_ = hpe.Collect()
				return hpe
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hpe := test.prepare()

			assert.NotPanics(t, hpe.Cleanup)
		})
	}
}

func TestHpssa_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestHpssa_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSsacliExec
		wantFail    bool
	}{
		"success P212 and P410i": {
			wantFail:    false,
			prepareMock: prepareMockOkP212andP410i,
		},
		"success P400ar": {
			wantFail:    false,
			prepareMock: prepareMockOkP400ar,
		},
		"success P400i with Unassigned": {
			wantFail:    false,
			prepareMock: prepareMockOkP400iUnassigned,
		},
		"fails if error on controllersInfo()": {
			wantFail:    true,
			prepareMock: prepareMockErr,
		},
		"fails if empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
		"fails if unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hpe := New()
			mock := test.prepareMock()
			hpe.exec = mock

			if test.wantFail {
				assert.Error(t, hpe.Check())
			} else {
				assert.NoError(t, hpe.Check())
			}
		})
	}
}

func TestHpssa_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockSsacliExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success P212 and P410i": {
			prepareMock: prepareMockOkP212andP410i,
			wantCharts: (len(controllerChartsTmpl)*2 - 6) +
				len(arrayChartsTmpl)*3 +
				len(logicalDriveChartsTmpl)*3 +
				len(physicalDriveChartsTmpl)*18,
			wantMetrics: map[string]int64{
				"array_A_cntrl_P212_slot_5_status_nok":                    0,
				"array_A_cntrl_P212_slot_5_status_ok":                     1,
				"array_A_cntrl_P410i_slot_0_status_nok":                   0,
				"array_A_cntrl_P410i_slot_0_status_ok":                    1,
				"array_B_cntrl_P410i_slot_0_status_nok":                   0,
				"array_B_cntrl_P410i_slot_0_status_ok":                    1,
				"cntrl_P212_slot_5_cache_battery_status_nok":              0,
				"cntrl_P212_slot_5_cache_battery_status_ok":               0,
				"cntrl_P212_slot_5_cache_presence_status_not_present":     0,
				"cntrl_P212_slot_5_cache_presence_status_present":         1,
				"cntrl_P212_slot_5_cache_status_nok":                      0,
				"cntrl_P212_slot_5_cache_status_ok":                       1,
				"cntrl_P212_slot_5_status_nok":                            0,
				"cntrl_P212_slot_5_status_ok":                             1,
				"cntrl_P410i_slot_0_cache_battery_status_nok":             0,
				"cntrl_P410i_slot_0_cache_battery_status_ok":              0,
				"cntrl_P410i_slot_0_cache_presence_status_not_present":    0,
				"cntrl_P410i_slot_0_cache_presence_status_present":        1,
				"cntrl_P410i_slot_0_cache_status_nok":                     0,
				"cntrl_P410i_slot_0_cache_status_ok":                      1,
				"cntrl_P410i_slot_0_status_nok":                           0,
				"cntrl_P410i_slot_0_status_ok":                            1,
				"ld_1_array_A_cntrl_P212_slot_5_status_nok":               0,
				"ld_1_array_A_cntrl_P212_slot_5_status_ok":                1,
				"ld_1_array_A_cntrl_P410i_slot_0_status_nok":              0,
				"ld_1_array_A_cntrl_P410i_slot_0_status_ok":               1,
				"ld_2_array_B_cntrl_P410i_slot_0_status_nok":              0,
				"ld_2_array_B_cntrl_P410i_slot_0_status_ok":               1,
				"pd_1I:1:1_ld_2_array_B_cntrl_P410i_slot_0_status_nok":    0,
				"pd_1I:1:1_ld_2_array_B_cntrl_P410i_slot_0_status_ok":     1,
				"pd_1I:1:1_ld_2_array_B_cntrl_P410i_slot_0_temperature":   37,
				"pd_1I:1:2_ld_2_array_B_cntrl_P410i_slot_0_status_nok":    0,
				"pd_1I:1:2_ld_2_array_B_cntrl_P410i_slot_0_status_ok":     1,
				"pd_1I:1:2_ld_2_array_B_cntrl_P410i_slot_0_temperature":   37,
				"pd_1I:1:3_ld_2_array_B_cntrl_P410i_slot_0_status_nok":    0,
				"pd_1I:1:3_ld_2_array_B_cntrl_P410i_slot_0_status_ok":     1,
				"pd_1I:1:3_ld_2_array_B_cntrl_P410i_slot_0_temperature":   43,
				"pd_1I:1:4_ld_2_array_B_cntrl_P410i_slot_0_status_nok":    0,
				"pd_1I:1:4_ld_2_array_B_cntrl_P410i_slot_0_status_ok":     1,
				"pd_1I:1:4_ld_2_array_B_cntrl_P410i_slot_0_temperature":   44,
				"pd_2E:1:10_ld_na_array_na_cntrl_P212_slot_5_status_nok":  0,
				"pd_2E:1:10_ld_na_array_na_cntrl_P212_slot_5_status_ok":   1,
				"pd_2E:1:10_ld_na_array_na_cntrl_P212_slot_5_temperature": 35,
				"pd_2E:1:11_ld_na_array_na_cntrl_P212_slot_5_status_nok":  0,
				"pd_2E:1:11_ld_na_array_na_cntrl_P212_slot_5_status_ok":   1,
				"pd_2E:1:11_ld_na_array_na_cntrl_P212_slot_5_temperature": 34,
				"pd_2E:1:12_ld_na_array_na_cntrl_P212_slot_5_status_nok":  0,
				"pd_2E:1:12_ld_na_array_na_cntrl_P212_slot_5_status_ok":   1,
				"pd_2E:1:12_ld_na_array_na_cntrl_P212_slot_5_temperature": 31,
				"pd_2E:1:1_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:1_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:1_ld_1_array_A_cntrl_P212_slot_5_temperature":    33,
				"pd_2E:1:2_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:2_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:2_ld_1_array_A_cntrl_P212_slot_5_temperature":    34,
				"pd_2E:1:3_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:3_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:3_ld_1_array_A_cntrl_P212_slot_5_temperature":    35,
				"pd_2E:1:4_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:4_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:4_ld_1_array_A_cntrl_P212_slot_5_temperature":    35,
				"pd_2E:1:5_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:5_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:5_ld_1_array_A_cntrl_P212_slot_5_temperature":    34,
				"pd_2E:1:6_ld_1_array_A_cntrl_P212_slot_5_status_nok":     0,
				"pd_2E:1:6_ld_1_array_A_cntrl_P212_slot_5_status_ok":      1,
				"pd_2E:1:6_ld_1_array_A_cntrl_P212_slot_5_temperature":    33,
				"pd_2E:1:7_ld_na_array_na_cntrl_P212_slot_5_status_nok":   0,
				"pd_2E:1:7_ld_na_array_na_cntrl_P212_slot_5_status_ok":    1,
				"pd_2E:1:7_ld_na_array_na_cntrl_P212_slot_5_temperature":  30,
				"pd_2E:1:8_ld_na_array_na_cntrl_P212_slot_5_status_nok":   0,
				"pd_2E:1:8_ld_na_array_na_cntrl_P212_slot_5_status_ok":    1,
				"pd_2E:1:8_ld_na_array_na_cntrl_P212_slot_5_temperature":  33,
				"pd_2E:1:9_ld_na_array_na_cntrl_P212_slot_5_status_nok":   0,
				"pd_2E:1:9_ld_na_array_na_cntrl_P212_slot_5_status_ok":    1,
				"pd_2E:1:9_ld_na_array_na_cntrl_P212_slot_5_temperature":  30,
				"pd_2I:1:5_ld_1_array_A_cntrl_P410i_slot_0_status_nok":    0,
				"pd_2I:1:5_ld_1_array_A_cntrl_P410i_slot_0_status_ok":     1,
				"pd_2I:1:5_ld_1_array_A_cntrl_P410i_slot_0_temperature":   38,
				"pd_2I:1:6_ld_1_array_A_cntrl_P410i_slot_0_status_nok":    0,
				"pd_2I:1:6_ld_1_array_A_cntrl_P410i_slot_0_status_ok":     1,
				"pd_2I:1:6_ld_1_array_A_cntrl_P410i_slot_0_temperature":   36,
			},
		},
		"success P400ar": {
			prepareMock: prepareMockOkP400ar,
			wantCharts: len(controllerChartsTmpl)*1 +
				len(arrayChartsTmpl)*2 +
				len(logicalDriveChartsTmpl)*2 +
				len(physicalDriveChartsTmpl)*8,
			wantMetrics: map[string]int64{
				"array_A_cntrl_P440ar_slot_0_status_nok":                 0,
				"array_A_cntrl_P440ar_slot_0_status_ok":                  1,
				"array_B_cntrl_P440ar_slot_0_status_nok":                 0,
				"array_B_cntrl_P440ar_slot_0_status_ok":                  1,
				"cntrl_P440ar_slot_0_cache_battery_status_nok":           0,
				"cntrl_P440ar_slot_0_cache_battery_status_ok":            1,
				"cntrl_P440ar_slot_0_cache_presence_status_not_present":  0,
				"cntrl_P440ar_slot_0_cache_presence_status_present":      1,
				"cntrl_P440ar_slot_0_cache_status_nok":                   0,
				"cntrl_P440ar_slot_0_cache_status_ok":                    1,
				"cntrl_P440ar_slot_0_cache_temperature":                  41,
				"cntrl_P440ar_slot_0_status_nok":                         0,
				"cntrl_P440ar_slot_0_status_ok":                          1,
				"cntrl_P440ar_slot_0_temperature":                        47,
				"ld_1_array_A_cntrl_P440ar_slot_0_status_nok":            0,
				"ld_1_array_A_cntrl_P440ar_slot_0_status_ok":             1,
				"ld_2_array_B_cntrl_P440ar_slot_0_status_nok":            0,
				"ld_2_array_B_cntrl_P440ar_slot_0_status_ok":             1,
				"pd_1I:1:1_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_1I:1:1_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_1I:1:1_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 27,
				"pd_1I:1:2_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_1I:1:2_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_1I:1:2_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 28,
				"pd_1I:1:3_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_1I:1:3_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_1I:1:3_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 27,
				"pd_1I:1:4_ld_2_array_B_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_1I:1:4_ld_2_array_B_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_1I:1:4_ld_2_array_B_cntrl_P440ar_slot_0_temperature": 30,
				"pd_2I:1:5_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_2I:1:5_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_2I:1:5_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 26,
				"pd_2I:1:6_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_2I:1:6_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_2I:1:6_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 28,
				"pd_2I:1:7_ld_1_array_A_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_2I:1:7_ld_1_array_A_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_2I:1:7_ld_1_array_A_cntrl_P440ar_slot_0_temperature": 27,
				"pd_2I:1:8_ld_2_array_B_cntrl_P440ar_slot_0_status_nok":  0,
				"pd_2I:1:8_ld_2_array_B_cntrl_P440ar_slot_0_status_ok":   1,
				"pd_2I:1:8_ld_2_array_B_cntrl_P440ar_slot_0_temperature": 29,
			},
		},
		"success P400i with Unassigned": {
			prepareMock: prepareMockOkP400iUnassigned,
			wantCharts: (len(controllerChartsTmpl)*1 - 2) +
				len(arrayChartsTmpl)*1 +
				len(logicalDriveChartsTmpl)*1 +
				len(physicalDriveChartsTmpl)*4,
			wantMetrics: map[string]int64{
				"array_A_cntrl_P400i_slot_0_status_nok":                   0,
				"array_A_cntrl_P400i_slot_0_status_ok":                    1,
				"cntrl_P400i_slot_0_cache_battery_status_nok":             1,
				"cntrl_P400i_slot_0_cache_battery_status_ok":              0,
				"cntrl_P400i_slot_0_cache_presence_status_not_present":    0,
				"cntrl_P400i_slot_0_cache_presence_status_present":        1,
				"cntrl_P400i_slot_0_cache_status_nok":                     1,
				"cntrl_P400i_slot_0_cache_status_ok":                      0,
				"cntrl_P400i_slot_0_status_nok":                           0,
				"cntrl_P400i_slot_0_status_ok":                            1,
				"ld_1_array_A_cntrl_P400i_slot_0_status_nok":              0,
				"ld_1_array_A_cntrl_P400i_slot_0_status_ok":               1,
				"pd_1I:1:1_ld_na_array_na_cntrl_P400i_slot_0_status_nok":  0,
				"pd_1I:1:1_ld_na_array_na_cntrl_P400i_slot_0_status_ok":   1,
				"pd_1I:1:1_ld_na_array_na_cntrl_P400i_slot_0_temperature": 28,
				"pd_1I:1:2_ld_na_array_na_cntrl_P400i_slot_0_status_nok":  0,
				"pd_1I:1:2_ld_na_array_na_cntrl_P400i_slot_0_status_ok":   1,
				"pd_1I:1:2_ld_na_array_na_cntrl_P400i_slot_0_temperature": 28,
				"pd_1I:1:3_ld_1_array_A_cntrl_P400i_slot_0_status_nok":    0,
				"pd_1I:1:3_ld_1_array_A_cntrl_P400i_slot_0_status_ok":     1,
				"pd_1I:1:3_ld_1_array_A_cntrl_P400i_slot_0_temperature":   23,
				"pd_1I:1:4_ld_1_array_A_cntrl_P400i_slot_0_status_nok":    0,
				"pd_1I:1:4_ld_1_array_A_cntrl_P400i_slot_0_status_ok":     1,
				"pd_1I:1:4_ld_1_array_A_cntrl_P400i_slot_0_temperature":   23,
			},
		},
		"fails if error on controllersInfo()": {
			prepareMock: prepareMockErr,
			wantMetrics: nil,
			wantCharts:  0,
		},
		"fails if empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
			wantCharts:  0,
		},
		"fails if unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
			wantCharts:  0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			hpe := New()
			mock := test.prepareMock()
			hpe.exec = mock

			mx := hpe.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *hpe.Charts(), test.wantCharts)
			testMetricsHasAllChartsDims(t, hpe, mx)
		})
	}
}

func TestHpssa_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Hpssa{}, dataConfigJSON, dataConfigYAML)
}

func prepareMockOkP212andP410i() *mockSsacliExec {
	return &mockSsacliExec{
		infoData: dataP212andP410i,
	}
}

func prepareMockOkP400ar() *mockSsacliExec {
	return &mockSsacliExec{
		infoData: dataP400ar,
	}
}

func prepareMockOkP400iUnassigned() *mockSsacliExec {
	return &mockSsacliExec{
		infoData: dataP400iUnassigned,
	}
}

func prepareMockErr() *mockSsacliExec {
	return &mockSsacliExec{
		errOnInfo: true,
	}
}

func prepareMockEmptyResponse() *mockSsacliExec {
	return &mockSsacliExec{}
}

func prepareMockUnexpectedResponse() *mockSsacliExec {
	resp := []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`)
	return &mockSsacliExec{
		infoData: resp,
	}
}

type mockSsacliExec struct {
	errOnInfo bool
	infoData  []byte
}

func (m *mockSsacliExec) controllersInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.controllersInfo() error")
	}
	return m.infoData, nil
}

func testMetricsHasAllChartsDims(t *testing.T, hpe *Hpssa, mx map[string]int64) {
	for _, chart := range *hpe.Charts() {
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
