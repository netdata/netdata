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

	dataIW, _ = os.ReadFile("testdata/iw_dev.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataIW":  dataIW,
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
				pf := New()
				pf.exec = prepareMockOK()
				_ = pf.Check()
				return pf
			},
		},
		"after collect": {
			prepare: func() *AP {
				pf := New()
				pf.exec = prepareMockOK()
				_ = pf.Collect()
				return pf
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
		prepareMock func() *mockIWExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOK,
		},
		"mail queue is empty": {
			wantFail:    false,
			prepareMock: prepareMockEmptyMailQueue,
		},
		"error on list call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnList,
		},
		"empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pf := New()
			mock := test.prepareMock()
			pf.exec = mock

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
		prepareMock func() *mockIWExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"emails": 12991,
				"size":   132422,
			},
		},
		"mail queue is empty": {
			prepareMock: prepareMockEmptyMailQueue,
			wantMetrics: map[string]int64{
				"emails": 0,
				"size":   0,
			},
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
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
			pf := New()
			mock := test.prepareMock()
			pf.exec = mock

			mx := pf.Collect()

			assert.Equal(t, test.wantMetrics, mx)
		})
	}
}

func prepareMockOK() *mockIWExec {
	return &mockIWExec{
		listData: dataIW,
	}
}

func prepareMockEmptyMailQueue() *mockIWExec {
	return &mockIWExec{
		listData: []byte("No output"),
	}
}

func prepareMockErrOnList() *mockIWExec {
	return &mockIWExec{
		errOnList: true,
	}
}

func prepareMockEmptyResponse() *mockIWExec {
	return &mockIWExec{}
}

func prepareMockUnexpectedResponse() *mockIWExec {
	return &mockIWExec{
		listData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockIWExec struct {
	errOnList bool
	listData  []byte
}

func (m *mockIWExec) list() ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	return m.listData, nil
}
