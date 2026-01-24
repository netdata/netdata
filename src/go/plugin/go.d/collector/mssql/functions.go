// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

// mssqlColumn embeds funcapi.ColumnMeta and adds MSSQL-specific fields.
type mssqlColumn struct {
	funcapi.ColumnMeta
	DBColumn       string // Column name in sys.query_store_runtime_stats
	IsMicroseconds bool   // Needs μs to milliseconds conversion
	IsSortOption   bool   // Show in sort dropdown
	SortLabel      string // Label for sort option
	IsDefaultSort  bool   // Is this the default sort option
	IsIdentity     bool   // Is this an identity column (query_hash, query_text, etc.)
	NeedsAvg       bool   // Needs weighted average calculation (avg_* columns)
}

func mssqlColumnSet(cols []mssqlColumn) funcapi.ColumnSet[mssqlColumn] {
	return funcapi.Columns(cols, func(c mssqlColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

// mssqlAllColumns defines ALL possible columns from Query Store
// Columns that don't exist in certain SQL Server versions will be filtered at runtime
var mssqlAllColumns = []mssqlColumn{
	// Identity columns - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Tooltip: "Query Hash", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true}, DBColumn: "query_hash", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true}, DBColumn: "query_sql_text", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect}, DBColumn: "database_name", IsIdentity: true},

	// Execution count - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "count_executions", IsSortOption: true, SortLabel: "Top queries by Number of Calls"},

	// Duration metrics (microseconds -> milliseconds) - SQL 2016+
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", IsMicroseconds: true, IsSortOption: true, SortLabel: "Top queries by Total Execution Time", IsDefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", IsMicroseconds: true, NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastTime", Tooltip: "Last Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevTime", Tooltip: "StdDev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_duration", IsMicroseconds: true},

	// CPU time metrics (microseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgCpu", Tooltip: "Avg CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_cpu_time", IsMicroseconds: true, NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average CPU Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastCpu", Tooltip: "Last CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minCpu", Tooltip: "Min CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxCpu", Tooltip: "Max CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevCpu", Tooltip: "StdDev CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_cpu_time", IsMicroseconds: true},

	// Logical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgReads", Tooltip: "Avg Logical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_reads", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Logical Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastReads", Tooltip: "Last Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minReads", Tooltip: "Min Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxReads", Tooltip: "Max Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevReads", Tooltip: "StdDev Logical Reads", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_reads"},

	// Logical I/O writes
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgWrites", Tooltip: "Avg Logical Writes", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_writes", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Logical Writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastWrites", Tooltip: "Last Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minWrites", Tooltip: "Min Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxWrites", Tooltip: "Max Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevWrites", Tooltip: "StdDev Logical Writes", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_writes"},

	// Physical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgPhysReads", Tooltip: "Avg Physical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_physical_io_reads", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Physical Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastPhysReads", Tooltip: "Last Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_physical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minPhysReads", Tooltip: "Min Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_physical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxPhysReads", Tooltip: "Max Physical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_physical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevPhysReads", Tooltip: "StdDev Physical Reads", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_physical_io_reads"},

	// CLR time (microseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgClr", Tooltip: "Avg CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_clr_time", IsMicroseconds: true, NeedsAvg: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastClr", Tooltip: "Last CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minClr", Tooltip: "Min CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxClr", Tooltip: "Max CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_clr_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevClr", Tooltip: "StdDev CLR Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_clr_time", IsMicroseconds: true},

	// DOP (degree of parallelism)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgDop", Tooltip: "Avg DOP", Type: funcapi.FieldTypeFloat, DecimalPoints: 1, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_dop", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Parallelism"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastDop", Tooltip: "Last DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minDop", Tooltip: "Min DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxDop", Tooltip: "Max DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevDop", Tooltip: "StdDev DOP", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_dop"},

	// Memory grant (8KB pages)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgMemory", Tooltip: "Avg Memory (8KB pages)", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_query_max_used_memory", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Memory Grant"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastMemory", Tooltip: "Last Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minMemory", Tooltip: "Min Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxMemory", Tooltip: "Max Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevMemory", Tooltip: "StdDev Memory", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_query_max_used_memory"},

	// Row count
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgRows", Tooltip: "Avg Rows", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_rowcount", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Row Count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastRows", Tooltip: "Last Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_rowcount"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minRows", Tooltip: "Min Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_rowcount"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxRows", Tooltip: "Max Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_rowcount"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevRows", Tooltip: "StdDev Rows", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_rowcount"},

	// SQL Server 2017+ log bytes
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgLogBytes", Tooltip: "Avg Log Bytes", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_log_bytes_used", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average Log Bytes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastLogBytes", Tooltip: "Last Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minLogBytes", Tooltip: "Min Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxLogBytes", Tooltip: "Max Log Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_log_bytes_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevLogBytes", Tooltip: "StdDev Log Bytes", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_log_bytes_used"},

	// SQL Server 2017+ tempdb space
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTempdb", Tooltip: "Avg TempDB (8KB pages)", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_tempdb_space_used", NeedsAvg: true, IsSortOption: true, SortLabel: "Top queries by Average TempDB Usage"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastTempdb", Tooltip: "Last TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTempdb", Tooltip: "Min TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTempdb", Tooltip: "Max TempDB (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_tempdb_space_used"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevTempdb", Tooltip: "StdDev TempDB", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_tempdb_space_used"},
}

type mssqlChartGroupDef struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var mssqlChartGroupDefs = []mssqlChartGroupDef{
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

var mssqlLabelColumnIDs = map[string]bool{
	"database": true,
}

const mssqlPrimaryLabelID = "database"

// mssqlMethods returns the available function methods for MSSQL
func mssqlMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	seen := make(map[string]bool) // Avoid duplicates from totalTime/avgTime using same DBColumn
	for _, col := range mssqlAllColumns {
		if col.IsSortOption && !seen[col.Name] {
			seen[col.Name] = true
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    col.SortLabel,
				Default: col.IsDefaultSort,
				Sort:    &sortDir,
			})
		}
	}

	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from Query Store",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				{
					ID:         "__sort",
					Name:       "Filter By",
					Help:       "Select the primary sort column",
					Selection:  funcapi.ParamSelect,
					Options:    sortOptions,
					UniqueView: true,
				},
			},
		},
	}
}

func mssqlMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	if collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// mssqlHandleMethod handles function requests for MSSQL
func mssqlHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	// Check if collector is initialized (first collect() may not have run yet)
	if collector.db == nil {
		return &module.FunctionResponse{
			Status:  503,
			Message: "collector is still initializing, please retry in a few seconds",
		}
	}

	switch method {
	case "top-queries":
		return collector.collectTopQueries(ctx, params.Column("__sort"))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

// detectMSSQLQueryStoreColumns queries any database with Query Store enabled to discover available columns
func (c *Collector) detectMSSQLQueryStoreColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	c.queryStoreColsMu.RLock()
	if c.queryStoreCols != nil {
		cols := c.queryStoreCols
		c.queryStoreColsMu.RUnlock()
		return cols, nil
	}
	c.queryStoreColsMu.RUnlock()

	// Slow path: query and cache
	c.queryStoreColsMu.Lock()
	defer c.queryStoreColsMu.Unlock()

	// Double-check after acquiring write lock
	if c.queryStoreCols != nil {
		return c.queryStoreCols, nil
	}

	// Find any database with Query Store enabled (excluding system databases)
	var sampleDB string
	err := c.db.QueryRowContext(ctx, `
		SELECT TOP 1 name
		FROM sys.databases
		WHERE is_query_store_on = 1
		  AND name NOT IN ('master', 'tempdb', 'model', 'msdb')
	`).Scan(&sampleDB)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no databases have Query Store enabled")
		}
		return nil, fmt.Errorf("failed to find database with Query Store: %w", err)
	}

	// Use dynamic SQL to get column metadata from that database's Query Store view
	// Three-part naming works: [DatabaseName].sys.query_store_runtime_stats
	query := fmt.Sprintf(`SELECT TOP 0 * FROM [%s].sys.query_store_runtime_stats`, sampleDB)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query Query Store columns from %s: %w", sampleDB, err)
	}
	defer rows.Close()

	// Get column names from result set metadata
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	cols := make(map[string]bool)
	for _, colName := range columnNames {
		// Normalize to lowercase for case-insensitive comparison
		cols[strings.ToLower(colName)] = true
	}

	// If we found no columns, something is wrong
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns found in sys.query_store_runtime_stats")
	}

	// Add identity columns that are always available (from other Query Store views)
	cols["query_hash"] = true
	cols["query_sql_text"] = true
	cols["database_name"] = true

	// Cache the result
	c.queryStoreCols = cols

	return cols, nil
}

