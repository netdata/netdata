// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
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
)

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             topQueriesMethodID,
		Name:           "Top Queries",
		UpdateEvery:    10,
		Help:           "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(topQueriesColumns)},
	}
}

// topQueriesColumn embeds funcapi.ColumnMeta and adds MSSQL-specific fields.
type topQueriesColumn struct {
	funcapi.ColumnMeta
	DBColumn       string // Column name in sys.query_store_runtime_stats
	IsMicroseconds bool   // Needs microseconds to milliseconds conversion
	sortOpt        bool   // Show in sort dropdown
	sortLbl        string // Label for sort option
	defaultSort    bool   // Is this the default sort option
	IsIdentity     bool   // Is this an identity column (query_hash, query_text, etc.)
	NeedsAvg       bool   // Needs weighted average calculation (avg_* columns)
}

// topQueriesColumns defines ALL possible columns from Query Store.
// Columns that don't exist in certain SQL Server versions will be filtered at runtime.
var topQueriesColumns = []topQueriesColumn{
	// Identity columns - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Tooltip: "Query Hash", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true}, DBColumn: "query_hash", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true}, DBColumn: "query_sql_text", IsIdentity: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect}, DBColumn: "database_name", IsIdentity: true},

	// Execution count - always available
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "count_executions", sortOpt: true, sortLbl: "Top queries by Number of Calls"},

	// Duration metrics (microseconds -> milliseconds) - SQL 2016+
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", IsMicroseconds: true, sortOpt: true, sortLbl: "Top queries by Total Execution Time", defaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgTime", Tooltip: "Avg Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_duration", IsMicroseconds: true, NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastTime", Tooltip: "Last Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_duration", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevTime", Tooltip: "StdDev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_duration", IsMicroseconds: true},

	// CPU time metrics (microseconds -> milliseconds)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgCpu", Tooltip: "Avg CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: true, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_cpu_time", IsMicroseconds: true, NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average CPU Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastCpu", Tooltip: "Last CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minCpu", Tooltip: "Min CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxCpu", Tooltip: "Max CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_cpu_time", IsMicroseconds: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevCpu", Tooltip: "StdDev CPU", Type: funcapi.FieldTypeDuration, Units: "milliseconds", DecimalPoints: 2, Visible: false, Transform: funcapi.FieldTransformDuration, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_cpu_time", IsMicroseconds: true},

	// Logical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgReads", Tooltip: "Avg Logical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_reads", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Logical Reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastReads", Tooltip: "Last Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minReads", Tooltip: "Min Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxReads", Tooltip: "Max Logical Reads", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_reads"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevReads", Tooltip: "StdDev Logical Reads", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_reads"},

	// Logical I/O writes
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgWrites", Tooltip: "Avg Logical Writes", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_logical_io_writes", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Logical Writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastWrites", Tooltip: "Last Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minWrites", Tooltip: "Min Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxWrites", Tooltip: "Max Logical Writes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_logical_io_writes"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevWrites", Tooltip: "StdDev Logical Writes", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_logical_io_writes"},

	// Physical I/O reads
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgPhysReads", Tooltip: "Avg Physical Reads", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_physical_io_reads", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Physical Reads"},
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
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgDop", Tooltip: "Avg DOP", Type: funcapi.FieldTypeFloat, DecimalPoints: 1, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_dop", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Parallelism"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastDop", Tooltip: "Last DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minDop", Tooltip: "Min DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxDop", Tooltip: "Max DOP", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_dop"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevDop", Tooltip: "StdDev DOP", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_dop"},

	// Memory grant (8KB pages)
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgMemory", Tooltip: "Avg Memory (8KB pages)", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_query_max_used_memory", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Memory Grant"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastMemory", Tooltip: "Last Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minMemory", Tooltip: "Min Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxMemory", Tooltip: "Max Memory (8KB pages)", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_query_max_used_memory"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stdevMemory", Tooltip: "StdDev Memory", Type: funcapi.FieldTypeFloat, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "stdev_query_max_used_memory"},

	// Row count
	{ColumnMeta: funcapi.ColumnMeta{Name: "avgRows", Tooltip: "Avg Rows", Type: funcapi.FieldTypeFloat, DecimalPoints: 0, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMean, Filter: funcapi.FieldFilterRange}, DBColumn: "avg_rowcount", NeedsAvg: true, sortOpt: true, sortLbl: "Top queries by Average Row Count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "lastRows", Tooltip: "Last Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "last_rowcount"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minRows", Tooltip: "Min Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange}, DBColumn: "min_rowcount"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxRows", Tooltip: "Max Rows", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange}, DBColumn: "max_rowcount"},
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
		return f.methodParams(ctx)
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

	availableCols, err := f.detectQueryStoreColumns(ctx)
	if err != nil {
		return nil, err
	}

	cols := f.buildAvailableColumns(availableCols)
	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns available in Query Store")
	}

	sortParam, _ := f.buildSortParam(cols)
	return []funcapi.ParamConfig{sortParam}, nil
}

