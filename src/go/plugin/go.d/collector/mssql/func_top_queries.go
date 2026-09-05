// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID      = "top-queries"
	topQueriesMaxTextLength = 4096
	topQueriesParamSort     = "__sort"
	queryStoreNotEnabled    = "Query Store is not enabled on any user database"
	topQueriesNoSource      = "top-queries found no usable query statistics source: this server exposes neither Query Store nor a plan cache with per-query hashes (sys.dm_exec_query_stats.query_hash, SQL Server 2008 and later)"

	topQueriesHelpQueryStore = "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII)."
	topQueriesHelpPlanCache  = "Top SQL queries from the plan cache (sys.dm_exec_query_stats), used because Query Store is unavailable or disabled on this server. Statistics cover only plans that are cached right now and are lost on restart, recompilation or cache eviction, so they are not query history. WARNING: Query text may contain unmasked literals (potential PII)."
)

// topQueriesSource identifies the server-side statistics store backing a response.
type topQueriesSource string

const (
	topQueriesSourceQueryStore topQueriesSource = "query-store"
	topQueriesSourcePlanCache  topQueriesSource = "plan-cache"
)

// errQueryStoreNotEnabled marks a configuration state, not a failure: the server supports
// Query Store but no user database has it turned on. It makes source resolution fall
// through to the plan cache instead of reporting the function unavailable.
var errQueryStoreNotEnabled = errors.New(queryStoreNotEnabled)

// errTopQueriesNoSource means neither statistics source can answer, which is the only
// remaining terminal unavailable state for top-queries.
var errTopQueriesNoSource = errors.New(topQueriesNoSource)

func topQueriesFunctionConfig() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top SQL queries from Query Store, or from the plan cache when Query Store is unavailable. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
}

// topQueriesColumn embeds funcapi.ColumnMeta and adds MSSQL-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta
	DBColumn       string // Column name in sys.query_store_runtime_stats
	CacheColumn    string // Column name in sys.dm_exec_query_stats; empty when the plan cache has no equivalent
	IsMicroseconds bool   // Needs microseconds to milliseconds conversion
	sortOpt        bool   // Show in sort dropdown
	sortLbl        string // Label for sort option
	defaultSort    bool   // Is this the default sort option
	IsIdentity     bool   // Is this an identity column (query_hash, query_text, etc.)
	NeedsAvg       bool   // Needs weighted average calculation (avg_* columns)
}

