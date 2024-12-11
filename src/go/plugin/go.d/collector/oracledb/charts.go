// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSessionsCount = module.Priority + iota
	prioActiveSessionsCount
	prioSessionsUtilization

	prioCurrentLogons
	prioLogonsRate

	prioTablespaceUtilization
	prioTablespaceUsage

	prioDatabaseWaitTimeRatio
	prioSqlServiceResponseTime
	prioWaitClassWaitTime
	prioEnqueueTimeouts

	prioActivities

	prioDiskIO
	prioDiskIOPS

	prioDiskSorts

	prioTableScans

	prioCacheHitRatio
	prioGlobalCacheBlocks
)

var globalCharts = module.Charts{
	sessionsCountChart.Copy(),
	averageActiveSessionsCountChart.Copy(),
	sessionsUtilizationChart.Copy(),

	currentLogonsChart.Copy(),
	logonsRateChart.Copy(),

	databaseWaitTimeRatioChart.Copy(),
	sqlServiceResponseTimeChart.Copy(),
	enqueueTimeoutsChart.Copy(),

	activityChart.Copy(),

	diskIOChart.Copy(),
	diskIOPSChart.Copy(),

	sortsChart.Copy(),

	tableScansChart.Copy(),

	cacheHitRatioChart.Copy(),
	globalCacheBlocksChart.Copy(),
}

var (
	sessionsCountChart = module.Chart{
		ID:       "sessions",
		Title:    "Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "oracledb.sessions",
		Type:     module.Line,
		Priority: prioSessionsCount,
		Dims: module.Dims{
			{ID: "Session Count", Name: "sessions", Div: precision},
		},
	}
	averageActiveSessionsCountChart = module.Chart{
		ID:       "average_active_sessions",
		Title:    "Average Active Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "oracledb.average_active_sessions",
		Type:     module.Line,
		Priority: prioActiveSessionsCount,
		Dims: module.Dims{
			{ID: "Average Active Sessions", Name: "active", Div: precision},
		},
	}
	sessionsUtilizationChart = module.Chart{
		ID:       "sessions_utilization",
		Title:    "Sessions Limit %",
		Units:    "percent",
		Fam:      "sessions",
		Ctx:      "oracledb.sessions_utilization",
		Type:     module.Area,
		Priority: prioSessionsUtilization,
		Dims: module.Dims{
			{ID: "Session Limit %", Name: "session_limit", Div: precision},
		},
	}

	currentLogonsChart = module.Chart{
		ID:       "current_logons",
		Title:    "Current Logons",
		Units:    "logons",
		Fam:      "logons",
		Ctx:      "oracledb.current_logons",
		Type:     module.Line,
		Priority: prioCurrentLogons,
		Dims: module.Dims{
			{ID: "logons current", Name: "logons"},
		},
	}
	logonsRateChart = module.Chart{
		ID:       "logons_rate",
		Title:    "Logons",
		Units:    "logons/s",
		Fam:      "logons",
		Ctx:      "oracledb.logons",
		Type:     module.Line,
		Priority: prioLogonsRate,
		Dims: module.Dims{
			{ID: "logons cumulative", Name: "logons", Algo: module.Incremental},
		},
	}

	databaseWaitTimeRatioChart = module.Chart{
		ID:       "database_wait_time_ratio",
		Title:    "Database Wait Time Ratio",
		Units:    "percent",
		Fam:      "performance",
		Ctx:      "oracledb.database_wait_time_ratio",
		Type:     module.Area,
		Priority: prioDatabaseWaitTimeRatio,
		Dims: module.Dims{
			{ID: "Database Wait Time Ratio", Name: "db_wait_time", Div: precision},
		},
	}
	sqlServiceResponseTimeChart = module.Chart{
		ID:       "sql_service_response_time",
		Title:    "SQL Service Response Time",
		Units:    "seconds",
		Fam:      "performance",
		Ctx:      "oracledb.sql_service_response_time",
		Type:     module.Line,
		Priority: prioSqlServiceResponseTime,
		Dims: module.Dims{
			{ID: "SQL Service Response Time", Name: "sql_resp_time", Div: precision * 100},
		},
	}
	enqueueTimeoutsChart = module.Chart{
		ID:       "enqueue_timeouts",
		Title:    "Enqueue Timeouts",
		Units:    "timeouts/s",
		Fam:      "performance",
		Ctx:      "oracledb.enqueue_timeouts",
		Type:     module.Line,
		Priority: prioEnqueueTimeouts,
		Dims: module.Dims{
			{ID: "enqueue timeouts", Name: "enqueue", Algo: module.Incremental},
		},
	}

	diskIOChart = module.Chart{
		ID:       "disk_io",
		Title:    "Disk IO",
		Units:    "bytes/s",
		Fam:      "disk",
		Ctx:      "oracledb.disk_io",
		Type:     module.Area,
		Priority: prioDiskIO,
		Dims: module.Dims{
			{ID: "physical read bytes", Name: "read", Algo: module.Incremental},
			{ID: "physical write bytes", Name: "written", Mul: -1, Algo: module.Incremental},
		},
	}

	diskIOPSChart = module.Chart{
		ID:       "disk_physical_iops",
		Title:    "Disk IOPS",
		Units:    "operations/s",
		Fam:      "disk",
		Ctx:      "oracledb.disk_iops",
		Type:     module.Line,
		Priority: prioDiskIOPS,
		Dims: module.Dims{
			{ID: "physical reads", Name: "read", Algo: module.Incremental},
			{ID: "physical writes", Name: "write", Mul: -1, Algo: module.Incremental},
		},
	}

	sortsChart = module.Chart{
		ID:       "sorts",
		Title:    "Sorts",
		Units:    "sorts/s",
		Fam:      "sorts",
		Ctx:      "oracledb.sorts",
		Type:     module.Line,
		Priority: prioDiskSorts,
		Dims: module.Dims{
			{ID: "sorts (memory)", Name: "memory", Algo: module.Incremental},
			{ID: "sorts (disk)", Name: "disk", Algo: module.Incremental},
		},
	}

	tableScansChart = module.Chart{
		ID:       "table_scans",
		Title:    "Table Scans",
		Units:    "scans/s",
		Fam:      "table scans",
		Ctx:      "oracledb.table_scans",
		Type:     module.Line,
		Priority: prioTableScans,
		Dims: module.Dims{
			{ID: "table scans (short tables)", Name: "short_table", Algo: module.Incremental},
			{ID: "table scans (long tables)", Name: "long_table", Algo: module.Incremental},
		},
	}

	cacheHitRatioChart = module.Chart{
		ID:       "cache_hit_ratio",
		Title:    "Cache Hit Ratio",
		Units:    "percent",
		Fam:      "cache",
		Ctx:      "oracledb.cache_hit_ratio",
		Type:     module.Line,
		Priority: prioCacheHitRatio,
		Dims: module.Dims{
			{ID: "Buffer Cache Hit Ratio", Name: "buffer", Div: precision},
			{ID: "Cursor Cache Hit Ratio", Name: "cursor", Div: precision},
			{ID: "Library Cache Hit Ratio", Name: "library", Div: precision},
			{ID: "Row Cache Hit Ratio", Name: "row", Div: precision},
		},
	}
	globalCacheBlocksChart = module.Chart{
		ID:       "global_cache_blocks",
		Title:    "Global Cache Blocks",
		Units:    "blocks/s",
		Fam:      "cache",
		Ctx:      "oracledb.global_cache_blocks",
		Type:     module.Line,
		Priority: prioGlobalCacheBlocks,
		Dims: module.Dims{
			{ID: "Global Cache Blocks Corrupted", Name: "corrupted", Algo: module.Incremental, Div: precision},
			{ID: "Global Cache Blocks Lost", Name: "lost", Algo: module.Incremental, Div: precision},
		},
	}
)

