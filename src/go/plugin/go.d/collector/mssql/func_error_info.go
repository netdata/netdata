// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

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

const errorInfoMethodID = "error-info"

type mssqlErrorRow struct {
	Time        time.Time
	ErrorNumber *int64
	ErrorState  *int64
	Message     string
	Query       string
	QueryHash   string
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
}

func errorInfoFunctionConfig() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             errorInfoMethodID,
		Name:           "Error Info",
		UpdateEvery:    10,
		Help:           "Recent SQL errors from Extended Events error_reported",
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{},
	}
}

// funcErrorInfo handles the error-info function.
type funcErrorInfo struct {
	router *funcRouter
}

func newFuncErrorInfo(r *funcRouter) *funcErrorInfo {
	return &funcErrorInfo{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcErrorInfo)(nil)

func (f *funcErrorInfo) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.collector.Functions.ErrorInfo.Disabled {
		return nil, fmt.Errorf("error-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (f *funcErrorInfo) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		db, err := f.router.collector.openConnection()
		if err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
		f.router.collector.db = db
	}
	queryCtx, cancel := context.WithTimeout(ctx, f.router.collector.errorInfoTimeout())
	defer cancel()
	if _, err := f.router.collector.ensureEngineEdition(queryCtx); err != nil {
		if response := mssqlFunctionContextError(queryCtx, err); response != nil {
			return response
		}
		return funcapi.ErrorResponse(500, "failed to detect SQL engine edition: %v", err)
	}
	return f.collectData(queryCtx)
}

func (f *funcErrorInfo) Cleanup(ctx context.Context) {}

func (f *funcErrorInfo) collectData(ctx context.Context) *funcapi.FunctionResponse {
	if f.router.collector.Functions.ErrorInfo.Disabled {
		return funcapi.UnavailableResponse("error-info function has been disabled in configuration")
	}

	sessionName := f.router.collector.errorInfoSessionName()
	limit := f.router.collector.topQueriesLimit()
	status, rows, err := f.router.collector.fetchMSSQLErrorRows(ctx, sessionName, limit)
	if err != nil {
		if response := mssqlFunctionContextError(ctx, err); response != nil {
			return response
		}
		if isDeadlockPermissionError(err) {
			return &funcapi.FunctionResponse{Status: 403, Message: f.router.collector.errorInfoPermissionMessage()}
		}
		if status == mssqlErrorAttrNotEnabled {
			targetName := "event_file"
			if f.router.collector.Functions.ErrorInfo.UseRingBuffer {
				targetName = "ring_buffer"
			}
			return &funcapi.FunctionResponse{Status: 503, Message: fmt.Sprintf("error-info not enabled: Extended Events session not found or %s target missing", targetName)}
		}
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("error-info query failed: %v", err)}
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
		Help:              "Recent SQL errors from Extended Events error_reported",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "timestamp",
	}
}

