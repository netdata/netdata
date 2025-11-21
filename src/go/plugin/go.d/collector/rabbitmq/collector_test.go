// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

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

	dataClusterWhoami, _      = os.ReadFile("testdata/v4.0.3/cluster/whoami.json")
	dataClusterDefinitions, _ = os.ReadFile("testdata/v4.0.3/cluster/definitions.json")
	dataClusterOverview, _    = os.ReadFile("testdata/v4.0.3/cluster/overview.json")
	dataClusterNodes, _       = os.ReadFile("testdata/v4.0.3/cluster/nodes.json")
	dataClusterVhosts, _      = os.ReadFile("testdata/v4.0.3/cluster/vhosts.json")
	dataClusterQueues, _      = os.ReadFile("testdata/v4.0.3/cluster/queues.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataClusterWhoami":      dataClusterWhoami,
		"dataClusterDefinitions": dataClusterDefinitions,
		"dataClusterOverview":    dataClusterOverview,
		"dataClusterNodes":       dataClusterNodes,
		"dataClusterVhosts":      dataClusterVhosts,
		"dataClusterQueues":      dataClusterQueues,
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })

	collr := New()
	require.NoError(t, collr.Init(context.Background()))

	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (*Collector, func())
		wantFail bool
	}{
		"success on valid response": {wantFail: false, prepare: caseClusterOk},
		"fails on invalid response": {wantFail: true, prepare: caseInvalidDataResponse},
		"fails on 404":              {wantFail: true, prepare: case404},
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
		prepare       func() (*Collector, func())
		wantCollected map[string]int64
		wantCharts    int
	}{
		"case cluster ok ": {
			prepare: caseClusterOk,
			wantCharts: len(overviewCharts) +
				len(nodeClusterPeerChartsTmpl)*2 +
				len(nodeChartsTmpl)*2 +
				len(vhostChartsTmpl)*2 +
				len(queueChartsTmpl)*4,
			wantCollected: map[string]int64{
				"churn_rates_channel_closed":                                                                         7,
				"churn_rates_channel_created":                                                                        7,
				"churn_rates_connection_closed":                                                                      8,
				"churn_rates_connection_created":                                                                     7,
				"churn_rates_queue_created":                                                                          2,
				"churn_rates_queue_declared":                                                                         3,
				"churn_rates_queue_deleted":                                                                          0,
				"message_stats_ack":                                                                                  0,
				"message_stats_confirm":                                                                              1,
				"message_stats_deliver":                                                                              0,
				"message_stats_deliver_get":                                                                          4,
				"message_stats_deliver_no_ack":                                                                       0,
				"message_stats_get":                                                                                  3,
				"message_stats_get_empty":                                                                            2,
				"message_stats_get_no_ack":                                                                           1,
				"message_stats_publish":                                                                              1,
				"message_stats_publish_in":                                                                           0,
				"message_stats_publish_out":                                                                          0,
				"message_stats_redeliver":                                                                            3,
				"message_stats_return_unroutable":                                                                    0,
				"node_rabbit@ilyam-deb11-play_avail_status_down":                                                     0,
				"node_rabbit@ilyam-deb11-play_avail_status_running":                                                  1,
				"node_rabbit@ilyam-deb11-play_disk_free_alarm_status_clear":                                          1,
				"node_rabbit@ilyam-deb11-play_disk_free_alarm_status_triggered":                                      0,
				"node_rabbit@ilyam-deb11-play_disk_free_bytes":                                                       46901432320,
				"node_rabbit@ilyam-deb11-play_fds_available":                                                         1048534,
				"node_rabbit@ilyam-deb11-play_fds_used":                                                              42,
				"node_rabbit@ilyam-deb11-play_mem_alarm_status_clear":                                                1,
				"node_rabbit@ilyam-deb11-play_mem_alarm_status_triggered":                                            0,
				"node_rabbit@ilyam-deb11-play_mem_available":                                                         9935852339,
				"node_rabbit@ilyam-deb11-play_mem_used":                                                              142905344,
				"node_rabbit@ilyam-deb11-play_network_partition_status_clear":                                        1,
				"node_rabbit@ilyam-deb11-play_network_partition_status_detected":                                     0,
				"node_rabbit@ilyam-deb11-play_peer_rabbit@pve-deb-work_cluster_link_recv_bytes":                      2374358706,
				"node_rabbit@ilyam-deb11-play_peer_rabbit@pve-deb-work_cluster_link_send_bytes":                      2297379728,
				"node_rabbit@ilyam-deb11-play_procs_available":                                                       1048138,
				"node_rabbit@ilyam-deb11-play_procs_used":                                                            438,
				"node_rabbit@ilyam-deb11-play_run_queue":                                                             1,
				"node_rabbit@ilyam-deb11-play_sockets_available":                                                     0,
				"node_rabbit@ilyam-deb11-play_sockets_used":                                                          0,
				"node_rabbit@ilyam-deb11-play_uptime":                                                                241932,
				"node_rabbit@pve-deb-work_avail_status_down":                                                         0,
				"node_rabbit@pve-deb-work_avail_status_running":                                                      1,
				"node_rabbit@pve-deb-work_disk_free_alarm_status_clear":                                              1,
				"node_rabbit@pve-deb-work_disk_free_alarm_status_triggered":                                          0,
				"node_rabbit@pve-deb-work_disk_free_bytes":                                                           103827365888,
				"node_rabbit@pve-deb-work_fds_available":                                                             1048528,
				"node_rabbit@pve-deb-work_fds_used":                                                                  48,
				"node_rabbit@pve-deb-work_mem_alarm_status_clear":                                                    1,
				"node_rabbit@pve-deb-work_mem_alarm_status_triggered":                                                0,
				"node_rabbit@pve-deb-work_mem_available":                                                             14958259404,
				"node_rabbit@pve-deb-work_mem_used":                                                                  160018432,
				"node_rabbit@pve-deb-work_network_partition_status_clear":                                            1,
				"node_rabbit@pve-deb-work_network_partition_status_detected":                                         0,
				"node_rabbit@pve-deb-work_peer_rabbit@ilyam-deb11-play_cluster_link_recv_bytes":                      2297460158,
				"node_rabbit@pve-deb-work_peer_rabbit@ilyam-deb11-play_cluster_link_send_bytes":                      2374459095,
				"node_rabbit@pve-deb-work_procs_available":                                                           1048109,
				"node_rabbit@pve-deb-work_procs_used":                                                                467,
				"node_rabbit@pve-deb-work_run_queue":                                                                 0,
				"node_rabbit@pve-deb-work_sockets_available":                                                         0,
				"node_rabbit@pve-deb-work_sockets_used":                                                              0,
				"node_rabbit@pve-deb-work_uptime":                                                                    73793,
				"object_totals_channels":                                                                             0,
				"object_totals_connections":                                                                          0,
				"object_totals_consumers":                                                                            0,
				"object_totals_exchanges":                                                                            14,
				"object_totals_queues":                                                                               4,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_ack":                              0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_confirm":                          0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver":                          0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver_get":                      4,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver_no_ack":                   0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get":                              3,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get_empty":                        2,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get_no_ack":                       1,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish":                          1,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish_in":                       0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish_out":                      0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_redeliver":                        3,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_return_unroutable":                0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_messages":                                       0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_messages_paged_out":                             0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_messages_persistent":                            0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_messages_ready":                                 0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_messages_unacknowledged":                        0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_crashed":                                 0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_down":                                    0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_idle":                                    0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_minority":                                0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_running":                                 1,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_stopped":                                 0,
				"queue_MyFirstQueue_vhost_/_node_rabbit@pve-deb-work_status_terminated":                              0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_ack":               0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_confirm":           0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_deliver":           0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_deliver_get":       0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_deliver_no_ack":    0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_get":               0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_get_empty":         0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_get_no_ack":        0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_publish":           0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_publish_in":        0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_publish_out":       0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_redeliver":         0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_message_stats_return_unroutable": 0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_messages":                        0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_messages_paged_out":              0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_messages_persistent":             0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_messages_ready":                  0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_messages_unacknowledged":         0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_crashed":                  0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_down":                     0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_idle":                     0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_minority":                 0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_running":                  1,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_stopped":                  0,
				"queue_MyFirstQueue_vhost_myFirstVhost_node_rabbit@ilyam-deb11-play_status_terminated":               0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_ack":                             0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_confirm":                         0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver":                         0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver_get":                     0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_deliver_no_ack":                  0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get":                             0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get_empty":                       0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_get_no_ack":                      0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish":                         0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish_in":                      0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_publish_out":                     0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_redeliver":                       0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_message_stats_return_unroutable":               0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_messages":                                      0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_messages_paged_out":                            0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_messages_persistent":                           0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_messages_ready":                                0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_messages_unacknowledged":                       0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_crashed":                                0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_down":                                   0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_idle":                                   0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_minority":                               0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_running":                                1,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_stopped":                                0,
				"queue_MySecondQueue_vhost_/_node_rabbit@pve-deb-work_status_terminated":                             0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_ack":                   0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_confirm":               0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_deliver":               0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_deliver_get":           0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_deliver_no_ack":        0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_get":                   0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_get_empty":             0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_get_no_ack":            0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_publish":               0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_publish_in":            0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_publish_out":           0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_redeliver":             0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_message_stats_return_unroutable":     0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_messages":                            0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_messages_paged_out":                  0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_messages_persistent":                 0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_messages_ready":                      0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_messages_unacknowledged":             0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_crashed":                      0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_down":                         0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_idle":                         0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_minority":                     0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_running":                      1,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_stopped":                      0,
				"queue_myFirstQueue_vhost_myFirstVhost_node_rabbit@pve-deb-work_status_terminated":                   0,
				"queue_totals_messages":                              0,
				"queue_totals_messages_ready":                        0,
				"queue_totals_messages_unacknowledged":               0,
				"vhost_/_message_stats_ack":                          0,
				"vhost_/_message_stats_confirm":                      1,
				"vhost_/_message_stats_deliver":                      0,
				"vhost_/_message_stats_deliver_get":                  4,
				"vhost_/_message_stats_deliver_no_ack":               0,
				"vhost_/_message_stats_get":                          3,
				"vhost_/_message_stats_get_empty":                    2,
				"vhost_/_message_stats_get_no_ack":                   1,
				"vhost_/_message_stats_publish":                      1,
				"vhost_/_message_stats_publish_in":                   0,
				"vhost_/_message_stats_publish_out":                  0,
				"vhost_/_message_stats_redeliver":                    3,
				"vhost_/_message_stats_return_unroutable":            0,
				"vhost_/_messages":                                   0,
				"vhost_/_messages_ready":                             0,
				"vhost_/_messages_unacknowledged":                    0,
				"vhost_/_status_partial":                             0,
				"vhost_/_status_running":                             1,
				"vhost_/_status_stopped":                             0,
				"vhost_myFirstVhost_message_stats_ack":               0,
				"vhost_myFirstVhost_message_stats_confirm":           0,
				"vhost_myFirstVhost_message_stats_deliver":           0,
				"vhost_myFirstVhost_message_stats_deliver_get":       0,
				"vhost_myFirstVhost_message_stats_deliver_no_ack":    0,
				"vhost_myFirstVhost_message_stats_get":               0,
				"vhost_myFirstVhost_message_stats_get_empty":         0,
				"vhost_myFirstVhost_message_stats_get_no_ack":        0,
				"vhost_myFirstVhost_message_stats_publish":           0,
				"vhost_myFirstVhost_message_stats_publish_in":        0,
				"vhost_myFirstVhost_message_stats_publish_out":       0,
				"vhost_myFirstVhost_message_stats_redeliver":         0,
				"vhost_myFirstVhost_message_stats_return_unroutable": 0,
				"vhost_myFirstVhost_messages":                        0,
				"vhost_myFirstVhost_messages_ready":                  0,
				"vhost_myFirstVhost_messages_unacknowledged":         0,
				"vhost_myFirstVhost_status_partial":                  0,
				"vhost_myFirstVhost_status_running":                  1,
				"vhost_myFirstVhost_status_stopped":                  0,
			},
		},
		"fails on unexpected JSON response": {
			prepare:       caseUnexpectedJsonResponse,
			wantCollected: nil,
		},
		"fails on invalid response": {
			prepare:       caseInvalidDataResponse,
			wantCollected: nil,
		},
		"fails on 404": {
			prepare:       case404,
			wantCollected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)

			if len(test.wantCollected) > 0 {
				assert.Equal(t, test.wantCharts, len(*collr.Charts()))
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func caseClusterOk() (*Collector, func()) {
	srv := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case urlPathAPIWhoami:
					_, _ = w.Write(dataClusterWhoami)
				case urlPathAPIDefinitions:
					_, _ = w.Write(dataClusterDefinitions)
				case urlPathAPIOverview:
					_, _ = w.Write(dataClusterOverview)
				case urlPathAPINodes:
					_, _ = w.Write(dataClusterNodes)
				case urlPathAPIVhosts:
					_, _ = w.Write(dataClusterVhosts)
				case urlPathAPIQueues:
					_, _ = w.Write(dataClusterQueues)
				default:
					w.WriteHeader(404)
				}
			}))
	collr := New()
	collr.URL = srv.URL
	collr.CollectQueues = true

	return collr, srv.Close
}

func caseUnexpectedJsonResponse() (*Collector, func()) {
	resp := `
{
    "elephant": {
        "burn": false,
        "mountain": true,
        "fog": false,
        "skin": -1561907625,
        "burst": "anyway",
        "shadow": 1558616893
    },
    "start": "ever",
    "base": 2093056027,
    "mission": -2007590351,
    "victory": 999053756,
    "die": false
}
`
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(resp))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func caseInvalidDataResponse() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func case404() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}