// buildAvailableMSSQLColumns filters columns based on what's available in the database
func (c *Collector) buildAvailableMSSQLColumns(availableCols map[string]bool) []mssqlColumn {
	var cols []mssqlColumn
	seen := make(map[string]bool)

	for _, col := range mssqlAllColumns {
		// Skip duplicates (e.g., totalTime and avgTime both use avg_duration)
		if seen[col.Name] {
			continue
		}
		// Identity columns are always available
		if col.IsIdentity {
			cols = append(cols, col)
			seen[col.Name] = true
			continue
		}
		// Check if the DBColumn exists
		if availableCols[col.DBColumn] {
			cols = append(cols, col)
			seen[col.Name] = true
		}
	}
	return cols
}

// mapAndValidateMSSQLSortColumn maps UI sort key to the appropriate sort expression
// Uses the filtered cols list to ensure the sort column is actually in the SELECT
func (c *Collector) mapAndValidateMSSQLSortColumn(sortKey string, cols []mssqlColumn) string {
	// First, check if the requested sort key is in the available columns
	for _, col := range cols {
		if col.Name == sortKey && col.IsSortOption {
			return col.Name
		}
	}

	// Fall back to the first available sort column
	for _, col := range cols {
		if col.IsSortOption {
			return col.Name
		}
	}

	// Last resort: use first non-identity column
	for _, col := range cols {
		if !col.IsIdentity {
			return col.Name
		}
	}

	// Absolute fallback: use first column in the list (must exist in SELECT)
	if len(cols) > 0 {
		return cols[0].Name
	}

	return "" // empty - will be handled by caller
}

// buildMSSQLSelectExpressions builds the SELECT expressions for a single database query
func (c *Collector) buildMSSQLSelectExpressions(cols []mssqlColumn, dbNameExpr string) []string {
	var selectParts []string

	for _, col := range cols {
		var expr string
		switch {
		case col.IsIdentity:
			switch col.Name {
			case "queryHash":
				expr = fmt.Sprintf("CONVERT(VARCHAR(64), q.query_hash, 1) AS [%s]", col.Name)
			case "query":
				expr = fmt.Sprintf("qt.query_sql_text AS [%s]", col.Name)
			case "database":
				expr = fmt.Sprintf("%s AS [%s]", dbNameExpr, col.Name)
			}
		case col.Name == "calls":
			expr = fmt.Sprintf("SUM(rs.count_executions) AS [%s]", col.Name)
		case col.Name == "totalTime":
			// Total time = sum of (avg_duration * executions) converted to milliseconds
			expr = fmt.Sprintf("SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS [%s]", col.Name)
		case col.NeedsAvg && col.IsMicroseconds:
			// Weighted average with μs to milliseconds conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) / 1000.0 ELSE 0 END AS [%s]", col.DBColumn, col.Name)
		case col.NeedsAvg:
			// Weighted average without time conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) ELSE 0 END AS [%s]", col.DBColumn, col.Name)
		case col.IsMicroseconds:
			// Aggregate with μs to milliseconds conversion
			aggFunc := "MAX"
			if strings.HasPrefix(col.DBColumn, "min_") {
				aggFunc = "MIN"
			}
			expr = fmt.Sprintf("%s(rs.%s) / 1000.0 AS [%s]", aggFunc, col.DBColumn, col.Name)
		default:
			// Simple aggregate
			aggFunc := "MAX"
			if strings.HasPrefix(col.DBColumn, "min_") {
				aggFunc = "MIN"
			}
			if strings.HasPrefix(col.DBColumn, "stdev_") {
				aggFunc = "MAX" // Use MAX for stddev aggregation
			}
			expr = fmt.Sprintf("%s(rs.%s) AS [%s]", aggFunc, col.DBColumn, col.Name)
		}
		if expr != "" {
			selectParts = append(selectParts, expr)
		}
	}
	return selectParts
}