// topQueriesColumns defines ALL possible columns, keyed to both statistics sources.
// A column is offered only when the active source exposes its backing column, which is
// probed at runtime, so version and service-pack differences resolve themselves.
// Query Store-only columns (no CacheColumn): every stdev_*, memory grant, log bytes and
// tempdb metric. The plan cache has no standard deviation, and its memory-grant columns
// measure a different quantity in different units than Query Store reports.
var topQueriesColumns = []topQueriesColumn{
	// Identity columns - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Tooltip: "Query Hash", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true}, DBColumn: "query_hash", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true}, DBColumn: "query_sql_text", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect}, DBColumn: "database_name", IsIdentity: true},

	// Execution count - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "count_executions", CacheColumn: "execution_count", sortOpt: true, sortLbl: "Top queries by Number of Calls"},

	// Duration metrics (microseconds -> milliseconds) - SQL 2016+
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", CacheColumn: "total_elapsed_time", IsMicroseconds: true, sortOpt: true, sortLbl: "Top queries by Total Execution Time", defaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", CacheColumn: "total_elapsed_time", IsMicroseconds: true, NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastTime", Tooltip: "Last Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_duration", CacheColumn: "last_elapsed_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_duration", CacheColumn: "min_elapsed_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_duration", CacheColumn: "max_elapsed_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevTime", Tooltip: "StdDev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_duration", IsMicroseconds: true},

	// CPU time metrics (microseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgCpu", Tooltip: "Avg CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_cpu_time", CacheColumn: "total_worker_time", IsMicroseconds: true, NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average CPU Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastCpu", Tooltip: "Last CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_cpu_time", CacheColumn: "last_worker_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minCpu", Tooltip: "Min CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_cpu_time", CacheColumn: "min_worker_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxCpu", Tooltip: "Max CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_cpu_time", CacheColumn: "max_worker_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevCpu", Tooltip: "StdDev CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_cpu_time", IsMicroseconds: true},

	// Logical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgReads", Tooltip: "Avg Logical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_reads", CacheColumn: "total_logical_reads", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Logical Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastReads", Tooltip: "Last Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_reads", CacheColumn: "last_logical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minReads", Tooltip: "Min Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_reads", CacheColumn: "min_logical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxReads", Tooltip: "Max Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_reads", CacheColumn: "max_logical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevReads", Tooltip: "StdDev Logical Reads", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_reads"},

	// Logical I/O writes
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgWrites", Tooltip: "Avg Logical Writes", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_writes", CacheColumn: "total_logical_writes", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Logical Writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastWrites", Tooltip: "Last Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_writes", CacheColumn: "last_logical_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minWrites", Tooltip: "Min Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_writes", CacheColumn: "min_logical_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxWrites", Tooltip: "Max Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_writes", CacheColumn: "max_logical_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevWrites", Tooltip: "StdDev Logical Writes", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_writes"},

	// Physical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgPhysReads", Tooltip: "Avg Physical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_physical_io_reads", CacheColumn: "total_physical_reads", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Physical Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastPhysReads", Tooltip: "Last Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_physical_io_reads", CacheColumn: "last_physical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minPhysReads", Tooltip: "Min Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_physical_io_reads", CacheColumn: "min_physical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxPhysReads", Tooltip: "Max Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_physical_io_reads", CacheColumn: "max_physical_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevPhysReads", Tooltip: "StdDev Physical Reads", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_physical_io_reads"},

	// CLR time (microseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgClr", Tooltip: "Avg CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_clr_time", CacheColumn: "total_clr_time", IsMicroseconds: true, NeedsAvg: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastClr", Tooltip: "Last CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_clr_time", CacheColumn: "last_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minClr", Tooltip: "Min CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_clr_time", CacheColumn: "min_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxClr", Tooltip: "Max CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_clr_time", CacheColumn: "max_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevClr", Tooltip: "StdDev CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_clr_time", IsMicroseconds: true},

	// DOP (degree of parallelism)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgDop", Tooltip: "Avg DOP", Type: funcapi.FieldTypeFloat, DecimalPoints: 1, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_dop", CacheColumn: "total_dop", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Parallelism"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastDop", Tooltip: "Last DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_dop", CacheColumn: "last_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minDop", Tooltip: "Min DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_dop", CacheColumn: "min_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxDop", Tooltip: "Max DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_dop", CacheColumn: "max_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevDop", Tooltip: "StdDev DOP", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_dop"},

	// Memory grant (8KB pages)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgMemory", Tooltip: "Avg Memory (8KB pages)", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_query_max_used_memory", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Memory Grant"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastMemory", Tooltip: "Last Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minMemory", Tooltip: "Min Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxMemory", Tooltip: "Max Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevMemory", Tooltip: "StdDev Memory", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_query_max_used_memory"},

	// Row count
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgRows", Tooltip: "Avg Rows", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_rowcount", CacheColumn: "total_rows", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Row Count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastRows", Tooltip: "Last Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_rowcount", CacheColumn: "last_rows"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minRows", Tooltip: "Min Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_rowcount", CacheColumn: "min_rows"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxRows", Tooltip: "Max Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_rowcount", CacheColumn: "max_rows"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevRows", Tooltip: "StdDev Rows", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_rowcount"},

	// SQL Server 2017+ log bytes
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgLogBytes", Tooltip: "Avg Log Bytes", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_log_bytes_used", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Log Bytes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastLogBytes", Tooltip: "Last Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minLogBytes", Tooltip: "Min Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxLogBytes", Tooltip: "Max Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevLogBytes", Tooltip: "StdDev Log Bytes", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_log_bytes_used"},

	// SQL Server 2017+ tempdb space
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTempdb", Tooltip: "Avg TempDB (8KB pages)", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_tempdb_space_used", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average TempDB Usage"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastTempdb", Tooltip: "Last TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTempdb", Tooltip: "Min TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTempdb", Tooltip: "Max TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevTempdb", Tooltip: "StdDev TempDB", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_tempdb_space_used"},
}

// funcapi.SortableColumn interface implementation for topQueriesColumn.
func (c topQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c topQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c topQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c topQueriesColumn) ColumnName() string  { return c.Name }
func (c topQueriesColumn) SortColumn() string  { return "" }

type topQueriesChartGroupDef struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var topQueriesChartGroupDefs = []topQueriesChartGroupDef{
	{key: "Calls", title: "Number of Calls", columns: []string{"calls"}, defaultChart: true},
	{key: "Time", title: "Execution Time", columns: []string{"totalTime", "avgTime", "lastTime", "minTime", "maxTime", "stdevTime"}, defaultChart: true},
	{key: "CPU", title: "CPU Time", columns: []string{"avgCpu", "lastCpu", "minCpu", "maxCpu", "stdevCpu"}},
	{key: "LogicalIO", title: "Logical I/O", columns: []string{"avgReads", "lastReads", "minReads", "maxReads", "stdevReads", "avgWrites", "lastWrites", "minWrites", "maxWrites", "stdevWrites"}},
	{key: "PhysicalIO", title: "Physical Reads", columns: []string{"avgPhysReads", "lastPhysReads", "minPhysReads", "maxPhysReads", "stdevPhysReads"}},
	{key: "CLR", title: "CLR Time", columns: []string{"avgClr", "lastClr", "minClr", "maxClr", "stdevClr"}},
	{key: "DOP", title: "Parallelism", columns: []string{"avgDop", "lastDop", "minDop", "maxDop", "stdevDop"}},
	{key: "Memory", title: "Memory Grant", columns: []string{"avgMemory", "lastMemory", "minMemory", "maxMemory", "stdevMemory"}},
	{key: "Rows", title: "Rows", columns: []string{"avgRows", "lastRows", "minRows", "maxRows", "stdevRows"}},
	{key: "LogBytes", title: "Log Bytes", columns: []string{"avgLogBytes", "lastLogBytes", "minLogBytes", "maxLogBytes", "stdevLogBytes"}},
	{key: "TempDB", title: "TempDB Usage", columns: []string{"avgTempdb", "lastTempdb", "minTempdb", "maxTempdb", "stdevTempdb"}},
}

