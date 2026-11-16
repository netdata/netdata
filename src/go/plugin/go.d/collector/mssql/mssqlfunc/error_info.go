// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	mssqlErrorAttrEnabled      = "enabled"
	mssqlErrorAttrNotEnabled   = "not_enabled"
	mssqlErrorAttrNotSupported = "not_supported"
	mssqlErrorAttrNoData       = "no_data"
)

const (
	mssqlErrorSourceConfigured   = "configured-session"
	mssqlErrorSourceSystemHealth = "system-health"

	errorInfoHelpConfigured   = "Recent SQL errors from the configured Extended Events error_reported session."
	errorInfoHelpSystemHealth = "Recent selected SQL errors from the built-in system_health Extended Events session. This fallback does not capture every error_reported event."
)

const errorInfoMethodID = "error-info"

type mssqlErrorRow struct {
	Time        time.Time
	ErrorNumber *int64
	ErrorState  *int64
	Message     string
	Query       string
	QueryHash   string
	Source      string
}

type mssqlPlanOps struct {
	HashMatch   int64
	MergeJoin   int64
	NestedLoops int64
	Sorts       int64
}

// errorInfoColumn defines a column for the error-info function.
type errorInfoColumn struct {
	funcapi.ColumnMeta
	Value func(*mssqlErrorRow) any
}

func errorInfoColumnSet(cols []errorInfoColumn) funcapi.ColumnSet[errorInfoColumn] {
	return funcapi.Columns(cols, func(c errorInfoColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var errorInfoColumns = []errorInfoColumn{
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "timestamp",
			Tooltip:   "When the error occurred",
			Type:      funcapi.FieldTypeTimestamp,
			Sortable:  true,
			Visible:   true,
			Transform: funcapi.FieldTransformDatetime,
		},
		Value: func(r *mssqlErrorRow) any { return r.Time },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "errorNumber",
			Tooltip:   "SQL Server error number",
			Type:      funcapi.FieldTypeInteger,
			Sortable:  true,
			Visible:   true,
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *mssqlErrorRow) any {
			if r.ErrorNumber == nil {
				return nil
			}
			return *r.ErrorNumber
		},
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "errorState",
			Tooltip:   "Diagnostic code indicating where the error was raised (1-255)",
			Type:      funcapi.FieldTypeInteger,
			Sortable:  true,
			Visible:   true,
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *mssqlErrorRow) any {
			if r.ErrorState == nil {
				return nil
			}
			return *r.ErrorState
		},
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "errorMessage",
			Tooltip:   "The error message text",
			Type:      funcapi.FieldTypeString,
			Sortable:  false,
			FullWidth: true,
			Visible:   true,
		},
		Value: func(r *mssqlErrorRow) any { return r.Message },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "query",
			Tooltip:   "The SQL statement that caused the error",
			Type:      funcapi.FieldTypeString,
			Sortable:  false,
			FullWidth: true,
			Visible:   true,
		},
		Value: func(r *mssqlErrorRow) any { return r.Query },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "queryHash",
			Tooltip:  "Hash of the query for grouping similar statements",
			Type:     funcapi.FieldTypeString,
			Sortable: true,
			Visible:  false,
		},
		Value: func(r *mssqlErrorRow) any { return r.QueryHash },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "source",
			Tooltip:  "Extended Events source that produced the row",
			Type:     funcapi.FieldTypeString,
			Sortable: true,
			Visible:  true,
			Filter:   funcapi.FieldFilterMultiselect,
		},
		Value: func(r *mssqlErrorRow) any { return r.Source },
	},
}

func errorInfoFunctionConfig() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             errorInfoMethodID,
		Name:           "Error Info",
		UpdateEvery:    10,
		Help:           "Recent SQL errors from the configured Extended Events session, with system_health fallback when unavailable.",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{},
	}
}

// funcErrorInfo handles the error-info function.
type funcErrorInfo struct {
	router *router
}

