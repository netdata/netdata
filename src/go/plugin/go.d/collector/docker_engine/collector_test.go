// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataNonDockerEngineMetrics, _ = os.ReadFile("testdata/non-docker-engine.txt")
	dataVer17050Metrics, _        = os.ReadFile("testdata/v17.05.0-ce.txt")
	dataVer18093Metrics, _        = os.ReadFile("testdata/v18.09.3-ce.txt")
	dataVer18093SwarmMetrics, _   = os.ReadFile("testdata/v18.09.3-ce-swarm.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":             dataConfigJSON,
		"dataConfigYAML":             dataConfigYAML,
		"dataNonDockerEngineMetrics": dataNonDockerEngineMetrics,
		"dataVer17050Metrics":        dataVer17050Metrics,
		"dataVer18093Metrics":        dataVer18093Metrics,
		"dataVer18093SwarmMetrics":   dataVer18093SwarmMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default": {
			config: New().Config,
		},
		"empty URL": {
			config:   Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: ""}}},
			wantFail: true,
		},
		"nonexistent TLS CA": {
			config: Config{HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9323/metrics"},
				ClientConfig:  web.ClientConfig{TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"}}}},
			wantFail: true,
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (*Collector, *httptest.Server)
		wantFail bool
	}{
		"v17.05.0-ce":        {prepare: prepareClientServerV17050CE},
		"v18.09.3-ce":        {prepare: prepareClientServerV18093CE},
		"v18.09.3-ce-swarm":  {prepare: prepareClientServerV18093CESwarm},
		"non docker engine":  {prepare: prepareClientServerNonDockerEngine, wantFail: true},
		"invalid data":       {prepare: prepareClientServerInvalidData, wantFail: true},
		"404":                {prepare: prepareClientServer404, wantFail: true},
		"connection refused": {prepare: prepareClientServerConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, srv := test.prepare(t)
			defer srv.Close()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	tests := map[string]struct {
		prepare       func(*testing.T) (*Collector, *httptest.Server)
		wantNumCharts int
	}{
		"v17.05.0-ce":       {prepare: prepareClientServerV17050CE, wantNumCharts: len(charts) - 1}, // no container states chart
		"v18.09.3-ce":       {prepare: prepareClientServerV18093CE, wantNumCharts: len(charts)},
		"v18.09.3-ce-swarm": {prepare: prepareClientServerV18093CESwarm, wantNumCharts: len(charts) + len(swarmManagerCharts)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, srv := test.prepare(t)
			defer srv.Close()

			require.NoError(t, collr.Check(context.Background()))
			assert.Len(t, *collr.Charts(), test.wantNumCharts)
		})
	}
}

func TestCollector_Collect_ReturnsNilOnErrors(t *testing.T) {
	tests := map[string]struct {
		prepare func(*testing.T) (*Collector, *httptest.Server)
	}{
		"non docker engine":  {prepare: prepareClientServerNonDockerEngine},
		"invalid data":       {prepare: prepareClientServerInvalidData},
		"404":                {prepare: prepareClientServer404},
		"connection refused": {prepare: prepareClientServerConnectionRefused},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, srv := test.prepare(t)
			defer srv.Close()

			assert.Nil(t, collr.Collect(context.Background()))
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (*Collector, *httptest.Server)
		expected map[string]int64
	}{
		"v17.05.0-ce": {
			prepare: prepareClientServerV17050CE,
			expected: map[string]int64{
				"builder_fails_build_canceled":                   1,
				"builder_fails_build_target_not_reachable_error": 2,
				"builder_fails_command_not_supported_error":      3,
				"builder_fails_dockerfile_empty_error":           4,
				"builder_fails_dockerfile_syntax_error":          5,
				"builder_fails_error_processing_commands_error":  6,
				"builder_fails_missing_onbuild_arguments_error":  7,
				"builder_fails_unknown_instruction_error":        8,
				"container_actions_changes":                      1,
				"container_actions_commit":                       1,
				"container_actions_create":                       1,
				"container_actions_delete":                       1,
				"container_actions_start":                        1,
				"health_checks_failed":                           33,
			},
		},
		"v18.09.3-ce": {
			prepare: prepareClientServerV18093CE,
			expected: map[string]int64{
				"builder_fails_build_canceled":                   1,
				"builder_fails_build_target_not_reachable_error": 2,
				"builder_fails_command_not_supported_error":      3,
				"builder_fails_dockerfile_empty_error":           4,
				"builder_fails_dockerfile_syntax_error":          5,
				"builder_fails_error_processing_commands_error":  6,
				"builder_fails_missing_onbuild_arguments_error":  7,
				"builder_fails_unknown_instruction_error":        8,
				"container_actions_changes":                      1,
				"container_actions_commit":                       1,
				"container_actions_create":                       1,
				"container_actions_delete":                       1,
				"container_actions_start":                        1,
				"container_states_paused":                        11,
				"container_states_running":                       12,
				"container_states_stopped":                       13,
				"health_checks_failed":                           33,
			},
		},
		"v18.09.3-ce-swarm": {
			prepare: prepareClientServerV18093CESwarm,
			expected: map[string]int64{
				"builder_fails_build_canceled":                   1,
				"builder_fails_build_target_not_reachable_error": 2,
				"builder_fails_command_not_supported_error":      3,
				"builder_fails_dockerfile_empty_error":           4,
				"builder_fails_dockerfile_syntax_error":          5,
				"builder_fails_error_processing_commands_error":  6,
				"builder_fails_missing_onbuild_arguments_error":  7,
				"builder_fails_unknown_instruction_error":        8,
				"container_actions_changes":                      1,
				"container_actions_commit":                       1,
				"container_actions_create":                       1,
				"container_actions_delete":                       1,
				"container_actions_start":                        1,
				"container_states_paused":                        11,
				"container_states_running":                       12,
				"container_states_stopped":                       13,
				"health_checks_failed":                           33,
				"swarm_manager_configs_total":                    1,
				"swarm_manager_leader":                           1,
				"swarm_manager_networks_total":                   3,
				"swarm_manager_nodes_state_disconnected":         1,
				"swarm_manager_nodes_state_down":                 2,
				"swarm_manager_nodes_state_ready":                3,
				"swarm_manager_nodes_state_unknown":              4,
				"swarm_manager_nodes_total":                      10,
				"swarm_manager_secrets_total":                    1,
				"swarm_manager_services_total":                   1,
				"swarm_manager_tasks_state_accepted":             1,
				"swarm_manager_tasks_state_assigned":             2,
				"swarm_manager_tasks_state_complete":             3,
				"swarm_manager_tasks_state_failed":               4,
				"swarm_manager_tasks_state_new":                  5,
				"swarm_manager_tasks_state_orphaned":             6,
				"swarm_manager_tasks_state_pending":              7,
				"swarm_manager_tasks_state_preparing":            8,
				"swarm_manager_tasks_state_ready":                9,
				"swarm_manager_tasks_state_rejected":             10,
				"swarm_manager_tasks_state_remove":               11,
				"swarm_manager_tasks_state_running":              12,
				"swarm_manager_tasks_state_shutdown":             13,
				"swarm_manager_tasks_state_starting":             14,
				"swarm_manager_tasks_total":                      105,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pulsar, srv := test.prepare(t)
			defer srv.Close()

			for i := 0; i < 10; i++ {
				_ = pulsar.Collect(context.Background())
			}
			mx := pulsar.Collect(context.Background())

			require.NotNil(t, mx)
			require.Equal(t, test.expected, mx)
			module.TestMetricsHasAllChartsDims(t, pulsar.Charts(), mx)
		})
	}
}

func prepareClientServerV17050CE(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer17050Metrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServerV18093CE(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer18093Metrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServerV18093CESwarm(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer18093SwarmMetrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServerNonDockerEngine(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataNonDockerEngineMetrics)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServerInvalidData(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServer404(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}

func prepareClientServerConnectionRefused(t *testing.T) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(nil)

	collr := New()
	collr.URL = "http://127.0.0.1:38001/metrics"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv
}
