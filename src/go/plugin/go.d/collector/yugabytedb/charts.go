// SPDX-License-Identifier: GPL-3.0-or-later

package yugabytedb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioMasterClientOps = module.Priority + iota
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
	masterClientOpsChartTmpl = module.Chart{
		ID:       "master_client_%s_operations",
		Title:    "Master Client Operations",
		Units:    "ops/s",
		Fam:      "master client ops",
		Ctx:      "yugabytedb.master_client_operations",
		Priority: prioMasterClientOps,
		Dims: module.Dims{
			{ID: metricPxMasterLatencyMasterClient + "_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	masterClientOpLatencyChartTmpl = module.Chart{
		ID:       "master_client_%s_operations_latency",
		Title:    "Master Client Operation Latency",
		Units:    "microseconds",
		Fam:      "master client ops",
		Ctx:      "yugabytedb.master_client_operations_latency",
		Priority: prioMasterClientOpsLatency,
		Dims: module.Dims{
			{ID: metricPxMasterLatencyMasterClient + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}

	masterDdlOpsChartTmpl = module.Chart{
		ID:       "master_ddl_%s_operations",
		Title:    "Master DDL Operations",
		Units:    "ops/s",
		Fam:      "ddl ops",
		Ctx:      "yugabytedb.master_ddl_operations",
		Priority: prioMasterDdlOps,
		Dims: module.Dims{
			{ID: metricPxMasterLatencyMasterDdl + "_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	masterDdlOpLatencyChartTmpl = module.Chart{
		ID:       "master_ddl_%s_operations_latency",
		Title:    "Master DDL Operations Latency",
		Units:    "microseconds",
		Fam:      "ddl ops",
		Ctx:      "yugabytedb.master_ddl_operations_latency",
		Priority: prioMasterDdlOpsLatency,
		Dims: module.Dims{
			{ID: metricPxMasterLatencyMasterDdl + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}

	masterServiceOpsChartTmpl = module.Chart{
		ID:       "master_%s_%s_operations",
		Title:    "Master %s Operations",
		Units:    "ops/s",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_operations",
		Priority: prioServiceOps,
		Dims: module.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	masterServiceOpLatencyChartTmpl = module.Chart{
		ID:       "master_%s_%s_operation_latency",
		Title:    "Master %s Operation Latency",
		Units:    "microseconds",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_operations_latency",
		Priority: prioServiceOpsLatency,
		Dims: module.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
	masterServiceTrafficChartTmpl = module.Chart{
		ID:       "master_%s_%s_traffic",
		Title:    "Master %s Traffic",
		Units:    "bytes/s",
		Fam:      "master svc operations",
		Ctx:      "yugabytedb.master_%s_traffic",
		Priority: prioServiceTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: metricPxTserverResponseBytes + "_%s_%s", Name: "received", Algo: module.Incremental},
			{ID: metricPxTserverRequestBytes + "_%s_%s", Name: "sent", Algo: module.Incremental},
		},
	}

	masterConsensusOpsChartTmpl = module.Chart{
		ID:       "master_consensus_%s_operations",
		Title:    "Master Consensus Operations",
		Units:    "ops/s",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_operations",
		Priority: prioConsensusOps,
		Dims: module.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	masterConsensusOpLatencyChartTmpl = module.Chart{
		ID:       "master_consensus_%s_operation_latency",
		Title:    "Master Consensus Operation Latency",
		Units:    "microseconds",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_operations_latency",
		Priority: prioConsensusOpsLatency,
		Dims: module.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
	masterConsensusTrafficChartTmpl = module.Chart{
		ID:       "master_consensus_%s_traffic",
		Title:    "Master Consensus Traffic",
		Units:    "bytes/s",
		Fam:      "master consensus operations",
		Ctx:      "yugabytedb.master_consensus_traffic",
		Priority: prioConsensusTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: metricPxServerResponseBytesConsensusService + "_%s", Name: "received", Algo: module.Incremental},
			{ID: metricPxServerRequestBytesConsensusService + "_%s", Name: "sent", Algo: module.Incremental},
		},
	}
)

var (
	tserverServiceOpsChartTmpl = module.Chart{
		ID:       "tserver_%s_%s_operations",
		Title:    "TServer %s Operations",
		Units:    "ops/s",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_operations",
		Priority: prioServiceOps,
		Dims: module.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	tserverServiceOpLatencyChartTmpl = module.Chart{
		ID:       "tserver_%s_%s_operation_latency",
		Title:    "TServer %s Operation Latency",
		Units:    "microseconds",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_operations_latency",
		Priority: prioServiceOpsLatency,
		Dims: module.Dims{
			{ID: metricPxTserverHandlerLatency + "_%s_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
	tserverServiceTrafficChartTmpl = module.Chart{
		ID:       "tserver_%s_%s_traffic",
		Title:    "TServer %s Traffic",
		Units:    "bytes/s",
		Fam:      "tserver svc operations",
		Ctx:      "yugabytedb.tserver_%s_traffic",
		Priority: prioServiceTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: metricPxTserverResponseBytes + "_%s_%s", Name: "received", Algo: module.Incremental},
			{ID: metricPxTserverRequestBytes + "_%s_%s", Name: "sent", Algo: module.Incremental},
		},
	}

	tserverConsensusOpsChartTmpl = module.Chart{
		ID:       "tserver_consensus_%s_operations",
		Title:    "TServer Consensus Operations",
		Units:    "ops/s",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_operations",
		Priority: prioConsensusOps,
		Dims: module.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_count", Name: "operations", Algo: module.Incremental},
		},
	}
	tserverConsensusOpLatencyChartTmpl = module.Chart{
		ID:       "tserver_consensus_%s_operation_latency",
		Title:    "TServer Consensus Operation Latency",
		Units:    "microseconds",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_operations_latency",
		Priority: prioConsensusOpsLatency,
		Dims: module.Dims{
			{ID: metricPxServerLatencyConsensusService + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
	tserverConsensusTrafficChartTmpl = module.Chart{
		ID:       "tserver_consensus_%s_traffic",
		Title:    "TServer Consensus Traffic",
		Units:    "bytes/s",
		Fam:      "tserver consensus operations",
		Ctx:      "yugabytedb.tserver_consensus_traffic",
		Priority: prioConsensusTraffic,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: metricPxServerResponseBytesConsensusService + "_%s", Name: "received", Algo: module.Incremental},
			{ID: metricPxServerRequestBytesConsensusService + "_%s", Name: "sent", Algo: module.Incremental},
		},
	}
)

var ysqlBaseCharts = module.Charts{
	ysqlConnectionUsageChart.Copy(),
	ysqlActiveConnectionsChart.Copy(),
	ysqlEstablishedConnectionsChart.Copy(),
	ysqlOverLimitConnectionsChart.Copy(),
}

var (
	ysqlConnectionUsageChart = module.Chart{
		ID:       "ysql_connections_usage",
		Title:    "YSQL Connections Usage",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_connection_usage",
		Type:     module.Stacked,
		Priority: prioYSQLConnectionUsage,
		Dims: module.Dims{
			{ID: "yb_ysqlserver_connection_available", Name: "available"},
			{ID: metricSqlConnTotal, Name: "used"},
		},
	}
	ysqlActiveConnectionsChart = module.Chart{
		ID:       "ysql_active_connections",
		Title:    "YSQL Active Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_active_connections",
		Priority: prioYSQLActiveConnections,
		Dims: module.Dims{
			{ID: metricSqlActiveConnTotal, Name: "active"},
		},
	}
	ysqlEstablishedConnectionsChart = module.Chart{
		ID:       "ysql_established_connections",
		Title:    "YSQL Established Connections",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_established_connections",
		Priority: prioYSQLEstablishedConnections,
		Dims: module.Dims{
			{ID: metricSqlNewConnTotal, Name: "established"},
		},
	}
	ysqlOverLimitConnectionsChart = module.Chart{
		ID:       "ysql_over_limit_connections",
		Title:    "YSQL Rejected Over Limit Connections",
		Units:    "rejects/s",
		Fam:      "connections",
		Ctx:      "yugabytedb.ysql_over_limit_connections",
		Priority: prioYSQLOverLimitConnections,
		Dims: module.Dims{
			{ID: metricSqlConnOverLimitTotal, Name: "over_limit", Algo: module.Incremental},
		},
	}

	ysqlStatementsChartTmpl = module.Chart{
		ID:       "ysql_sql_%s_statements",
		Title:    "YSQL SQL Statements",
		Units:    "statements/s",
		Fam:      "ysql statements",
		Ctx:      "yugabytedb.ysql_sql_statements",
		Priority: prioYSQLStatements,
		Dims: module.Dims{
			{ID: metricPxYSQLLatencySQLProcessor + "_%s_count", Name: "statements", Algo: module.Incremental},
		},
	}
	ysqlStatementsLatencyChartTmpl = module.Chart{
		ID:       "ysql_sql_%s_statements_latency",
		Title:    "YSQL SQL Statements Latency",
		Units:    "microseconds",
		Fam:      "ysql statements",
		Ctx:      "yugabytedb.ysql_sql_statements_latency",
		Priority: prioYSQLStatementsLatency,
		Dims: module.Dims{
			{ID: metricPxYSQLLatencySQLProcessor + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
)

var (
	ycqlStatementsChartTmpl = module.Chart{
		ID:       "ycql_sql_%s_statements",
		Title:    "YCQL SQL Statements",
		Units:    "statements/s",
		Fam:      "ycql statements",
		Ctx:      "yugabytedb.ycql_sql_statements",
		Priority: prioYCQLStatements,
		Dims: module.Dims{
			{ID: metricPxYCQLLatencySQLProcessor + "_%s_count", Name: "statements", Algo: module.Incremental},
		},
	}
	ycqlStatementsLatencyChartTmpl = module.Chart{
		ID:       "ysql_cql_%s_statements_latency",
		Title:    "YSQL SQL Statements Latency",
		Units:    "microseconds",
		Fam:      "ycql statements",
		Ctx:      "yugabytedb.ycql_sql_statements_latency",
		Priority: prioYCQLStatementsLatency,
		Dims: module.Dims{
			{ID: metricPxYCQLLatencySQLProcessor + "_%s_sum", Name: "latency", Algo: module.Incremental},
		},
	}
)

func (c *Collector) addMasterClientOpCharts(op string) {
	charts := module.Charts{
		masterClientOpsChartTmpl.Copy(),
		masterClientOpLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []module.Label{
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
	charts := module.Charts{
		masterDdlOpsChartTmpl.Copy(),
		masterDdlOpLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []module.Label{
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
	var charts module.Charts
	if c.srvType == srvTypeMaster {
		charts = module.Charts{
			masterConsensusOpsChartTmpl.Copy(),
			masterConsensusOpLatencyChartTmpl.Copy(),
			masterConsensusTrafficChartTmpl.Copy(),
		}
	} else {
		charts = module.Charts{
			tserverConsensusOpsChartTmpl.Copy(),
			tserverConsensusOpLatencyChartTmpl.Copy(),
			tserverConsensusTrafficChartTmpl.Copy(),
		}
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, op)
		chart.Labels = []module.Label{
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
	var charts module.Charts
	if c.srvType == srvTypeMaster {
		charts = module.Charts{
			masterServiceOpsChartTmpl.Copy(),
			masterServiceOpLatencyChartTmpl.Copy(),
			masterServiceTrafficChartTmpl.Copy(),
		}
	} else {
		charts = module.Charts{
			tserverServiceOpsChartTmpl.Copy(),
			tserverServiceOpLatencyChartTmpl.Copy(),
			tserverServiceTrafficChartTmpl.Copy(),
		}
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, svc, op)
		chart.Title = fmt.Sprintf(chart.Title, svc)
		chart.Ctx = fmt.Sprintf(chart.Ctx, strings.ToLower(svc))
		chart.Labels = []module.Label{
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
	charts := module.Charts{
		ycqlStatementsChartTmpl.Copy(),
		ycqlStatementsLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, stmt)
		chart.Labels = []module.Label{
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
	charts := module.Charts{
		ysqlStatementsChartTmpl.Copy(),
		ysqlStatementsLatencyChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, stmt)
		chart.Labels = []module.Label{
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
