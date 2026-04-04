// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	collr := New()

	assert.IsType(t, (*Collector)(nil), collr)
	assert.NotNil(t, collr.charts)
	assert.NotNil(t, collr.seen)
}

func TestCollector_Init(t *testing.T) {
	t.Setenv("SVDIR", "testdata")

	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success on default config": {
			config: New().Config,
		},
		"success when 'dir' is existing directory": {
			config: Config{Dir: "testdata"},
		},
		"fails when 'dir' is empty": {
			wantFail: true,
			config:   Config{Dir: ""},
		},
		"fails when 'dir' is not a directory": {
			wantFail: true,
			config:   Config{Dir: "collector_test.go"},
		},
		"fails when 'dir' is not exist": {
			wantFail: true,
			config:   Config{Dir: "nosuch"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(t.Context()))
			} else {
				assert.NoError(t, collr.Init(t.Context()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	collr := New()

	assert.NotNil(t, collr.Charts())
	assert.Len(t, *collr.Charts(), 1) // summary chart
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"success on valid response": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{data: dataSvStatusMultipleServices})
			},
		},
		"fails on error": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{err: errors.New("mock error")})
			},
		},
		"success on empty response": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{data: []byte{}})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			if test.wantFail {
				assert.Error(t, collr.Check(t.Context()))
			} else {
				assert.NoError(t, collr.Check(t.Context()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
		wantCollected map[string]int64
		wantCharts    int
	}{
		"multiple services in various states": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{data: dataSvStatusMultipleServices})
			},
			wantCollected: map[string]int64{
				// Summary: sshd=running, rest are not.
				"running_services":     1,
				"not_running_services": 5,
				// sshd: run, 12345s — running, enabled (no "normally down").
				"service_sshd_state_running":          1,
				"service_sshd_state_down":             0,
				"service_sshd_state_failed":           0,
				"service_sshd_state_starting":         0,
				"service_sshd_state_stopping":         0,
				"service_sshd_state_paused":           0,
				"service_sshd_state_enabled":          1,
				"service_sshd_state_duration_running": 12345,
				"service_sshd_state_duration_down":    0,
				// cron: run, (pid 123) 999s, paused — paused, enabled.
				"service_cron_state_running":          0,
				"service_cron_state_down":             0,
				"service_cron_state_failed":           0,
				"service_cron_state_starting":         0,
				"service_cron_state_stopping":         0,
				"service_cron_state_paused":           1,
				"service_cron_state_enabled":          1,
				"service_cron_state_duration_running": 999,
				"service_cron_state_duration_down":    0,
				// getty-5: down, 60s, normally up — failed, enabled.
				"service_getty-5_state_running":          0,
				"service_getty-5_state_down":             0,
				"service_getty-5_state_failed":           1,
				"service_getty-5_state_starting":         0,
				"service_getty-5_state_stopping":         0,
				"service_getty-5_state_paused":           0,
				"service_getty-5_state_enabled":          1,
				"service_getty-5_state_duration_running": 0,
				"service_getty-5_state_duration_down":    60,
				// socklog: down, 5s, normally up, want up — starting, enabled.
				"service_socklog_state_running":          0,
				"service_socklog_state_down":             0,
				"service_socklog_state_failed":           0,
				"service_socklog_state_starting":         1,
				"service_socklog_state_stopping":         0,
				"service_socklog_state_paused":           0,
				"service_socklog_state_enabled":          1,
				"service_socklog_state_duration_running": 0,
				"service_socklog_state_duration_down":    5,
				// ntpd: down, 100s — down, not enabled.
				"service_ntpd_state_running":          0,
				"service_ntpd_state_down":             1,
				"service_ntpd_state_failed":           0,
				"service_ntpd_state_starting":         0,
				"service_ntpd_state_stopping":         0,
				"service_ntpd_state_paused":           0,
				"service_ntpd_state_enabled":          0,
				"service_ntpd_state_duration_running": 0,
				"service_ntpd_state_duration_down":    100,
				// dhclient: finish, 3s — stopping, enabled (no "normally down").
				"service_dhclient_state_running":          0,
				"service_dhclient_state_down":             0,
				"service_dhclient_state_failed":           0,
				"service_dhclient_state_starting":         0,
				"service_dhclient_state_stopping":         1,
				"service_dhclient_state_paused":           0,
				"service_dhclient_state_enabled":          1,
				"service_dhclient_state_duration_running": 0,
				"service_dhclient_state_duration_down":    0,
			},
			wantCharts: 1 + 6*2, // summary + 6 services * 2 charts each
		},
		"service with log subprocess": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{data: dataSvStatusWithLog})
			},
			wantCollected: map[string]int64{
				"running_services":     2,
				"not_running_services": 0,
				// socklog: run, 500s; log subprocess also running. Both enabled.
				"service_socklog_state_running":              1,
				"service_socklog_state_down":                 0,
				"service_socklog_state_failed":               0,
				"service_socklog_state_starting":             0,
				"service_socklog_state_stopping":             0,
				"service_socklog_state_paused":               0,
				"service_socklog_state_enabled":              1,
				"service_socklog_state_duration_running":     500,
				"service_socklog_state_duration_down":        0,
				"service_socklog/log_state_running":          1,
				"service_socklog/log_state_down":             0,
				"service_socklog/log_state_failed":           0,
				"service_socklog/log_state_starting":         0,
				"service_socklog/log_state_stopping":         0,
				"service_socklog/log_state_paused":           0,
				"service_socklog/log_state_enabled":          1,
				"service_socklog/log_state_duration_running": 500,
				"service_socklog/log_state_duration_down":    0,
			},
			wantCharts: 1 + 2*2, // summary + 2 services (main + log) * 2 charts
		},
		"service disappears on second collect": {
			prepare: func() *Collector {
				mock := &mockSvCli{data: dataSvStatusMultipleServices}
				collr := prepareMockCollector(mock)
				// First collect — all services appear.
				collr.Collect(t.Context())
				// Second collect — only sshd remains.
				mock.data = dataSvStatusSingleService
				return collr
			},
			wantCollected: map[string]int64{
				"running_services":                    1,
				"not_running_services":                0,
				"service_sshd_state_running":          1,
				"service_sshd_state_down":             0,
				"service_sshd_state_failed":           0,
				"service_sshd_state_starting":         0,
				"service_sshd_state_stopping":         0,
				"service_sshd_state_paused":           0,
				"service_sshd_state_enabled":          1,
				"service_sshd_state_duration_running": 12345,
				"service_sshd_state_duration_down":    0,
			},
			wantCharts: 1 + 1*2, // summary + 1 service * 2 charts (others removed)
		},
		"empty response": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{data: []byte{}})
			},
			wantCollected: map[string]int64{
				"running_services":     0,
				"not_running_services": 0,
			},
			wantCharts: 1, // only summary
		},
		"error response": {
			prepare: func() *Collector {
				return prepareMockCollector(&mockSvCli{err: errors.New("mock error")})
			},
			wantCollected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			mx := collr.Collect(t.Context())

			assert.Equal(t, test.wantCollected, mx)
			if test.wantCharts > 0 {
				assert.Equal(t, test.wantCharts, calcActiveCharts(collr.Charts()))
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(t.Context()) })
}

