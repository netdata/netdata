// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	topQueriesMethodID = "top-queries"
	maxQueryTextLength = 4096
	paramSort          = "__sort"
)

// queryStatsSourceName is the type for query stats source
type queryStatsSourceName string

const (
	queryStatsSourcePgStatMonitor    queryStatsSourceName = "pg_stat_monitor"
	queryStatsSourcePgStatStatements queryStatsSourceName = "pg_stat_statements"
	queryStatsSourceNone             queryStatsSourceName = ""
)

// getQueryStatsSource detects and returns the best available query stats source.
// Prefers pg_stat_monitor if available, falls back to pg_stat_statements.
// Result is cached after first detection.
func (f *funcTopQueries) getQueryStatsSource(ctx context.Context) (queryStatsSourceName, error) {
	c := f.router.collector

	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	source := c.queryStatsSource
	c.pgStatStatementsMu.RUnlock()
	if source != "" {
		return queryStatsSourceName(source), nil
	}

	// Slow path: detect and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.queryStatsSource != "" {
		return queryStatsSourceName(c.queryStatsSource), nil
	}

	// Check pg_stat_monitor first (preferred)
	var hasPgStatMonitor bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_monitor')`
	if err := c.db.QueryRowContext(ctx, query).Scan(&hasPgStatMonitor); err != nil {
		return queryStatsSourceNone, fmt.Errorf("failed to check pg_stat_monitor: %v", err)
	}
	if hasPgStatMonitor {
		c.queryStatsSource = string(queryStatsSourcePgStatMonitor)
		c.pgStatMonitorAvail = true
		return queryStatsSourcePgStatMonitor, nil
	}

	// Fall back to pg_stat_statements
	var hasPgStatStatements bool
	query = `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	if err := c.db.QueryRowContext(ctx, query).Scan(&hasPgStatStatements); err != nil {
		return queryStatsSourceNone, fmt.Errorf("failed to check pg_stat_statements: %v", err)
	}
	if hasPgStatStatements {
		c.queryStatsSource = string(queryStatsSourcePgStatStatements)
		c.pgStatStatementsAvail = true
		return queryStatsSourcePgStatStatements, nil
	}

	return queryStatsSourceNone, nil
}

// detectPgStatStatementsColumns queries the database to find available columns
func (f *funcTopQueries) detectPgStatStatementsColumns(ctx context.Context) (map[string]bool, error) {
	c := f.router.collector

	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	if c.pgStatStatementsColumns != nil {
		cols := c.pgStatStatementsColumns
		c.pgStatStatementsMu.RUnlock()
		return cols, nil
	}
	c.pgStatStatementsMu.RUnlock()

	// Slow path: query and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.pgStatStatementsColumns != nil {
		return c.pgStatStatementsColumns, nil
	}

	cols, err := sqlquery.FetchTableColumns(
		ctx,
		c.db,
		"public",
		"pg_stat_statements",
		sqlquery.PlaceholderDollar,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}

	// Cache the result
	c.pgStatStatementsColumns = cols
	return cols, nil
}

// detectPgStatMonitorColumns queries the database to find available columns
func (f *funcTopQueries) detectPgStatMonitorColumns(ctx context.Context) (map[string]bool, error) {
	c := f.router.collector

	// Fast path: return cached result
	c.pgStatStatementsMu.RLock()
	if c.pgStatMonitorColumns != nil {
		cols := c.pgStatMonitorColumns
		c.pgStatStatementsMu.RUnlock()
		return cols, nil
	}
	c.pgStatStatementsMu.RUnlock()

	// Slow path: query and cache
	c.pgStatStatementsMu.Lock()
	defer c.pgStatStatementsMu.Unlock()

	// Double-check after acquiring write lock
	if c.pgStatMonitorColumns != nil {
		return c.pgStatMonitorColumns, nil
	}

	cols, err := sqlquery.FetchTableColumns(
		ctx,
		c.db,
		"public",
		"pg_stat_monitor",
		sqlquery.PlaceholderDollar,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}

	// Cache the result
	c.pgStatMonitorColumns = cols
	return cols, nil
}

// pgColumn defines metadata for a pg_stat_statements/pg_stat_monitor column.
// Embeds funcapi.ColumnMeta for UI rendering and adds PG-specific fields.
type pgColumn struct {
	funcapi.ColumnMeta

	// DBColumn is the database column expression (e.g., "s.queryid::text", "d.datname")
	DBColumn string
	// IsSortOption indicates whether this column appears in the sort dropdown
	IsSortOption bool
	// SortLabel is the label shown in the sort dropdown (if IsSortOption)
	SortLabel string
	// IsDefaultSort indicates whether this is the default sort column
	IsDefaultSort bool
	// OnlyPgStatMonitor indicates this column only exists in pg_stat_monitor
	OnlyPgStatMonitor bool
}

// pgColumnSet creates a ColumnSet from a slice of pgColumn.
func pgColumnSet(cols []pgColumn) funcapi.ColumnSet[pgColumn] {
	return funcapi.Columns(cols, func(c pgColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

// pgAllColumns defines ALL possible columns from pg_stat_statements.
// Order matters - this determines column index in the response.
var pgAllColumns = []pgColumn{
	// Core identification columns (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryid", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, UniqueKey: true, Sortable: true}, DBColumn: "s.queryid::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, FullWidth: true, Sortable: true}, DBColumn: "s.query"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "database", Tooltip: "Database", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "d.datname"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "u.usename"},

	// Execution count (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Tooltip: "Calls", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.calls", IsSortOption: true, SortLabel: "Number of Calls"},

	// Execution time columns (names vary by version - detected dynamically)
	// PG <13: total_time, mean_time, min_time, max_time, stddev_time
	// PG 13+: total_exec_time, mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "total_time", IsSortOption: true, SortLabel: "Total Execution Time", IsDefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "meanTime", Tooltip: "Mean Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "mean_time", IsSortOption: true, SortLabel: "Average Execution Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minTime", Tooltip: "Min Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "min_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxTime", Tooltip: "Max Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "max_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stddevTime", Tooltip: "Stddev Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "stddev_time"},

	// Planning time columns (PG 13+ only)
	{ColumnMeta: funcapi.ColumnMeta{Name: "plans", Tooltip: "Plans", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.plans"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "totalPlanTime", Tooltip: "Total Plan Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "total_plan_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "meanPlanTime", Tooltip: "Mean Plan Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "mean_plan_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "minPlanTime", Tooltip: "Min Plan Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "min_plan_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "maxPlanTime", Tooltip: "Max Plan Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "max_plan_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stddevPlanTime", Tooltip: "Stddev Plan Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "stddev_plan_time"},

	// Row count (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "rows", Tooltip: "Rows", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.rows", IsSortOption: true, SortLabel: "Rows Returned"},

	// Shared buffer statistics (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "sharedBlksHit", Tooltip: "Shared Blocks Hit", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.shared_blks_hit", IsSortOption: true, SortLabel: "Shared Blocks Hit (Cache)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sharedBlksRead", Tooltip: "Shared Blocks Read", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.shared_blks_read", IsSortOption: true, SortLabel: "Shared Blocks Read (Disk I/O)"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sharedBlksDirtied", Tooltip: "Shared Blocks Dirtied", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.shared_blks_dirtied"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sharedBlksWritten", Tooltip: "Shared Blocks Written", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.shared_blks_written"},

	// Local buffer statistics (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "localBlksHit", Tooltip: "Local Blocks Hit", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.local_blks_hit"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "localBlksRead", Tooltip: "Local Blocks Read", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.local_blks_read"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "localBlksDirtied", Tooltip: "Local Blocks Dirtied", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.local_blks_dirtied"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "localBlksWritten", Tooltip: "Local Blocks Written", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.local_blks_written"},

	// Temp buffer statistics (always present)
	{ColumnMeta: funcapi.ColumnMeta{Name: "tempBlksRead", Tooltip: "Temp Blocks Read", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.temp_blks_read"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "tempBlksWritten", Tooltip: "Temp Blocks Written", Type: funcapi.FieldTypeInteger, Visible: true, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.temp_blks_written", IsSortOption: true, SortLabel: "Temp Blocks Written"},

	// I/O timing (requires track_io_timing, always present but may be 0)
	{ColumnMeta: funcapi.ColumnMeta{Name: "blkReadTime", Tooltip: "Block Read Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.blk_read_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "blkWriteTime", Tooltip: "Block Write Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.blk_write_time"},

	// WAL statistics (PG 13+ only)
	{ColumnMeta: funcapi.ColumnMeta{Name: "walRecords", Tooltip: "WAL Records", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.wal_records"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "walFpi", Tooltip: "WAL Full Page Images", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.wal_fpi"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "walBytes", Tooltip: "WAL Bytes", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.wal_bytes"},

	// JIT statistics (PG 15+ only)
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitFunctions", Tooltip: "JIT Functions", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_functions"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitGenerationTime", Tooltip: "JIT Generation Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_generation_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitInliningCount", Tooltip: "JIT Inlining Count", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_inlining_count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitInliningTime", Tooltip: "JIT Inlining Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_inlining_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitOptimizationCount", Tooltip: "JIT Optimization Count", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_optimization_count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitOptimizationTime", Tooltip: "JIT Optimization Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_optimization_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitEmissionCount", Tooltip: "JIT Emission Count", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_emission_count"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "jitEmissionTime", Tooltip: "JIT Emission Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.jit_emission_time"},

	// Temp file statistics (PG 15+ only)
	{ColumnMeta: funcapi.ColumnMeta{Name: "tempBlkReadTime", Tooltip: "Temp Block Read Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.temp_blk_read_time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "tempBlkWriteTime", Tooltip: "Temp Block Write Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.temp_blk_write_time"},

	// pg_stat_monitor-specific columns (only available with pg_stat_monitor extension)
	{ColumnMeta: funcapi.ColumnMeta{Name: "applicationName", Tooltip: "Application Name", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.application_name", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientIp", Tooltip: "Client IP Address", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.client_ip::text", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cmdType", Tooltip: "Query Type", Type: funcapi.FieldTypeString, Visible: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualPill, Sortable: true}, DBColumn: "s.cmd_type_text", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "comments", Tooltip: "Query Comments", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Sortable: true}, DBColumn: "s.comments", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "relations", Tooltip: "Involved Tables", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Sortable: true}, DBColumn: "array_to_string(s.relations, ', ')", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuUserTime", Tooltip: "User CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.cpu_user_time", IsSortOption: true, SortLabel: "User CPU Time", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpuSysTime", Tooltip: "System CPU Time", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.cpu_sys_time", IsSortOption: true, SortLabel: "System CPU Time", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "elevel", Tooltip: "Error Level", Type: funcapi.FieldTypeInteger, Visible: false, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "s.elevel", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "sqlcode", Tooltip: "SQL Error Code", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.sqlcode", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "message", Tooltip: "Error Message", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, FullWidth: true, Sortable: true}, DBColumn: "s.message", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "toplevel", Tooltip: "Top-level Statement", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.toplevel::text", OnlyPgStatMonitor: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "bucketStartTime", Tooltip: "Bucket Start Time", Type: funcapi.FieldTypeString, Visible: false, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortDescending, Sortable: true}, DBColumn: "s.bucket_start_time::text", OnlyPgStatMonitor: true},
}

// pgChartGroupDefs defines chart groupings for columns. These are applied at runtime via decoratePgColumns.
var pgChartGroupDefs = []struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}{
	{key: "Calls", title: "Number of Calls", columns: []string{"calls"}, defaultChart: true},
	{key: "Time", title: "Execution Time", columns: []string{"totalTime", "meanTime", "minTime", "maxTime", "stddevTime"}, defaultChart: true},
	{key: "PlanTime", title: "Planning Time", columns: []string{"totalPlanTime", "meanPlanTime", "minPlanTime", "maxPlanTime", "stddevPlanTime"}},
	{key: "Plans", title: "Plans", columns: []string{"plans"}},
	{key: "Rows", title: "Rows Returned", columns: []string{"rows"}},
	{key: "SharedBlocks", title: "Shared Blocks", columns: []string{"sharedBlksHit", "sharedBlksRead", "sharedBlksDirtied", "sharedBlksWritten"}},
	{key: "LocalBlocks", title: "Local Blocks", columns: []string{"localBlksHit", "localBlksRead", "localBlksDirtied", "localBlksWritten"}},
	{key: "TempBlocks", title: "Temp Blocks", columns: []string{"tempBlksRead", "tempBlksWritten"}},
	{key: "IOTime", title: "Block I/O Time", columns: []string{"blkReadTime", "blkWriteTime"}},
	{key: "WALRecords", title: "WAL Records", columns: []string{"walRecords", "walFpi"}},
	{key: "WALBytes", title: "WAL Bytes", columns: []string{"walBytes"}},
	{key: "JITCounts", title: "JIT Counts", columns: []string{"jitFunctions", "jitInliningCount", "jitOptimizationCount", "jitEmissionCount"}},
	{key: "JITTime", title: "JIT Time", columns: []string{"jitGenerationTime", "jitInliningTime", "jitOptimizationTime", "jitEmissionTime"}},
	{key: "TempIOTime", title: "Temp Block I/O Time", columns: []string{"tempBlkReadTime", "tempBlkWriteTime"}},
	// pg_stat_monitor-specific chart groups
	{key: "CPUTime", title: "CPU Time", columns: []string{"cpuUserTime", "cpuSysTime"}},
	{key: "Errors", title: "Error Info", columns: []string{"elevel", "sqlcode", "message"}},
}

// pgLabelColumnIDs defines which columns are available for group-by.
var pgLabelColumnIDs = map[string]bool{
	"database":        true,
	"user":            true,
	"applicationName": true, // pg_stat_monitor only
	"cmdType":         true, // pg_stat_monitor only
}

const pgPrimaryLabelID = "database"

func topQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topQueriesMethodID,
		Name:         "Top Queries",
		UpdateEvery:  10,
		Help:         "Top SQL queries from pg_stat_statements",
		RequireCloud: true,
		RequiredParams: []funcapi.ParamConfig{
			{
				ID:         paramSort,
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    buildPgSortOptions(),
				UniqueView: true,
			},
		},
	}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopQueries)(nil)

// funcTopQueries handles the "top-queries" function for PostgreSQL.
type funcTopQueries struct {
	router *funcRouter
}

func newFuncTopQueries(r *funcRouter) *funcTopQueries {
	return &funcTopQueries{router: r}
}

// MethodParams implements funcapi.MethodHandler.
func (f *funcTopQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.collector.Functions.TopQueries.Disabled {
		return nil, fmt.Errorf("top-queries function disabled in configuration")
	}
	if f.router.collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	return f.topQueriesParams(ctx)
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.Functions.TopQueries.Disabled {
		return funcapi.UnavailableResponse("top-queries function has been disabled in configuration")
	}
	if f.router.collector.db == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.topQueriesTimeout())
	defer cancel()
	return f.collectTopQueries(queryCtx, params.Column(paramSort))
}

// buildPgSortOptions builds sort options from pgAllColumns.
func buildPgSortOptions() []funcapi.ParamOption {
	var opts []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range pgAllColumns {
		if col.IsSortOption {
			opts = append(opts, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    "Top queries by " + col.SortLabel,
				Default: col.IsDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return opts
}

// collectTopQueries queries pg_stat_statements or pg_stat_monitor for top queries.
// It auto-detects pg_stat_monitor and uses it when available, falling back to pg_stat_statements.
func (f *funcTopQueries) collectTopQueries(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	c := f.router.collector

	// Auto-detect best available query stats source
	source, err := f.getQueryStatsSource(ctx)
	if err != nil {
		return funcapi.InternalErrorResponse("failed to detect query stats source: %v", err)
	}
	if source == queryStatsSourceNone {
		return funcapi.UnavailableResponse("No query statistics extension is installed in this database. " +
			"Install pg_stat_monitor (recommended) or pg_stat_statements:\n\n" +
			"For pg_stat_monitor:\n" +
			"  ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_monitor';\n" +
			"  -- restart PostgreSQL\n" +
			"  CREATE EXTENSION pg_stat_monitor;\n\n" +
			"For pg_stat_statements:\n" +
			"  ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';\n" +
			"  -- restart PostgreSQL\n" +
			"  CREATE EXTENSION pg_stat_statements;")
	}

	// Detect available columns based on source
	var availableCols map[string]bool
	if source == queryStatsSourcePgStatMonitor {
		availableCols, err = f.detectPgStatMonitorColumns(ctx)
	} else {
		availableCols, err = f.detectPgStatStatementsColumns(ctx)
	}
	if err != nil {
		return funcapi.InternalErrorResponse("failed to detect available columns: %v", err)
	}

	// Build list of columns to query based on what's available and source
	queryCols := f.buildAvailableColumns(availableCols, source)
	if len(queryCols) == 0 {
		return funcapi.InternalErrorResponse("no queryable columns found in %s", source)
	}

	// Map and validate sort column
	actualSortCol := f.mapAndValidateSortColumn(sortColumn, availableCols, source)

	// Get query limit (default 500)
	limit := c.topQueriesLimit()

	// Build and execute query
	query := f.buildDynamicSQL(queryCols, actualSortCol, limit, source)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("query failed: %v", err)
	}
	defer rows.Close()

	// Process rows and build response
	data, err := f.scanDynamicRows(rows, queryCols)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	if err := rows.Err(); err != nil {
		return funcapi.InternalErrorResponse("rows iteration error: %v", err)
	}

	// Build dynamic sort options from available columns (only those actually detected)
	sortParam, sortOptions := f.topQueriesSortParam(queryCols)

	// Find default sort column from metadata
	defaultSort := ""
	for _, col := range queryCols {
		if col.IsDefaultSort && col.IsSortOption {
			defaultSort = col.Name
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	// Decorate columns with chart/label metadata and build using ColumnSet
	annotatedCols := decoratePgColumns(queryCols)
	cs := pgColumnSet(annotatedCols)

	// Build help message based on source
	helpMsg := "Top SQL queries from pg_stat_statements"
	if source == queryStatsSourcePgStatMonitor {
		helpMsg = "Top SQL queries from pg_stat_monitor (includes application, client IP, CPU time, and error info)"
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              helpMsg,
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		ChartingConfig:    cs.BuildCharting(),
	}
}

// decoratePgColumns adds label and chart metadata to columns for ColumnSet builders.
func decoratePgColumns(cols []pgColumn) []pgColumn {
	out := make([]pgColumn, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.Name] = i
	}

	// Mark groupby columns
	for i := range out {
		if pgLabelColumnIDs[out[i].Name] {
			out[i].GroupBy = &funcapi.GroupByOptions{
				IsDefault: out[i].Name == pgPrimaryLabelID,
			}
		}
	}

	// Mark chart columns
	for _, group := range pgChartGroupDefs {
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

// buildAvailableColumns returns column metadata for columns that exist in this PG version and source.
func (f *funcTopQueries) buildAvailableColumns(availableCols map[string]bool, source queryStatsSourceName) []pgColumn {
	c := f.router.collector
	var result []pgColumn
	isPgStatMonitor := source == queryStatsSourcePgStatMonitor

	for _, col := range pgAllColumns {
		// Skip pg_stat_monitor-only columns when using pg_stat_statements
		if col.OnlyPgStatMonitor && !isPgStatMonitor {
			continue
		}

		// Extract the actual column name for availability check
		colName := col.DBColumn

		// Strip array_to_string wrapper FIRST (before table prefix removal)
		// e.g., "array_to_string(s.relations, ', ')" -> "s.relations"
		if strings.HasPrefix(colName, "array_to_string(") {
			colName = strings.TrimPrefix(colName, "array_to_string(")
			if idx := strings.Index(colName, ","); idx != -1 {
				colName = colName[:idx]
			}
		}

		// Remove table prefix (e.g., "s.relations" -> "relations")
		if idx := strings.LastIndex(colName, "."); idx != -1 {
			colName = colName[idx+1:]
		}

		// Remove PostgreSQL type cast suffix (e.g., "queryid::text" -> "queryid")
		if idx := strings.Index(colName, "::"); idx != -1 {
			colName = colName[:idx]
		}

		// Handle version-specific column names for time columns.
		// pg_stat_statements PG 13+ and pg_stat_monitor both use: total_exec_time, mean_exec_time, etc.
		// pg_stat_statements < PG 13 uses: total_time, mean_time, etc.
		actualColName := colName
		if isPgStatMonitor || c.pgVersion >= pgVersion13 {
			switch colName {
			case "total_time":
				actualColName = "total_exec_time"
			case "mean_time":
				actualColName = "mean_exec_time"
			case "min_time":
				actualColName = "min_exec_time"
			case "max_time":
				actualColName = "max_exec_time"
			case "stddev_time":
				actualColName = "stddev_exec_time"
			}
		}

		// Check if column exists (either directly or via join)
		// Join columns (database, user) come from other tables (d.datname, u.usename)
		// pg_stat_monitor has datname directly, pg_stat_statements needs join
		isJoinCol := col.Name == "database" || col.Name == "user"
		if isPgStatMonitor && col.Name == "database" {
			// pg_stat_monitor has datname directly
			isJoinCol = false
		}
		if isJoinCol || availableCols[actualColName] {
			// Create a copy with the actual column name for this version
			colCopy := col
			if actualColName != colName {
				// Update DBColumn to use the version-specific name with alias
				if strings.HasPrefix(col.DBColumn, "s.") {
					colCopy.DBColumn = "s." + actualColName
				}
			}
			// For pg_stat_monitor, database comes from s.datname directly
			if isPgStatMonitor && col.Name == "database" {
				colCopy.DBColumn = "s.datname"
			}
			result = append(result, colCopy)
		}
	}

	return result
}

// mapAndValidateSortColumn maps the semantic sort column to actual SQL column.
func (f *funcTopQueries) mapAndValidateSortColumn(sortColumn string, availableCols map[string]bool, source queryStatsSourceName) string {
	c := f.router.collector
	isPgStatMonitor := source == queryStatsSourcePgStatMonitor

	// Map column ID back to DBColumn
	for _, col := range pgAllColumns {
		if col.Name == sortColumn || col.DBColumn == sortColumn {
			// Get actual column name (strip table prefix and type cast)
			colName := col.DBColumn
			if idx := strings.LastIndex(colName, "."); idx != -1 {
				colName = colName[idx+1:]
			}
			if idx := strings.Index(colName, "::"); idx != -1 {
				colName = colName[:idx]
			}

			// Handle version-specific mapping for time columns.
			// pg_stat_statements PG 13+ and pg_stat_monitor both use: total_exec_time, mean_exec_time, etc.
			// pg_stat_statements < PG 13 uses: total_time, mean_time, etc.
			if isPgStatMonitor || c.pgVersion >= pgVersion13 {
				switch colName {
				case "total_time":
					colName = "total_exec_time"
				case "mean_time":
					colName = "mean_exec_time"
				case "min_time":
					colName = "min_exec_time"
				case "max_time":
					colName = "max_exec_time"
				case "stddev_time":
					colName = "stddev_exec_time"
				}
			}

			// Validate column exists
			if availableCols[colName] {
				return colName
			}
		}
	}

	// Default fallback
	if isPgStatMonitor || c.pgVersion >= pgVersion13 {
		return "total_exec_time"
	}
	return "total_time"
}

// buildDynamicSQL builds the SQL query with only available columns.
func (f *funcTopQueries) buildDynamicSQL(cols []pgColumn, sortColumn string, limit int, source queryStatsSourceName) string {
	c := f.router.collector
	var selectCols []string

	for _, col := range cols {
		colExpr := col.DBColumn

		// Handle version-specific column names for time columns.
		// pg_stat_statements PG 13+ and pg_stat_monitor both use: total_exec_time, mean_exec_time, etc.
		// pg_stat_statements < PG 13 uses: total_time, mean_time, etc.
		if source == queryStatsSourcePgStatMonitor || c.pgVersion >= pgVersion13 {
			switch {
			case strings.HasSuffix(colExpr, ".total_time"):
				colExpr = strings.Replace(colExpr, ".total_time", ".total_exec_time", 1)
			case strings.HasSuffix(colExpr, ".mean_time"):
				colExpr = strings.Replace(colExpr, ".mean_time", ".mean_exec_time", 1)
			case strings.HasSuffix(colExpr, ".min_time"):
				colExpr = strings.Replace(colExpr, ".min_time", ".min_exec_time", 1)
			case strings.HasSuffix(colExpr, ".max_time"):
				colExpr = strings.Replace(colExpr, ".max_time", ".max_exec_time", 1)
			case strings.HasSuffix(colExpr, ".stddev_time"):
				colExpr = strings.Replace(colExpr, ".stddev_time", ".stddev_exec_time", 1)
			case colExpr == "total_time":
				colExpr = "total_exec_time"
			case colExpr == "mean_time":
				colExpr = "mean_exec_time"
			case colExpr == "min_time":
				colExpr = "min_exec_time"
			case colExpr == "max_time":
				colExpr = "max_exec_time"
			case colExpr == "stddev_time":
				colExpr = "stddev_exec_time"
			}
		}

		// Use column ID as the SQL alias for consistent naming
		// Use double quotes to handle reserved keywords like "database", "user"
		selectCols = append(selectCols, fmt.Sprintf("%s AS \"%s\"", colExpr, col.Name))
	}

	// Build query based on source
	if source == queryStatsSourcePgStatMonitor {
		// pg_stat_monitor has datname and username columns directly
		return fmt.Sprintf(`