// buildMSSQLDynamicSQL builds dynamic SQL that aggregates across all databases with Query Store enabled
// Uses sp_executesql to execute the built query
func (c *Collector) buildMSSQLDynamicSQL(cols []mssqlColumn, sortColumn string, timeWindowDays int, limit int) string {
	// Build the SELECT expressions template (with placeholder for database name)
	// We use ''' + name + N''' to close the outer string, concatenate the db name, and reopen
	// This produces a properly quoted string literal like 'DatabaseName' in the final SQL
	selectParts := c.buildMSSQLSelectExpressions(cols, "''' + name + N'''")
	selectExpr := strings.Join(selectParts, ",\n        ")

	// Time window filter
	timeFilter := ""
	if timeWindowDays > 0 {
		timeFilter = fmt.Sprintf("WHERE rsi.start_time >= DATEADD(day, -%d, GETUTCDATE())", timeWindowDays)
	}

	// Validate sort column
	orderByExpr := sortColumn
	if orderByExpr == "" {
		for _, col := range cols {
			if !col.IsIdentity {
				orderByExpr = col.Name
				break
			}
		}
		if orderByExpr == "" && len(cols) > 0 {
			orderByExpr = cols[0].Name
		}
	}

	// Build the dynamic SQL that creates UNION ALL across all databases
	// The database names come from sys.databases, ensuring safety (no user input)
	return fmt.Sprintf(`
DECLARE @sql NVARCHAR(MAX) = N'';

SELECT @sql = @sql +
    CASE WHEN @sql = N'' THEN N'' ELSE N' UNION ALL ' END +
    N'SELECT
        %s
    FROM ' + QUOTENAME(name) + N'.sys.query_store_query q
    INNER JOIN ' + QUOTENAME(name) + N'.sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
    INNER JOIN ' + QUOTENAME(name) + N'.sys.query_store_plan p ON q.query_id = p.query_id
    INNER JOIN ' + QUOTENAME(name) + N'.sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
    INNER JOIN ' + QUOTENAME(name) + N'.sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
    %s
    GROUP BY q.query_hash, qt.query_sql_text'
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
`, selectExpr, timeFilter, limit, orderByExpr)
}

// mssqlRowScanner interface for testing
type mssqlRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanMSSQLDynamicRows scans rows dynamically based on column types
func (c *Collector) scanMSSQLDynamicRows(rows mssqlRowScanner, cols []mssqlColumn) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

		for i, col := range cols {
			switch col.Type {
			case funcapi.FieldTypeString:
				var v sql.NullString
				values[i] = &v
			case funcapi.FieldTypeInteger:
				var v sql.NullInt64
				values[i] = &v
			case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
				var v sql.NullFloat64
				values[i] = &v
			default:
				var v any
				values[i] = &v
			}
			valuePtrs[i] = values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}

		// Convert scanned values to output format
		row := make([]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case *sql.NullString:
				if v.Valid {
					s := v.String
					// Truncate query text
					if col.Name == "query" {
						s = strmutil.TruncateText(s, maxQueryTextLength)
					}
					row[i] = s
				} else {
					row[i] = ""
				}
			case *sql.NullInt64:
				if v.Valid {
					row[i] = v.Int64
				} else {
					row[i] = int64(0)
				}
			case *sql.NullFloat64:
				if v.Valid {
					row[i] = v.Float64
				} else {
					row[i] = float64(0)
				}
			default:
				row[i] = nil
			}
		}
		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return data, nil
}

