// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

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

	dataBBUInfoOld, _     = os.ReadFile("testdata/mega-bbu-info-old.txt")
	dataBBUInfoRecent, _  = os.ReadFile("testdata/mega-bbu-info-recent.txt")
	dataPhysDrivesInfo, _ = os.ReadFile("testdata/mega-phys-drives-info.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataBBUInfoOld":     dataBBUInfoOld,
		"dataBBUInfoRecent":  dataBBUInfoRecent,
		"dataPhysDrivesInfo": dataPhysDrivesInfo,
	} {
		require.NotNil(t, data, name)
	}
}

func TestMegaCli_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &MegaCli{}, dataConfigJSON, dataConfigYAML)
}

func TestMegaCli_Init(t *testing.T) {
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
			mega := New()

			if test.wantFail {
				assert.Error(t, mega.Init())
			} else {
				assert.NoError(t, mega.Init())
			}
		})
	}
}

func TestMegaCli_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *MegaCli
	}{
		"not initialized exec": {
			prepare: func() *MegaCli {
				return New()
			},
		},
		"after check": {
			prepare: func() *MegaCli {
				mega := New()
				mega.exec = prepareMockOK()
				_ = mega.Check()
				return mega
			},
		},
		"after collect": {
			prepare: func() *MegaCli {
				mega := New()
				mega.exec = prepareMockOK()
				_ = mega.Collect()
				return mega
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mega := test.prepare()

			assert.NotPanics(t, mega.Cleanup)
		})
	}
}

func TestMegaCli_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestMegaCli_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockMegaCliExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOK,
		},
		"success case old bbu": {
			wantFail:    false,
			prepareMock: prepareMockOldBbuOK,
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
			mega := New()
			mock := test.prepareMock()
			mega.exec = mock

			if test.wantFail {
				assert.Error(t, mega.Check())
			} else {
				assert.NoError(t, mega.Check())
			}
		})
	}
}

func TestMegaCli_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockMegaCliExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantCharts:  len(adapterChartsTmpl)*1 + len(physDriveChartsTmpl)*8 + len(bbuChartsTmpl)*1,
			wantMetrics: map[string]int64{
				"adapter_0_health_state_degraded":                      0,
				"adapter_0_health_state_failed":                        0,
				"adapter_0_health_state_optimal":                       1,
				"adapter_0_health_state_partially_degraded":            0,
				"bbu_adapter_0_absolute_state_of_charge":               63,
				"bbu_adapter_0_capacity_degradation_perc":              10,
				"bbu_adapter_0_cycle_count":                            4,
				"bbu_adapter_0_relative_state_of_charge":               71,
				"bbu_adapter_0_temperature":                            33,
				"phys_drive_5002538c00019b96_media_error_count":        0,
				"phys_drive_5002538c00019b96_predictive_failure_count": 0,
				"phys_drive_5002538c4002da83_media_error_count":        0,
				"phys_drive_5002538c4002da83_predictive_failure_count": 0,
				"phys_drive_5002538c4002dade_media_error_count":        0,
				"phys_drive_5002538c4002dade_predictive_failure_count": 0,
				"phys_drive_5002538c4002e6e9_media_error_count":        0,
				"phys_drive_5002538c4002e6e9_predictive_failure_count": 0,
				"phys_drive_5002538c4002e707_media_error_count":        0,
				"phys_drive_5002538c4002e707_predictive_failure_count": 0,
				"phys_drive_5002538c4002e70f_media_error_count":        0,
				"phys_drive_5002538c4002e70f_predictive_failure_count": 0,
				"phys_drive_5002538c4002e712_media_error_count":        0,
				"phys_drive_5002538c4002e712_predictive_failure_count": 0,
				"phys_drive_5002538c4002e713_media_error_count":        0,
				"phys_drive_5002538c4002e713_predictive_failure_count": 0,
			},
		},
		"success case old bbu": {
			prepareMock: prepareMockOldBbuOK,
			wantCharts:  len(adapterChartsTmpl)*1 + len(physDriveChartsTmpl)*8 + len(bbuChartsTmpl)*1,
			wantMetrics: map[string]int64{
				"adapter_0_health_state_degraded":                      0,
				"adapter_0_health_state_failed":                        0,
				"adapter_0_health_state_optimal":                       1,
				"adapter_0_health_state_partially_degraded":            0,
				"bbu_adapter_0_absolute_state_of_charge":               83,
				"bbu_adapter_0_capacity_degradation_perc":              17,
				"bbu_adapter_0_cycle_count":                            61,
				"bbu_adapter_0_relative_state_of_charge":               100,
				"bbu_adapter_0_temperature":                            31,
				"phys_drive_5002538c00019b96_media_error_count":        0,
				"phys_drive_5002538c00019b96_predictive_failure_count": 0,
				"phys_drive_5002538c4002da83_media_error_count":        0,
				"phys_drive_5002538c4002da83_predictive_failure_count": 0,
				"phys_drive_5002538c4002dade_media_error_count":        0,
				"phys_drive_5002538c4002dade_predictive_failure_count": 0,
				"phys_drive_5002538c4002e6e9_media_error_count":        0,
				"phys_drive_5002538c4002e6e9_predictive_failure_count": 0,
				"phys_drive_5002538c4002e707_media_error_count":        0,
				"phys_drive_5002538c4002e707_predictive_failure_count": 0,
				"phys_drive_5002538c4002e70f_media_error_count":        0,
				"phys_drive_5002538c4002e70f_predictive_failure_count": 0,
				"phys_drive_5002538c4002e712_media_error_count":        0,
				"phys_drive_5002538c4002e712_predictive_failure_count": 0,
				"phys_drive_5002538c4002e713_media_error_count":        0,
				"phys_drive_5002538c4002e713_predictive_failure_count": 0,
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
			mega := New()
			mock := test.prepareMock()
			mega.exec = mock

			mx := mega.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *mega.Charts(), test.wantCharts)
			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, mega.Charts(), mx)
			}
		})
	}
}

func prepareMockOK() *mockMegaCliExec {
	return &mockMegaCliExec{
		physDrivesInfoData: dataPhysDrivesInfo,
		bbuInfoData:        dataBBUInfoRecent,
	}
}

func prepareMockOldBbuOK() *mockMegaCliExec {
	return &mockMegaCliExec{
		physDrivesInfoData: dataPhysDrivesInfo,
		bbuInfoData:        dataBBUInfoOld,
	}
}

func prepareMockErr() *mockMegaCliExec {
	return &mockMegaCliExec{
		errOnInfo: true,
	}
}

func prepareMockUnexpectedResponse() *mockMegaCliExec {
	resp := []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`)
	return &mockMegaCliExec{
		physDrivesInfoData: resp,
		bbuInfoData:        resp,
	}
}

func prepareMockEmptyResponse() *mockMegaCliExec {
	return &mockMegaCliExec{}
}

type mockMegaCliExec struct {
	errOnInfo          bool
	physDrivesInfoData []byte
	bbuInfoData        []byte
}

func (m *mockMegaCliExec) physDrivesInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.physDrivesInfo() error")
	}
	return m.physDrivesInfoData, nil
}

func (m *mockMegaCliExec) bbuInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.bbuInfo() error")
	}
	return m.bbuInfoData, nil
}