func newFuncErrorInfo(r *router) *funcErrorInfo {
	return &funcErrorInfo{
		router: r,
	}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcErrorInfo)(nil)

func (f *funcErrorInfo) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.cfg.ErrorInfo.Disabled {
		return nil, fmt.Errorf("error-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (f *funcErrorInfo) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	r := f.router
	return r.runFunction(ctx, r.cfg.errorInfoTimeout(), f.collectData)
}

func (f *funcErrorInfo) Cleanup(ctx context.Context) {}

func (f *funcErrorInfo) collectData(ctx context.Context) *funcapi.FunctionResponse {
	if f.router.cfg.ErrorInfo.Disabled {
		return funcapi.UnavailableResponse("error-info function has been disabled in configuration")
	}

	sessionName := f.router.cfg.errorInfoSessionName()
	limit := f.router.cfg.topQueriesLimit()
	status, source, rows, err := f.router.fetchMSSQLErrorRows(ctx, sessionName, limit)
	if err != nil {
		if response := f.router.cfg.errorInfoTimeout().contextError(ctx, err); response != nil {
			return response
		}
		if isDeadlockPermissionError(err) {
			return &funcapi.FunctionResponse{
				Status:  403,
				Message: f.router.errorInfoPermissionMessage(),
			}
		}
		if status == mssqlErrorAttrNotEnabled {
			targetName := "event_file"
			if f.router.cfg.ErrorInfo.UseRingBuffer {
				targetName = "ring_buffer"
			}
			return &funcapi.FunctionResponse{
				Status:  503,
				Message: fmt.Sprintf("error-info not enabled: Extended Events session not found or %s target missing", targetName),
			}
		}
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("error-info query failed: %v", err),
		}
	}

	data := make([][]any, 0, len(rows))
	for i := range rows {
		row := make([]any, len(errorInfoColumns))
		for j, col := range errorInfoColumns {
			row[j] = col.Value(&rows[i])
		}
		data = append(data, row)
	}

	cs := errorInfoColumnSet(errorInfoColumns)

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              errorInfoHelp(source),
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "timestamp",
	}
}

func errorInfoHelp(source string) string {
	if source == mssqlErrorSourceSystemHealth {
		return errorInfoHelpSystemHealth
	}
	return errorInfoHelpConfigured
}

func (r *router) errorInfoPermissionMessage() string {
	permission := r.xeReadPermission()
	if r.deps.ServerInfo().AzureSQLDatabase && r.cfg.ErrorInfo.UseRingBuffer {
		permission = "VIEW DATABASE STATE"
	}
	return fmt.Sprintf("error-info requires %s permission. Grant with: GRANT %s TO [netdata_user];", permission, permission)
}

func mssqlErrorAttributionColumns() []topQueriesColumn {
	return []topQueriesColumn{
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "errorAttribution",
				Tooltip:   "Source of error data (enabled, not_enabled, no_data)",
				Type:      funcapi.FieldTypeString,
				Visible:   true,
				Transform: funcapi.FieldTransformNone,
				Sort:      funcapi.FieldSortAscending,
				Summary:   funcapi.FieldSummaryCount,
				Filter:    funcapi.FieldFilterMultiselect,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "errorNumber",
				Tooltip:   "Error Number",
				Type:      funcapi.FieldTypeInteger,
				Visible:   true,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummaryMax,
				Filter:    funcapi.FieldFilterRange,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "errorState",
				Tooltip:   "Error State",
				Type:      funcapi.FieldTypeInteger,
				Visible:   false,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummaryMax,
				Filter:    funcapi.FieldFilterRange,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "errorMessage",
				Tooltip:   "Error Message",
				Type:      funcapi.FieldTypeString,
				Visible:   true,
				Transform: funcapi.FieldTransformNone,
				Sort:      funcapi.FieldSortAscending,
				Summary:   funcapi.FieldSummaryCount,
				Filter:    funcapi.FieldFilterMultiselect,
				FullWidth: true,
			},
		},
	}
}

