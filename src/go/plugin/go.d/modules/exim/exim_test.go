// SPDX-License-Identifier: GPL-3.0-or-later

package exim

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
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)

	}
}

func TestExim_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Exim{}, dataConfigJSON, dataConfigYAML)
}

func TestExim_Init(t *testing.T) {
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
			exim := New()
			exim.Config = test.config

			if test.wantFail {
				assert.Error(t, exim.Init())
			} else {
				assert.NoError(t, exim.Init())
			}
		})
	}
}

func TestExim_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Exim
	}{
		"not initialized exec": {
			prepare: func() *Exim {
				return New()
			},
		},
		"after check": {
			prepare: func() *Exim {
				exim := New()
				exim.exec = prepareMockOK()
				_ = exim.Check()
				return exim
			},
		},
		"after collect": {
			prepare: func() *Exim {
				exim := New()
				exim.exec = prepareMockOK()
				_ = exim.Collect()
				return exim
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			exim := test.prepare()

			assert.NotPanics(t, exim.Cleanup)
		})
	}
}

func TestEximCharts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestExim_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockEximExec
		wantFail    bool
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantFail:    false,
		},
		"error on exec": {
			prepareMock: prepareMockErr,
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
			exim := New()
			mock := test.prepareMock()
			exim.exec = mock

			if test.wantFail {
				assert.Error(t, exim.Check())
			} else {
				assert.NoError(t, exim.Check())
			}
		})
	}
}

func TestExim_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockEximExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"emails": 99,
			},
		},
		"error on exec": {
			prepareMock: prepareMockErr,
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
			exim := New()
			mock := test.prepareMock()
			exim.exec = mock

			mx := exim.Collect()

			assert.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Len(t, *exim.Charts(), len(charts))
				module.TestMetricsHasAllChartsDims(t, exim.Charts(), mx)
			}
		})
	}
}

func prepareMockOK() *mockEximExec {
	return &mockEximExec{
		data: []byte("99"),
	}
}

func prepareMockErr() *mockEximExec {
	return &mockEximExec{
		err: true,
	}
}

func prepareMockEmptyResponse() *mockEximExec {
	return &mockEximExec{}
}

func prepareMockUnexpectedResponse() *mockEximExec {
	return &mockEximExec{
		data: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockEximExec struct {
	err  bool
	data []byte
}

func (m *mockEximExec) countMessagesInQueue() ([]byte, error) {
	if m.err {
		return nil, errors.New("mock.countMessagesInQueue() error")
	}
	return m.data, nil
}
