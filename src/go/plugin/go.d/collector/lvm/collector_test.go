// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

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

	dataLvsReportJson, _       = os.ReadFile("testdata/lvs-report.json")
	dataLvsReportNoThinJson, _ = os.ReadFile("testdata/lvs-report-no-thin.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataLvsReportJson":       dataLvsReportJson,
		"dataLvsReportNoThinJson": dataLvsReportNoThinJson,
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
		"success with default config": {
			wantFail: false,
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
				collr.exec = prepareMockOK()
				_ = collr.Check(context.Background())
				return collr
			},
		},
		"after collect": {
			prepare: func() *Collector {
				collr := New()
				collr.exec = prepareMockOK()
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
		prepareMock func() *mockLvmCliExec
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"no thin volumes": {
			prepareMock: prepareMockNoThinVolumes,
			wantFail:    true,
		},
		"error on lvs report call": {
			prepareMock: prepareMockErrOnLvsReportJson,
			wantFail:    true,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantFail:    true,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantFail:    true,
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
		prepareMock func() *mockLvmCliExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"lv_root_vg_cm-vg_data_percent":     7889,
				"lv_root_vg_cm-vg_metadata_percent": 1925,
			},
		},
		"no thin volumes": {
			prepareMock: prepareMockNoThinVolumes,
			wantMetrics: nil,
		},
		"error on lvs report call": {
			prepareMock: prepareMockErrOnLvsReportJson,
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
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *collr.Charts(), len(lvThinPoolChartsTmpl)*len(collr.lvmThinPools))
			}
		})
	}
}

func prepareMockOK() *mockLvmCliExec {
	return &mockLvmCliExec{
		lvsReportJsonData: dataLvsReportJson,
	}
}

func prepareMockNoThinVolumes() *mockLvmCliExec {
	return &mockLvmCliExec{
		lvsReportJsonData: dataLvsReportNoThinJson,
	}
}

func prepareMockErrOnLvsReportJson() *mockLvmCliExec {
	return &mockLvmCliExec{
		errOnLvsReportJson: true,
	}
}

func prepareMockEmptyResponse() *mockLvmCliExec {
	return &mockLvmCliExec{}
}

func prepareMockUnexpectedResponse() *mockLvmCliExec {
	return &mockLvmCliExec{
		lvsReportJsonData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockLvmCliExec struct {
	errOnLvsReportJson bool
	lvsReportJsonData  []byte
}

func (m *mockLvmCliExec) lvsReportJson() ([]byte, error) {
	if m.errOnLvsReportJson {
		return nil, errors.New("mock.lvsReportJson() error")
	}

	return m.lvsReportJsonData, nil
}