func mssqlPlanAttributionColumns() []topQueriesColumn {
	return []topQueriesColumn{
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "hashMatch",
				Tooltip:   "Hash Match Joins",
				Type:      funcapi.FieldTypeInteger,
				Visible:   true,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummarySum,
				Filter:    funcapi.FieldFilterRange,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "mergeJoin",
				Tooltip:   "Merge Joins",
				Type:      funcapi.FieldTypeInteger,
				Visible:   true,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummarySum,
				Filter:    funcapi.FieldFilterRange,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "nestedLoops",
				Tooltip:   "Nested Loops",
				Type:      funcapi.FieldTypeInteger,
				Visible:   true,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummarySum,
				Filter:    funcapi.FieldFilterRange,
			},
		},
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "sorts",
				Tooltip:   "Sorts",
				Type:      funcapi.FieldTypeInteger,
				Visible:   true,
				Transform: funcapi.FieldTransformNumber,
				Sort:      funcapi.FieldSortDescending,
				Summary:   funcapi.FieldSummarySum,
				Filter:    funcapi.FieldFilterRange,
			},
		},
	}
}

func normalizeSQLText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	fields := strings.Fields(text)
	normalized := strings.Join(fields, " ")
	normalized = strings.TrimSpace(normalized)
	normalized = strings.TrimRight(normalized, ";")
	return strings.TrimSpace(normalized)
}

func rowString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// The router shares error data access between error-info and top-query attribution.
func (r *router) collectMSSQLErrorDetails(ctx context.Context) (string, map[string]mssqlErrorRow) {
	status, _, rows, err := r.fetchMSSQLErrorRows(ctx, r.cfg.errorInfoSessionName(), r.cfg.topQueriesLimit())
	if err != nil {
		if status == mssqlErrorAttrNotEnabled {
			return mssqlErrorAttrNotEnabled, nil
		}
		mapped := classifyMSSQLErrorAttrError(err)
		r.log.Debugf("error attribution query failed: %v (status=%s)", err, mapped)
		return mapped, nil
	}

	if len(rows) == 0 {
		return mssqlErrorAttrNoData, nil
	}

	out := make(map[string]mssqlErrorRow, len(rows)*2)
	for _, row := range rows {
		if row.QueryHash != "" {
			if _, ok := out[row.QueryHash]; !ok {
				out[row.QueryHash] = row
			}
		}
		key := normalizeSQLText(row.Query)
		if key != "" {
			if _, ok := out[key]; !ok {
				out[key] = row
			}
		}
	}
	return mssqlErrorAttrEnabled, out
}

func classifyMSSQLErrorAttrError(err error) string {
	if err == nil {
		return mssqlErrorAttrNotEnabled
	}
	if isDeadlockPermissionError(err) {
		return mssqlErrorAttrNotEnabled
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "permission") || strings.Contains(msg, "denied") {
		return mssqlErrorAttrNotEnabled
	}
	if strings.Contains(msg, "invalid column") ||
		strings.Contains(msg, "invalid object") ||
		strings.Contains(msg, "could not find") {
		return mssqlErrorAttrNotSupported
	}
	return mssqlErrorAttrNotEnabled
}

func (r *router) collectMSSQLPlanOps(ctx context.Context, data [][]any, cols []topQueriesColumn) map[string]map[string]mssqlPlanOps {
	dbIdx := -1
	hashIdx := -1
	for i, col := range cols {
		switch col.Name {
		case "database":
			dbIdx = i
		case "queryHash":
			hashIdx = i
		}
	}
	if dbIdx < 0 || hashIdx < 0 {
		return map[string]map[string]mssqlPlanOps{}
	}

	hashesByDB := make(map[string][]string)
	seen := make(map[string]map[string]bool)
	for _, row := range data {
		if dbIdx >= len(row) || hashIdx >= len(row) {
			continue
		}
		dbName := rowString(row[dbIdx])
		queryHash := rowString(row[hashIdx])
		if dbName == "" || queryHash == "" {
			continue
		}
		if seen[dbName] == nil {
			seen[dbName] = make(map[string]bool)
		}
		if seen[dbName][queryHash] {
			continue
		}
		seen[dbName][queryHash] = true
		hashesByDB[dbName] = append(hashesByDB[dbName], queryHash)
	}

	out := make(map[string]map[string]mssqlPlanOps)
	for dbName, hashes := range hashesByDB {
		ops, err := r.fetchMSSQLPlanOpsForDB(ctx, dbName, hashes)
		if err != nil {
			r.log.Debugf("plan attribution query failed for %s: %v", dbName, err)
			continue
		}
		out[dbName] = ops
	}
	return out
}

