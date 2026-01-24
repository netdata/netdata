// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

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

	summaryCount  = funcapi.FieldSummaryCount
	summarySum    = funcapi.FieldSummarySum
	summaryMin    = funcapi.FieldSummaryMin
	summaryMax    = funcapi.FieldSummaryMax
	summaryMean   = funcapi.FieldSummaryMean
	summaryMedian = funcapi.FieldSummaryMedian

	filterMulti = funcapi.FieldFilterMultiselect
	filterRange = funcapi.FieldFilterRange
)

// pgColumnMeta defines metadata for a pg_stat_statements column
type pgColumnMeta struct {
	// Database column name (may vary by version)
	dbColumn string
	// Canonical name used everywhere: SQL alias, UI key, sort key
	uiKey string
	// Display name in UI
	displayName string
	// Data type: "string", "integer", "float", "duration"
	dataType funcapi.FieldType
	// Unit for duration/numeric types
	units string
	// Whether visible by default
	visible bool
	// Transform for value_options
	transform funcapi.FieldTransform
	// Decimal points for display
	decimalPoints int
	// Sort direction preference
	sortDir funcapi.FieldSort
	// Summary function
	summary funcapi.FieldSummary
	// Filter type
	filter funcapi.FieldFilter
	// Whether this is a sortable option for the sort dropdown
	isSortOption bool
	// Sort option label (if isSortOption)
	sortLabel string
	// Whether this is the default sort
	isDefaultSort bool
	// Whether this is the unique key
	isUniqueKey bool
	// Whether this column is sticky (stays visible when scrolling)
	isSticky bool
	// Whether this column should take full width
	fullWidth bool
	// Whether this column is a label for grouping
	isLabel bool
	// Whether this label is the primary grouping
	isPrimary bool
	// Whether this column is a chartable metric
	isMetric bool
	// Chart group key
	chartGroup string
	// Chart title
	chartTitle string
	// Include this chart group in defaults
	isDefaultChart bool
}

