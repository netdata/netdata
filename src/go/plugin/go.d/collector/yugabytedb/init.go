// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

const (
	metricPxMasterLatencyMasterClient           = "handler_latency_yb_master_MasterClient"
	metricPxMasterLatencyMasterDdl              = "handler_latency_yb_master_MasterDdl"
	metricPxTserverHandlerLatency               = "handler_latency_yb_tserver"
	metricPxTserverResponseBytes                = "service_response_bytes_yb_tserver"
	metricPxTserverRequestBytes                 = "service_request_bytes_yb_tserver"
	metricPxServerLatencyConsensusService       = "handler_latency_yb_consensus_ConsensusService"
	metricPxServerResponseBytesConsensusService = "service_response_bytes_yb_consensus_ConsensusService"
	metricPxServerRequestBytesConsensusService  = "service_request_bytes_yb_consensus_ConsensusService"
	metricPxYCQLLatencySQLProcessor             = "handler_latency_yb_cqlserver_SQLProcessor"
	metricPxYSQLLatencySQLProcessor             = "handler_latency_yb_ysqlserver_SQLProcessor"
)

const (
	metricHybridClockSkew       = "hybrid_clock_skew"
	metricSqlActiveConnTotal    = "yb_ysqlserver_active_connection_total"
	metricSqlConnTotal          = "yb_ysqlserver_connection_total"
	metricSqlMaxConnTotal       = "yb_ysqlserver_max_connection_total"
	metricSqlConnOverLimitTotal = "yb_ysqlserver_connection_over_limit_total"
	metricSqlNewConnTotal       = "yb_ysqlserver_new_connection_total"
)

func (c *Collector) initPrometheusClient(httpClient *http.Client) (prometheus.Prometheus, error) {
	// Frequently used metrics
	// https://docs.yugabyte.com/preview/launch-and-manage/monitor-and-alert/metrics/#frequently-used-metrics

	se := selector.Expr{Allow: []string{
		metricPxMasterLatencyMasterClient + "*",
		metricPxMasterLatencyMasterDdl + "*",

		metricPxTserverRequestBytes + "*",
		metricPxTserverResponseBytes + "*",
		metricPxTserverHandlerLatency + "*",

		metricPxServerLatencyConsensusService + "*",
		metricPxServerResponseBytesConsensusService + "*",
		metricPxServerRequestBytesConsensusService + "*",

		metricHybridClockSkew,

		// https://docs.yugabyte.com/preview/launch-and-manage/monitor-and-alert/metrics/throughput/#ysql-query-processing
		metricPxYSQLLatencySQLProcessor + "*",

		// https://docs.yugabyte.com/preview/launch-and-manage/monitor-and-alert/metrics/connections/
		metricSqlActiveConnTotal,
		metricSqlConnTotal,
		metricSqlMaxConnTotal,
		metricSqlConnOverLimitTotal,
		metricSqlNewConnTotal,

		metricPxYCQLLatencySQLProcessor + "*",
	}}

	sr, err := se.Parse()
	if err != nil {
		return nil, err
	}

	return prometheus.NewWithSelector(httpClient, c.RequestConfig, sr), nil
}
