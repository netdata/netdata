// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataOverviewStats, _ = os.ReadFile("testdata/v3.11.5/api-overview.json")
	dataNodeStats, _     = os.ReadFile("testdata/v3.11.5/api-nodes-node.json")
	dataVhostsStats, _   = os.ReadFile("testdata/v3.11.5/api-vhosts.json")
	dataQueuesStats, _   = os.ReadFile("testdata/v3.11.5/api-queues.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":    dataConfigJSON,
		"dataConfigYAML":    dataConfigYAML,
		"dataOverviewStats": dataOverviewStats,
		"dataNodeStats":     dataNodeStats,
		"dataVhostsStats":   dataVhostsStats,
		"dataQueuesStats":   dataQueuesStats,
	} {
		require.NotNil(t, data, name)
	}
}

func TestRabbitMQ_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &RabbitMQ{}, dataConfigJSON, dataConfigYAML)
}

func TestRabbitMQ_Init(t *testing.T) {
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
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rabbit := New()
			rabbit.Config = test.config

			if test.wantFail {
				assert.Error(t, rabbit.Init())
			} else {
				assert.NoError(t, rabbit.Init())
			}
		})
	}
}

func TestRabbitMQ_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestRabbitMQ_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)

	rabbit := New()
	require.NoError(t, rabbit.Init())

	assert.NotPanics(t, rabbit.Cleanup)
}

func TestRabbitMQ_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (*RabbitMQ, func())
		wantFail bool
	}{
		"success on valid response": {wantFail: false, prepare: caseSuccessAllRequests},
		"fails on invalid response": {wantFail: true, prepare: caseInvalidDataResponse},
		"fails on 404":              {wantFail: true, prepare: case404},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rabbit, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, rabbit.Init())

			if test.wantFail {
				assert.Error(t, rabbit.Check())
			} else {
				assert.NoError(t, rabbit.Check())
			}
		})
	}
}