func (f *funcTopQueries) collectData(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	if f.router.collector.Functions.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}

	availableCols, err := f.detectQueryStoreColumns(ctx)
	if err != nil {
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	cols := f.buildAvailableColumns(availableCols)
	if len(cols) == 0 {
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: "no columns available in Query Store",
		}
	}

	validatedSortColumn := f.mapAndValidateSortColumn(sortColumn, cols)

	timeWindowDays := f.router.collector.topQueriesTimeWindowDays()
	limit := f.router.collector.topQueriesLimit()
	query := f.buildDynamicSQL(cols, validatedSortColumn, timeWindowDays, limit)

	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &funcapi.FunctionResponse{Status: 504, Message: "query timed out"}
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
		return &funcapi.FunctionResponse{Status: 500, Message: err.Error()}
	}

	errorStatus, errorDetails := f.router.collector.collectMSSQLErrorDetails(ctx)
	planOpsByDB := f.router.collector.collectMSSQLPlanOps(ctx, data, cols)
	extraCols := append(mssqlErrorAttributionColumns(), mssqlPlanAttributionColumns()...)

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

	if len(extraCols) > 0 {
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
	}

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
		Help:              "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
	}
}

func (f *funcTopQueries) columnSet(cols []topQueriesColumn) funcapi.ColumnSet[topQueriesColumn] {
	return funcapi.Columns(cols, func(c topQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcTopQueries) detectQueryStoreColumns(ctx context.Context) (map[string]bool, error) {
	// Fast path: return cached result
	f.router.collector.queryStoreColsMu.RLock()
	if f.router.collector.queryStoreCols != nil {
		cols := f.router.collector.queryStoreCols
		f.router.collector.queryStoreColsMu.RUnlock()
		return cols, nil
	}
	f.router.collector.queryStoreColsMu.RUnlock()

	// Slow path: query and cache
	f.router.collector.queryStoreColsMu.Lock()
	defer f.router.collector.queryStoreColsMu.Unlock()

	// Double-check after acquiring write lock
	if f.router.collector.queryStoreCols != nil {
		return f.router.collector.queryStoreCols, nil
	}

	// Find any database with Query Store enabled (excluding system databases)
	var sampleDB string
	err := f.router.collector.db.QueryRowContext(ctx, `
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
	query := fmt.Sprintf(`SELECT TOP 0 * FROM [%s].sys.query_store_runtime_stats`, sampleDB)
	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
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

	f.router.collector.queryStoreCols = cols

	return cols, nil
}

func (f *funcTopQueries) buildAvailableColumns(availableCols map[string]bool) []topQueriesColumn {
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
		if availableCols[col.DBColumn] {
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

func (f *funcTopQueries) buildSelectExpressions(cols []topQueriesColumn, dbNameExpr string) []string {
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
			expr = fmt.Sprintf("SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS [%s]", col.Name)
		case col.NeedsAvg && col.IsMicroseconds:
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) / 1000.0 ELSE 0 END AS [%s]", col.DBColumn, col.Name)
		case col.NeedsAvg:
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) ELSE 0 END AS [%s]", col.DBColumn, col.Name)
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
			if strings.HasPrefix(col.DBColumn, "stdev_") {
				aggFunc = "MAX"
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
	selectParts := f.buildSelectExpressions(cols, "''' + name + N'''")
	selectExpr := strings.Join(selectParts, ",\n        ")

	timeFilter := ""
	if timeWindowDays > 0 {
		timeFilter = fmt.Sprintf("WHERE rsi.start_time >= DATEADD(day, -%d, GETUTCDATE())", timeWindowDays)
	}

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