SELECT %s
FROM pg_stat_monitor s
JOIN pg_user u ON s.userid = u.usesysid
ORDER BY "%s" DESC
LIMIT %d
`, strings.Join(selectCols, ", "), sortColumn, limit)
	}

	// pg_stat_statements needs joins for database and user names
	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_statements s
JOIN pg_database d ON s.dbid = d.oid
JOIN pg_user u ON s.userid = u.usesysid
ORDER BY "%s" DESC
LIMIT %d
`, strings.Join(selectCols, ", "), sortColumn, limit)
}

// scanDynamicRows scans rows into the data array based on column types.
// Uses sql.Null* types to handle NULL values safely.
func (f *funcTopQueries) scanDynamicRows(rows dbRows, cols []pgColumn) ([][]any, error) {
	specs := make([]sqlquery.ScanColumnSpec, len(cols))
	for i, col := range cols {
		specs[i] = pgTopQueriesScanSpec(col)
	}

	data, err := sqlquery.ScanTypedRows(rows, specs)
	if err != nil {
		return nil, fmt.Errorf("row scan failed: %v", err)
	}
	return data, nil
}

func pgTopQueriesScanSpec(col pgColumn) sqlquery.ScanColumnSpec {
	spec := sqlquery.ScanColumnSpec{}
	switch col.Type {
	case funcapi.FieldTypeString:
		spec.Type = sqlquery.ScanValueString
		if col.Name == "query" {
			spec.Transform = func(v any) any {
				s, _ := v.(string)
				return strmutil.TruncateText(s, maxQueryTextLength)
			}
		}
	case funcapi.FieldTypeInteger:
		spec.Type = sqlquery.ScanValueInteger
	case funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
		spec.Type = sqlquery.ScanValueFloat
	default:
		spec.Type = sqlquery.ScanValueString
	}
	return spec
}