var topQueriesLabelColumnIDs = map[string]bool{
	"database": true,
}

const topQueriesPrimaryLabelID = "database"

// topQueriesRowScanner interface for testing.
type topQueriesRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// funcTopQueries implements funcapi.MethodHandler for MSSQL top-queries.
// All function-related logic is encapsulated here, keeping Collector focused on metrics collection.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case topQueriesMethodID:
		paramsCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
		defer cancel()
		if _, err := f.router.collector.ensureEngineEdition(paramsCtx); err != nil {
			// Handle owns the final 499/500/504 response classification.
			return nil, nil
		}
		return f.methodParams(paramsCtx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	switch method {
	case topQueriesMethodID:
		queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
		defer cancel()
		if _, err := f.router.collector.ensureEngineEdition(queryCtx); err != nil {
			if response := mssqlFunctionContextError(queryCtx, err); response != nil {
				return response
			}
			return funcapi.ErrorResponse(500, "failed to detect SQL engine edition: %v", err)
		}
		return f.collectData(queryCtx, params.Column(topQueriesParamSort))
	default:
		return funcapi.NotFoundResponse(method)
	}
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

func (f *funcTopQueries) methodParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	if f.router.collector.Functions.TopQueries.Disabled {
		return nil, fmt.Errorf("top-queries function disabled in configuration")
	}
	source, availableCols, err := f.resolveTopQueriesSource(ctx)
	if err != nil {
		if errors.Is(err, errTopQueriesNoSource) {
			return nil, err
		}
		// Let Handle classify transient discovery failures as 499/500/504. Job Manager
		// maps MethodParams errors to 503 before the handler can preserve that distinction.
		return nil, nil
	}

	cols := f.buildAvailableColumns(source, availableCols)
	if len(cols) == 0 {
		return nil, errTopQueriesNoSource
	}

	sortParam, _ := f.buildSortParam(cols)
	return []funcapi.ParamConfig{sortParam}, nil
}

func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	if f.router.collector.Functions.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}
	source, availableCols, err := f.resolveTopQueriesSource(ctx)
	if err != nil {
		if response := mssqlFunctionContextError(ctx, err); response != nil {
			return response
		}
		if isDeadlockPermissionError(err) {
			return funcapi.ErrorResponse(403, "%s", f.router.collector.topQueriesPermissionMessage())
		}
		if errors.Is(err, errTopQueriesNoSource) {
			return funcapi.UnavailableResponse(topQueriesNoSource)
		}
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect a query statistics source: %v", err),
		}
	}

	cols := f.buildAvailableColumns(source, availableCols)
	if len(cols) == 0 {
		return funcapi.UnavailableResponse(topQueriesNoSource)
	}

	validatedSortColumn := f.mapAndValidateSortColumn(sortColumn, cols)

	timeWindowDays := f.router.collector.topQueriesTimeWindowDays()
	limit := f.router.collector.topQueriesLimit()
	query := f.buildTopQueriesSQL(source, cols, validatedSortColumn, timeWindowDays, limit)

	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
		if response := mssqlFunctionContextError(ctx, err); response != nil {
			return response
		}
		if isDeadlockPermissionError(err) {
			return funcapi.ErrorResponse(403, "%s", f.router.collector.topQueriesPermissionMessage())
		}
		colIDs := make([]string, len(cols))
		for i, col := range cols {
			colIDs[i] = col.Name
		}
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("query failed: %v (sort: %s, detected cols: %v)", err, validatedSortColumn, colIDs),
		}
	}
	defer rows.Close()

	data, err := f.scanDynamicRows(rows, cols)
	if err != nil {
		if response := mssqlFunctionContextError(ctx, err); response != nil {
			return response
		}
		return &funcapi.FunctionResponse{Status: 500, Message: err.Error()}
	}

	errorStatus, errorDetails := f.router.collector.collectMSSQLErrorDetails(ctx)
	// Plan attribution reads Query Store plan XML, so it has nothing to answer with when
	// the plan cache is the source. Its columns stay empty, as they already do whenever
	// the attribution query fails.
	var planOpsByDB map[string]map[string]mssqlPlanOps
	if source == topQueriesSourceQueryStore {
		planOpsByDB = f.router.collector.collectMSSQLPlanOps(ctx, data, cols)
	}
	if response := mssqlFunctionContextError(ctx, ctx.Err()); response != nil {
		return response
	}
	extraCols := []topQueriesColumn{topQueriesSourceColumn(source)}
	extraCols = append(extraCols, mssqlErrorAttributionColumns()...)
	extraCols = append(extraCols, mssqlPlanAttributionColumns()...)

	queryIdx := -1
	queryHashIdx := -1
	dbIdx := -1
	for i, col := range cols {
		switch col.Name {
		case "query":
			queryIdx = i
		case "queryHash":
			queryHashIdx = i
		case "database":
			dbIdx = i
		}
	}

	for i := range data {
		status := errorStatus
		var errRow mssqlErrorRow
		if errorStatus == mssqlErrorAttrEnabled {
			found := false
			if queryHashIdx >= 0 && queryHashIdx < len(data[i]) {
				queryHash := rowString(data[i][queryHashIdx])
				if queryHash != "" {
					if row, ok := errorDetails[queryHash]; ok {
						status = mssqlErrorAttrEnabled
						errRow = row
						found = true
					}
				}
			}
			if !found && queryIdx >= 0 && queryIdx < len(data[i]) {
				queryText := normalizeSQLText(rowString(data[i][queryIdx]))
				if queryText != "" {
					if row, ok := errorDetails[queryText]; ok {
						status = mssqlErrorAttrEnabled
						errRow = row
						found = true
					}
				}
			}
			if !found {
				status = mssqlErrorAttrNoData
			}
		}

		var hashMatch, mergeJoin, nestedLoops, sorts any
		if dbIdx >= 0 && dbIdx < len(data[i]) && queryHashIdx >= 0 && queryHashIdx < len(data[i]) {
			dbName := rowString(data[i][dbIdx])
			queryHash := rowString(data[i][queryHashIdx])
			if dbName != "" && queryHash != "" {
				if opsByHash, ok := planOpsByDB[dbName]; ok {
					if ops, ok := opsByHash[queryHash]; ok {
						hashMatch = ops.HashMatch
						mergeJoin = ops.MergeJoin
						nestedLoops = ops.NestedLoops
						sorts = ops.Sorts
					}
				}
			}
		}

		var errNo any
		if errRow.ErrorNumber != nil {
			errNo = *errRow.ErrorNumber
		}
		var errState any
		if errRow.ErrorState != nil {
			errState = *errRow.ErrorState
		}
		data[i] = append(data[i],
			string(source),
			status,
			errNo,
			errState,
			nullableString(rowString(errRow.Message)),
			hashMatch,
			mergeJoin,
			nestedLoops,
			sorts,
		)
	}
	cols = append(cols, extraCols...)

	sortParam, sortOptions := f.buildSortParam(cols)

	defaultSort := ""
	for _, col := range cols {
		if col.IsDefaultSort() && col.IsSortOption() {
			defaultSort = col.Name
			break
		}
	}
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := f.decorateColumns(cols)
	cs := f.columnSet(annotatedCols)

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              topQueriesHelp(source),
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
	}
}

