// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const maxQueryTextLength = 4096

const paramSort = "__sort"

// pgColumn defines metadata for a pg_stat_statements column.
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
}

// pgLabelColumnIDs defines which columns are available for group-by.
var pgLabelColumnIDs = map[string]bool{
	"database": true,
	"user":     true,
}

const pgPrimaryLabelID = "database"

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
	if f.router.collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	return f.topQueriesParams(ctx)
}

// Handle implements funcapi.MethodHandler.
func (f *funcTopQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	return f.collectTopQueries(ctx, params.Column(paramSort))
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

// collectTopQueries queries pg_stat_statements for top queries.
func (f *funcTopQueries) collectTopQueries(ctx context.Context, sortColumn string) *funcapi.FunctionResponse {
	c := f.router.collector

	// Check pg_stat_statements availability (lazy check)
	available, err := c.checkPgStatStatements(ctx)
	if err != nil {
		return funcapi.InternalErrorResponse("failed to check pg_stat_statements availability: %v", err)
	}
	if !available {
		return funcapi.UnavailableResponse("pg_stat_statements extension is not installed in this database. " +
			"Run 'CREATE EXTENSION pg_stat_statements;' in the database the collector connects to.")
	}

	// Detect available columns (lazy detection, cached)
	availableCols, err := c.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return funcapi.InternalErrorResponse("failed to detect available columns: %v", err)
	}

	// Build list of columns to query based on what's available
	queryCols := f.buildAvailableColumns(availableCols)
	if len(queryCols) == 0 {
		return funcapi.InternalErrorResponse("no queryable columns found in pg_stat_statements")
	}

	// Map and validate sort column
	actualSortCol := f.mapAndValidateSortColumn(sortColumn, availableCols)

	// Get query limit (default 500)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	// Build and execute query
	query := f.buildDynamicSQL(queryCols, actualSortCol, limit)
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

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements",
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

// buildAvailableColumns returns column metadata for columns that exist in this PG version.
func (f *funcTopQueries) buildAvailableColumns(availableCols map[string]bool) []pgColumn {
	c := f.router.collector
	var result []pgColumn

	for _, col := range pgAllColumns {
		// Extract the actual column name (remove table prefix and type cast)
		colName := col.DBColumn
		if idx := strings.LastIndex(colName, "."); idx != -1 {
			colName = colName[idx+1:]
		}
		// Remove PostgreSQL type cast suffix (e.g., "::text")
		if idx := strings.Index(colName, "::"); idx != -1 {
			colName = colName[:idx]
		}

		// Handle version-specific column names for time columns
		// PG 13+ renamed time columns: total_time -> total_exec_time, etc.
		actualColName := colName
		if c.pgVersion >= pgVersion13 {
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
		isJoinCol := col.Name == "database" || col.Name == "user"
		if isJoinCol || availableCols[actualColName] {
			// Create a copy with the actual column name for this version
			colCopy := col
			if actualColName != colName {
				// Update DBColumn to use the version-specific name with alias
				if strings.HasPrefix(col.DBColumn, "s.") {
					colCopy.DBColumn = "s." + actualColName
				}
			}
			result = append(result, colCopy)
		}
	}

	return result
}

// mapAndValidateSortColumn maps the semantic sort column to actual SQL column.
func (f *funcTopQueries) mapAndValidateSortColumn(sortColumn string, availableCols map[string]bool) string {
	c := f.router.collector

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

			// Handle version-specific mapping
			if c.pgVersion >= pgVersion13 {
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
	if c.pgVersion >= pgVersion13 {
		return "total_exec_time"
	}
	return "total_time"
}

// buildDynamicSQL builds the SQL query with only available columns.
func (f *funcTopQueries) buildDynamicSQL(cols []pgColumn, sortColumn string, limit int) string {
	c := f.router.collector
	var selectCols []string

	for _, col := range cols {
		colExpr := col.DBColumn

		// Handle version-specific column names
		if c.pgVersion >= pgVersion13 {
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
	data := make([][]any, 0, 500)

	// Create value holders for scanning (reuse across rows for efficiency)
	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
		// Reset value holders for each row
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
				var v sql.NullString
				values[i] = &v
			}
			valuePtrs[i] = values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %v", err)
		}

		// Convert scanned values to output format
		row := make([]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case *sql.NullString:
				if v.Valid {
					s := v.String
					if col.Name == "query" {
						row[i] = strmutil.TruncateText(s, maxQueryTextLength)
					} else {
						row[i] = s
					}
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
			}
		}

		data = append(data, row)
	}

	return data, nil
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
	c := f.router.collector

	available, err := c.checkPgStatStatements(ctx)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("pg_stat_statements extension is not installed")
	}

	availableCols, err := c.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return nil, err
	}

	queryCols := f.buildAvailableColumns(availableCols)
	if len(queryCols) == 0 {
		return nil, fmt.Errorf("no queryable columns found in pg_stat_statements")
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