// buildMSSQLDynamicSortOptions builds sort options from available columns
// Returns only sort options for columns that actually exist in the database
func (c *Collector) buildMSSQLDynamicSortOptions(cols []mssqlColumn) []funcapi.ParamOption {
	var sortOpts []funcapi.ParamOption
	seen := make(map[string]bool)
	sortDir := funcapi.FieldSortDescending

	for _, col := range cols {
		if col.IsSortOption && !seen[col.Name] {
			seen[col.Name] = true
			sortOpts = append(sortOpts, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    col.SortLabel,
				Default: col.IsDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOpts
}

func (c *Collector) topQueriesSortParam(cols []mssqlColumn) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := c.buildMSSQLDynamicSortOptions(cols)
	sortParam := funcapi.ParamConfig{
		ID:         "__sort",
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    sortOptions,
		UniqueView: true,
	}
	return sortParam, sortOptions
}

func (c *Collector) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	if !c.Config.GetQueryStoreFunctionEnabled() {
		return nil, fmt.Errorf("query store function disabled")
	}

	availableCols, err := c.detectMSSQLQueryStoreColumns(ctx)
	if err != nil {
		return nil, err
	}

	cols := c.buildAvailableMSSQLColumns(availableCols)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in Query Store")
	}

	sortParam, _ := c.topQueriesSortParam(cols)
	return []funcapi.ParamConfig{sortParam}, nil
}

// collectTopQueries queries Query Store for top queries using dynamic columns
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check if function is enabled
	if !c.Config.GetQueryStoreFunctionEnabled() {
		return &module.FunctionResponse{
			Status: 403,
			Message: "Query Store function has been disabled in configuration. " +
				"To enable, set query_store_function_enabled: true in the MSSQL collector config.",
		}
	}

	// Detect available columns
	availableCols, err := c.detectMSSQLQueryStoreColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	// Build list of available columns
	cols := c.buildAvailableMSSQLColumns(availableCols)
	if len(cols) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: "no columns available in Query Store",
		}
	}

	// Validate and map sort column (use filtered cols to ensure sort column is in SELECT)
	validatedSortColumn := c.mapAndValidateMSSQLSortColumn(sortColumn, cols)

	// Build and execute query
	timeWindowDays := c.Config.GetQueryStoreTimeWindowDays()
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}
	query := c.buildMSSQLDynamicSQL(cols, validatedSortColumn, timeWindowDays, limit)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		// Include diagnostic info: which columns were detected and used
		colIDs := make([]string, len(cols))
		for i, col := range cols {
			colIDs[i] = col.Name
		}
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("query failed: %v (sort: %s, detected cols: %v)", err, validatedSortColumn, colIDs),
		}
	}
	defer rows.Close()

	// Scan rows dynamically
	data, err := c.scanMSSQLDynamicRows(rows, cols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	// Build dynamic sort options from available columns (only those actually detected)
	sortParam, sortOptions := c.topQueriesSortParam(cols)

	// Find default sort column ID
	defaultSort := ""
	for _, col := range cols {
		if col.IsDefaultSort && col.IsSortOption {
			defaultSort = col.Name
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := decorateMSSQLColumns(cols)
	cs := mssqlColumnSet(annotatedCols)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},

		// Charts for aggregated visualization
		Charts:        cs.BuildCharts(),
		DefaultCharts: cs.BuildDefaultCharts(),
		GroupBy:       cs.BuildGroupBy(),
	}
}

func decorateMSSQLColumns(cols []mssqlColumn) []mssqlColumn {
	out := make([]mssqlColumn, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.Name] = i
	}

	for i := range out {
		if mssqlLabelColumnIDs[out[i].Name] {
			out[i].GroupBy = &funcapi.GroupByOptions{
				IsDefault: out[i].Name == mssqlPrimaryLabelID,
			}
		}
	}

	for _, group := range mssqlChartGroupDefs {
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