func TestClassifyState(t *testing.T) {
	tests := map[string]struct {
		status *ServiceStatus
		want   string
	}{
		"run state": {
			status: &ServiceStatus{State: ServiceStateRun},
			want:   stateRunning,
		},
		"run but paused": {
			status: &ServiceStatus{State: ServiceStateRun, Paused: true},
			want:   statePaused,
		},
		"finish state": {
			status: &ServiceStatus{State: ServiceStateFinish},
			want:   stateStopping,
		},
		"finish but paused": {
			status: &ServiceStatus{State: ServiceStateFinish, Paused: true},
			want:   statePaused,
		},
		"down, not enabled": {
			status: &ServiceStatus{State: ServiceStateDown, Enabled: false},
			want:   stateDown,
		},
		"down, enabled (failed)": {
			status: &ServiceStatus{State: ServiceStateDown, Enabled: true},
			want:   stateFailed,
		},
		"down, want up (starting)": {
			status: &ServiceStatus{State: ServiceStateDown, WantUp: true},
			want:   stateStarting,
		},
		"down, enabled, want up (starting wins)": {
			status: &ServiceStatus{State: ServiceStateDown, Enabled: true, WantUp: true},
			want:   stateStarting,
		},
		"down, paused": {
			status: &ServiceStatus{State: ServiceStateDown, Paused: true},
			want:   statePaused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, classifyState(test.status))
		})
	}
}

