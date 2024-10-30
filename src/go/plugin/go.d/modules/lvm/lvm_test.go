// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

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

func TestLVM_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &LVM{}, dataConfigJSON, dataConfigYAML)
}

func TestLVM_Init(t *testing.T) {
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
			lvm := New()
			lvm.Config = test.config

			if test.wantFail {
				assert.Error(t, lvm.Init())
			} else {
				assert.NoError(t, lvm.Init())
			}
		})
	}
}

func TestLVM_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *LVM
	}{
		"not initialized exec": {
			prepare: func() *LVM {
				return New()
			},
		},
		"after check": {
			prepare: func() *LVM {
				lvm := New()
				lvm.exec = prepareMockOK()
				_ = lvm.Check()
				return lvm
			},
		},
		"after collect": {
			prepare: func() *LVM {
				lvm := New()
				lvm.exec = prepareMockOK()
				_ = lvm.Collect()
				return lvm
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lvm := test.prepare()

			assert.NotPanics(t, lvm.Cleanup)
		})
	}
}

func TestLVM_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestLVM_Check(t *testing.T) {
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
			lvm := New()
			mock := test.prepareMock()
			lvm.exec = mock

			if test.wantFail {
				assert.Error(t, lvm.Check())
			} else {
				assert.NoError(t, lvm.Check())
			}
		})
	}
}

func TestLVM_Collect(t *testing.T) {
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
			lvm := New()
			mock := test.prepareMock()
			lvm.exec = mock

			mx := lvm.Collect()

			assert.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *lvm.Charts(), len(lvThinPoolChartsTmpl)*len(lvm.lvmThinPools))
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
