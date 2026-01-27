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

const (
	runningQueriesMethodID      = "running-queries"
	runningQueriesMaxTextLength = 4096
)

func runningQueriesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             runningQueriesMethodID,
		Name:           "Running Queries",
		UpdateEvery:    10,
		Help:           "Currently executing queries from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{funcapi.BuildSortParam(runningQueriesColumns)},
	}
}

// runningQueriesColumn defines metadata for a pg_stat_activity column.
type runningQueriesColumn struct {
	funcapi.ColumnMeta

	// DBColumn is the database column expression
	DBColumn string
	// sortOpt indicates whether this column appears in the sort dropdown
	sortOpt bool
	// sortLbl is the label shown in the sort dropdown
	sortLbl string
	// defaultSort indicates whether this is the default sort column
	defaultSort bool
	// minVersion is the minimum PostgreSQL version (0 means all versions)
	minVersion int
}

// funcapi.SortableColumn interface implementation.
func (c runningQueriesColumn) IsSortOption() bool  { return c.sortOpt }
func (c runningQueriesColumn) SortLabel() string   { return c.sortLbl }
func (c runningQueriesColumn) IsDefaultSort() bool { return c.defaultSort }
func (c runningQueriesColumn) ColumnName() string  { return c.Name }
func (c runningQueriesColumn) SortColumn() string  { return "" }

