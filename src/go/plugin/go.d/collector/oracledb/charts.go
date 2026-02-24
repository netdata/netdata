// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioSessionsCount = collectorapi.Priority + iota
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

var globalCharts = collectorapi.Charts{
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
	sessionsCountChart = collectorapi.Chart{
		ID:       "sessions",
		Title:    "Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "oracledb.sessions",
		Type:     collectorapi.Line,
		Priority: prioSessionsCount,
		Dims: collectorapi.Dims{
			{ID: "Session Count", Name: "sessions", Div: precision},
		},
	}
	averageActiveSessionsCountChart = collectorapi.Chart{
		ID:       "average_active_sessions",
		Title:    "Average Active Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "oracledb.average_active_sessions",
		Type:     collectorapi.Line,
		Priority: prioActiveSessionsCount,
		Dims: collectorapi.Dims{
			{ID: "Average Active Sessions", Name: "active", Div: precision},
		},
	}
	sessionsUtilizationChart = collectorapi.Chart{
		ID:       "sessions_utilization",
		Title:    "Sessions Limit %",
		Units:    "percent",
		Fam:      "sessions",
		Ctx:      "oracledb.sessions_utilization",
		Type:     collectorapi.Area,
		Priority: prioSessionsUtilization,
		Dims: collectorapi.Dims{
			{ID: "Session Limit %", Name: "session_limit", Div: precision},
		},
	}

	currentLogonsChart = collectorapi.Chart{
		ID:       "current_logons",
		Title:    "Current Logons",
		Units:    "logons",
		Fam:      "logons",
		Ctx:      "oracledb.current_logons",
		Type:     collectorapi.Line,
		Priority: prioCurrentLogons,
		Dims: collectorapi.Dims{
			{ID: "logons current", Name: "logons"},
		},
	}
	logonsRateChart = collectorapi.Chart{
		ID:       "logons_rate",
		Title:    "Logons",
		Units:    "logons/s",
		Fam:      "logons",
		Ctx:      "oracledb.logons",
		Type:     collectorapi.Line,
		Priority: prioLogonsRate,
		Dims: collectorapi.Dims{
			{ID: "logons cumulative", Name: "logons", Algo: collectorapi.Incremental},
		},
	}

	databaseWaitTimeRatioChart = collectorapi.Chart{
		ID:       "database_wait_time_ratio",
		Title:    "Database Wait Time Ratio",
		Units:    "percent",
		Fam:      "performance",
		Ctx:      "oracledb.database_wait_time_ratio",
		Type:     collectorapi.Area,
		Priority: prioDatabaseWaitTimeRatio,
		Dims: collectorapi.Dims{
			{ID: "Database Wait Time Ratio", Name: "db_wait_time", Div: precision},
		},
	}
	sqlServiceResponseTimeChart = collectorapi.Chart{
		ID:       "sql_service_response_time",
		Title:    "SQL Service Response Time",
		Units:    "seconds",
		Fam:      "performance",
		Ctx:      "oracledb.sql_service_response_time",
		Type:     collectorapi.Line,
		Priority: prioSqlServiceResponseTime,
		Dims: collectorapi.Dims{
			{ID: "SQL Service Response Time", Name: "sql_resp_time", Div: precision * 100},
		},
	}
	enqueueTimeoutsChart = collectorapi.Chart{
		ID:       "enqueue_timeouts",
		Title:    "Enqueue Timeouts",
		Units:    "timeouts/s",
		Fam:      "performance",
		Ctx:      "oracledb.enqueue_timeouts",
		Type:     collectorapi.Line,
		Priority: prioEnqueueTimeouts,
		Dims: collectorapi.Dims{
			{ID: "enqueue timeouts", Name: "enqueue", Algo: collectorapi.Incremental},
		},
	}

	diskIOChart = collectorapi.Chart{
		ID:       "disk_io",
		Title:    "Disk IO",
		Units:    "bytes/s",
		Fam:      "disk",
		Ctx:      "oracledb.disk_io",
		Type:     collectorapi.Area,
		Priority: prioDiskIO,
		Dims: collectorapi.Dims{
			{ID: "physical read bytes", Name: "read", Algo: collectorapi.Incremental},
			{ID: "physical write bytes", Name: "written", Mul: -1, Algo: collectorapi.Incremental},
		},
	}

	diskIOPSChart = collectorapi.Chart{
		ID:       "disk_physical_iops",
		Title:    "Disk IOPS",
		Units:    "operations/s",
		Fam:      "disk",
		Ctx:      "oracledb.disk_iops",
		Type:     collectorapi.Line,
		Priority: prioDiskIOPS,
		Dims: collectorapi.Dims{
			{ID: "physical reads", Name: "read", Algo: collectorapi.Incremental},
			{ID: "physical writes", Name: "write", Mul: -1, Algo: collectorapi.Incremental},
		},
	}

	sortsChart = collectorapi.Chart{
		ID:       "sorts",
		Title:    "Sorts",
		Units:    "sorts/s",
		Fam:      "sorts",
		Ctx:      "oracledb.sorts",
		Type:     collectorapi.Line,
		Priority: prioDiskSorts,
		Dims: collectorapi.Dims{
			{ID: "sorts (memory)", Name: "memory", Algo: collectorapi.Incremental},
			{ID: "sorts (disk)", Name: "disk", Algo: collectorapi.Incremental},
		},
	}

	tableScansChart = collectorapi.Chart{
		ID:       "table_scans",
		Title:    "Table Scans",
		Units:    "scans/s",
		Fam:      "table scans",
		Ctx:      "oracledb.table_scans",
		Type:     collectorapi.Line,
		Priority: prioTableScans,
		Dims: collectorapi.Dims{
			{ID: "table scans (short tables)", Name: "short_table", Algo: collectorapi.Incremental},
			{ID: "table scans (long tables)", Name: "long_table", Algo: collectorapi.Incremental},
		},
	}

	cacheHitRatioChart = collectorapi.Chart{
		ID:       "cache_hit_ratio",
		Title:    "Cache Hit Ratio",
		Units:    "percent",
		Fam:      "cache",
		Ctx:      "oracledb.cache_hit_ratio",
		Type:     collectorapi.Line,
		Priority: prioCacheHitRatio,
		Dims: collectorapi.Dims{
			{ID: "Buffer Cache Hit Ratio", Name: "buffer", Div: precision},
			{ID: "Cursor Cache Hit Ratio", Name: "cursor", Div: precision},
			{ID: "Library Cache Hit Ratio", Name: "library", Div: precision},
			{ID: "Row Cache Hit Ratio", Name: "row", Div: precision},
		},
	}
	globalCacheBlocksChart = collectorapi.Chart{
		ID:       "global_cache_blocks",
		Title:    "Global Cache Blocks",
		Units:    "blocks/s",
		Fam:      "cache",
		Ctx:      "oracledb.global_cache_blocks",
		Type:     collectorapi.Line,
		Priority: prioGlobalCacheBlocks,
		Dims: collectorapi.Dims{
			{ID: "Global Cache Blocks Corrupted", Name: "corrupted", Algo: collectorapi.Incremental, Div: precision},
			{ID: "Global Cache Blocks Lost", Name: "lost", Algo: collectorapi.Incremental, Div: precision},
		},
	}
)