func (c *Collector) errorInfoPermissionMessage() string {
	permission := c.xeReadPermission()
	if c.isAzureSQLDatabase() && c.Functions.ErrorInfo.UseRingBuffer {
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

// TODO: Refactor error data access into a shared mssqlErrorData type.
// Currently these methods live on Collector because they're used by both:
// - funcErrorInfo (for error-info function)
// - funcTopQueries (for error attribution columns)
// A cleaner design would be a mssqlErrorData type on funcRouter that both handlers use.

func (c *Collector) collectMSSQLErrorDetails(ctx context.Context) (string, map[string]mssqlErrorRow) {
	status, rows, err := c.fetchMSSQLErrorRows(ctx, c.errorInfoSessionName(), c.topQueriesLimit())
	if err != nil {
		if status == mssqlErrorAttrNotEnabled {
			return mssqlErrorAttrNotEnabled, nil
		}
		mapped := classifyMSSQLErrorAttrError(err)
		c.Debugf("error attribution query failed: %v (status=%s)", err, mapped)
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

func (c *Collector) collectMSSQLPlanOps(ctx context.Context, data [][]any, cols []topQueriesColumn) map[string]map[string]mssqlPlanOps {
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
		ops, err := c.fetchMSSQLPlanOpsForDB(ctx, dbName, hashes)
		if err != nil {
			c.Debugf("plan attribution query failed for %s: %v", dbName, err)
			continue
		}
		out[dbName] = ops
	}
	return out
}

func (c *Collector) fetchMSSQLPlanOpsForDB(ctx context.Context, dbName string, hashes []string) (map[string]mssqlPlanOps, error) {
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
	if !c.isAzureSQLDatabase() {
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

	rows, err := c.db.QueryContext(ctx, query)
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

func (c *Collector) fetchMSSQLErrorRows(ctx context.Context, sessionName string, limit int) (string, []mssqlErrorRow, error) {
	if limit <= 0 {
		limit = 500
	}

	target, available, err := c.resolveMSSQLErrorReadTarget(ctx, sessionName)
	if err != nil {
		return mssqlErrorAttrNotEnabled, nil, err
	}
	if !available {
		return mssqlErrorAttrNotEnabled, nil, errors.New("session not found")
	}

	query := queryMSSQLErrorInfoEventFile
	args := []any{
		sql.Named("filePath", target.filePath),
		sql.Named("filePrefix", target.filePrefix),
		sql.Named("limit", limit),
	}
	if c.Functions.ErrorInfo.UseRingBuffer {
		query = queryMSSQLErrorInfoRingBuffer
		if c.isAzureSQLDatabase() {
			query = queryMSSQLErrorInfoDatabaseRingBuffer
		}
		args = []any{sql.Named("sessionName", sessionName), sql.Named("limit", limit)}
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return mssqlErrorAttrNotSupported, nil, err
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
			return mssqlErrorAttrNotSupported, nil, err
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
		})
	}
	if err := rows.Err(); err != nil {
		return mssqlErrorAttrNotSupported, nil, err
	}

	return mssqlErrorAttrEnabled, results, nil
}

// mssqlErrorReadTarget says where error_reported events should be read from.
type mssqlErrorReadTarget struct {
	// filePath is the event_file read pattern. Empty when reading the ring buffer.
	filePath   string
	filePrefix string
}

// resolveMSSQLErrorReadTarget locates the Extended Events target for a session, reporting
// false when the session or the requested target does not exist.
//
// For the event_file target the on-disk name is read from the catalog rather than derived
// from the session name: the filename is operator-chosen and the two frequently differ.
// The ring_buffer target only exists while the session is running, so that path still
// goes through the runtime DMVs.
func (c *Collector) resolveMSSQLErrorReadTarget(ctx context.Context, sessionName string) (mssqlErrorReadTarget, bool, error) {
	if !c.Functions.ErrorInfo.UseRingBuffer {
		query := queryMSSQLErrorSessionEventFilePath
		if c.isAzureSQLDatabase() {
			query = queryMSSQLErrorDatabaseSessionEventFilePath
		}
		var configured sql.NullString
		err := c.db.QueryRowContext(ctx, query, sql.Named("sessionName", sessionName)).Scan(&configured)
		if errors.Is(err, sql.ErrNoRows) {
			return mssqlErrorReadTarget{}, false, nil
		}
		if err != nil {
			return mssqlErrorReadTarget{}, false, err
		}
		target := eventFileReadTarget(configured.String)
		if target.filePath == "" {
			return mssqlErrorReadTarget{}, false, nil
		}
		return target, true, nil
	}

	activeSessionQuery := queryMSSQLErrorActiveSessionExists
	ringBufferQuery := queryMSSQLErrorSessionHasRingBuffer
	if c.isAzureSQLDatabase() {
		activeSessionQuery = queryMSSQLErrorActiveDatabaseSessionExists
		ringBufferQuery = queryMSSQLErrorDatabaseSessionHasRingBuffer
	}
	var count int
	err := c.db.QueryRowContext(ctx, activeSessionQuery, sql.Named("sessionName", sessionName)).Scan(&count)
	if err != nil {
		return mssqlErrorReadTarget{}, false, err
	}
	if count == 0 {
		return mssqlErrorReadTarget{}, false, nil
	}

	err = c.db.QueryRowContext(ctx, ringBufferQuery, sql.Named("sessionName", sessionName)).Scan(&count)
	if err != nil {
		return mssqlErrorReadTarget{}, false, err
	}
	return mssqlErrorReadTarget{}, count > 0, nil
}

// eventFileReadTarget turns the configured event_file filename into the read path and
// exact generated-file prefix. Local files use a wildcard; Azure Storage uses the
// wildcard-free blob prefix required by sys.fn_xe_file_target_read_file.
func eventFileReadTarget(configured string) mssqlErrorReadTarget {
	path := strings.TrimSpace(configured)
	if path == "" {
		return mssqlErrorReadTarget{}
	}
	if strings.HasSuffix(strings.ToLower(path), ".xel") {
		path = path[:len(path)-len(".xel")]
	}

	prefix := path + "_0_"
	base := strings.ReplaceAll(path, `\`, "/")
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	filePrefix := base + "_0_"
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return mssqlErrorReadTarget{filePath: prefix, filePrefix: filePrefix}
	}
	return mssqlErrorReadTarget{filePath: prefix + "*.xel", filePrefix: filePrefix}
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