// queryStoreSupported reports whether this instance exposes Query Store.
//
// It deliberately does NOT compare ProductVersion: Azure SQL Database reports 12.x while
// being newer than SQL Server 2016, so a "major < 13" test would wrongly disable
// top-queries there. Probing the catalog is version-proof, and it also fails closed on
// servers whose version string cannot be parsed.
func (f *funcTopQueries) queryStoreSupported(ctx context.Context) (bool, error) {
	c := f.router.collector

	c.queryStoreSupportedMu.RLock()
	if cached := c.queryStoreSupported; cached != nil {
		c.queryStoreSupportedMu.RUnlock()
		return *cached, nil
	}
	c.queryStoreSupportedMu.RUnlock()

	if c.isAzureSQLDatabase() {
		supported := true
		c.queryStoreSupportedMu.Lock()
		if c.queryStoreSupported == nil {
			c.queryStoreSupported = &supported
		}
		supported = *c.queryStoreSupported
		c.queryStoreSupportedMu.Unlock()
		return supported, nil
	}

	var count int
	if err := c.db.QueryRowContext(ctx, queryQueryStoreSupported).Scan(&count); err != nil {
		return false, err
	}

	supported := count > 0
	c.queryStoreSupportedMu.Lock()
	if c.queryStoreSupported == nil {
		c.queryStoreSupported = &supported
	}
	supported = *c.queryStoreSupported
	c.queryStoreSupportedMu.Unlock()

	return supported, nil
}

// resolveTopQueriesSource picks the statistics store that answers this request.
//
// Query Store is preferred: it keeps per-database history that survives restarts and plan
// cache eviction. The plan cache is the fallback for servers that cannot use it - Query
// Store does not exist before SQL Server 2016 (13.x) and may be disabled on newer servers.
func (f *funcTopQueries) resolveTopQueriesSource(ctx context.Context) (topQueriesSource, map[string]bool, error) {
	supported, err := f.queryStoreSupported(ctx)
	if err != nil {
		return "", nil, err
	}
	if supported {
		cols, err := f.detectQueryStoreColumns(ctx)
		if err == nil {
			return topQueriesSourceQueryStore, cols, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", nil, err
		}
	}

	cols, err := f.detectPlanCacheColumns(ctx)
	if err != nil {
		return "", nil, err
	}
	return topQueriesSourcePlanCache, cols, nil
}