func (r *router) fetchMSSQLPlanOpsForDB(ctx context.Context, dbName string, hashes []string) (map[string]mssqlPlanOps, error) {
	if len(hashes) == 0 {
		return map[string]mssqlPlanOps{}, nil
	}

	validHashes := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		if strings.HasPrefix(hash, "0x") {
			validHashes = append(validHashes, hash)
		}
	}
	if len(validHashes) == 0 {
		return map[string]mssqlPlanOps{}, nil
	}

	queryView := "sys.query_store_query"
	planView := "sys.query_store_plan"
	if !r.deps.ServerInfo().AzureSQLDatabase {
		escapedDB := strings.ReplaceAll(dbName, "]", "]]")
		queryView = fmt.Sprintf("[%s].sys.query_store_query", escapedDB)
		planView = fmt.Sprintf("[%s].sys.query_store_plan", escapedDB)
	}
	query := fmt.Sprintf(`
SELECT
  CONVERT(VARCHAR(64), q.query_hash, 1) AS query_hash,
  CAST(p.query_plan AS NVARCHAR(MAX)) AS query_plan
FROM %s q
INNER JOIN %s p ON q.query_id = p.query_id
WHERE q.query_hash IN (%s);
`, queryView, planView, strings.Join(validHashes, ","))

	rows, err := r.deps.DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]mssqlPlanOps)
	for rows.Next() {
		var hash sql.NullString
		var plan sql.NullString
		if err := rows.Scan(&hash, &plan); err != nil {
			return nil, err
		}
		if !hash.Valid || !plan.Valid {
			continue
		}
		ops := countPlanOperators(plan.String)
		current := out[hash.String]
		current.HashMatch += ops.HashMatch
		current.MergeJoin += ops.MergeJoin
		current.NestedLoops += ops.NestedLoops
		current.Sorts += ops.Sorts
		out[hash.String] = current
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *router) fetchMSSQLErrorRows(ctx context.Context, sessionName string, limit int) (string, string, []mssqlErrorRow, error) {
	if limit <= 0 {
		limit = 500
	}

	target, available, err := r.resolveMSSQLXEventReadTarget(ctx, sessionName, r.cfg.ErrorInfo.UseRingBuffer)
	if err != nil {
		return mssqlErrorAttrNotEnabled, "", nil, err
	}
	if !available {
		if r.deps.ServerInfo().AzureSQLDatabase {
			return mssqlErrorAttrNotEnabled, "", nil, errors.New("session not found")
		}
		return r.fetchMSSQLErrorRowsFromSystemHealth(ctx, limit)
	}
	status, source, rows, err := r.fetchMSSQLErrorRowsFromTarget(ctx, target, sessionName, limit, mssqlErrorSourceConfigured)
	if err == nil || r.deps.ServerInfo().AzureSQLDatabase || !shouldFallbackErrorInfo(err) {
		return status, source, rows, err
	}
	return r.fetchMSSQLErrorRowsFromSystemHealth(ctx, limit)
}