func TestRabbitMQ_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (*RabbitMQ, func())
		wantCollected map[string]int64
		wantCharts    int
	}{
		"success on valid response": {
			prepare:    caseSuccessAllRequests,
			wantCharts: len(baseCharts) + len(chartsTmplVhost)*3 + len(chartsTmplQueue)*4,
			wantCollected: map[string]int64{
				"churn_rates_channel_closed":      0,
				"churn_rates_channel_created":     0,
				"churn_rates_connection_closed":   0,
				"churn_rates_connection_created":  0,
				"churn_rates_queue_created":       6,
				"churn_rates_queue_declared":      6,
				"churn_rates_queue_deleted":       2,
				"disk_free":                       189799186432,
				"fd_total":                        1048576,
				"fd_used":                         43,
				"mem_limit":                       6713820774,
				"mem_used":                        172720128,
				"message_stats_ack":               0,
				"message_stats_confirm":           0,
				"message_stats_deliver":           0,
				"message_stats_deliver_get":       0,
				"message_stats_deliver_no_ack":    0,
				"message_stats_get":               0,
				"message_stats_get_no_ack":        0,
				"message_stats_publish":           0,
				"message_stats_publish_in":        0,
				"message_stats_publish_out":       0,
				"message_stats_redeliver":         0,
				"message_stats_return_unroutable": 0,
				"object_totals_channels":          0,
				"object_totals_connections":       0,
				"object_totals_consumers":         0,
				"object_totals_exchanges":         21,
				"object_totals_queues":            4,
				"proc_available":                  1048135,
				"proc_total":                      1048576,
				"proc_used":                       441,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_ack":               0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_confirm":           0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_deliver":           0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_deliver_get":       0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_deliver_no_ack":    0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_get":               0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_get_no_ack":        0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_publish":           0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_publish_in":        0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_publish_out":       0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_redeliver":         0,
				"queue_MyFirstQueue_vhost_mySecondVhost_message_stats_return_unroutable": 0,
				"queue_MyFirstQueue_vhost_mySecondVhost_messages":                        1,
				"queue_MyFirstQueue_vhost_mySecondVhost_messages_paged_out":              1,
				"queue_MyFirstQueue_vhost_mySecondVhost_messages_persistent":             1,
				"queue_MyFirstQueue_vhost_mySecondVhost_messages_ready":                  1,
				"queue_MyFirstQueue_vhost_mySecondVhost_messages_unacknowledged":         1,
				"queue_myFirstQueue_vhost_/_message_stats_ack":                           0,
				"queue_myFirstQueue_vhost_/_message_stats_confirm":                       0,
				"queue_myFirstQueue_vhost_/_message_stats_deliver":                       0,
				"queue_myFirstQueue_vhost_/_message_stats_deliver_get":                   0,
				"queue_myFirstQueue_vhost_/_message_stats_deliver_no_ack":                0,
				"queue_myFirstQueue_vhost_/_message_stats_get":                           0,
				"queue_myFirstQueue_vhost_/_message_stats_get_no_ack":                    0,
				"queue_myFirstQueue_vhost_/_message_stats_publish":                       0,
				"queue_myFirstQueue_vhost_/_message_stats_publish_in":                    0,
				"queue_myFirstQueue_vhost_/_message_stats_publish_out":                   0,
				"queue_myFirstQueue_vhost_/_message_stats_redeliver":                     0,
				"queue_myFirstQueue_vhost_/_message_stats_return_unroutable":             0,
				"queue_myFirstQueue_vhost_/_messages":                                    1,
				"queue_myFirstQueue_vhost_/_messages_paged_out":                          1,
				"queue_myFirstQueue_vhost_/_messages_persistent":                         1,
				"queue_myFirstQueue_vhost_/_messages_ready":                              1,
				"queue_myFirstQueue_vhost_/_messages_unacknowledged":                     1,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_ack":                0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_confirm":            0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_deliver":            0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_deliver_get":        0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_deliver_no_ack":     0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_get":                0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_get_no_ack":         0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_publish":            0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_publish_in":         0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_publish_out":        0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_redeliver":          0,
				"queue_myFirstQueue_vhost_myFirstVhost_message_stats_return_unroutable":  0,
				"queue_myFirstQueue_vhost_myFirstVhost_messages":                         1,
				"queue_myFirstQueue_vhost_myFirstVhost_messages_paged_out":               1,
				"queue_myFirstQueue_vhost_myFirstVhost_messages_persistent":              1,
				"queue_myFirstQueue_vhost_myFirstVhost_messages_ready":                   1,
				"queue_myFirstQueue_vhost_myFirstVhost_messages_unacknowledged":          1,
				"queue_mySecondQueue_vhost_/_message_stats_ack":                          0,
				"queue_mySecondQueue_vhost_/_message_stats_confirm":                      0,
				"queue_mySecondQueue_vhost_/_message_stats_deliver":                      0,
				"queue_mySecondQueue_vhost_/_message_stats_deliver_get":                  0,
				"queue_mySecondQueue_vhost_/_message_stats_deliver_no_ack":               0,
				"queue_mySecondQueue_vhost_/_message_stats_get":                          0,
				"queue_mySecondQueue_vhost_/_message_stats_get_no_ack":                   0,
				"queue_mySecondQueue_vhost_/_message_stats_publish":                      0,
				"queue_mySecondQueue_vhost_/_message_stats_publish_in":                   0,
				"queue_mySecondQueue_vhost_/_message_stats_publish_out":                  0,
				"queue_mySecondQueue_vhost_/_message_stats_redeliver":                    0,
				"queue_mySecondQueue_vhost_/_message_stats_return_unroutable":            0,
				"queue_mySecondQueue_vhost_/_messages":                                   1,
				"queue_mySecondQueue_vhost_/_messages_paged_out":                         1,
				"queue_mySecondQueue_vhost_/_messages_persistent":                        1,
				"queue_mySecondQueue_vhost_/_messages_ready":                             1,
				"queue_mySecondQueue_vhost_/_messages_unacknowledged":                    1,
				"queue_totals_messages":                                                  0,
				"queue_totals_messages_ready":                                            0,
				"queue_totals_messages_unacknowledged":                                   0,
				"run_queue":                                                              1,
				"sockets_total":                                                          943629,
				"sockets_used":                                                           0,
				"vhost_/_message_stats_ack":                                              0,
				"vhost_/_message_stats_confirm":                                          0,
				"vhost_/_message_stats_deliver":                                          0,
				"vhost_/_message_stats_deliver_get":                                      0,
				"vhost_/_message_stats_deliver_no_ack":                                   0,
				"vhost_/_message_stats_get":                                              0,
				"vhost_/_message_stats_get_no_ack":                                       0,
				"vhost_/_message_stats_publish":                                          0,
				"vhost_/_message_stats_publish_in":                                       0,
				"vhost_/_message_stats_publish_out":                                      0,
				"vhost_/_message_stats_redeliver":                                        0,
				"vhost_/_message_stats_return_unroutable":                                0,
				"vhost_/_messages":                                    1,
				"vhost_/_messages_ready":                              1,
				"vhost_/_messages_unacknowledged":                     1,
				"vhost_myFirstVhost_message_stats_ack":                0,
				"vhost_myFirstVhost_message_stats_confirm":            0,
				"vhost_myFirstVhost_message_stats_deliver":            0,
				"vhost_myFirstVhost_message_stats_deliver_get":        0,
				"vhost_myFirstVhost_message_stats_deliver_no_ack":     0,
				"vhost_myFirstVhost_message_stats_get":                0,
				"vhost_myFirstVhost_message_stats_get_no_ack":         0,
				"vhost_myFirstVhost_message_stats_publish":            0,
				"vhost_myFirstVhost_message_stats_publish_in":         0,
				"vhost_myFirstVhost_message_stats_publish_out":        0,
				"vhost_myFirstVhost_message_stats_redeliver":          0,
				"vhost_myFirstVhost_message_stats_return_unroutable":  0,
				"vhost_myFirstVhost_messages":                         1,
				"vhost_myFirstVhost_messages_ready":                   1,
				"vhost_myFirstVhost_messages_unacknowledged":          1,
				"vhost_mySecondVhost_message_stats_ack":               0,
				"vhost_mySecondVhost_message_stats_confirm":           0,
				"vhost_mySecondVhost_message_stats_deliver":           0,
				"vhost_mySecondVhost_message_stats_deliver_get":       0,
				"vhost_mySecondVhost_message_stats_deliver_no_ack":    0,
				"vhost_mySecondVhost_message_stats_get":               0,
				"vhost_mySecondVhost_message_stats_get_no_ack":        0,
				"vhost_mySecondVhost_message_stats_publish":           0,
				"vhost_mySecondVhost_message_stats_publish_in":        0,
				"vhost_mySecondVhost_message_stats_publish_out":       0,
				"vhost_mySecondVhost_message_stats_redeliver":         0,
				"vhost_mySecondVhost_message_stats_return_unroutable": 0,
				"vhost_mySecondVhost_messages":                        1,
				"vhost_mySecondVhost_messages_ready":                  1,
				"vhost_mySecondVhost_messages_unacknowledged":         1,
			},
		},
		"fails on invalid response": {
			prepare:       caseInvalidDataResponse,
			wantCollected: nil,
			wantCharts:    len(baseCharts),
		},
		"fails on 404": {
			prepare:       case404,
			wantCollected: nil,
			wantCharts:    len(baseCharts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rabbit, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, rabbit.Init())

			mx := rabbit.Collect()

			assert.Equal(t, test.wantCollected, mx)
			assert.Equal(t, test.wantCharts, len(*rabbit.Charts()))
		})
	}
}

func caseSuccessAllRequests() (*RabbitMQ, func()) {
	srv := prepareRabbitMQEndpoint()
	rabbit := New()
	rabbit.URL = srv.URL
	rabbit.CollectQueues = true

	return rabbit, srv.Close
}

func caseInvalidDataResponse() (*RabbitMQ, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	rabbit := New()
	rabbit.URL = srv.URL

	return rabbit, srv.Close
}

func case404() (*RabbitMQ, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	rabbit := New()
	rabbit.URL = srv.URL

	return rabbit, srv.Close
}

func prepareRabbitMQEndpoint() *httptest.Server {
	srv := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case urlPathAPIOverview:
					_, _ = w.Write(dataOverviewStats)
				case filepath.Join(urlPathAPINodes, "rabbit@localhost"):
					_, _ = w.Write(dataNodeStats)
				case urlPathAPIVhosts:
					_, _ = w.Write(dataVhostsStats)
				case urlPathAPIQueues:
					_, _ = w.Write(dataQueuesStats)
				default:
					w.WriteHeader(404)
				}
			}))
	return srv
}