var (
	activityChart = collectorapi.Chart{
		ID:       "activity",
		Title:    "Activities",
		Units:    "events/s",
		Fam:      "activity",
		Ctx:      "oracledb.activity",
		Type:     collectorapi.Line,
		Priority: prioActivities,
		Dims: collectorapi.Dims{
			{ID: "parse count (total)", Name: "parse", Algo: collectorapi.Incremental},
			{ID: "execute count", Name: "execute", Algo: collectorapi.Incremental},
			{ID: "user commits", Name: "user_commits", Algo: collectorapi.Incremental},
			{ID: "user rollbacks", Name: "user_rollbacks", Algo: collectorapi.Incremental},
		},
	}
)

var waitClassChartsTmpl = collectorapi.Charts{
	waitClassWaitTimeChartTmpl.Copy(),
}

var (
	waitClassWaitTimeChartTmpl = collectorapi.Chart{
		ID:       "wait_class_%s_wait_time",
		Title:    "Wait Class Wait Time",
		Units:    "milliseconds",
		Fam:      "performance",
		Ctx:      "oracledb.wait_class_wait_time",
		Type:     collectorapi.Line,
		Priority: prioWaitClassWaitTime,
		Dims: collectorapi.Dims{
			{ID: "wait_class_%s_wait_time", Name: "wait_time", Div: precision},
		},
	}
)

var tablespaceChartsTmpl = collectorapi.Charts{
	tablespaceUtilizationChartTmpl.Copy(),
	tablespaceUsageChartTmpl.Copy(),
}

var (
	tablespaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "tablespace_%s_utilization",
		Title:    "Tablespace Utilization",
		Units:    "percent",
		Fam:      "tablespace",
		Ctx:      "oracledb.tablespace_utilization",
		Type:     collectorapi.Area,
		Priority: prioTablespaceUtilization,
		Dims: collectorapi.Dims{
			{ID: "tablespace_%s_utilization", Name: "utilization", Div: precision},
		},
	}
	tablespaceUsageChartTmpl = collectorapi.Chart{
		ID:       "tablespace_%s_usage",
		Title:    "Tablespace Usage",
		Units:    "bytes",
		Fam:      "tablespace",
		Ctx:      "oracledb.tablespace_usage",
		Type:     collectorapi.Stacked,
		Priority: prioTablespaceUsage,
		Dims: collectorapi.Dims{
			{ID: "tablespace_%s_avail_bytes", Name: "avail"},
			{ID: "tablespace_%s_used_bytes", Name: "used"},
		},
	}
)

func (c *Collector) addTablespaceCharts(ts tablespaceInfo) {
	charts := tablespaceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, ts.name))
		chart.Labels = []collectorapi.Label{
			{Key: "tablespace", Value: ts.name},
			{Key: "autoextend_status", Value: ts.autoExtent},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ts.name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add tablespace '%s' charts: %v", ts.name, err)
	}
}

func (c *Collector) removeTablespaceChart(tablespace string) {}

func (c *Collector) addWaitClassCharts(waitClass string) {
	charts := waitClassChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = cleanChartId(fmt.Sprintf(chart.ID, waitClass))
		chart.Labels = []collectorapi.Label{
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