func (r *router) fetchMSSQLErrorRowsFromSystemHealth(ctx context.Context, limit int) (string, string, []mssqlErrorRow, error) {
	if !r.cfg.ErrorInfo.UseRingBuffer {
		target, available, err := r.resolveMSSQLXEventReadTarget(ctx, "system_health", false)
		if err != nil && !shouldFallbackErrorInfo(err) {
			return mssqlErrorAttrNotSupported, mssqlErrorSourceSystemHealth, nil, err
		}
		if err == nil && available {
			status, source, rows, err := r.fetchMSSQLErrorRowsFromTarget(ctx, target, "system_health", limit, mssqlErrorSourceSystemHealth)
			if err == nil || !shouldFallbackErrorInfo(err) {
				return status, source, rows, err
			}
		}
	}

	target, available, err := r.resolveMSSQLXEventReadTarget(ctx, "system_health", true)
	if err != nil {
		return mssqlErrorAttrNotSupported, mssqlErrorSourceSystemHealth, nil, err
	}
	if !available {
		return mssqlErrorAttrNotEnabled, mssqlErrorSourceSystemHealth, nil, errors.New("system_health ring_buffer target unavailable")
	}
	return r.fetchMSSQLErrorRowsFromTarget(ctx, target, "system_health", limit, mssqlErrorSourceSystemHealth)
}

func shouldFallbackErrorInfo(err error) bool {
	return !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) &&
		!isDeadlockPermissionError(err)
}

func (r *router) fetchMSSQLErrorRowsFromTarget(ctx context.Context, target mssqlXEventReadTarget, sessionName string, limit int, source string) (string, string, []mssqlErrorRow, error) {
	var query string
	var args []any
	if target.filePath != "" {
		query = queryMSSQLErrorInfoEventFile
		args = append(target.fileQueryArgs("error_reported"), sql.Named("limit", limit))
	} else {
		query = queryMSSQLErrorInfoRingBuffer
		if r.deps.ServerInfo().AzureSQLDatabase {
			query = queryMSSQLErrorInfoDatabaseRingBuffer
		}
		args = []any{sql.Named("sessionName", sessionName), sql.Named("limit", limit)}
	}

	rows, err := r.deps.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return mssqlErrorAttrNotSupported, source, nil, err
	}
	defer rows.Close()

	var results []mssqlErrorRow
	for rows.Next() {
		var (
			ts          sql.NullTime
			errNo       sql.NullInt64
			errState    sql.NullInt64
			message     sql.NullString
			sqlText     sql.NullString
			queryHash   sql.NullString
			errNoPtr    *int64
			errStatePtr *int64
		)
		if err := rows.Scan(&ts, &errNo, &errState, &message, &sqlText, &queryHash); err != nil {
			return mssqlErrorAttrNotSupported, source, nil, err
		}
		if errNo.Valid {
			val := errNo.Int64
			errNoPtr = &val
		}
		if errState.Valid {
			val := errState.Int64
			errStatePtr = &val
		}
		results = append(results, mssqlErrorRow{
			Time:        ts.Time,
			ErrorNumber: errNoPtr,
			ErrorState:  errStatePtr,
			Message:     message.String,
			Query:       sqlText.String,
			QueryHash:   mssqlQueryHashToHex(queryHash.String),
			Source:      source,
		})
	}
	if err := rows.Err(); err != nil {
		return mssqlErrorAttrNotSupported, source, nil, err
	}

	return mssqlErrorAttrEnabled, source, results, nil
}

// mssqlQueryHashToHex converts the unsigned-64-bit decimal rendering that Extended Events
// uses for query_hash into the 0x-prefixed form Query Store comparisons use, so error
// attribution can join top-queries rows. Returns "" when the value is not a uint64.
func mssqlQueryHashToHex(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return s
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("0x%016X", v)
}

func countPlanOperators(planXML string) mssqlPlanOps {
	var ops mssqlPlanOps
	if strings.TrimSpace(planXML) == "" {
		return ops
	}
	decoder := xml.NewDecoder(strings.NewReader(planXML))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "RelOp" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local != "PhysicalOp" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(attr.Value)) {
			case "hash match":
				ops.HashMatch++
			case "merge join":
				ops.MergeJoin++
			case "nested loops":
				ops.NestedLoops++
			case "sort":
				ops.Sorts++
			}
			break
		}
	}
	return ops
}