var (
	activityChart = module.Chart{
		ID:       "activity",
		Title:    "Activities",
		Units:    "events/s",
		Fam:      "activity",
		Ctx:      "oracledb.activity",
		Type:     module.Line,
		Priority: prioActivities,
		Dims: module.Dims{
			{ID: "parse count (total)", Name: "parse", Algo: module.Incremental},
			{ID: "execute count", Name: "execute", Algo: module.Incremental},
			{ID: "user commits", Name: "user_commits", Algo: module.Incremental},
			{ID: "user rollbacks", Name: "user_rollbacks", Algo: module.Incremental},
		},
	}
)

var waitClassChartsTmpl = module.Charts{
	waitClassWaitTimeChartTmpl.Copy(),
}

var (
	waitClassWaitTimeChartTmpl = module.Chart{
		ID:       "wait_class_%s_wait_time",
		Title:    "Wait Class Wait Time",
		Units:    "milliseconds",
		Fam:      "performance",
		Ctx:      "oracledb.wait_class_wait_time",
		Type:     module.Line,
		Priority: prioWaitClassWaitTime,
		Dims: module.Dims{
			{ID: "wait_class_%s_wait_time", Name: "wait_time", Div: precision},
		},
	}
)

var tablespaceChartsTmpl = module.Charts{
	tablespaceUtilizationChartTmpl.Copy(),
	tablespaceUsageChartTmpl.Copy(),
}

var (
	tablespaceUtilizationChartTmpl = module.Chart{
		ID:       "tablespace_%s_utilization",
		Title:    "Tablespace Utilization",
		Units:    "percent",
		Fam:      "tablespace",
		Ctx:      "oracledb.tablespace_utilization",
		Type:     module.Area,
		Priority: prioTablespaceUtilization,
		Dims: module.Dims{
			{ID: "tablespace_%s_utilization", Name: "utilization", Div: precision},
		},
	}
	tablespaceUsageChartTmpl = module.Chart{
		ID:       "tablespace_%s_usage",
		Title:    "Tablespace Usage",
		Units:    "bytes",
		Fam:      "tablespace",
		Ctx:      "oracledb.tablespace_usage",
		Type:     module.Stacked,
		Priority: prioTablespaceUsage,
		Dims: module.Dims{
			{ID: "tablespace_%s_avail_bytes", Name: "avail"},
			{ID: "tablespace_%s_used_bytes", Name: "used"},
		},
	}
)

func (c *Collector) addTablespaceCharts(tablespace string) {
	charts := tablespaceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, tablespace))
		chart.Labels = []module.Label{
			{Key: "tablespace", Value: tablespace},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, tablespace)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add tablespace '%s' charts: %v", tablespace, err)
	}
}

func (c *Collector) removeTablespaceChart(tablespace string) {}

func (c *Collector) addWaitClassCharts(waitClass string) {
	charts := waitClassChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, waitClass))
		chart.Labels = []module.Label{
			{Key: "wait_class", Value: waitClass},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, waitClass)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add wait class '%s' charts: %v", waitClass, err)
	}
}

func cleanChartId(id string) string {
	r := strings.NewReplacer(" ", "_", ".", "_")
	return r.Replace(id)
}
