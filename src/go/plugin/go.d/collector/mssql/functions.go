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

const (
	paramSort = "__sort"

	ftString   = funcapi.FieldTypeString
	ftInteger  = funcapi.FieldTypeInteger
	ftFloat    = funcapi.FieldTypeFloat
	ftDuration = funcapi.FieldTypeDuration

	trNone     = funcapi.FieldTransformNone
	trNumber   = funcapi.FieldTransformNumber
	trDuration = funcapi.FieldTransformDuration

	sortAsc  = funcapi.FieldSortAscending
	sortDesc = funcapi.FieldSortDescending

	summaryCount = funcapi.FieldSummaryCount
	summarySum   = funcapi.FieldSummarySum
	summaryMin   = funcapi.FieldSummaryMin
	summaryMax   = funcapi.FieldSummaryMax
	summaryMean  = funcapi.FieldSummaryMean

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

// mssqlColumnMeta defines metadata for a single column
type mssqlColumnMeta struct {
	dbColumn       string                 // Column name in sys.query_store_runtime_stats
	uiKey          string                 // Canonical name used everywhere: SQL alias, UI key, sort key
	displayName    string                 // Display name in UI
	dataType       funcapi.FieldType      // "string", "integer", "float", "duration"
	units          string                 // Unit for duration types
	visible        bool                   // Default visibility
	transform      funcapi.FieldTransform // Transform for value_options
	decimalPoints  int                    // Decimal points for display
	sortDir        funcapi.FieldSort      // Sort direction: "ascending" or "descending"
	summary        funcapi.FieldSummary   // Summary function
	filter         funcapi.FieldFilter    // Filter type
	isMicroseconds bool                   // Needs μs to milliseconds conversion
	isSortOption   bool                   // Show in sort dropdown
	sortLabel      string                 // Label for sort option
	isDefaultSort  bool                   // Is this the default sort option
	isUniqueKey    bool                   // Is this column a unique key
	isSticky       bool                   // Is this column sticky in UI
	fullWidth      bool                   // Should column take full width
	isIdentity     bool                   // Is this an identity column (query_hash, query_text, etc.)
	needsAvg       bool                   // Needs weighted average calculation (avg_* columns)
	isLabel        bool                   // Is this column a label for grouping
	isPrimary      bool                   // Is this label the primary grouping
	isMetric       bool                   // Is this column a chartable metric
	chartGroup     string                 // Chart group key
	chartTitle     string                 // Chart title
	isDefaultChart bool                   // Include this chart group in defaults
}

// mssqlAllColumns defines ALL possible columns from Query Store
// Columns that don't exist in certain SQL Server versions will be filtered at runtime
var mssqlAllColumns = []mssqlColumnMeta{
	// Identity columns - always available
	{dbColumn: "query_hash", uiKey: "queryHash", displayName: "Query Hash", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isUniqueKey: true, isIdentity: true},
	{dbColumn: "query_sql_text", uiKey: "query", displayName: "Query", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isSticky: true, fullWidth: true, isIdentity: true},
	{dbColumn: "database_name", uiKey: "database", displayName: "Database", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isIdentity: true},

	// Execution count - always available
	{dbColumn: "count_executions", uiKey: "calls", displayName: "Calls", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Top queries by Number of Calls"},

	// Duration metrics (microseconds -> milliseconds) - SQL 2016+
	{dbColumn: "avg_duration", uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isMicroseconds: true, isSortOption: true, sortLabel: "Top queries by Total Execution Time", isDefaultSort: true},
	{dbColumn: "avg_duration", uiKey: "avgTime", displayName: "Avg Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isMicroseconds: true, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Execution Time"},
	{dbColumn: "last_duration", uiKey: "lastTime", displayName: "Last Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "min_duration", uiKey: "minTime", displayName: "Min Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, isMicroseconds: true},
	{dbColumn: "max_duration", uiKey: "maxTime", displayName: "Max Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "stdev_duration", uiKey: "stdevTime", displayName: "StdDev Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},

	// CPU time metrics (microseconds -> milliseconds)
	{dbColumn: "avg_cpu_time", uiKey: "avgCpu", displayName: "Avg CPU", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isMicroseconds: true, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average CPU Time"},
	{dbColumn: "last_cpu_time", uiKey: "lastCpu", displayName: "Last CPU", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "min_cpu_time", uiKey: "minCpu", displayName: "Min CPU", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, isMicroseconds: true},
	{dbColumn: "max_cpu_time", uiKey: "maxCpu", displayName: "Max CPU", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "stdev_cpu_time", uiKey: "stdevCpu", displayName: "StdDev CPU", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},

	// Logical I/O reads
	{dbColumn: "avg_logical_io_reads", uiKey: "avgReads", displayName: "Avg Logical Reads", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Logical Reads"},
	{dbColumn: "last_logical_io_reads", uiKey: "lastReads", displayName: "Last Logical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_logical_io_reads", uiKey: "minReads", displayName: "Min Logical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_logical_io_reads", uiKey: "maxReads", displayName: "Max Logical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_logical_io_reads", uiKey: "stdevReads", displayName: "StdDev Logical Reads", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Logical I/O writes
	{dbColumn: "avg_logical_io_writes", uiKey: "avgWrites", displayName: "Avg Logical Writes", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Logical Writes"},
	{dbColumn: "last_logical_io_writes", uiKey: "lastWrites", displayName: "Last Logical Writes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_logical_io_writes", uiKey: "minWrites", displayName: "Min Logical Writes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_logical_io_writes", uiKey: "maxWrites", displayName: "Max Logical Writes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_logical_io_writes", uiKey: "stdevWrites", displayName: "StdDev Logical Writes", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Physical I/O reads
	{dbColumn: "avg_physical_io_reads", uiKey: "avgPhysReads", displayName: "Avg Physical Reads", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Physical Reads"},
	{dbColumn: "last_physical_io_reads", uiKey: "lastPhysReads", displayName: "Last Physical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_physical_io_reads", uiKey: "minPhysReads", displayName: "Min Physical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_physical_io_reads", uiKey: "maxPhysReads", displayName: "Max Physical Reads", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_physical_io_reads", uiKey: "stdevPhysReads", displayName: "StdDev Physical Reads", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// CLR time (microseconds -> milliseconds)
	{dbColumn: "avg_clr_time", uiKey: "avgClr", displayName: "Avg CLR Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMean, filter: filterRange, isMicroseconds: true, needsAvg: true},
	{dbColumn: "last_clr_time", uiKey: "lastClr", displayName: "Last CLR Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "min_clr_time", uiKey: "minClr", displayName: "Min CLR Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange, isMicroseconds: true},
	{dbColumn: "max_clr_time", uiKey: "maxClr", displayName: "Max CLR Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},
	{dbColumn: "stdev_clr_time", uiKey: "stdevClr", displayName: "StdDev CLR Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isMicroseconds: true},

	// DOP (degree of parallelism)
	{dbColumn: "avg_dop", uiKey: "avgDop", displayName: "Avg DOP", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 1, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Parallelism"},
	{dbColumn: "last_dop", uiKey: "lastDop", displayName: "Last DOP", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_dop", uiKey: "minDop", displayName: "Min DOP", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_dop", uiKey: "maxDop", displayName: "Max DOP", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_dop", uiKey: "stdevDop", displayName: "StdDev DOP", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Memory grant (8KB pages)
	{dbColumn: "avg_query_max_used_memory", uiKey: "avgMemory", displayName: "Avg Memory (8KB pages)", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Memory Grant"},
	{dbColumn: "last_query_max_used_memory", uiKey: "lastMemory", displayName: "Last Memory (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_query_max_used_memory", uiKey: "minMemory", displayName: "Min Memory (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_query_max_used_memory", uiKey: "maxMemory", displayName: "Max Memory (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_query_max_used_memory", uiKey: "stdevMemory", displayName: "StdDev Memory", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Row count
	{dbColumn: "avg_rowcount", uiKey: "avgRows", displayName: "Avg Rows", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Row Count"},
	{dbColumn: "last_rowcount", uiKey: "lastRows", displayName: "Last Rows", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_rowcount", uiKey: "minRows", displayName: "Min Rows", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_rowcount", uiKey: "maxRows", displayName: "Max Rows", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_rowcount", uiKey: "stdevRows", displayName: "StdDev Rows", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// SQL Server 2017+ log bytes
	{dbColumn: "avg_log_bytes_used", uiKey: "avgLogBytes", displayName: "Avg Log Bytes", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average Log Bytes"},
	{dbColumn: "last_log_bytes_used", uiKey: "lastLogBytes", displayName: "Last Log Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_log_bytes_used", uiKey: "minLogBytes", displayName: "Min Log Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_log_bytes_used", uiKey: "maxLogBytes", displayName: "Max Log Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_log_bytes_used", uiKey: "stdevLogBytes", displayName: "StdDev Log Bytes", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// SQL Server 2017+ tempdb space
	{dbColumn: "avg_tempdb_space_used", uiKey: "avgTempdb", displayName: "Avg TempDB (8KB pages)", dataType: ftFloat, visible: true, transform: trNumber, decimalPoints: 0, sortDir: sortDesc, summary: summaryMean, filter: filterRange, needsAvg: true, isSortOption: true, sortLabel: "Top queries by Average TempDB Usage"},
	{dbColumn: "last_tempdb_space_used", uiKey: "lastTempdb", displayName: "Last TempDB (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_tempdb_space_used", uiKey: "minTempdb", displayName: "Min TempDB (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_tempdb_space_used", uiKey: "maxTempdb", displayName: "Max TempDB (8KB pages)", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stdev_tempdb_space_used", uiKey: "stdevTempdb", displayName: "StdDev TempDB", dataType: ftFloat, visible: false, transform: trNumber, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
}

type mssqlChartGroup struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var mssqlChartGroups = []mssqlChartGroup{
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

var mssqlLabelColumns = map[string]bool{
	"database": true,
}

const mssqlPrimaryLabel = "database"

// mssqlMethods returns the available function methods for MSSQL
func mssqlMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	seen := make(map[string]bool) // Avoid duplicates from totalTime/avgTime using same dbColumn
	for _, col := range mssqlAllColumns {
		if col.isSortOption && !seen[col.uiKey] {
			seen[col.uiKey] = true
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.uiKey, // Use UI key for sort, we'll map internally
				Name:    col.sortLabel,
				Default: col.isDefaultSort,
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
					ID:         paramSort,
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
		return collector.collectTopQueries(ctx, params.Column(paramSort))
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
func (c *Collector) buildAvailableMSSQLColumns(availableCols map[string]bool) []mssqlColumnMeta {
	var cols []mssqlColumnMeta
	seen := make(map[string]bool)

	for _, col := range mssqlAllColumns {
		// Skip duplicates (e.g., totalTime and avgTime both use avg_duration)
		if seen[col.uiKey] {
			continue
		}
		// Identity columns are always available
		if col.isIdentity {
			cols = append(cols, col)
			seen[col.uiKey] = true
			continue
		}
		// Check if the dbColumn exists
		if availableCols[col.dbColumn] {
			cols = append(cols, col)
			seen[col.uiKey] = true
		}
	}
	return cols
}

// mapAndValidateMSSQLSortColumn maps UI sort key to the appropriate sort expression
// Uses the filtered cols list to ensure the sort column is actually in the SELECT
func (c *Collector) mapAndValidateMSSQLSortColumn(sortKey string, cols []mssqlColumnMeta) string {
	// First, check if the requested sort key is in the available columns
	for _, col := range cols {
		if col.uiKey == sortKey && col.isSortOption {
			return col.uiKey
		}
	}

	// Fall back to the first available sort column
	for _, col := range cols {
		if col.isSortOption {
			return col.uiKey
		}
	}

	// Last resort: use first non-identity column
	for _, col := range cols {
		if !col.isIdentity {
			return col.uiKey
		}
	}

	// Absolute fallback: use first column in the list (must exist in SELECT)
	if len(cols) > 0 {
		return cols[0].uiKey
	}

	return "" // empty - will be handled by caller
}

// buildMSSQLSelectExpressions builds the SELECT expressions for a single database query
func (c *Collector) buildMSSQLSelectExpressions(cols []mssqlColumnMeta, dbNameExpr string) []string {
	var selectParts []string

	for _, col := range cols {
		var expr string
		switch {
		case col.isIdentity:
			switch col.uiKey {
			case "queryHash":
				expr = fmt.Sprintf("CONVERT(VARCHAR(64), q.query_hash, 1) AS [%s]", col.uiKey)
			case "query":
				expr = fmt.Sprintf("qt.query_sql_text AS [%s]", col.uiKey)
			case "database":
				expr = fmt.Sprintf("%s AS [%s]", dbNameExpr, col.uiKey)
			}
		case col.uiKey == "calls":
			expr = fmt.Sprintf("SUM(rs.count_executions) AS [%s]", col.uiKey)
		case col.uiKey == "totalTime":
			// Total time = sum of (avg_duration * executions) converted to milliseconds
			expr = fmt.Sprintf("SUM(rs.avg_duration * rs.count_executions) / 1000.0 AS [%s]", col.uiKey)
		case col.needsAvg && col.isMicroseconds:
			// Weighted average with μs to milliseconds conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) / 1000.0 ELSE 0 END AS [%s]", col.dbColumn, col.uiKey)
		case col.needsAvg:
			// Weighted average without time conversion
			expr = fmt.Sprintf("CASE WHEN SUM(rs.count_executions) > 0 THEN SUM(rs.%s * rs.count_executions) / SUM(rs.count_executions) ELSE 0 END AS [%s]", col.dbColumn, col.uiKey)
		case col.isMicroseconds:
			// Aggregate with μs to milliseconds conversion
			aggFunc := "MAX"
			if strings.HasPrefix(col.dbColumn, "min_") {
				aggFunc = "MIN"
			}
			expr = fmt.Sprintf("%s(rs.%s) / 1000.0 AS [%s]", aggFunc, col.dbColumn, col.uiKey)
		default:
			// Simple aggregate
			aggFunc := "MAX"
			if strings.HasPrefix(col.dbColumn, "min_") {
				aggFunc = "MIN"
			}
			if strings.HasPrefix(col.dbColumn, "stdev_") {
				aggFunc = "MAX" // Use MAX for stddev aggregation
			}
			expr = fmt.Sprintf("%s(rs.%s) AS [%s]", aggFunc, col.dbColumn, col.uiKey)
		}
		if expr != "" {
			selectParts = append(selectParts, expr)
		}
	}
	return selectParts
}

// buildMSSQLDynamicSQL builds dynamic SQL that aggregates across all databases with Query Store enabled
// Uses sp_executesql to execute the built query
func (c *Collector) buildMSSQLDynamicSQL(cols []mssqlColumnMeta, sortColumn string, timeWindowDays int, limit int) string {
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
			if !col.isIdentity {
				orderByExpr = col.uiKey
				break
			}
		}
		if orderByExpr == "" && len(cols) > 0 {
			orderByExpr = cols[0].uiKey
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
func (c *Collector) scanMSSQLDynamicRows(rows mssqlRowScanner, cols []mssqlColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

		for i, col := range cols {
			switch col.dataType {
			case ftString:
				var v sql.NullString
				values[i] = &v
			case ftInteger:
				var v sql.NullInt64
				values[i] = &v
			case ftFloat, ftDuration:
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
					if col.uiKey == "query" {
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
func (c *Collector) buildMSSQLDynamicSortOptions(cols []mssqlColumnMeta) []funcapi.ParamOption {
	var sortOpts []funcapi.ParamOption
	seen := make(map[string]bool)
	sortDir := funcapi.FieldSortDescending

	for _, col := range cols {
		if col.isSortOption && !seen[col.uiKey] {
			seen[col.uiKey] = true
			sortOpts = append(sortOpts, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.uiKey,
				Name:    col.sortLabel,
				Default: col.isDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOpts
}

func (c *Collector) topQueriesSortParam(cols []mssqlColumnMeta) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := c.buildMSSQLDynamicSortOptions(cols)
	sortParam := funcapi.ParamConfig{
		ID:         paramSort,
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

// buildMSSQLDynamicColumns builds column definitions for the response
func (c *Collector) buildMSSQLDynamicColumns(cols []mssqlColumnMeta) map[string]any {
	columns := make(map[string]any)
	for i, col := range cols {
		visual := funcapi.FieldVisualValue
		if col.dataType == ftDuration {
			visual = funcapi.FieldVisualBar
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.displayName,
			Type:                  col.dataType,
			Units:                 col.units,
			Visualization:         visual,
			Sort:                  col.sortDir,
			Sortable:              true,
			Sticky:                col.isSticky,
			Summary:               col.summary,
			Filter:                col.filter,
			FullWidth:             col.fullWidth,
			Wrap:                  false,
			DefaultExpandedFilter: false,
			UniqueKey:             col.isUniqueKey,
			Visible:               col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimalPoints,
				DefaultValue:  nil,
			},
		}
		columns[col.uiKey] = colDef.BuildColumn()
	}
	return columns
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
		colUIKeys := make([]string, len(cols))
		for i, col := range cols {
			colUIKeys[i] = col.uiKey
		}
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("query failed: %v (sort: %s, detected cols: %v)", err, validatedSortColumn, colUIKeys),
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

	// Find default sort column UI key
	defaultSort := ""
	for _, col := range cols {
		if col.isDefaultSort && col.isSortOption {
			defaultSort = col.uiKey
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := decorateMSSQLColumns(cols)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from Query Store. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           c.buildMSSQLDynamicColumns(cols),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},

		// Charts for aggregated visualization
		Charts:        mssqlTopQueriesCharts(annotatedCols),
		DefaultCharts: mssqlTopQueriesDefaultCharts(annotatedCols),
		GroupBy:       mssqlTopQueriesGroupBy(annotatedCols),
	}
}

func decorateMSSQLColumns(cols []mssqlColumnMeta) []mssqlColumnMeta {
	out := make([]mssqlColumnMeta, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.uiKey] = i
	}

	for i := range out {
		if mssqlLabelColumns[out[i].uiKey] {
			out[i].isLabel = true
			if out[i].uiKey == mssqlPrimaryLabel {
				out[i].isPrimary = true
			}
		}
	}

	for _, group := range mssqlChartGroups {
		for _, key := range group.columns {
			idx, ok := index[key]
			if !ok {
				continue
			}
			out[idx].isMetric = true
			out[idx].chartGroup = group.key
			out[idx].chartTitle = group.title
			if group.defaultChart {
				out[idx].isDefaultChart = true
			}
		}
	}

	return out
}

func mssqlTopQueriesCharts(cols []mssqlColumnMeta) map[string]module.ChartConfig {
	charts := make(map[string]module.ChartConfig)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		cfg, ok := charts[col.chartGroup]
		if !ok {
			title := col.chartTitle
			if title == "" {
				title = col.chartGroup
			}
			cfg = module.ChartConfig{Name: title, Type: "stacked-bar"}
		}
		cfg.Columns = append(cfg.Columns, col.uiKey)
		charts[col.chartGroup] = cfg
	}
	return charts
}

func mssqlTopQueriesDefaultCharts(cols []mssqlColumnMeta) [][]string {
	label := primaryMSSQLLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultMSSQLChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func mssqlTopQueriesGroupBy(cols []mssqlColumnMeta) map[string]module.GroupByConfig {
	groupBy := make(map[string]module.GroupByConfig)
	for _, col := range cols {
		if !col.isLabel {
			continue
		}
		groupBy[col.uiKey] = module.GroupByConfig{
			Name:    "Group by " + col.displayName,
			Columns: []string{col.uiKey},
		}
	}
	return groupBy
}

func primaryMSSQLLabel(cols []mssqlColumnMeta) string {
	for _, col := range cols {
		if col.isPrimary {
			return col.uiKey
		}
	}
	for _, col := range cols {
		if col.isLabel {
			return col.uiKey
		}
	}
	return ""
}

func defaultMSSQLChartGroups(cols []mssqlColumnMeta) []string {
	groups := make([]string, 0)
	seen := make(map[string]bool)
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" || !col.isDefaultChart {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	if len(groups) > 0 {
		return groups
	}
	for _, col := range cols {
		if !col.isMetric || col.chartGroup == "" {
			continue
		}
		if !seen[col.chartGroup] {
			seen[col.chartGroup] = true
			groups = append(groups, col.chartGroup)
		}
	}
	return groups
}
