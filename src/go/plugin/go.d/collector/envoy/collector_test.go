// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataEnvoyConsulDataplane, _ = os.ReadFile("testdata/consul-dataplane.txt")
	dataEnvoy, _                = os.ReadFile("testdata/envoy.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataEnvoyConsulDataplane": dataEnvoyConsulDataplane,
		"dataEnvoy":                dataEnvoy,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
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
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	require.NoError(t, collr.Init(context.Background()))
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_Charts(t *testing.T) {
	collr, cleanup := prepareCaseEnvoyStats()
	defer cleanup()

	require.Empty(t, *collr.Charts())

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Collect(context.Background())
	require.NotEmpty(t, *collr.Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"case envoy consul dataplane": {
			wantFail: false,
			prepare:  prepareCaseEnvoyConsulDataplaneStats,
		},
		"case envoy": {
			wantFail: false,
			prepare:  prepareCaseEnvoyStats,
		},
		"case invalid data response": {
			wantFail: true,
			prepare:  prepareCaseInvalidDataResponse,
		},
		"case 404": {
			wantFail: true,
			prepare:  prepareCase404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

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
		prepare     func() (collr *Collector, cleanup func())
		wantMetrics map[string]int64
	}{
		"case envoy consul dataplane": {
			prepare: prepareCaseEnvoyConsulDataplaneStats,
			wantMetrics: map[string]int64{
				"envoy_cluster_manager_active_clusters_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                           4,
				"envoy_cluster_manager_cluster_added_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                             4,
				"envoy_cluster_manager_cluster_modified_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                          0,
				"envoy_cluster_manager_cluster_removed_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                           0,
				"envoy_cluster_manager_cluster_updated_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                           2,
				"envoy_cluster_manager_cluster_updated_via_merge_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                 0,
				"envoy_cluster_manager_update_merge_cancelled_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                    0,
				"envoy_cluster_manager_update_out_of_merge_window_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                0,
				"envoy_cluster_manager_warming_clusters_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                          0,
				"envoy_cluster_membership_change_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                1,
				"envoy_cluster_membership_change_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                       1,
				"envoy_cluster_membership_change_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                            2,
				"envoy_cluster_membership_change_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                              1,
				"envoy_cluster_membership_degraded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                              0,
				"envoy_cluster_membership_degraded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                     0,
				"envoy_cluster_membership_degraded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                          0,
				"envoy_cluster_membership_degraded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                            0,
				"envoy_cluster_membership_excluded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                              0,
				"envoy_cluster_membership_excluded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                     0,
				"envoy_cluster_membership_excluded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                          0,
				"envoy_cluster_membership_excluded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                            0,
				"envoy_cluster_membership_healthy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                               1,
				"envoy_cluster_membership_healthy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                      1,
				"envoy_cluster_membership_healthy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                           0,
				"envoy_cluster_membership_healthy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                             1,
				"envoy_cluster_update_empty_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                     0,
				"envoy_cluster_update_empty_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                            0,
				"envoy_cluster_update_empty_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                                 0,
				"envoy_cluster_update_empty_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                                   0,
				"envoy_cluster_update_failure_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                   0,
				"envoy_cluster_update_failure_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                          0,
				"envoy_cluster_update_failure_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                               0,
				"envoy_cluster_update_failure_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                                 0,
				"envoy_cluster_update_no_rebuild_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                0,
				"envoy_cluster_update_no_rebuild_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                       0,
				"envoy_cluster_update_no_rebuild_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                            0,
				"envoy_cluster_update_no_rebuild_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                              0,
				"envoy_cluster_update_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                   0,
				"envoy_cluster_update_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                          0,
				"envoy_cluster_update_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                               0,
				"envoy_cluster_update_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                                 0,
				"envoy_cluster_upstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                               1,
				"envoy_cluster_upstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                      0,
				"envoy_cluster_upstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                           0,
				"envoy_cluster_upstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                             2,
				"envoy_cluster_upstream_cx_connect_attempts_exceeded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":            0,
				"envoy_cluster_upstream_cx_connect_attempts_exceeded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                   0,
				"envoy_cluster_upstream_cx_connect_attempts_exceeded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":        0,
				"envoy_cluster_upstream_cx_connect_attempts_exceeded_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":          0,
				"envoy_cluster_upstream_cx_connect_fail_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                         0,
				"envoy_cluster_upstream_cx_connect_fail_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                0,
				"envoy_cluster_upstream_cx_connect_fail_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                     0,
				"envoy_cluster_upstream_cx_connect_fail_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                       0,
				"envoy_cluster_upstream_cx_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                      0,
				"envoy_cluster_upstream_cx_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                             0,
				"envoy_cluster_upstream_cx_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                  0,
				"envoy_cluster_upstream_cx_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                    0,
				"envoy_cluster_upstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                              0,
				"envoy_cluster_upstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                     6507,
				"envoy_cluster_upstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                          1,
				"envoy_cluster_upstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                            0,
				"envoy_cluster_upstream_cx_destroy_local_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                        0,
				"envoy_cluster_upstream_cx_destroy_local_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                               6507,
				"envoy_cluster_upstream_cx_destroy_local_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                    0,
				"envoy_cluster_upstream_cx_destroy_local_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                      0,
				"envoy_cluster_upstream_cx_destroy_remote_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                       0,
				"envoy_cluster_upstream_cx_destroy_remote_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                              0,
				"envoy_cluster_upstream_cx_destroy_remote_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                   1,
				"envoy_cluster_upstream_cx_destroy_remote_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                     0,
				"envoy_cluster_upstream_cx_http1_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                          0,
				"envoy_cluster_upstream_cx_http1_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                 0,
				"envoy_cluster_upstream_cx_http1_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                      0,
				"envoy_cluster_upstream_cx_http1_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                        2,
				"envoy_cluster_upstream_cx_http2_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                          1,
				"envoy_cluster_upstream_cx_http2_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                 0,
				"envoy_cluster_upstream_cx_http2_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                      0,
				"envoy_cluster_upstream_cx_http2_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                        0,
				"envoy_cluster_upstream_cx_http3_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                          0,
				"envoy_cluster_upstream_cx_http3_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                 0,
				"envoy_cluster_upstream_cx_http3_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                      0,
				"envoy_cluster_upstream_cx_http3_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                        0,
				"envoy_cluster_upstream_cx_idle_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                         0,
				"envoy_cluster_upstream_cx_idle_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                0,
				"envoy_cluster_upstream_cx_idle_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                     0,
				"envoy_cluster_upstream_cx_idle_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                       0,
				"envoy_cluster_upstream_cx_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                 0,
				"envoy_cluster_upstream_cx_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                        0,
				"envoy_cluster_upstream_cx_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":             0,
				"envoy_cluster_upstream_cx_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":               0,
				"envoy_cluster_upstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                             0,
				"envoy_cluster_upstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                    0,
				"envoy_cluster_upstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                         0,
				"envoy_cluster_upstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                           0,
				"envoy_cluster_upstream_cx_rx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                    17,
				"envoy_cluster_upstream_cx_rx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                           0,
				"envoy_cluster_upstream_cx_rx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                0,
				"envoy_cluster_upstream_cx_rx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                  102618,
				"envoy_cluster_upstream_cx_rx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                       3853,
				"envoy_cluster_upstream_cx_rx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                              0,
				"envoy_cluster_upstream_cx_rx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                   8645645,
				"envoy_cluster_upstream_cx_rx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                     724779,
				"envoy_cluster_upstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                1,
				"envoy_cluster_upstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                       6507,
				"envoy_cluster_upstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                            1,
				"envoy_cluster_upstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                              2,
				"envoy_cluster_upstream_cx_tx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                    0,
				"envoy_cluster_upstream_cx_tx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                           0,
				"envoy_cluster_upstream_cx_tx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                0,
				"envoy_cluster_upstream_cx_tx_bytes_buffered_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                  0,
				"envoy_cluster_upstream_cx_tx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                       114982,
				"envoy_cluster_upstream_cx_tx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                              0,
				"envoy_cluster_upstream_cx_tx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                   1240,
				"envoy_cluster_upstream_cx_tx_bytes_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                     732,
				"envoy_cluster_upstream_rq_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                               1,
				"envoy_cluster_upstream_rq_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                      0,
				"envoy_cluster_upstream_rq_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                           0,
				"envoy_cluster_upstream_rq_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                             1,
				"envoy_cluster_upstream_rq_cancelled_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                            0,
				"envoy_cluster_upstream_rq_cancelled_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                   4749,
				"envoy_cluster_upstream_rq_cancelled_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                        0,
				"envoy_cluster_upstream_rq_cancelled_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                          0,
				"envoy_cluster_upstream_rq_maintenance_mode_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                     0,
				"envoy_cluster_upstream_rq_maintenance_mode_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                            0,
				"envoy_cluster_upstream_rq_maintenance_mode_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                 0,
				"envoy_cluster_upstream_rq_maintenance_mode_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                   0,
				"envoy_cluster_upstream_rq_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                 0,
				"envoy_cluster_upstream_rq_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                        0,
				"envoy_cluster_upstream_rq_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":             0,
				"envoy_cluster_upstream_rq_max_duration_reached_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":               0,
				"envoy_cluster_upstream_rq_pending_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                       0,
				"envoy_cluster_upstream_rq_pending_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                              0,
				"envoy_cluster_upstream_rq_pending_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                   0,
				"envoy_cluster_upstream_rq_pending_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                     0,
				"envoy_cluster_upstream_rq_pending_failure_eject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                0,
				"envoy_cluster_upstream_rq_pending_failure_eject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                       0,
				"envoy_cluster_upstream_rq_pending_failure_eject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":            0,
				"envoy_cluster_upstream_rq_pending_failure_eject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":              0,
				"envoy_cluster_upstream_rq_pending_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                     0,
				"envoy_cluster_upstream_rq_pending_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                            0,
				"envoy_cluster_upstream_rq_pending_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                 0,
				"envoy_cluster_upstream_rq_pending_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                   0,
				"envoy_cluster_upstream_rq_pending_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                        1,
				"envoy_cluster_upstream_rq_pending_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                               6507,
				"envoy_cluster_upstream_rq_pending_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                    1,
				"envoy_cluster_upstream_rq_pending_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                      2,
				"envoy_cluster_upstream_rq_per_try_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                      0,
				"envoy_cluster_upstream_rq_per_try_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                             0,
				"envoy_cluster_upstream_rq_per_try_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                  0,
				"envoy_cluster_upstream_rq_per_try_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                    0,
				"envoy_cluster_upstream_rq_retry_backoff_exponential_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":            0,
				"envoy_cluster_upstream_rq_retry_backoff_exponential_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                   0,
				"envoy_cluster_upstream_rq_retry_backoff_exponential_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":        0,
				"envoy_cluster_upstream_rq_retry_backoff_exponential_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":          0,
				"envoy_cluster_upstream_rq_retry_backoff_ratelimited_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":            0,
				"envoy_cluster_upstream_rq_retry_backoff_ratelimited_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                   0,
				"envoy_cluster_upstream_rq_retry_backoff_ratelimited_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":        0,
				"envoy_cluster_upstream_rq_retry_backoff_ratelimited_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":          0,
				"envoy_cluster_upstream_rq_retry_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                0,
				"envoy_cluster_upstream_rq_retry_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                       0,
				"envoy_cluster_upstream_rq_retry_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                            0,
				"envoy_cluster_upstream_rq_retry_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                              0,
				"envoy_cluster_upstream_rq_retry_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                        0,
				"envoy_cluster_upstream_rq_retry_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                               0,
				"envoy_cluster_upstream_rq_retry_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                    0,
				"envoy_cluster_upstream_rq_retry_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                      0,
				"envoy_cluster_upstream_rq_rx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                             0,
				"envoy_cluster_upstream_rq_rx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                    0,
				"envoy_cluster_upstream_rq_rx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                         0,
				"envoy_cluster_upstream_rq_rx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                           0,
				"envoy_cluster_upstream_rq_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                              0,
				"envoy_cluster_upstream_rq_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                     0,
				"envoy_cluster_upstream_rq_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                          0,
				"envoy_cluster_upstream_rq_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                            0,
				"envoy_cluster_upstream_rq_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                                1,
				"envoy_cluster_upstream_rq_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                       1758,
				"envoy_cluster_upstream_rq_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                            1,
				"envoy_cluster_upstream_rq_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                              3,
				"envoy_cluster_upstream_rq_tx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_consul-dataplane_mynginx":                             0,
				"envoy_cluster_upstream_rq_tx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_local_app_mynginx":                                    0,
				"envoy_cluster_upstream_rq_tx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_original-destination_mynginx":                         0,
				"envoy_cluster_upstream_rq_tx_reset_consul-sandbox-cluster-0159c9d3_default_default_mynginx_prometheus_backend_mynginx":                           0,
				"envoy_listener_admin_downstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                       1,
				"envoy_listener_admin_downstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                      2,
				"envoy_listener_admin_downstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                     0,
				"envoy_listener_admin_downstream_cx_overload_reject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                              0,
				"envoy_listener_admin_downstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                        3,
				"envoy_listener_admin_downstream_cx_transport_socket_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":             0,
				"envoy_listener_admin_downstream_global_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                              0,
				"envoy_listener_admin_downstream_listener_filter_error_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                           0,
				"envoy_listener_admin_downstream_listener_filter_remote_close_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                    0,
				"envoy_listener_admin_downstream_pre_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                   0,
				"envoy_listener_admin_downstream_pre_cx_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                  0,
				"envoy_listener_downstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                               1,
				"envoy_listener_downstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                           0,
				"envoy_listener_downstream_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                             0,
				"envoy_listener_downstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                              3,
				"envoy_listener_downstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                          6507,
				"envoy_listener_downstream_cx_destroy_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                            1,
				"envoy_listener_downstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                             0,
				"envoy_listener_downstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                         0,
				"envoy_listener_downstream_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                           0,
				"envoy_listener_downstream_cx_overload_reject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                      0,
				"envoy_listener_downstream_cx_overload_reject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                  0,
				"envoy_listener_downstream_cx_overload_reject_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                    0,
				"envoy_listener_downstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                                4,
				"envoy_listener_downstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                            6507,
				"envoy_listener_downstream_cx_total_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                              1,
				"envoy_listener_downstream_cx_transport_socket_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":     0,
				"envoy_listener_downstream_cx_transport_socket_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx": 0,
				"envoy_listener_downstream_cx_transport_socket_connect_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":   0,
				"envoy_listener_downstream_global_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                      0,
				"envoy_listener_downstream_global_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                  0,
				"envoy_listener_downstream_global_cx_overflow_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                    0,
				"envoy_listener_downstream_listener_filter_error_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                   0,
				"envoy_listener_downstream_listener_filter_error_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":               0,
				"envoy_listener_downstream_listener_filter_error_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                 0,
				"envoy_listener_downstream_listener_filter_remote_close_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":            0,
				"envoy_listener_downstream_listener_filter_remote_close_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":        0,
				"envoy_listener_downstream_listener_filter_remote_close_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":          0,
				"envoy_listener_downstream_pre_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                           0,
				"envoy_listener_downstream_pre_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                       0,
				"envoy_listener_downstream_pre_cx_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                         0,
				"envoy_listener_downstream_pre_cx_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_0.0.0.0_20200_mynginx":                          0,
				"envoy_listener_downstream_pre_cx_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_10.50.132.6_20000_mynginx":                      0,
				"envoy_listener_downstream_pre_cx_timeout_consul-sandbox-cluster-0159c9d3_default_default_mynginx_127.0.0.1_15001_mynginx":                        0,
				"envoy_listener_manager_listener_added_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                           3,
				"envoy_listener_manager_listener_create_failure_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                  0,
				"envoy_listener_manager_listener_create_success_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                  6,
				"envoy_listener_manager_listener_in_place_updated_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                0,
				"envoy_listener_manager_listener_modified_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                        0,
				"envoy_listener_manager_listener_removed_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                         0,
				"envoy_listener_manager_listener_stopped_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                         0,
				"envoy_listener_manager_total_listeners_active_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                   3,
				"envoy_listener_manager_total_listeners_draining_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                 0,
				"envoy_listener_manager_total_listeners_warming_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                  0,
				"envoy_server_memory_allocated_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                   7742368,
				"envoy_server_memory_heap_size_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                   14680064,
				"envoy_server_memory_physical_size_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                               19175778,
				"envoy_server_parent_connections_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                 0,
				"envoy_server_state_draining_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                     0,
				"envoy_server_state_initializing_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                 0,
				"envoy_server_state_live_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                         1,
				"envoy_server_state_pre_initializing_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                             0,
				"envoy_server_total_connections_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                  0,
				"envoy_server_uptime_consul-sandbox-cluster-0159c9d3_default_default_mynginx_mynginx":                                                             32527,
			},
		},
		"case envoy": {
			prepare: prepareCaseEnvoyStats,
			wantMetrics: map[string]int64{
				"envoy_cluster_manager_active_clusters":                                       1,
				"envoy_cluster_manager_cluster_added":                                         1,
				"envoy_cluster_manager_cluster_modified":                                      0,
				"envoy_cluster_manager_cluster_removed":                                       0,
				"envoy_cluster_manager_cluster_updated":                                       0,
				"envoy_cluster_manager_cluster_updated_via_merge":                             0,
				"envoy_cluster_manager_update_merge_cancelled":                                0,
				"envoy_cluster_manager_update_out_of_merge_window":                            0,
				"envoy_cluster_manager_warming_clusters":                                      0,
				"envoy_cluster_membership_change_service_envoyproxy_io":                       1,
				"envoy_cluster_membership_degraded_service_envoyproxy_io":                     0,
				"envoy_cluster_membership_excluded_service_envoyproxy_io":                     0,
				"envoy_cluster_membership_healthy_service_envoyproxy_io":                      1,
				"envoy_cluster_update_empty_service_envoyproxy_io":                            0,
				"envoy_cluster_update_failure_service_envoyproxy_io":                          0,
				"envoy_cluster_update_no_rebuild_service_envoyproxy_io":                       0,
				"envoy_cluster_update_success_service_envoyproxy_io":                          1242,
				"envoy_cluster_upstream_cx_active_service_envoyproxy_io":                      0,
				"envoy_cluster_upstream_cx_connect_attempts_exceeded_service_envoyproxy_io":   0,
				"envoy_cluster_upstream_cx_connect_fail_service_envoyproxy_io":                0,
				"envoy_cluster_upstream_cx_connect_timeout_service_envoyproxy_io":             0,
				"envoy_cluster_upstream_cx_destroy_local_service_envoyproxy_io":               0,
				"envoy_cluster_upstream_cx_destroy_remote_service_envoyproxy_io":              0,
				"envoy_cluster_upstream_cx_destroy_service_envoyproxy_io":                     0,
				"envoy_cluster_upstream_cx_http1_total_service_envoyproxy_io":                 0,
				"envoy_cluster_upstream_cx_http2_total_service_envoyproxy_io":                 0,
				"envoy_cluster_upstream_cx_http3_total_service_envoyproxy_io":                 0,
				"envoy_cluster_upstream_cx_idle_timeout_service_envoyproxy_io":                0,
				"envoy_cluster_upstream_cx_max_duration_reached_service_envoyproxy_io":        0,
				"envoy_cluster_upstream_cx_overflow_service_envoyproxy_io":                    0,
				"envoy_cluster_upstream_cx_rx_bytes_buffered_service_envoyproxy_io":           0,
				"envoy_cluster_upstream_cx_rx_bytes_total_service_envoyproxy_io":              0,
				"envoy_cluster_upstream_cx_total_service_envoyproxy_io":                       0,
				"envoy_cluster_upstream_cx_tx_bytes_buffered_service_envoyproxy_io":           0,
				"envoy_cluster_upstream_cx_tx_bytes_total_service_envoyproxy_io":              0,
				"envoy_cluster_upstream_rq_active_service_envoyproxy_io":                      0,
				"envoy_cluster_upstream_rq_cancelled_service_envoyproxy_io":                   0,
				"envoy_cluster_upstream_rq_maintenance_mode_service_envoyproxy_io":            0,
				"envoy_cluster_upstream_rq_max_duration_reached_service_envoyproxy_io":        0,
				"envoy_cluster_upstream_rq_pending_active_service_envoyproxy_io":              0,
				"envoy_cluster_upstream_rq_pending_failure_eject_service_envoyproxy_io":       0,
				"envoy_cluster_upstream_rq_pending_overflow_service_envoyproxy_io":            0,
				"envoy_cluster_upstream_rq_pending_total_service_envoyproxy_io":               0,
				"envoy_cluster_upstream_rq_per_try_timeout_service_envoyproxy_io":             0,
				"envoy_cluster_upstream_rq_retry_backoff_exponential_service_envoyproxy_io":   0,
				"envoy_cluster_upstream_rq_retry_backoff_ratelimited_service_envoyproxy_io":   0,
				"envoy_cluster_upstream_rq_retry_service_envoyproxy_io":                       0,
				"envoy_cluster_upstream_rq_retry_success_service_envoyproxy_io":               0,
				"envoy_cluster_upstream_rq_rx_reset_service_envoyproxy_io":                    0,
				"envoy_cluster_upstream_rq_timeout_service_envoyproxy_io":                     0,
				"envoy_cluster_upstream_rq_total_service_envoyproxy_io":                       0,
				"envoy_cluster_upstream_rq_tx_reset_service_envoyproxy_io":                    0,
				"envoy_listener_admin_downstream_cx_active":                                   2,
				"envoy_listener_admin_downstream_cx_destroy":                                  4,
				"envoy_listener_admin_downstream_cx_overflow":                                 0,
				"envoy_listener_admin_downstream_cx_overload_reject":                          0,
				"envoy_listener_admin_downstream_cx_total":                                    6,
				"envoy_listener_admin_downstream_cx_transport_socket_connect_timeout":         0,
				"envoy_listener_admin_downstream_global_cx_overflow":                          0,
				"envoy_listener_admin_downstream_listener_filter_error":                       0,
				"envoy_listener_admin_downstream_listener_filter_remote_close":                0,
				"envoy_listener_admin_downstream_pre_cx_active":                               0,
				"envoy_listener_admin_downstream_pre_cx_timeout":                              0,
				"envoy_listener_downstream_cx_active_0.0.0.0_10000":                           0,
				"envoy_listener_downstream_cx_destroy_0.0.0.0_10000":                          0,
				"envoy_listener_downstream_cx_overflow_0.0.0.0_10000":                         0,
				"envoy_listener_downstream_cx_overload_reject_0.0.0.0_10000":                  0,
				"envoy_listener_downstream_cx_total_0.0.0.0_10000":                            0,
				"envoy_listener_downstream_cx_transport_socket_connect_timeout_0.0.0.0_10000": 0,
				"envoy_listener_downstream_global_cx_overflow_0.0.0.0_10000":                  0,
				"envoy_listener_downstream_listener_filter_error_0.0.0.0_10000":               0,
				"envoy_listener_downstream_listener_filter_remote_close_0.0.0.0_10000":        0,
				"envoy_listener_downstream_pre_cx_active_0.0.0.0_10000":                       0,
				"envoy_listener_downstream_pre_cx_timeout_0.0.0.0_10000":                      0,
				"envoy_listener_manager_listener_added":                                       1,
				"envoy_listener_manager_listener_create_failure":                              0,
				"envoy_listener_manager_listener_create_success":                              16,
				"envoy_listener_manager_listener_in_place_updated":                            0,
				"envoy_listener_manager_listener_modified":                                    0,
				"envoy_listener_manager_listener_removed":                                     0,
				"envoy_listener_manager_listener_stopped":                                     0,
				"envoy_listener_manager_total_listeners_active":                               1,
				"envoy_listener_manager_total_listeners_draining":                             0,
				"envoy_listener_manager_total_listeners_warming":                              0,
				"envoy_server_memory_allocated":                                               7630184,
				"envoy_server_memory_heap_size":                                               16777216,
				"envoy_server_memory_physical_size":                                           28426958,
				"envoy_server_parent_connections":                                             0,
				"envoy_server_state_draining":                                                 0,
				"envoy_server_state_initializing":                                             0,
				"envoy_server_state_live":                                                     1,
				"envoy_server_state_pre_initializing":                                         0,
				"envoy_server_total_connections":                                              0,
				"envoy_server_uptime":                                                         6225,
			},
		},
		"case invalid data response": {
			prepare:     prepareCaseInvalidDataResponse,
			wantMetrics: nil,
		},
		"case 404": {
			prepare:     prepareCase404,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)
			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareCaseEnvoyConsulDataplaneStats() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataEnvoyConsulDataplane)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareCaseEnvoyStats() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataEnvoy)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareCaseInvalidDataResponse() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareCase404() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}
