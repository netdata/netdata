// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"encoding/xml"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataGetResults, _        = os.ReadFile("testdata/get_results.xml")
	dataGetResultsNoTasks, _ = os.ReadFile("testdata/get_results_no_tasks.xml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":        dataConfigJSON,
		"dataConfigYAML":        dataConfigYAML,
		"dataGetResults":        dataGetResults,
		"dataGetResultsNoTasks": dataGetResultsNoTasks,
	} {
		require.NotNil(t, data, name)
	}
}

func TestBoinc_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Boinc{}, dataConfigJSON, dataConfigYAML)
}

func TestBoinc_Init(t *testing.T) {
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
			boinc := New()
			boinc.Config = test.config

			if test.wantFail {
				assert.Error(t, boinc.Init())
			} else {
				assert.NoError(t, boinc.Init())
			}
		})
	}
}

func TestBoinc_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Boinc
	}{
		"not initialized": {
			prepare: func() *Boinc {
				return New()
			},
		},
		"after check": {
			prepare: func() *Boinc {
				boinc := New()
				boinc.newConn = func(Config, *logger.Logger) boincConn { return prepareMockOk() }
				_ = boinc.Check()
				return boinc
			},
		},
		"after collect": {
			prepare: func() *Boinc {
				boinc := New()
				boinc.newConn = func(Config, *logger.Logger) boincConn { return prepareMockOk() }
				_ = boinc.Collect()
				return boinc
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			boinc := test.prepare()

			assert.NotPanics(t, boinc.Cleanup)
		})
	}
}

func TestBoinc_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestBoinc_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockBoincConn
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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			boinc := New()
			mock := test.prepareMock()
			boinc.newConn = func(Config, *logger.Logger) boincConn { return mock }

			if test.wantFail {
				assert.Error(t, boinc.Check())
			} else {
				assert.NoError(t, boinc.Check())
			}
		})
	}
}

func TestBoinc_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock             func() *mockBoincConn
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case with tasks": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"abort_pending":     0,
				"aborted":           0,
				"active":            14,
				"compute_error":     0,
				"copy_pending":      0,
				"executing":         11,
				"files_downloaded":  110,
				"files_downloading": 0,
				"files_uploaded":    8,
				"files_uploading":   0,
				"new":               0,
				"preempted":         3,
				"quit_pending":      0,
				"scheduled":         11,
				"suspended":         3,
				"total":             118,
				"uninitialized":     0,
				"upload_failed":     0,
			},
		},
		"success case no tasks": {
			prepareMock:             prepareMockOkNoTasks,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"abort_pending":     0,
				"aborted":           0,
				"active":            0,
				"compute_error":     0,
				"copy_pending":      0,
				"executing":         0,
				"files_downloaded":  0,
				"files_downloading": 0,
				"files_uploaded":    0,
				"files_uploading":   0,
				"new":               0,
				"preempted":         0,
				"quit_pending":      0,
				"scheduled":         0,
				"suspended":         0,
				"total":             0,
				"uninitialized":     0,
				"upload_failed":     0,
			},
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on get results": {
			prepareMock:             prepareMockErrOnGetResults,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			boinc := New()
			mock := test.prepareMock()
			boinc.newConn = func(Config, *logger.Logger) boincConn { return mock }

			mx := boinc.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, boinc.Charts(), mx)
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			boinc.Cleanup()
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOk() *mockBoincConn {
	return &mockBoincConn{
		getResultsResp: dataGetResults,
	}
}

func prepareMockOkNoTasks() *mockBoincConn {
	return &mockBoincConn{
		getResultsResp: dataGetResultsNoTasks,
	}
}

func prepareMockErrOnConnect() *mockBoincConn {
	return &mockBoincConn{
		errOnConnect: true,
	}
}

func prepareMockErrOnGetResults() *mockBoincConn {
	return &mockBoincConn{
		errOnGetResults: true,
	}
}

type mockBoincConn struct {
	errOnConnect bool

	errOnGetResults bool
	getResultsResp  []byte

	authCalled       bool
	disconnectCalled bool
}

func (m *mockBoincConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockBoincConn) disconnect() {
	m.disconnectCalled = true
}

func (m *mockBoincConn) authenticate() error {
	m.authCalled = true
	return nil
}

func (m *mockBoincConn) getResults() ([]boincReplyResult, error) {
	if m.errOnGetResults {
		return nil, errors.New("mock.getResults() error")
	}

	var resp boincReply
	if err := xml.Unmarshal(m.getResultsResp, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}