// detectPlanCacheColumns probes which sys.dm_exec_query_stats columns this release exposes.
// The DMV grew columns over time (rows in 2008 R2 SP1, DOP and memory grants in 2016), and
// probing keeps the query free of version arithmetic.
func (f *funcTopQueries) detectPlanCacheColumns(ctx context.Context) (map[string]bool, error) {
	c := f.router.collector

	c.planCacheColsMu.RLock()
	if cached := c.planCacheCols; cached != nil {
		c.planCacheColsMu.RUnlock()
		return cached, nil
	}
	c.planCacheColsMu.RUnlock()

	rows, err := c.db.QueryContext(ctx, queryPlanCacheColumns)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to query plan cache columns: %w", err)
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	cols := make(map[string]bool, len(columnNames))
	for _, colName := range columnNames {
		cols[strings.ToLower(colName)] = true
	}

	// query_hash arrived in SQL Server 2008 and is the grouping key; execution_count is the
	// denominator of every average. Without them the plan cache cannot answer at all.
	if !cols["query_hash"] || !cols["execution_count"] {
		return nil, errTopQueriesNoSource
	}

	c.planCacheColsMu.Lock()
	if c.planCacheCols == nil {
		c.planCacheCols = cols
	}
	cols = c.planCacheCols
	c.planCacheColsMu.Unlock()

	return cols, nil
}

// topQueriesSourceColumn reports which store produced the rows. It is hidden on the
// Query Store path, where it is the expected source and the value is constant, and shown on
// the plan cache fallback, where cache-resident numbers must not be read as history.
func topQueriesSourceColumn(source topQueriesSource) topQueriesColumn {
	return topQueriesColumn{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "source",
			Tooltip:   "Statistics source (query-store, plan-cache)",
			Type:      funcapi.FieldTypeString,
			Visible:   source == topQueriesSourcePlanCache,
			Transform: funcapi.FieldTransformNone,
			Sort:      funcapi.FieldSortAscending,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterMultiselect,
		},
	}
}

func topQueriesHelp(source topQueriesSource) string {
	if source == topQueriesSourcePlanCache {
		return topQueriesHelpPlanCache
	}
	return topQueriesHelpQueryStore
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) detectQueryStoreColumns(ctx context.Context) (map[string]bool, error) {
	// Enablement may change between requests even though the column schema is cached.
	var sampleDB string
	sampleQuery := `
		SELECT TOP 1 name
		FROM sys.databases
		WHERE is_query_store_on = 1
		  AND name NOT IN ('master', 'tempdb', 'model', 'msdb')
	`
	if f.router.collector.isAzureSQLDatabase() {
		sampleQuery = `
			SELECT DB_NAME()
			FROM sys.database_query_store_options
			WHERE actual_state IN (1, 2, 4)
		`
	}
	err := f.router.collector.db.QueryRowContext(ctx, sampleQuery).Scan(&sampleDB)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errQueryStoreNotEnabled
		}
		return nil, fmt.Errorf("failed to find database with Query Store: %w", err)
	}

	f.router.collector.queryStoreColsMu.RLock()
	if cols := f.router.collector.queryStoreCols; cols != nil {
		f.router.collector.queryStoreColsMu.RUnlock()
		return cols, nil
	}
	f.router.collector.queryStoreColsMu.RUnlock()

	query := `SELECT TOP 0 * FROM sys.query_store_runtime_stats`
	if !f.router.collector.isAzureSQLDatabase() {
		escapedDB := strings.ReplaceAll(sampleDB, "]", "]]")
		query = fmt.Sprintf(`SELECT TOP 0 * FROM [%s].sys.query_store_runtime_stats`, escapedDB)
	}
	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to query Query Store columns from %s: %w", sampleDB, err)
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	cols := make(map[string]bool)
	for _, colName := range columnNames {
		cols[strings.ToLower(colName)] = true
	}

	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns found in sys.query_store_runtime_stats")
	}

	// Add identity columns that are always available (from other Query Store views)
	cols["query_hash"] = true
	cols["query_sql_text"] = true
	cols["database_name"] = true

	f.router.collector.queryStoreColsMu.Lock()
	if f.router.collector.queryStoreCols == nil {
		f.router.collector.queryStoreCols = cols
	}
	cols = f.router.collector.queryStoreCols
	f.router.collector.queryStoreColsMu.Unlock()

	return cols, nil
}

func (f *funcTopQueries) buildAvailableColumns(source topQueriesSource, availableCols map[string]bool) []topQueriesColumn {
	var cols []topQueriesColumn
	seen := make(map[string]bool)

	for _, col := range topQueriesColumns {
		if seen[col.Name] {
			continue
		}
		if col.IsIdentity {
			cols = append(cols, col)
			seen[col.Name] = true
			continue
		}
		dbColumn := col.DBColumn
		if source == topQueriesSourcePlanCache {
			dbColumn = col.CacheColumn
		}
		if dbColumn == "" {
			continue
		}
		if availableCols[dbColumn] {
			cols = append(cols, col)
			seen[col.Name] = true
		}
	}
	return cols
}

func (f *funcTopQueries) mapAndValidateSortColumn(sortKey string, cols []topQueriesColumn) string {
	for _, col := range cols {
		if col.Name == sortKey && col.IsSortOption() {
			return col.Name
		}
	}

	for _, col := range cols {
		if col.IsSortOption() {
			return col.Name
		}
	}

	for _, col := range cols {
		if !col.IsIdentity {
			return col.Name
		}
	}

	if len(cols) > 0 {
		return cols[0].Name
	}

	return ""
}

