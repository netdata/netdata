// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package adaptecraid

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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

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
				collr.exec = prepareMockOkCurrent()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOkCurrent()
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
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

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
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)
			assert.Len(t, *collr.Charts(), test.wantCharts)
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