// pgAllColumns defines ALL possible columns from pg_stat_statements
// Order matters - this determines column index in the response
var pgAllColumns = []pgColumnMeta{
	// Core identification columns (always present)
	{dbColumn: "s.queryid::text", uiKey: "queryid", displayName: "Query ID", dataType: ftString, visible: false, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isUniqueKey: true},
	{dbColumn: "s.query", uiKey: "query", displayName: "Query", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti, isSticky: true, fullWidth: true},
	{dbColumn: "d.datname", uiKey: "database", displayName: "Database", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},
	{dbColumn: "u.usename", uiKey: "user", displayName: "User", dataType: ftString, visible: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},

	// Execution count (always present)
	{dbColumn: "s.calls", uiKey: "calls", displayName: "Calls", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Number of Calls"},

	// Execution time columns (names vary by version - detected dynamically)
	// PG <13: total_time, mean_time, min_time, max_time, stddev_time
	// PG 13+: total_exec_time, mean_exec_time, min_exec_time, max_exec_time, stddev_exec_time
	{dbColumn: "total_time", uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Total Execution Time", isDefaultSort: true},
	{dbColumn: "mean_time", uiKey: "meanTime", displayName: "Mean Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange, isSortOption: true, sortLabel: "Average Execution Time"},
	{dbColumn: "min_time", uiKey: "minTime", displayName: "Min Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_time", uiKey: "maxTime", displayName: "Max Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stddev_time", uiKey: "stddevTime", displayName: "Stddev Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Planning time columns (PG 13+ only)
	{dbColumn: "s.plans", uiKey: "plans", displayName: "Plans", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "total_plan_time", uiKey: "totalPlanTime", displayName: "Total Plan Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "mean_plan_time", uiKey: "meanPlanTime", displayName: "Mean Plan Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "min_plan_time", uiKey: "minPlanTime", displayName: "Min Plan Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMin, filter: filterRange},
	{dbColumn: "max_plan_time", uiKey: "maxPlanTime", displayName: "Max Plan Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange},
	{dbColumn: "stddev_plan_time", uiKey: "stddevPlanTime", displayName: "Stddev Plan Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summaryMax, filter: filterRange},

	// Row count (always present)
	{dbColumn: "s.rows", uiKey: "rows", displayName: "Rows", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Rows Returned"},

	// Shared buffer statistics (always present)
	{dbColumn: "s.shared_blks_hit", uiKey: "sharedBlksHit", displayName: "Shared Blocks Hit", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Shared Blocks Hit (Cache)"},
	{dbColumn: "s.shared_blks_read", uiKey: "sharedBlksRead", displayName: "Shared Blocks Read", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Shared Blocks Read (Disk I/O)"},
	{dbColumn: "s.shared_blks_dirtied", uiKey: "sharedBlksDirtied", displayName: "Shared Blocks Dirtied", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.shared_blks_written", uiKey: "sharedBlksWritten", displayName: "Shared Blocks Written", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// Local buffer statistics (always present)
	{dbColumn: "s.local_blks_hit", uiKey: "localBlksHit", displayName: "Local Blocks Hit", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.local_blks_read", uiKey: "localBlksRead", displayName: "Local Blocks Read", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.local_blks_dirtied", uiKey: "localBlksDirtied", displayName: "Local Blocks Dirtied", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.local_blks_written", uiKey: "localBlksWritten", displayName: "Local Blocks Written", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// Temp buffer statistics (always present)
	{dbColumn: "s.temp_blks_read", uiKey: "tempBlksRead", displayName: "Temp Blocks Read", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.temp_blks_written", uiKey: "tempBlksWritten", displayName: "Temp Blocks Written", dataType: ftInteger, visible: true, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange, isSortOption: true, sortLabel: "Temp Blocks Written"},

	// I/O timing (requires track_io_timing, always present but may be 0)
	{dbColumn: "s.blk_read_time", uiKey: "blkReadTime", displayName: "Block Read Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.blk_write_time", uiKey: "blkWriteTime", displayName: "Block Write Time", dataType: ftDuration, units: "milliseconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// WAL statistics (PG 13+ only)
	{dbColumn: "s.wal_records", uiKey: "walRecords", displayName: "WAL Records", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.wal_fpi", uiKey: "walFpi", displayName: "WAL Full Page Images", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.wal_bytes", uiKey: "walBytes", displayName: "WAL Bytes", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// JIT statistics (PG 15+ only)
	{dbColumn: "s.jit_functions", uiKey: "jitFunctions", displayName: "JIT Functions", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_generation_time", uiKey: "jitGenerationTime", displayName: "JIT Generation Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_inlining_count", uiKey: "jitInliningCount", displayName: "JIT Inlining Count", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_inlining_time", uiKey: "jitInliningTime", displayName: "JIT Inlining Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_optimization_count", uiKey: "jitOptimizationCount", displayName: "JIT Optimization Count", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_optimization_time", uiKey: "jitOptimizationTime", displayName: "JIT Optimization Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_emission_count", uiKey: "jitEmissionCount", displayName: "JIT Emission Count", dataType: ftInteger, visible: false, transform: trNumber, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.jit_emission_time", uiKey: "jitEmissionTime", displayName: "JIT Emission Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},

	// Temp file statistics (PG 15+ only)
	{dbColumn: "s.temp_blk_read_time", uiKey: "tempBlkReadTime", displayName: "Temp Block Read Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
	{dbColumn: "s.temp_blk_write_time", uiKey: "tempBlkWriteTime", displayName: "Temp Block Write Time", dataType: ftDuration, units: "milliseconds", visible: false, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
}

type pgChartGroup struct {
	key          string
	title        string
	columns      []string
	defaultChart bool
}

var pgChartGroups = []pgChartGroup{
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

var pgLabelColumns = map[string]bool{
	"database": true,
	"user":     true,
}

const pgPrimaryLabel = "database"

// pgMethods returns the available function methods for PostgreSQL
// Sort options are built dynamically based on available columns
func pgMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range pgAllColumns {
		if col.isSortOption {
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.uiKey,
				Column:  col.uiKey,
				Name:    "Top queries by " + col.sortLabel,
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
			Help:         "Top SQL queries from pg_stat_statements",
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

func pgMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
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

// pgHandleMethod handles function requests for PostgreSQL
func pgHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
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

// collectTopQueries queries pg_stat_statements for top queries
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Check pg_stat_statements availability (lazy check)
	available, err := c.checkPgStatStatements(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check pg_stat_statements availability: %v", err),
		}
	}
	if !available {
		return &module.FunctionResponse{
			Status: 503,
			Message: "pg_stat_statements extension is not installed in this database. " +
				"Run 'CREATE EXTENSION pg_stat_statements;' in the database the collector connects to.",
		}
	}

	// Detect available columns (lazy detection, cached)
	availableCols, err := c.detectPgStatStatementsColumns(ctx)
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to detect available columns: %v", err),
		}
	}

	// Build list of columns to query based on what's available
	queryCols := c.buildAvailableColumns(availableCols)
	if len(queryCols) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: "no queryable columns found in pg_stat_statements",
		}
	}

	// Map and validate sort column
	actualSortCol := c.mapAndValidateSortColumn(sortColumn, availableCols)

	// Get query limit (default 500)
	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	// Build and execute query
	query := c.buildDynamicSQL(queryCols, actualSortCol, limit)
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &module.FunctionResponse{Status: 504, Message: "query timed out"}
		}
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	// Process rows and build response
	data, err := c.scanDynamicRows(rows, queryCols)
	if err != nil {
		return &module.FunctionResponse{Status: 500, Message: err.Error()}
	}

	if err := rows.Err(); err != nil {
		return &module.FunctionResponse{Status: 500, Message: fmt.Sprintf("rows iteration error: %v", err)}
	}

	// Build dynamic sort options from available columns (only those actually detected)
	sortParam, sortOptions := c.topQueriesSortParam(queryCols)

	// Find default sort column UI key from metadata
	defaultSort := ""
	for _, col := range queryCols {
		if col.isDefaultSort && col.isSortOption {
			defaultSort = col.uiKey
			break
		}
	}
	// Fallback to first sort option if no default
	if defaultSort == "" && len(sortOptions) > 0 {
		defaultSort = sortOptions[0].ID
	}

	annotatedCols := decoratePgColumns(queryCols)

	return &module.FunctionResponse{
		Status:            200,
		Help:              "Top SQL queries from pg_stat_statements",
		Columns:           c.buildDynamicColumns(queryCols),
		Data:              data,
		DefaultSortColumn: defaultSort,
		RequiredParams:    []funcapi.ParamConfig{sortParam},

		// Charts for aggregated visualization
		Charts:        pgTopQueriesCharts(annotatedCols),
		DefaultCharts: pgTopQueriesDefaultCharts(annotatedCols),
		GroupBy:       pgTopQueriesGroupBy(annotatedCols),
	}
}

func decoratePgColumns(cols []pgColumnMeta) []pgColumnMeta {
	out := make([]pgColumnMeta, len(cols))
	index := make(map[string]int, len(cols))
	for i, col := range cols {
		out[i] = col
		index[col.uiKey] = i
	}

	for i := range out {
		if pgLabelColumns[out[i].uiKey] {
			out[i].isLabel = true
			if out[i].uiKey == pgPrimaryLabel {
				out[i].isPrimary = true
			}
		}
	}

	for _, group := range pgChartGroups {
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

func pgTopQueriesCharts(cols []pgColumnMeta) map[string]module.ChartConfig {
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

func pgTopQueriesDefaultCharts(cols []pgColumnMeta) [][]string {
	label := primaryPgLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultPgChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

func pgTopQueriesGroupBy(cols []pgColumnMeta) map[string]module.GroupByConfig {
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

func primaryPgLabel(cols []pgColumnMeta) string {
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

func defaultPgChartGroups(cols []pgColumnMeta) []string {
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

// detectPgStatStatementsColumns queries the database to find available columns
func (c *Collector) detectPgStatStatementsColumns(ctx context.Context) (map[string]bool, error) {
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

	// Query available columns from pg_stat_statements
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'pg_stat_statements'
		AND table_schema = 'public'
	`
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, fmt.Errorf("failed to scan column name: %v", err)
		}
		cols[colName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	// Cache the result
	c.pgStatStatementsColumns = cols
	return cols, nil
}

// buildAvailableColumns returns column metadata for columns that exist in this PG version
func (c *Collector) buildAvailableColumns(availableCols map[string]bool) []pgColumnMeta {
	var result []pgColumnMeta

	for _, col := range pgAllColumns {
		// Extract the actual column name (remove table prefix and type cast)
		colName := col.dbColumn
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
		isJoinCol := col.uiKey == "database" || col.uiKey == "user"
		if isJoinCol || availableCols[actualColName] {
			// Create a copy with the actual column name for this version
			colCopy := col
			if actualColName != colName {
				// Update dbColumn to use the version-specific name with alias
				prefix := "s."
				if strings.HasPrefix(col.dbColumn, "s.") {
					prefix = ""
					colCopy.dbColumn = "s." + actualColName
				}
				_ = prefix // suppress unused warning
			}
			result = append(result, colCopy)
		}
	}

	return result
}

// mapAndValidateSortColumn maps the semantic sort column to actual SQL column
func (c *Collector) mapAndValidateSortColumn(sortColumn string, availableCols map[string]bool) string {
	// Map UI key back to dbColumn
	for _, col := range pgAllColumns {
		if col.uiKey == sortColumn || col.dbColumn == sortColumn {
			// Get actual column name (strip table prefix and type cast)
			colName := col.dbColumn
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

// buildDynamicSQL builds the SQL query with only available columns
func (c *Collector) buildDynamicSQL(cols []pgColumnMeta, sortColumn string, limit int) string {
	var selectCols []string

	for _, col := range cols {
		colExpr := col.dbColumn

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

		// Always use uiKey as the SQL alias for consistent naming
		// Use double quotes to handle reserved keywords like "database", "user"
		selectCols = append(selectCols, fmt.Sprintf("%s AS \"%s\"", colExpr, col.uiKey))
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

// scanDynamicRows scans rows into the data array based on column types
// Uses sql.Null* types to handle NULL values safely
func (c *Collector) scanDynamicRows(rows dbRows, cols []pgColumnMeta) ([][]any, error) {
	data := make([][]any, 0, 500)

	// Create value holders for scanning (reuse across rows for efficiency)
	valuePtrs := make([]any, len(cols))
	values := make([]any, len(cols))

	for rows.Next() {
		// Reset value holders for each row
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
					if col.uiKey == "query" {
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

// buildDynamicColumns builds column definitions for the response
func (c *Collector) buildDynamicColumns(cols []pgColumnMeta) map[string]any {
	result := make(map[string]any)

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
		result[col.uiKey] = colDef.BuildColumn()
	}

	return result
}

// buildDynamicSortOptions builds sort options from available columns
// Returns only sort options for columns that actually exist in the database
func (c *Collector) buildDynamicSortOptions(cols []pgColumnMeta) []funcapi.ParamOption {
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

func (c *Collector) topQueriesSortParam(queryCols []pgColumnMeta) (funcapi.ParamConfig, []funcapi.ParamOption) {
	sortOptions := c.buildDynamicSortOptions(queryCols)
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

	queryCols := c.buildAvailableColumns(availableCols)
	if len(queryCols) == 0 {
		return nil, fmt.Errorf("no queryable columns found in pg_stat_statements")
	}

	sortParam, _ := c.topQueriesSortParam(queryCols)
	return []funcapi.ParamConfig{sortParam}, nil
}

// checkPgStatStatements checks if pg_stat_statements extension is available
// Only positive results are cached - negative results are re-checked each time
// so users don't need to restart after installing the extension
func (c *Collector) checkPgStatStatements(ctx context.Context) (bool, error) {
	// Fast path: return cached positive result
	c.pgStatStatementsMu.RLock()
	avail := c.pgStatStatementsAvail
	c.pgStatStatementsMu.RUnlock()
	if avail {
		return true, nil
	}

	// Slow path: query the database
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	err := c.db.QueryRowContext(ctx, query).Scan(&exists)
	if err != nil {
		return false, err
	}

	// Only cache positive results
	if exists {
		c.pgStatStatementsMu.Lock()
		c.pgStatStatementsAvail = true
		c.pgStatStatementsMu.Unlock()
	}

	return exists, nil
}

// dbRows interface for testing
type dbRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}