// buildDynamicSortOptions builds sort options from available columns.
// Returns only sort options for columns that actually exist in the database.
func (f *funcTopQueries) buildDynamicSortOptions(cols []pgColumn) []funcapi.ParamOption {
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

func (f *funcTopQueries) topQueriesSortParam(queryCols []pgColumn) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := f.buildDynamicSortOptions(queryCols)
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

func (f *funcTopQueries) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	// Auto-detect best available query stats source
	source, err := f.getQueryStatsSource(ctx)
	if err != nil {
		return nil, err
	}
	if source == queryStatsSourceNone {
		return nil, fmt.Errorf("no query statistics extension is installed (pg_stat_monitor or pg_stat_statements)")
	}

	// Detect available columns based on source
	var availableCols map[string]bool
	if source == queryStatsSourcePgStatMonitor {
		availableCols, err = f.detectPgStatMonitorColumns(ctx)
	} else {
		availableCols, err = f.detectPgStatStatementsColumns(ctx)
	}
	if err != nil {
		return nil, err
	}

	queryCols := f.buildAvailableColumns(availableCols, source)
	if len(queryCols) == 0 {
		return nil, fmt.Errorf("no queryable columns found in %s", source)
	}

	sortParam, _ := f.topQueriesSortParam(queryCols)

	return []funcapi.ParamConfig{sortParam}, nil
}

// Cleanup implements funcapi.MethodHandler.
func (f *funcTopQueries) Cleanup(ctx context.Context) {}

// dbRows interface for testing
type dbRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}