func (f *funcTopQueries) buildSelectExpressions(cols []topQueriesColumn, dbNameExpr, prefix string) []string {
	var selectParts []string

	for _, col := range cols {
		var expr string
		switch {
		case col.IsIdentity:
			switch col.Name {
			case "queryHash":
				expr = fmt.Sprintf("CONVERT(VARCHAR(64), rs.query_hash, 1) AS [%s]", col.Name)
			case "query":
				expr = fmt.Sprintf("(SELECT qt.query_sql_text FROM %ssys.query_store_query_text qt WHERE qt.query_text_id = rs.query_text_id) AS [%s]", prefix, col.Name)
			case "database":
				expr = fmt.Sprintf("%s AS [%s]", dbNameExpr, col.Name)
			}
		case col.Name == "calls":
			expr = fmt.Sprintf("SUM(rs.count_executions) AS [%s]", col.Name)
		case col.Name == "totalTime":
			expr = fmt.Sprintf("SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS [%s]", col.Name)
		case col.NeedsAvg && col.IsMicroseconds:
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) / 1000.0 ELSE 0 END AS [%s]", col.DBColumn, col.Name)
		case col.NeedsAvg:
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) ELSE 0 END AS [%s]", col.DBColumn, col.Name)
		case strings.HasPrefix(col.DBColumn, "last_"):
			expr = topQueriesLastValueExpression(col, "rs", col.DBColumn)
		case col.IsMicroseconds:
			aggFunc := "MAX"
			if strings.HasPrefix(col.DBColumn, "min_") {
				aggFunc = "MIN"
			}
			expr = fmt.Sprintf("%s(rs.%s) / 1000.0 AS [%s]", aggFunc, col.DBColumn, col.Name)
		default:
			aggFunc := "MAX"
			if strings.HasPrefix(col.DBColumn, "min_") {
				aggFunc = "MIN"
			}
			expr = fmt.Sprintf("%s(rs.%s) AS [%s]", aggFunc, col.DBColumn, col.Name)
		}
		if expr != "" {
			selectParts = append(selectParts, expr)
		}
	}
	return selectParts
}

func (f *funcTopQueries) buildDynamicSQL(cols []topQueriesColumn, sortColumn string, timeWindowDays int, limit int) string {
	selectSQL := f.buildQueryStoreSelectSQL(cols, "' + QUOTENAME(name, '''') + N'", "' + QUOTENAME(name) + N'.", timeWindowDays)

	orderByExpr := sortColumn
	if orderByExpr == "" {
		orderByExpr = defaultTopQueriesOrderColumn(cols)
	}

	return fmt.Sprintf(`
DECLARE @sql NVARCHAR(MAX) = N'';

SELECT @sql = @sql +
    CASE WHEN @sql = N'' THEN N'' ELSE N' UNION ALL ' END +
    N'%s'
FROM sys.databases
WHERE is_query_store_on = 1
  AND name NOT IN ('master', 'tempdb', 'model', 'msdb');

IF @sql = N''
BEGIN
    RAISERROR('No databases have Query Store enabled', 16, 1);
    RETURN;
END

SET @sql = N'SELECT TOP %d * FROM (' + @sql + N') AS combined ORDER BY [%s] DESC';
EXEC sp_executesql @sql;
`, selectSQL, limit, orderByExpr)
}

// Select every last_* value from one execution while aggregating all matching history.
func topQueriesLastValueExpression(col topQueriesColumn, alias, dbColumn string) string {
	expr := fmt.Sprintf("MAX(CASE WHEN %s.execution_rank = 1 THEN %s.%s END)", alias, alias, dbColumn)
	if col.IsMicroseconds {
		expr += " / 1000.0"
	}
	return fmt.Sprintf("%s AS [%s]", expr, col.Name)
}

func (f *funcTopQueries) buildQueryStoreSelectSQL(cols []topQueriesColumn, dbNameExpr, prefix string, timeWindowDays int) string {
	selectExpr := strings.Join(f.buildSelectExpressions(cols, dbNameExpr, prefix), ",\n  ")
	timeFilter := ""
	if timeWindowDays > 0 {
		timeFilter = fmt.Sprintf("WHERE rsi.start_time >= DATEADD(day, -%d, GETUTCDATE())", timeWindowDays)
	}
	// Canonicalize equal texts only for queries with matching history. The history sort
	// then carries numeric IDs instead of repeated nvarchar(max) values through exchanges.
	return fmt.Sprintf(`SELECT
  %s
FROM (
  SELECT rs.*, q.query_hash, q.query_text_id,
         ROW_NUMBER() OVER (
           PARTITION BY q.query_hash, q.query_text_id
           ORDER BY rs.last_execution_time DESC, rs.runtime_stats_id DESC
         ) AS execution_rank
  FROM (
    SELECT q.query_id, q.query_hash,
           MIN(q.query_text_id) OVER (PARTITION BY q.query_hash, qt.query_sql_text) AS query_text_id
    FROM %ssys.query_store_query q
    INNER JOIN %ssys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
    WHERE q.query_id IN (
      SELECT p.query_id
      FROM %ssys.query_store_plan p
      INNER JOIN %ssys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
      INNER JOIN %ssys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
      %s
    )
  ) AS q
  INNER JOIN %ssys.query_store_plan p ON q.query_id = p.query_id
  INNER JOIN %ssys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
  INNER JOIN %ssys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
  %s
) AS rs
GROUP BY rs.query_hash, rs.query_text_id`, selectExpr, prefix, prefix, prefix, prefix, prefix, timeFilter, prefix, prefix, prefix, timeFilter)
}