func TestServicesCli(t *testing.T) {
	tests := map[string]struct {
		data     []byte
		wantKeys []string
		wantNil  []string
	}{
		"fail: lines are skipped": {
			data: []byte("" +
				"run: /service/sshd: 100s\n" +
				"fail: /service/broken: unable to change to service directory: file does not exist\n",
			),
			wantKeys: []string{"sshd"},
			wantNil:  []string{"broken"},
		},
		"warning: lines are skipped": {
			data: []byte("" +
				"run: /service/sshd: 100s\n" +
				"warning: /service/nosuper: unable to open supervise/ok: file does not exist\n",
			),
			wantKeys: []string{"sshd"},
			wantNil:  []string{"nosuper"},
		},
		"got TERM flag is parsed": {
			data:     []byte("run: /service/nginx: (pid 42) 10s, want down, got TERM\n"),
			wantKeys: []string{"nginx"},
		},
		"normally down running service": {
			data:     []byte("run: /service/oneshot: (pid 99) 5s, normally down\n"),
			wantKeys: []string{"oneshot"},
		},
		"log subprocess in down state": {
			data: []byte("" +
				"run: /service/socklog: (pid 456) 500s; down: log: 3s, normally up, want up\n",
			),
			wantKeys: []string{"socklog", "socklog/log"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := prepareMockCollector(&mockSvCli{data: test.data})
			services, err := collr.servicesCli()
			require.NoError(t, err)
			for _, k := range test.wantKeys {
				assert.Contains(t, services, k)
			}
			for _, k := range test.wantNil {
				assert.NotContains(t, services, k)
			}
		})
	}
}

func TestParseServiceStatus(t *testing.T) {
	tests := map[string]struct {
		data []byte
		name string
		want ServiceStatus
	}{
		"running, normally down (not enabled)": {
			data: []byte("run: /service/oneshot: (pid 99) 5s, normally down\n"),
			name: "oneshot",
			want: ServiceStatus{
				State:         ServiceStateRun,
				StateDuration: 5 * 1e9, // 5s as time.Duration
				Enabled:       false,
			},
		},
		"running with got TERM": {
			data: []byte("run: /service/nginx: (pid 42) 10s, want down, got TERM\n"),
			name: "nginx",
			want: ServiceStatus{
				State:         ServiceStateRun,
				StateDuration: 10 * 1e9,
				Enabled:       true,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := prepareMockCollector(&mockSvCli{data: test.data})
			services, err := collr.servicesCli()
			require.NoError(t, err)
			require.Contains(t, services, test.name)
			got := services[test.name]
			assert.Equal(t, test.want.State, got.State)
			assert.Equal(t, test.want.StateDuration, got.StateDuration)
			assert.Equal(t, test.want.Enabled, got.Enabled)
			assert.Equal(t, test.want.Paused, got.Paused)
			assert.Equal(t, test.want.WantUp, got.WantUp)
		})
	}
}

func TestCollector_Collect_FinishStateDuration(t *testing.T) {
	collr := prepareMockCollector(&mockSvCli{
		data: []byte("finish: /service/dhclient: 3s\n"),
	})

	mx := collr.Collect(t.Context())

	// finish is a transitional state — neither down nor running duration is set.
	assert.Equal(t, int64(0), mx["service_dhclient_state_duration_down"])
	assert.Equal(t, int64(0), mx["service_dhclient_state_duration_running"])
	assert.Equal(t, int64(1), mx["service_dhclient_state_stopping"])
}

func TestCollector_Collect_NormallyDownRunning(t *testing.T) {
	collr := prepareMockCollector(&mockSvCli{
		data: []byte("run: /service/oneshot: (pid 99) 5s, normally down\n"),
	})

	mx := collr.Collect(t.Context())

	// Running but normally down: enabled=false (should not be running).
	assert.Equal(t, int64(1), mx["service_oneshot_state_running"])
	assert.Equal(t, int64(0), mx["service_oneshot_state_enabled"])
}

// --- helpers ---

func prepareMockCollector(mock svCli) *Collector {
	collr := New()
	collr.Dir = "/service"
	collr.exec = mock
	return collr
}

func calcActiveCharts(charts *module.Charts) int {
	n := 0
	for _, c := range *charts {
		if !c.Obsolete {
			n++
		}
	}
	return n
}

type mockSvCli struct {
	data []byte
	err  error
}

func (m *mockSvCli) StatusAll(string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

// Test data: sv status output lines.
// Each line ends with \n and represents one service.
var (
	// 6 services in different states.
	dataSvStatusMultipleServices = []byte("" +
		"run: /service/sshd: 12345s\n" +
		"run: /service/cron: (pid 123) 999s, paused\n" +
		"down: /service/getty-5: 60s, normally up\n" +
		"down: /service/socklog: 5s, normally up, want up\n" +
		"down: /service/ntpd: 100s\n" +
		"finish: /service/dhclient: 3s\n",
	)

	// Single running service.
	dataSvStatusSingleService = []byte("" +
		"run: /service/sshd: 12345s\n",
	)

	// Service with log subprocess.
	dataSvStatusWithLog = []byte("" +
		"run: /service/socklog: (pid 456) 500s; run: log: (pid 789) 500s\n",
	)
)
