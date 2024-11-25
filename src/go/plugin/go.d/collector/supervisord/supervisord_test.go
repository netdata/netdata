// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

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

func TestSupervisord_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Supervisord{}, dataConfigJSON, dataConfigYAML)
}

func TestSupervisord_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'url'": {
			wantFail: true,
			config:   Config{URL: ""},
		},
		"fails on unexpected 'url' scheme": {
			wantFail: true,
			config:   Config{URL: "tcp://127.0.0.1:9001/RPC2"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supvr := New()
			supvr.Config = test.config

			if test.wantFail {
				assert.Error(t, supvr.Init())
			} else {
				assert.NoError(t, supvr.Init())
			}
		})
	}
}

func TestSupervisord_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(t *testing.T) *Supervisord
		wantFail bool
	}{
		"success on valid response": {
			prepare: prepareSupervisordSuccessOnGetAllProcessInfo,
		},
		"success on zero processes response": {
			prepare: prepareSupervisordZeroProcessesOnGetAllProcessInfo,
		},
		"fails on error": {
			wantFail: true,
			prepare:  prepareSupervisordErrorOnGetAllProcessInfo,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supvr := test.prepare(t)
			defer supvr.Cleanup()

			if test.wantFail {
				assert.Error(t, supvr.Check())
			} else {
				assert.NoError(t, supvr.Check())
			}
		})
	}
}

func TestSupervisord_Charts(t *testing.T) {
	supvr := New()
	require.NoError(t, supvr.Init())

	assert.NotNil(t, supvr.Charts())
}

func TestSupervisord_Cleanup(t *testing.T) {
	supvr := New()
	assert.NotPanics(t, supvr.Cleanup)

	require.NoError(t, supvr.Init())
	m := &mockSupervisorClient{}
	supvr.client = m

	supvr.Cleanup()

	assert.True(t, m.calledCloseIdleConnections)
}

func TestSupervisord_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) *Supervisord
		wantCollected map[string]int64
	}{
		"success on valid response": {
			prepare: prepareSupervisordSuccessOnGetAllProcessInfo,
			wantCollected: map[string]int64{
				"group_proc1_non_running_processes":  1,
				"group_proc1_process_00_downtime":    16276,
				"group_proc1_process_00_exit_status": 0,
				"group_proc1_process_00_state_code":  200,
				"group_proc1_process_00_uptime":      0,
				"group_proc1_running_processes":      0,
				"group_proc2_non_running_processes":  0,
				"group_proc2_process_00_downtime":    0,
				"group_proc2_process_00_exit_status": 0,
				"group_proc2_process_00_state_code":  20,
				"group_proc2_process_00_uptime":      2,
				"group_proc2_process_01_downtime":    0,
				"group_proc2_process_01_exit_status": 0,
				"group_proc2_process_01_state_code":  20,
				"group_proc2_process_01_uptime":      2,
				"group_proc2_process_02_downtime":    0,
				"group_proc2_process_02_exit_status": 0,
				"group_proc2_process_02_state_code":  20,
				"group_proc2_process_02_uptime":      8,
				"group_proc2_running_processes":      3,
				"group_proc3_non_running_processes":  0,
				"group_proc3_process_00_downtime":    0,
				"group_proc3_process_00_exit_status": 0,
				"group_proc3_process_00_state_code":  20,
				"group_proc3_process_00_uptime":      16291,
				"group_proc3_running_processes":      1,
				"non_running_processes":              1,
				"running_processes":                  4,
			},
		},
		"success on response with zero processes": {
			prepare: prepareSupervisordZeroProcessesOnGetAllProcessInfo,
			wantCollected: map[string]int64{
				"non_running_processes": 0,
				"running_processes":     0,
			},
		},
		"fails on error on getAllProcessesInfo": {
			prepare: prepareSupervisordErrorOnGetAllProcessInfo,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			supvr := test.prepare(t)
			defer supvr.Cleanup()

			mx := supvr.Collect()
			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, supvr.Charts(), mx)
			}
		})
	}
}

func prepareSupervisordSuccessOnGetAllProcessInfo(t *testing.T) *Supervisord {
	supvr := New()
	require.NoError(t, supvr.Init())
	supvr.client = &mockSupervisorClient{}
	return supvr
}

func prepareSupervisordZeroProcessesOnGetAllProcessInfo(t *testing.T) *Supervisord {
	supvr := New()
	require.NoError(t, supvr.Init())
	supvr.client = &mockSupervisorClient{returnZeroProcesses: true}
	return supvr
}

func prepareSupervisordErrorOnGetAllProcessInfo(t *testing.T) *Supervisord {
	supvr := New()
	require.NoError(t, supvr.Init())
	supvr.client = &mockSupervisorClient{errOnGetAllProcessInfo: true}
	return supvr
}

type mockSupervisorClient struct {
	errOnGetAllProcessInfo     bool
	returnZeroProcesses        bool
	calledCloseIdleConnections bool
}

func (m *mockSupervisorClient) getAllProcessInfo() ([]processStatus, error) {
	if m.errOnGetAllProcessInfo {
		return nil, errors.New("mock errOnGetAllProcessInfo")
	}
	if m.returnZeroProcesses {
		return nil, nil
	}
	info := []processStatus{
		{
			name: "00", group: "proc1",
			start: 1613374760, stop: 1613374762, now: 1613391038,
			state: 200, stateName: "FATAL",
			exitStatus: 0,
		},
		{
			name: "00", group: "proc2",
			start: 1613391036, stop: 1613391036, now: 1613391038,
			state: 20, stateName: "RUNNING",
			exitStatus: 0,
		},
		{
			name: "01", group: "proc2",
			start: 1613391036, stop: 1613391036, now: 1613391038,
			state: 20, stateName: "RUNNING",
			exitStatus: 0,
		},
		{
			name: "02", group: "proc2",
			start: 1613391030, stop: 1613391029, now: 1613391038,
			state: 20, stateName: "RUNNING",
			exitStatus: 0,
		},
		{
			name: "00", group: "proc3",
			start: 1613374747, stop: 0, now: 1613391038,
			state: 20, stateName: "RUNNING",
			exitStatus: 0,
		},
	}
	return info, nil
}

func (m *mockSupervisorClient) closeIdleConnections() {
	m.calledCloseIdleConnections = true
}
