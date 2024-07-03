// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

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

	dataLogicalDevicesOld, _      = os.ReadFile("testdata/getconfig-ld-old.txt")
	dataPhysicalDevicesOld, _     = os.ReadFile("testdata/getconfig-pd-old.txt")
	dataLogicalDevicesCurrent, _  = os.ReadFile("testdata/getconfig-ld-current.txt")
	dataPhysicalDevicesCurrent, _ = os.ReadFile("testdata/getconfig-pd-current.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataLogicalDevicesOld":      dataLogicalDevicesOld,
		"dataPhysicalDevicesOld":     dataPhysicalDevicesOld,
		"dataLogicalDevicesCurrent":  dataLogicalDevicesCurrent,
		"dataPhysicalDevicesCurrent": dataPhysicalDevicesCurrent,
	} {
		require.NotNil(t, data, name)
	}
}

func TestAdaptecRaid_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &AdaptecRaid{}, dataConfigJSON, dataConfigYAML)
}

func TestAdaptecRaid_Init(t *testing.T) {
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
			adaptec := New()

			if test.wantFail {
				assert.Error(t, adaptec.Init())
			} else {
				assert.NoError(t, adaptec.Init())
			}
		})
	}
}

func TestAdaptecRaid_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *AdaptecRaid
	}{
		"not initialized exec": {
			prepare: func() *AdaptecRaid {
				return New()
			},
		},
		"after check": {
			prepare: func() *AdaptecRaid {
				adaptec := New()
				adaptec.exec = prepareMockOkCurrent()
				_ = adaptec.Check()
				return adaptec
			},
		},
		"after collect": {
			prepare: func() *AdaptecRaid {
				adaptec := New()
				adaptec.exec = prepareMockOkCurrent()
				_ = adaptec.Collect()
				return adaptec
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			adaptec := test.prepare()

			assert.NotPanics(t, adaptec.Cleanup)
		})
	}
}

func TestAdaptecRaid_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestAdaptecRaid_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockArcconfExec
		wantFail    bool
	}{
		"success case old data": {
			wantFail:    false,
			prepareMock: prepareMockOkOld,
		},
		"success case current data": {
			wantFail:    false,
			prepareMock: prepareMockOkCurrent,
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
			adaptec := New()
			mock := test.prepareMock()
			adaptec.exec = mock

			if test.wantFail {
				assert.Error(t, adaptec.Check())
			} else {
				assert.NoError(t, adaptec.Check())
			}
		})
	}
}

func TestAdaptecRaid_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockArcconfExec
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case old data": {
			prepareMock: prepareMockOkOld,
			wantCharts:  len(ldChartsTmpl)*1 + (len(pdChartsTmpl)-1)*4,
			wantMetrics: map[string]int64{
				"ld_0_health_state_critical": 0,
				"ld_0_health_state_ok":       1,
				"pd_0_health_state_critical": 0,
				"pd_0_health_state_ok":       1,
				"pd_0_smart_warnings":        0,
				"pd_1_health_state_critical": 0,
				"pd_1_health_state_ok":       1,
				"pd_1_smart_warnings":        0,
				"pd_2_health_state_critical": 0,
				"pd_2_health_state_ok":       1,
				"pd_2_smart_warnings":        0,
				"pd_3_health_state_critical": 0,
				"pd_3_health_state_ok":       1,
				"pd_3_smart_warnings":        0,
			},
		},
		"success case current data": {
			prepareMock: prepareMockOkCurrent,
			wantCharts:  len(ldChartsTmpl)*1 + (len(pdChartsTmpl)-1)*6,
			wantMetrics: map[string]int64{
				"ld_0_health_state_critical": 0,
				"ld_0_health_state_ok":       1,
				"pd_0_health_state_critical": 0,
				"pd_0_health_state_ok":       1,
				"pd_0_smart_warnings":        0,
				"pd_1_health_state_critical": 0,
				"pd_1_health_state_ok":       1,
				"pd_1_smart_warnings":        0,
				"pd_2_health_state_critical": 0,
				"pd_2_health_state_ok":       1,
				"pd_2_smart_warnings":        0,
				"pd_3_health_state_critical": 0,
				"pd_3_health_state_ok":       1,
				"pd_3_smart_warnings":        0,
				"pd_4_health_state_critical": 0,
				"pd_4_health_state_ok":       1,
				"pd_4_smart_warnings":        0,
				"pd_5_health_state_critical": 0,
				"pd_5_health_state_ok":       1,
				"pd_5_smart_warnings":        0,
			},
		},
		"err on exec": {
			prepareMock: prepareMockErr,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
		},
		"empty response": {
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			adaptec := New()
			mock := test.prepareMock()
			adaptec.exec = mock

			mx := adaptec.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *adaptec.Charts(), test.wantCharts)
		})
	}
}

func prepareMockOkOld() *mockArcconfExec {
	return &mockArcconfExec{
		ldData: dataLogicalDevicesOld,
		pdData: dataPhysicalDevicesOld,
	}
}

func prepareMockOkCurrent() *mockArcconfExec {
	return &mockArcconfExec{
		ldData: dataLogicalDevicesCurrent,
		pdData: dataPhysicalDevicesCurrent,
	}
}

func prepareMockErr() *mockArcconfExec {
	return &mockArcconfExec{
		errOnInfo: true,
	}
}

func prepareMockUnexpectedResponse() *mockArcconfExec {
	resp := []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`)
	return &mockArcconfExec{
		ldData: resp,
		pdData: resp,
	}
}

func prepareMockEmptyResponse() *mockArcconfExec {
	return &mockArcconfExec{}
}

type mockArcconfExec struct {
	errOnInfo bool
	ldData    []byte
	pdData    []byte
}

func (m *mockArcconfExec) logicalDevicesInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.logicalDevicesInfo() error")
	}
	return m.ldData, nil
}

func (m *mockArcconfExec) physicalDevicesInfo() ([]byte, error) {
	if m.errOnInfo {
		return nil, errors.New("mock.physicalDevicesInfo() error")
	}
	return m.pdData, nil
}