func (f *funcTopQueries) buildTopQueriesSQL(source topQueriesSource, cols []topQueriesColumn, sortColumn string, timeWindowDays int, limit int) string {
	if source == topQueriesSourcePlanCache {
		return f.buildPlanCacheSQL(cols, sortColumn, timeWindowDays, limit)
	}
	return f.buildQueryStoreSQL(cols, sortColumn, timeWindowDays, limit)
}

// buildPlanCacheSelectExpressions mirrors buildSelectExpressions for sys.dm_exec_query_stats.
// The two sources need separate aggregation: Query Store stores per-interval averages that
// must be re-weighted by execution count, while the plan cache stores running totals that
// only need dividing. Averages multiply by 1.0 first because both operands are bigint and
// integer division would truncate fractional values such as DOP.
func (f *funcTopQueries) buildPlanCacheSelectExpressions(cols []topQueriesColumn) []string {
	var selectParts []string

	for _, col := range cols {
		var expr string
		switch {
		case col.IsIdentity:
			switch col.Name {
			case "queryHash":
				// Same hex rendering as the Query Store path, so error attribution keeps matching.
				expr = fmt.Sprintf("CONVERT(VARCHAR(64), qs.query_hash, 1) AS [%s]", col.Name)
			case "query":
				expr = fmt.Sprintf("MIN(qs.query_sql_text) AS [%s]", col.Name)
			case "database":
				expr = fmt.Sprintf("MIN(qs.database_name) AS [%s]", col.Name)
			}
		case col.Name == "calls":
			expr = fmt.Sprintf("SUM(qs.execution_count) AS [%s]", col.Name)
		case col.Name == "totalTime":
			expr = fmt.Sprintf("SUM(qs.total_elapsed_time) / 1000.0 AS [%s]", col.Name)
		case col.NeedsAvg && col.IsMicroseconds:
			expr = fmt.Sprintf("CASE WHEN SUM(qs.execution_count) > 0 THEN SUM(qs.%s) * 1.0 / SUM(qs.execution_count) / 1000.0 ELSE 0 END AS [%s]", col.CacheColumn, col.Name)
		case col.NeedsAvg:
			expr = fmt.Sprintf("CASE WHEN SUM(qs.execution_count) > 0 THEN SUM(qs.%s) * 1.0 / SUM(qs.execution_count) ELSE 0 END AS [%s]", col.CacheColumn, col.Name)
		case strings.HasPrefix(col.CacheColumn, "last_"):
			expr = topQueriesLastValueExpression(col, "qs", col.CacheColumn)
		default:
			aggFunc := "MAX"
			if strings.HasPrefix(col.CacheColumn, "min_") {
				aggFunc = "MIN"
			}
			if col.IsMicroseconds {
				expr = fmt.Sprintf("%s(qs.%s) / 1000.0 AS [%s]", aggFunc, col.CacheColumn, col.Name)
			} else {
				expr = fmt.Sprintf("%s(qs.%s) AS [%s]", aggFunc, col.CacheColumn, col.Name)
			}
		}
		if expr != "" {
			selectParts = append(selectParts, expr)
		}
	}
	return selectParts
}

// buildPlanCacheSQL aggregates cached plans by query hash across the whole instance.
//
// The statement text lives in the plan cache rather than alongside the statistics, so the
// text function has to be applied before grouping. The recency predicate is pushed into the
// derived table so the optimizer can discard rows before that apply. Unlike Query Store,
// the plan cache has no interval history, so the configured window filters by last
// execution instead of aggregating a period.
func (f *funcTopQueries) buildPlanCacheSQL(cols []topQueriesColumn, sortColumn string, timeWindowDays int, limit int) string {
	selectExpr := strings.Join(f.buildPlanCacheSelectExpressions(cols), ",\n  ")

	recencyFilter := ""
	if timeWindowDays > 0 {
		// last_execution_time is recorded in the server's own time zone, so compare with GETDATE().
		recencyFilter = fmt.Sprintf("AND qs.last_execution_time >= DATEADD(day, -%d, GETDATE())", timeWindowDays)
	}

	orderByExpr := sortColumn
	if orderByExpr == "" {
		orderByExpr = defaultTopQueriesOrderColumn(cols)
	}

	return fmt.Sprintf(`
SELECT TOP %d
  %s
FROM (
    SELECT qs.*,
           ROW_NUMBER() OVER (
             PARTITION BY qs.query_hash
             ORDER BY qs.last_execution_time DESC, qs.plan_handle, qs.statement_start_offset
           ) AS execution_rank,
           DB_NAME(qt.dbid) AS database_name,
           SUBSTRING(qt.text,
                     (qs.statement_start_offset / 2) + 1,
                     ((CASE qs.statement_end_offset
                         WHEN -1 THEN DATALENGTH(qt.text)
                         ELSE qs.statement_end_offset
                       END - qs.statement_start_offset) / 2) + 1) AS query_sql_text
    FROM sys.dm_exec_query_stats AS qs
    CROSS APPLY sys.dm_exec_sql_text(qs.sql_handle) AS qt
    WHERE (DB_NAME(qt.dbid) IS NULL OR DB_NAME(qt.dbid) NOT IN ('master', 'tempdb', 'model', 'msdb'))
    %s
) AS qs
GROUP BY qs.query_hash
ORDER BY [%s] DESC;
`, limit, selectExpr, recencyFilter, orderByExpr)
}

