// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioMasterClientOps = collectorapi.Priority + iota
	prioMasterClientOpsLatency
	prioMasterDdlOps
	prioMasterDdlOpsLatency

	prioServiceOps
	prioServiceOpsLatency
	prioServiceTraffic

	prioConsensusOps
	prioConsensusOpsLatency
	prioConsensusTraffic

	prioYCQLStatements
	prioYCQLStatementsLatency

	prioYSQLConnectionUsage
	prioYSQLActiveConnections
	prioYSQLEstablishedConnections
	prioYSQLOverLimitConnections
	prioYSQLStatements
	prioYSQLStatementsLatency
)

var (
	masterClientOpsChartTmpl = collectorapi.Chart{
		ID:       "master_client_%s_operations",
		Title:    "Master Client Operations",
		Units:    "ops/s",
		Fam:      "master client ops",
		Ctx:      "yugabytedb.master_client_operations",
		Priority: prioMasterClientOps,
		Dims: collectorapi.Dims{
			{ID: metricPxMasterLatencyMasterClient + "_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	masterClientOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "master_client_%s_operations_latency",
		Title:    "Master Client Operation Latency",
		Units:    "microseconds",
		Fam:      "master client ops",
		Ctx:      "yugabytedb.master_client_operations_latency",
		Priority: prioMasterClientOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxMasterLatencyMasterClient + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}

	masterDdlOpsChartTmpl = collectorapi.Chart{
		ID:       "master_ddl_%s_operations",
		Title:    "Master DDL Operations",
		Units:    "ops/s",
		Fam:      "ddl ops",
		Ctx:      "yugabytedb.master_ddl_operations",
		Priority: prioMasterDdlOps,
		Dims: collectorapi.Dims{
			{ID: metricPxMasterLatencyMasterDdl + "_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	masterDdlOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "master_ddl_%s_operations_latency",
		Title:    "Master DDL Operations Latency",
		Units:    "microseconds",
		Fam:      "ddl ops",
		Ctx:      "yugabytedb.master_ddl_operations_latency",
		Priority: prioMasterDdlOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxMasterLatencyMasterDdl + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}

	masterServiceOpsChartTmpl = collectorapi.Chart{
		ID:       "master_%s_%s_operations",
		Title:    "Master %s Operations",
		Units:    "ops/s",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_operations",
		Priority: prioServiceOps,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	masterServiceOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "master_%s_%s_operation_latency",
		Title:    "Master %s Operation Latency",
		Units:    "microseconds",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_operations_latency",
		Priority: prioServiceOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
	masterServiceTrafficChartTmpl = collectorapi.Chart{
		ID:       "master_%s_%s_traffic",
		Title:    "Master %s Traffic",
		Units:    "bytes/s",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_traffic",
		Priority: prioServiceTraffic,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverResponseBytes + "_%s_%s", Name: "received", Algo: collectorapi.Incremental},
			{ID: metricPxTserverRequestBytes + "_%s_%s", Name: "sent", Algo: collectorapi.Incremental},
		},
	}

	masterConsensusOpsChartTmpl = collectorapi.Chart{
		ID:       "master_consensus_%s_operations",
		Title:    "Master Consensus Operations",
		Units:    "ops/s",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_operations",
		Priority: prioConsensusOps,
		Dims: collectorapi.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	masterConsensusOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "master_consensus_%s_operation_latency",
		Title:    "Master Consensus Operation Latency",
		Units:    "microseconds",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_operations_latency",
		Priority: prioConsensusOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
	masterConsensusTrafficChartTmpl = collectorapi.Chart{
		ID:       "master_consensus_%s_traffic",
		Title:    "Master Consensus Traffic",
		Units:    "bytes/s",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_traffic",
		Priority: prioConsensusTraffic,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: metricPxServerResponseBytesConsensusService + "_%s", Name: "received", Algo: collectorapi.Incremental},
			{ID: metricPxServerRequestBytesConsensusService + "_%s", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
)

var (
	tserverServiceOpsChartTmpl = collectorapi.Chart{
		ID:       "tserver_%s_%s_operations",
		Title:    "TServer %s Operations",
		Units:    "ops/s",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_operations",
		Priority: prioServiceOps,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	tserverServiceOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "tserver_%s_%s_operation_latency",
		Title:    "TServer %s Operation Latency",
		Units:    "microseconds",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_operations_latency",
		Priority: prioServiceOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
	tserverServiceTrafficChartTmpl = collectorapi.Chart{
		ID:       "tserver_%s_%s_traffic",
		Title:    "TServer %s Traffic",
		Units:    "bytes/s",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_traffic",
		Priority: prioServiceTraffic,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: metricPxTserverResponseBytes + "_%s_%s", Name: "received", Algo: collectorapi.Incremental},
			{ID: metricPxTserverRequestBytes + "_%s_%s", Name: "sent", Algo: collectorapi.Incremental},
		},
	}

	tserverConsensusOpsChartTmpl = collectorapi.Chart{
		ID:       "tserver_consensus_%s_operations",
		Title:    "TServer Consensus Operations",
		Units:    "ops/s",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_operations",
		Priority: prioConsensusOps,
		Dims: collectorapi.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_count", Name: "operations", Algo: collectorapi.Incremental},
		},
	}
	tserverConsensusOpLatencyChartTmpl = collectorapi.Chart{
		ID:       "tserver_consensus_%s_operation_latency",
		Title:    "TServer Consensus Operation Latency",
		Units:    "microseconds",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_operations_latency",
		Priority: prioConsensusOpsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
	tserverConsensusTrafficChartTmpl = collectorapi.Chart{
		ID:       "tserver_consensus_%s_traffic",
		Title:    "TServer Consensus Traffic",
		Units:    "bytes/s",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_traffic",
		Priority: prioConsensusTraffic,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: metricPxServerResponseBytesConsensusService + "_%s", Name: "received", Algo: collectorapi.Incremental},
			{ID: metricPxServerRequestBytesConsensusService + "_%s", Name: "sent", Algo: collectorapi.Incremental},
		},
	}
)

var ysqlBaseCharts = collectorapi.Charts{
	ysqlConnectionUsageChart.Copy(),
	ysqlActiveConnectionsChart.Copy(),
	ysqlEstablishedConnectionsChart.Copy(),
	ysqlOverLimitConnectionsChart.Copy(),
}

var (
	ysqlConnectionUsageChart = collectorapi.Chart{
		ID:       "ysql_connections_usage",
		Title:    "YSQL Connections Usage",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_connection_usage",
		Type:     collectorapi.Stacked,
		Priority: prioYSQLConnectionUsage,
		Dims: collectorapi.Dims{
			{ID: "yb_ysqlserver_connection_available", Name: "available"},
			{ID: metricSqlConnTotal, Name: "used"},
		},
	}
	ysqlActiveConnectionsChart = collectorapi.Chart{
		ID:       "ysql_active_connections",
		Title:    "YSQL Active Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_active_connections",
		Priority: prioYSQLActiveConnections,
		Dims: collectorapi.Dims{
			{ID: metricSqlActiveConnTotal, Name: "active"},
		},
	}
	ysqlEstablishedConnectionsChart = collectorapi.Chart{
		ID:       "ysql_established_connections",
		Title:    "YSQL Established Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_established_connections",
		Priority: prioYSQLEstablishedConnections,
		Dims: collectorapi.Dims{
			{ID: metricSqlNewConnTotal, Name: "established"},
		},
	}
	ysqlOverLimitConnectionsChart = collectorapi.Chart{
		ID:       "ysql_over_limit_connections",
		Title:    "YSQL Rejected Over Limit Connections",
		Units:    "rejects/s",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_over_limit_connections",
		Priority: prioYSQLOverLimitConnections,
		Dims: collectorapi.Dims{
			{ID: metricSqlConnOverLimitTotal, Name: "over_limit", Algo: collectorapi.Incremental},
		},
	}

	ysqlStatementsChartTmpl = collectorapi.Chart{
		ID:       "ysql_sql_%s_statements",
		Title:    "YSQL SQL Statements",
		Units:    "statements/s",
		Fam:      "ysql statements",
		Ctx:      "yugabytedb.ysql_sql_statements",
		Priority: prioYSQLStatements,
		Dims: collectorapi.Dims{
			{ID: metricPxYSQLLatencySQLProcessor + "_%s_count", Name: "statements", Algo: collectorapi.Incremental},
		},
	}
	ysqlStatementsLatencyChartTmpl = collectorapi.Chart{
		ID:       "ysql_sql_%s_statements_latency",
		Title:    "YSQL SQL Statements Latency",
		Units:    "microseconds",
		Fam:      "ysql statements",
		Ctx:      "yugabytedb.ysql_sql_statements_latency",
		Priority: prioYSQLStatementsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxYSQLLatencySQLProcessor + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
)

var (
	ycqlStatementsChartTmpl = collectorapi.Chart{
		ID:       "ycql_sql_%s_statements",
		Title:    "YCQL SQL Statements",
		Units:    "statements/s",
		Fam:      "ycql statements",
		Ctx:      "yugabytedb.ycql_sql_statements",
		Priority: prioYCQLStatements,
		Dims: collectorapi.Dims{
			{ID: metricPxYCQLLatencySQLProcessor + "_%s_count", Name: "statements", Algo: collectorapi.Incremental},
		},
	}
	ycqlStatementsLatencyChartTmpl = collectorapi.Chart{
		ID:       "ysql_cql_%s_statements_latency",
		Title:    "YSQL SQL Statements Latency",
		Units:    "microseconds",
		Fam:      "ycql statements",
		Ctx:      "yugabytedb.ycql_sql_statements_latency",
		Priority: prioYCQLStatementsLatency,
		Dims: collectorapi.Dims{
			{ID: metricPxYCQLLatencySQLProcessor + "_%s_sum", Name: "latency", Algo: collectorapi.Incremental},
		},
	}
)

func (c *Collector) addMasterClientOpCharts(op string) {
	charts := collectorapi.Charts{
		masterClientOpsChartTmpl.Copy(),
		masterClientOpLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []collectorapi.Label{
			{Key: "operation", Value: op},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, op)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addMasterDDLOpCharts(op string) {
	charts := collectorapi.Charts{
		masterDdlOpsChartTmpl.Copy(),
		masterDdlOpLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []collectorapi.Label{
			{Key: "operation", Value: op},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, op)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addConsensusServiceOpCharts(op string) {
	var charts collectorapi.Charts
	if c.srvType == srvTypeMaster {
		charts = collectorapi.Charts{
			masterConsensusOpsChartTmpl.Copy(),
			masterConsensusOpLatencyChartTmpl.Copy(),
			masterConsensusTrafficChartTmpl.Copy(),
		}
	} else {
		charts = collectorapi.Charts{
			tserverConsensusOpsChartTmpl.Copy(),
			tserverConsensusOpLatencyChartTmpl.Copy(),
			tserverConsensusTrafficChartTmpl.Copy(),
		}
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []collectorapi.Label{
			{Key: "operation", Value: op},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, op)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addServiceOpCharts(svc, op string) {
	var charts collectorapi.Charts
	if c.srvType == srvTypeMaster {
		charts = collectorapi.Charts{
			masterServiceOpsChartTmpl.Copy(),
			masterServiceOpLatencyChartTmpl.Copy(),
			masterServiceTrafficChartTmpl.Copy(),
		}
	} else {
		charts = collectorapi.Charts{
			tserverServiceOpsChartTmpl.Copy(),
			tserverServiceOpLatencyChartTmpl.Copy(),
			tserverServiceTrafficChartTmpl.Copy(),
		}
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, svc, op)
		chart.Title = fmt.Sprintf(chart.Title, svc)
		chart.Ctx = fmt.Sprintf(chart.Ctx, strings.ToLower(svc))
		chart.Labels = []collectorapi.Label{
			{Key: "operation", Value: op},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, svc, op)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addCQLStatementCharts(stmt string) {
	charts := collectorapi.Charts{
		ycqlStatementsChartTmpl.Copy(),
		ycqlStatementsLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, stmt)
		chart.Labels = []collectorapi.Label{
			{Key: "statement", Value: stmt},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, stmt)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addSQLStatementCharts(stmt string) {
	charts := collectorapi.Charts{
		ysqlStatementsChartTmpl.Copy(),
		ysqlStatementsLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, stmt)
		chart.Labels = []collectorapi.Label{
			{Key: "statement", Value: stmt},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, stmt)
		}
		if err := c.Charts().Add(chart.Copy()); err != nil {
			c.Warning(err)
		}
	}
}

func (c *Collector) addBaseCharts() {
	switch c.srvType {
	case srvTypeMaster:
	case srvTypeTServer:
	case srvTypeCQL:
	case srvTypeSQL:
		if err := c.Charts().Add(*ysqlBaseCharts.Copy()...); err != nil {
			c.Warning(err)
		}
	}
}
