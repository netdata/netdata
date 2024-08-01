// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

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

	dataStats, _            = os.ReadFile("testdata/stats.txt")
	dataListTubes, _        = os.ReadFile("testdata/list-tubes.txt")
	dataStatsTubeDefault, _ = os.ReadFile("testdata/stats-tube_default.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,

		"dataStats":            dataStats,
		"dataListTubes":        dataListTubes,
		"dataStatsTubeDefault": dataStatsTubeDefault,
	} {
		require.NotNil(t, data, name)
	}
}

func TestBeanstalk_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Beanstalk{}, dataConfigJSON, dataConfigYAML)
}

func TestBeanstalk_Init(t *testing.T) {
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
			b := New()
			b.Config = test.config

			if test.wantFail {
				assert.Error(t, b.Init())
			} else {
				assert.NoError(t, b.Init())
			}
		})
	}
}

func TestBeanstalk_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Beanstalk
	}{
		"not initialized": {
			prepare: func() *Beanstalk {
				return New()
			},
		},
		"after check": {
			prepare: func() *Beanstalk {
				b := New()
				b.newBeanstalkConn = func(config Config) beanstalkConn { return prepareMockOk() }
				_ = b.Check()
				return b
			},
		},
		"after collect": {
			prepare: func() *Beanstalk {
				b := New()
				b.newBeanstalkConn = func(config Config) beanstalkConn { return prepareMockOk() }
				_ = b.Collect()
				return b
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := test.prepare()

			assert.NotPanics(t, b.Cleanup)
		})
	}
}

func TestBeanstalk_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestBeanstalk_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockBeanstalkConn
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"err on connect": {
			wantFail:    true,
			prepareMock: prepareMockErrOnConnect,
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
			b := New()
			mock := test.prepareMock()
			b.newBeanstalkConn = func(config Config) beanstalkConn { return mock }

			if test.wantFail {
				assert.Error(t, b.Check())
			} else {
				assert.NoError(t, b.Check())
			}
		})
	}
}

func TestBeanstalk_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock             func() *mockBeanstalkConn
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"binlog-records-migrated":       0,
				"binlog-records-written":        0,
				"cmd-bury":                      0,
				"cmd-delete":                    0,
				"cmd-ignore":                    0,
				"cmd-kick":                      0,
				"cmd-list-tube-used":            0,
				"cmd-list-tubes":                0,
				"cmd-list-tubes-watched":        0,
				"cmd-pause-tube":                0,
				"cmd-peek":                      0,
				"cmd-peek-buried":               0,
				"cmd-peek-delayed":              0,
				"cmd-peek-ready":                0,
				"cmd-put":                       0,
				"cmd-release":                   0,
				"cmd-reserve":                   0,
				"cmd-stats":                     2,
				"cmd-stats-job":                 0,
				"cmd-stats-tube":                0,
				"cmd-use":                       0,
				"cmd-watch":                     0,
				"current-connections":           1,
				"current-jobs-buried":           0,
				"current-jobs-delayed":          0,
				"current-jobs-ready":            0,
				"current-jobs-reserved":         0,
				"current-jobs-urgent":           0,
				"current-producers":             0,
				"current-tubes":                 1,
				"current-waiting":               0,
				"current-workers":               0,
				"default_cmd-delete":            0,
				"default_cmd-pause-tube":        0,
				"default_current-jobs-buried":   0,
				"default_current-jobs-delayed":  0,
				"default_current-jobs-ready":    0,
				"default_current-jobs-reserved": 0,
				"default_current-jobs-urgent":   0,
				"default_current-using":         2,
				"default_current-waiting":       0,
				"default_current-watching":      2,
				"default_pause":                 0,
				"default_pause-time-left":       0,
				"default_total-jobs":            0,
				"job-timeouts":                  0,
				"rusage-stime":                  9,
				"rusage-utime":                  4,
				"total-connections":             2,
				"total-jobs":                    0,
				"uptime":                        6393,
			},
		},
		"error response": {
			prepareMock:             prepareMockErrorResponse,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"unexpected response": {
			prepareMock:             prepareMockUnexpectedResponse,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"empty response": {
			prepareMock:             prepareMockEmptyResponse,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on query stats": {
			prepareMock:             prepareMockErrOnQueryStats,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"err on list tubes": {
			prepareMock:             prepareMockErrOnListTubes,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
		"err on stats tube": {
			prepareMock:             prepareMockErrOnStatsTube,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := New()
			mock := test.prepareMock()
			b.newBeanstalkConn = func(config Config) beanstalkConn { return mock }

			mx := b.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, b.Charts(), mx)
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			b.Cleanup()
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOk() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		statsResponse:     dataStats,
		listTubesResponse: dataListTubes,
		statsTubeResponse: dataStatsTubeDefault,
	}
}

func prepareMockErrorResponse() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		statsResponse: []byte("ERROR"),
		// listTubeResponse: []byte("ERROR"),
		// statsTubeResponse: []byte("ERROR"),
	}
}

func prepareMockErrOnConnect() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		errOnConnect: true,
	}
}

func prepareMockUnexpectedResponse() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		statsResponse: []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit."),
	}
}

func prepareMockEmptyResponse() *mockBeanstalkConn {
	return &mockBeanstalkConn{}
}

func prepareMockErrOnQueryStats() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		errOnQueryStats: true,
	}
}

func prepareMockErrOnListTubes() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		statsResponse:  dataStats,
		errOnListTubes: true,
	}
}

func prepareMockErrOnStatsTube() *mockBeanstalkConn {
	return &mockBeanstalkConn{
		statsResponse:     dataStats,
		listTubesResponse: dataListTubes,
		errOnStatsTube:    true,
	}
}

type mockBeanstalkConn struct {
	errOnConnect      bool
	errOnQueryStats   bool
	errOnListTubes    bool
	errOnStatsTube    bool
	statsResponse     []byte
	listTubesResponse []byte
	statsTubeResponse []byte
	disconnectCalled  bool
}

func (m *mockBeanstalkConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockBeanstalkConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockBeanstalkConn) queryStats() ([]byte, error) {
	if m.errOnQueryStats {
		return nil, errors.New("mock.queryStats() error")
	}
	return m.statsResponse, nil
}

func (m *mockBeanstalkConn) listTubes() ([]byte, error) {
	if m.errOnListTubes {
		return nil, errors.New("mock.listTubes() error")
	}
	return m.listTubesResponse, nil
}

func (m *mockBeanstalkConn) statsTube(name string) ([]byte, error) {
	if m.errOnStatsTube {
		return nil, errors.New("mock.statsTube() error")
	}
	return m.statsTubeResponse, nil
}