func defaultTopQueriesOrderColumn(cols []topQueriesColumn) string {
	for _, col := range cols {
		if !col.IsIdentity {
			return col.Name
		}
	}
	if len(cols) > 0 {
		return cols[0].Name
	}
	return ""
}

func (f *funcTopQueries) buildQueryStoreSQL(cols []topQueriesColumn, sortColumn string, timeWindowDays int, limit int) string {
	if !f.router.collector.isAzureSQLDatabase() {
		return f.buildDynamicSQL(cols, sortColumn, timeWindowDays, limit)
	}

	selectSQL := f.buildQueryStoreSelectSQL(cols, "DB_NAME()", "", timeWindowDays)
	return fmt.Sprintf("SELECT TOP %d * FROM (%s) AS combined ORDER BY [%s] DESC;", limit, selectSQL, sortColumn)
}

func (f *funcTopQueries) scanDynamicRows(rows topQueriesRowScanner, cols []topQueriesColumn) ([][]any, error) {
	specs := make([]sqlquery.ScanColumnSpec, len(cols))
	for i, col := range cols {
		specs[i] = mssqlTopQueriesScanSpec(col)
	}

	data, err := sqlquery.ScanTypedRows(rows, specs)
	if err != nil {
		return nil, fmt.Errorf("row scan failed: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return data, nil
}

func (c *Collector) topQueriesPermissionMessage() string {
	if c.isAzureSQLDatabase() {
		return "top-queries requires VIEW DATABASE PERFORMANCE STATE for Query Store; plan-cache access requires VIEW DATABASE STATE or administrator/##MS_ServerStateReader## access, depending on the Azure SQL Database service tier"
	}
	if c.currentMajorVersion() >= 16 {
		return "top-queries requires VIEW SERVER PERFORMANCE STATE for the plan cache, or VIEW DATABASE PERFORMANCE STATE in every queried user database for Query Store"
	}
	return "top-queries requires VIEW SERVER STATE for the plan cache, or VIEW DATABASE STATE in every queried user database for Query Store"
}

func mssqlTopQueriesScanSpec(col topQueriesColumn) sqlquery.ScanColumnSpec {
	spec := sqlquery.ScanColumnSpec{}
	switch col.Type {
	case funcapi.FieldTypeString:
		spec.Type = sqlquery.ScanValueString
		if col.Name == "query" {
			spec.Transform = func(v any) any {
				s, _ := v.(string)
				return strmutil.TruncateText(s, topQueriesMaxTextLength)
			}
		}
	case funcapi.FieldTypeInteger:
		spec.Type = sqlquery.ScanValueInteger
	case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
		spec.Type = sqlquery.ScanValueFloat
	default:
		spec.Type = sqlquery.ScanValueDiscard
	}
	return spec
}

func (f *funcTopQueries) buildSortParam(cols []topQueriesColumn) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := buildTopQueriesSortOptions(cols)
	sortParam := funcapi.ParamConfig{
		ID:         topQueriesParamSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    sortOptions,
		UniqueView: true,
	}
	return sortParam, sortOptions
}

func (f *funcTopQueries) decorateColumns(cols []topQueriesColumn) []topQueriesColumn {
	out := make([]topQueriesColumn, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.Name] = i
	}

	for i := range out {
		if topQueriesLabelColumnIDs[out[i].Name] {
			out[i].GroupBy = &funcapi.GroupByOptions{
				IsDefault: out[i].Name == topQueriesPrimaryLabelID,
			}
		}
	}

	for _, group := range topQueriesChartGroupDefs {
		for _, key := range group.columns {
			idx, ok := index[key]
			if !ok {
				continue
			}
			out[idx].Chart = &funcapi.ChartOptions{
				Group:     group.key,
				Title:     group.title,
				IsDefault: group.defaultChart,
			}
		}
	}

	return out
}

// buildTopQueriesSortOptions builds sort options for method registration (before handler exists).
func buildTopQueriesSortOptions(cols []topQueriesColumn) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	seen := make(map[string]bool)
	for _, col := range cols {
		if col.IsSortOption() && !seen[col.Name] {
			seen[col.Name] = true
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    col.SortLabel(),
				Default: col.IsDefaultSort(),
				Sort:    &sortDir,
			})
		}
	}
	return sortOptions
}