// runningQueriesColumns defines ALL columns from pg_stat_activity (PostgreSQL 14+).
// Order matters - this determines column index in the response.
var runningQueriesColumns = []runningQueriesColumn{
	// Session identification
	{ColumnMeta: funcapi.ColumnMeta{Name: "pid", Tooltip: "Process ID of this backend", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, UniqueKey: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "pid"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "leaderPid", Tooltip: "Process ID of parallel group leader (NULL if this is leader or not parallel)", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "leader_pid", minVersion: pgVersion13},

	// Database and user
	{ColumnMeta: funcapi.ColumnMeta{Name: "datid", Tooltip: "OID of the database", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "datid"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "datname", Tooltip: "Name of the database", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "datname"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "usesysid", Tooltip: "OID of the user", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: false, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "usesysid"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "usename", Tooltip: "Name of the user logged into this backend", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "usename"},

	// Application/client info
	{ColumnMeta: funcapi.ColumnMeta{Name: "applicationName", Tooltip: "Name of the application connected to this backend", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "application_name"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientAddr", Tooltip: "IP address of the client (NULL for Unix socket or internal process)", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "client_addr::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientHostname", Tooltip: "Hostname of the client via reverse DNS (only if log_hostname enabled)", Type: funcapi.FieldTypeString, Visible: false, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "client_hostname"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "clientPort", Tooltip: "TCP port of client (-1 for Unix socket, NULL for internal process)", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformNumber, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "client_port"},

	// Timestamps
	{ColumnMeta: funcapi.ColumnMeta{Name: "backendStart", Tooltip: "Time when this process/connection started", Type: funcapi.FieldTypeTimestamp, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, DBColumn: "backend_start"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "xactStart", Tooltip: "Time when current transaction started (NULL if no transaction)", Type: funcapi.FieldTypeTimestamp, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, DBColumn: "xact_start"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryStart", Tooltip: "Time when current/last query started", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, DBColumn: "query_start", sortOpt: true, sortLbl: "Query Start Time"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "stateChange", Tooltip: "Time when state was last changed", Type: funcapi.FieldTypeTimestamp, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDatetime, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, DBColumn: "state_change"},

	// Calculated duration
	{ColumnMeta: funcapi.ColumnMeta{Name: "durationMs", Tooltip: "Query duration in milliseconds (since query_start)", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax}, DBColumn: "EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - query_start)) * 1000", sortOpt: true, sortLbl: "Query Duration", defaultSort: true},

	// Wait events
	{ColumnMeta: funcapi.ColumnMeta{Name: "waitEventType", Tooltip: "Type of event the backend is waiting for (Activity, BufferPin, Client, Extension, IO, IPC, Lock, LWLock, Timeout)", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "wait_event_type"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "waitEvent", Tooltip: "Specific wait event name if backend is currently waiting", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "wait_event"},

	// State
	{ColumnMeta: funcapi.ColumnMeta{Name: "state", Tooltip: "Current state: active, idle, idle in transaction, idle in transaction (aborted), fastpath function call, disabled", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Visualization: funcapi.FieldVisualPill}, DBColumn: "state"},

	// Transaction IDs
	{ColumnMeta: funcapi.ColumnMeta{Name: "backendXid", Tooltip: "Top-level transaction identifier of this backend", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "backend_xid::text"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "backendXmin", Tooltip: "Backend's xmin horizon", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "backend_xmin::text"},

	// Query identification
	{ColumnMeta: funcapi.ColumnMeta{Name: "queryId", Tooltip: "Query identifier (requires compute_query_id or extension)", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "query_id::text", minVersion: 14_00_00},

	// Query text
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query text (may be truncated at track_activity_query_size)", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sticky: true, FullWidth: true, Wrap: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "query"},

	// Backend type
	{ColumnMeta: funcapi.ColumnMeta{Name: "backendType", Tooltip: "Type of backend: client backend, autovacuum worker, parallel worker, walsender, walreceiver, etc.", Type: funcapi.FieldTypeString, Visible: true, Sortable: true, Filter: funcapi.FieldFilterMultiselect, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount}, DBColumn: "backend_type", minVersion: pgVersion10},
}

// funcRunningQueries handles the running-queries function.
type funcRunningQueries struct {
	router *funcRouter
}

func newFuncRunningQueries(r *funcRouter) *funcRunningQueries {
	return &funcRunningQueries{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRunningQueries)(nil)

func (f *funcRunningQueries) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	return []funcapi.ParamConfig{funcapi.BuildSortParam(f.getColumnsForVersion())}, nil
}

func (f *funcRunningQueries) Cleanup(ctx context.Context) {}

func (f *funcRunningQueries) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	cols := f.getColumnsForVersion()
	sortColumn := f.resolveSortColumn(params.Column("__sort"), cols)
	if sortColumn == "" {
		return funcapi.InternalErrorResponse("no sortable columns available")
	}

	// Build the query
	query := f.buildQuery(cols, sortColumn)

	rows, err := f.router.collector.db.QueryContext(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.InternalErrorResponse("running queries query failed: %v", err)
	}
	defer rows.Close()

	data, err := f.scanRows(rows, cols)
	if err != nil {
		return funcapi.InternalErrorResponse("%s", err)
	}

	cs := f.columnSet(cols)
	if len(data) == 0 {
		return &funcapi.FunctionResponse{
			Status:            200,
			Message:           "No active queries found.",
			Help:              "Currently executing queries from pg_stat_activity",
			Columns:           cs.BuildColumns(),
			Data:              [][]any{},
			DefaultSortColumn: sortColumn,
			RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(cols)},
		}
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Currently executing queries from pg_stat_activity. WARNING: Query text may contain unmasked literals (potential PII).",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: sortColumn,
		RequiredParams:    []funcapi.ParamConfig{funcapi.BuildSortParam(cols)},
	}
}

func (f *funcRunningQueries) getColumnsForVersion() []runningQueriesColumn {
	version := f.router.collector.pgVersion
	if version == 0 {
		version = 14_00_00 // Assume recent version if not detected
	}

	var cols []runningQueriesColumn
	for _, col := range runningQueriesColumns {
		if col.minVersion == 0 || version >= col.minVersion {
			cols = append(cols, col)
		}
	}
	return cols
}

func (f *funcRunningQueries) columnSet(cols []runningQueriesColumn) funcapi.ColumnSet[runningQueriesColumn] {
	return funcapi.Columns(cols, func(c runningQueriesColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

func (f *funcRunningQueries) resolveSortColumn(requested string, cols []runningQueriesColumn) string {
	for _, col := range cols {
		if col.IsSortOption() && col.Name == requested {
			return col.Name
		}
	}
	// Return default sort column
	for _, col := range cols {
		if col.IsDefaultSort() {
			return col.Name
		}
	}
	// Fallback to first sortable column
	for _, col := range cols {
		if col.IsSortOption() {
			return col.Name
		}
	}
	return ""
}

func (f *funcRunningQueries) buildSelectClause(cols []runningQueriesColumn) string {
	var parts []string
	for _, col := range cols {
		parts = append(parts, col.DBColumn)
	}
	return strings.Join(parts, ", ")
}

func (f *funcRunningQueries) buildQuery(cols []runningQueriesColumn, sortColumn string) string {
	// Find the actual DB column for sorting
	sortExpr := "query_start"
	for _, col := range cols {
		if col.Name == sortColumn {
			sortExpr = col.DBColumn
			break
		}
	}

	return fmt.Sprintf(`
SELECT %s
FROM pg_stat_activity
WHERE state = 'active'
  AND pid != pg_backend_pid()
  AND query NOT LIKE '%%pg_stat_activity%%'
ORDER BY %s DESC NULLS LAST
LIMIT 500
`, f.buildSelectClause(cols), sortExpr)
}

func (f *funcRunningQueries) scanRows(rows *sql.Rows, cols []runningQueriesColumn) ([][]any, error) {
	var result [][]any

	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		row := make([]any, len(cols))
		for i, col := range cols {
			row[i] = f.formatValue(values[i], col)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return result, nil
}

func (f *funcRunningQueries) formatValue(v any, col runningQueriesColumn) any {
	if v == nil {
		return nil
	}

	switch col.Type {
	case funcapi.FieldTypeString:
		s := fmt.Sprintf("%v", v)
		// Truncate long strings (like query text)
		if col.Name == "query" && len(s) > runningQueriesMaxTextLength {
			s = strmutil.TruncateText(s, runningQueriesMaxTextLength)
		}
		return s
	case funcapi.FieldTypeInteger:
		switch val := v.(type) {
		case int64:
			return val
		case int32:
			return int64(val)
		case int:
			return int64(val)
		default:
			return v
		}
	case funcapi.FieldTypeDuration:
		switch val := v.(type) {
		case float64:
			return val
		case int64:
			return float64(val)
		default:
			return v
		}
	case funcapi.FieldTypeTimestamp:
		return v
	default:
		return v
	}
}
