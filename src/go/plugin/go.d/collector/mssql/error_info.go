// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
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

func errorInfoMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
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
	if !f.router.collector.Config.GetErrorInfoFunctionEnabled() {
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
	return f.router.collector.collectErrorInfo(ctx)
}

func (f *funcErrorInfo) Cleanup(ctx context.Context) {}

type mssqlErrorRow struct {
	Time        time.Time
	ErrorNumber *int64
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

func (c *Collector) errorInfoParams(context.Context) ([]funcapi.ParamConfig, error) {
	if !c.Config.GetErrorInfoFunctionEnabled() {
		return nil, fmt.Errorf("error-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (c *Collector) collectErrorInfo(ctx context.Context) *funcapi.FunctionResponse {
	if !c.Config.GetErrorInfoFunctionEnabled() {
		return &funcapi.FunctionResponse{
			Status: 503,
			Message: "error-info not enabled: function disabled in configuration. " +
				"To enable, set error_info_function_enabled: true in the MSSQL collector config.",
		}
	}

	sessionName := c.Config.GetErrorInfoSessionName()
	status, rows, err := c.fetchMSSQLErrorRows(ctx, sessionName, c.TopQueriesLimit)
	if err != nil {
		if isDeadlockPermissionError(err) {
			return &funcapi.FunctionResponse{Status: 403, Message: deadlockPermissionMessage()}
		}
		if status == mssqlErrorAttrNotEnabled {
			return &funcapi.FunctionResponse{Status: 503, Message: "error-info not enabled: Extended Events session not found or ring_buffer target missing"}
		}
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("error-info query failed: %v", err)}
	}

	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		var errNo any
		if row.ErrorNumber != nil {
			errNo = *row.ErrorNumber
		}
		data = append(data, []any{
			row.Time,
			errNo,
			row.Message,
			row.Query,
			row.QueryHash,
		})
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Recent SQL errors from Extended Events error_reported",
		Columns:           buildMSSQLErrorInfoColumns(),
		Data:              data,
		DefaultSortColumn: "timestamp",
	}
}

func buildMSSQLErrorInfoColumns() map[string]any {
	columns := map[string]any{
		"timestamp": funcapi.Column{
			Index:        0,
			Name:         "Timestamp",
			Type:         funcapi.FieldTypeTimestamp,
			Sortable:     true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformDatetime},
		}.BuildColumn(),
		"errorNumber": funcapi.Column{
			Index:        1,
			Name:         "Error Number",
			Type:         funcapi.FieldTypeInteger,
			Sortable:     true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNumber},
		}.BuildColumn(),
		"errorMessage": funcapi.Column{
			Index:        2,
			Name:         "Error Message",
			Type:         funcapi.FieldTypeString,
			Sortable:     false,
			FullWidth:    true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"query": funcapi.Column{
			Index:        3,
			Name:         "Query",
			Type:         funcapi.FieldTypeString,
			Sortable:     false,
			FullWidth:    true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"queryHash": funcapi.Column{
			Index:        4,
			Name:         "Query Hash",
			Type:         funcapi.FieldTypeString,
			Sortable:     true,
			Visible:      false,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
	}
	return columns
}

func mssqlErrorAttributionColumns() []topQueriesColumn {
	return []topQueriesColumn{
		{
			ColumnMeta: funcapi.ColumnMeta{
				Name:      "errorAttribution",
				Tooltip:   "Error Attribution",
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
				Name:      "sqlState",
				Tooltip:   "SQL State",
				Type:      funcapi.FieldTypeString,
				Visible:   false,
				Transform: funcapi.FieldTransformNone,
				Sort:      funcapi.FieldSortAscending,
				Summary:   funcapi.FieldSummaryCount,
				Filter:    funcapi.FieldFilterMultiselect,
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

func (c *Collector) collectMSSQLErrorDetails(ctx context.Context) (string, map[string]mssqlErrorRow) {
	status, rows, err := c.fetchMSSQLErrorRows(ctx, c.Config.GetErrorInfoSessionName(), c.TopQueriesLimit)
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

	escapedDB := strings.ReplaceAll(dbName, "]", "]]")
	query := fmt.Sprintf(`
SELECT
  CONVERT(VARCHAR(64), q.query_hash, 1) AS query_hash,
  CAST(p.query_plan AS NVARCHAR(MAX)) AS query_plan
FROM [%s].sys.query_store_query q
INNER JOIN [%s].sys.query_store_plan p ON q.query_id = p.query_id
WHERE q.query_hash IN (%s);
`, escapedDB, escapedDB, strings.Join(validHashes, ","))

	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, query)
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

	sessionExists, err := c.mssqlErrorSessionAvailable(ctx, sessionName)
	if err != nil {
		return mssqlErrorAttrNotEnabled, nil, err
	}
	if !sessionExists {
		return mssqlErrorAttrNotEnabled, nil, fmt.Errorf("session not found")
	}

	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, queryMSSQLErrorInfo, sql.Named("sessionName", sessionName), sql.Named("limit", limit))
	if err != nil {
		return mssqlErrorAttrNotSupported, nil, err
	}
	defer rows.Close()

	var results []mssqlErrorRow
	for rows.Next() {
		var (
			ts        sql.NullTime
			errNo     sql.NullInt64
			message   sql.NullString
			sqlText   sql.NullString
			queryHash sql.NullString
			errNoPtr  *int64
		)
		if err := rows.Scan(&ts, &errNo, &message, &sqlText, &queryHash); err != nil {
			return mssqlErrorAttrNotSupported, nil, err
		}
		if errNo.Valid {
			val := errNo.Int64
			errNoPtr = &val
		}
		results = append(results, mssqlErrorRow{
			Time:        ts.Time,
			ErrorNumber: errNoPtr,
			Message:     message.String,
			Query:       sqlText.String,
			QueryHash:   queryHash.String,
		})
	}
	if err := rows.Err(); err != nil {
		return mssqlErrorAttrNotSupported, nil, err
	}

	return mssqlErrorAttrEnabled, results, nil
}

func (c *Collector) mssqlErrorSessionAvailable(ctx context.Context, sessionName string) (bool, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	var count int
	err := c.db.QueryRowContext(qctx, queryMSSQLErrorSessionExists, sql.Named("sessionName", sessionName)).Scan(&count)
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}

	err = c.db.QueryRowContext(qctx, queryMSSQLErrorSessionHasRingBuffer, sql.Named("sessionName", sessionName)).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
