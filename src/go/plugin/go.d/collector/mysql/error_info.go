// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	mysqlErrorAttrEnabled      = "enabled"
	mysqlErrorAttrNotEnabled   = "not_enabled"
	mysqlErrorAttrNotSupported = "not_supported"
	mysqlErrorAttrNoData       = "no_data"
)

const errorInfoMethodID = "error-info"

func errorInfoMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             errorInfoMethodID,
		Name:           "Error Info",
		UpdateEvery:    10,
		Help:           "Recent SQL errors from performance_schema statement history tables",
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
		if err := f.router.collector.openConnection(); err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
	}
	return f.router.collector.collectErrorInfo(ctx)
}

func (f *funcErrorInfo) Cleanup(ctx context.Context) {}

type mysqlErrorSource struct {
	table         string
	fallbackTable string
	columns       map[string]bool
	status        string
	reason        string
}

type mysqlErrorRow struct {
	Digest      string
	Query       string
	Schema      string
	ErrorNumber *int64
	SQLState    string
	Message     string
}

func mysqlErrorAttributionColumns() []topQueriesColumn {
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

func (c *Collector) collectMySQLErrorDetailsForDigests(ctx context.Context, digests []string) (string, map[string]mysqlErrorRow) {
	source, err := c.detectMySQLErrorHistorySource(ctx)
	if err != nil {
		c.Debugf("error attribution: %v", err)
		return mysqlErrorAttrNotEnabled, nil
	}
	if source.status != mysqlErrorAttrEnabled {
		return source.status, nil
	}

	rows, err := c.fetchMySQLErrorRows(ctx, source, digests, len(digests))
	if err != nil {
		c.Debugf("error attribution query failed: %v", err)
		return mysqlErrorAttrNotEnabled, nil
	}
	if len(rows) == 0 && source.fallbackTable != "" {
		fallback, ferr := c.buildMySQLErrorSource(ctx, source.fallbackTable)
		if ferr == nil && fallback.status == mysqlErrorAttrEnabled {
			fallbackRows, ferr := c.fetchMySQLErrorRows(ctx, fallback, digests, len(digests))
			if ferr == nil && len(fallbackRows) > 0 {
				rows = fallbackRows
			}
		}
	}

	out := make(map[string]mysqlErrorRow, len(rows))
	for _, row := range rows {
		if row.Digest == "" {
			continue
		}
		if _, ok := out[row.Digest]; ok {
			continue
		}
		out[row.Digest] = row
	}
	return mysqlErrorAttrEnabled, out
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
				"To enable, set error_info_function_enabled: true in the MySQL collector config.",
		}
	}

	available, err := c.checkPerformanceSchema(ctx)
	if err != nil {
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check performance_schema availability: %v", err),
		}
	}
	if !available {
		return &funcapi.FunctionResponse{Status: 503, Message: "performance_schema is not enabled"}
	}

	source, err := c.detectMySQLErrorHistorySource(ctx)
	if err != nil {
		return &funcapi.FunctionResponse{Status: 503, Message: fmt.Sprintf("error-info not enabled: %v", err)}
	}
	if source.status != mysqlErrorAttrEnabled {
		msg := "error-info not enabled"
		if source.reason != "" {
			msg = fmt.Sprintf("%s: %s", msg, source.reason)
		}
		return &funcapi.FunctionResponse{Status: 503, Message: msg}
	}

	limit := c.TopQueriesLimit
	if limit <= 0 {
		limit = 500
	}

	rows, err := c.fetchMySQLErrorRows(ctx, source, nil, limit)
	if err != nil {
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("error-info query failed: %v", err)}
	}
	if len(rows) == 0 && source.fallbackTable != "" {
		fallback, ferr := c.buildMySQLErrorSource(ctx, source.fallbackTable)
		if ferr == nil && fallback.status == mysqlErrorAttrEnabled {
			fallbackRows, ferr := c.fetchMySQLErrorRows(ctx, fallback, nil, limit)
			if ferr == nil && len(fallbackRows) > 0 {
				rows = fallbackRows
			}
		}
	}

	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		var errNo any
		if row.ErrorNumber != nil {
			errNo = *row.ErrorNumber
		}
		data = append(data, []any{
			row.Digest,
			row.Query,
			row.Schema,
			errNo,
			row.SQLState,
			row.Message,
		})
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Recent SQL errors from performance_schema statement history tables",
		Columns:           buildMySQLErrorInfoColumns(),
		Data:              data,
		DefaultSortColumn: "errorNumber",
	}
}

func buildMySQLErrorInfoColumns() map[string]any {
	columns := map[string]any{
		"digest": funcapi.Column{
			Index:        0,
			Name:         "Digest",
			Type:         funcapi.FieldTypeString,
			Sortable:     true,
			Visible:      false,
			UniqueKey:    true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"query": funcapi.Column{
			Index:        1,
			Name:         "Query",
			Type:         funcapi.FieldTypeString,
			Sortable:     true,
			Sticky:       true,
			FullWidth:    true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"schema": funcapi.Column{
			Index:        2,
			Name:         "Schema",
			Type:         funcapi.FieldTypeString,
			Sortable:     true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"errorNumber": funcapi.Column{
			Index:        3,
			Name:         "Error Number",
			Type:         funcapi.FieldTypeInteger,
			Sortable:     true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNumber},
		}.BuildColumn(),
		"sqlState": funcapi.Column{
			Index:        4,
			Name:         "SQL State",
			Type:         funcapi.FieldTypeString,
			Sortable:     true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
		"errorMessage": funcapi.Column{
			Index:        5,
			Name:         "Error Message",
			Type:         funcapi.FieldTypeString,
			Sortable:     false,
			FullWidth:    true,
			ValueOptions: funcapi.ValueOptions{Transform: funcapi.FieldTransformNone},
		}.BuildColumn(),
	}
	return columns
}

func (c *Collector) checkPerformanceSchema(ctx context.Context) (bool, error) {
	c.varPerfSchemaMu.RLock()
	cached := c.varPerformanceSchema
	c.varPerfSchemaMu.RUnlock()
	if cached != "" {
		return cached == "ON" || cached == "1", nil
	}

	c.varPerfSchemaMu.Lock()
	defer c.varPerfSchemaMu.Unlock()

	if c.varPerformanceSchema != "" {
		return c.varPerformanceSchema == "ON" || c.varPerformanceSchema == "1", nil
	}

	var value string
	query := "SELECT @@performance_schema"
	if err := c.db.QueryRowContext(ctx, query).Scan(&value); err != nil {
		return false, err
	}

	c.varPerformanceSchema = value
	return value == "ON" || value == "1", nil
}

func (c *Collector) detectMySQLErrorHistorySource(ctx context.Context) (mysqlErrorSource, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, `
SELECT NAME, ENABLED
FROM performance_schema.setup_consumers
WHERE NAME IN ('events_statements_history_long','events_statements_history','events_statements_current');
`)
	if err != nil {
		return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "unable to read performance_schema.setup_consumers"}, err
	}
	defer rows.Close()

	enabled := map[string]bool{}
	present := map[string]bool{}
	for rows.Next() {
		var name, enabledVal string
		if err := rows.Scan(&name, &enabledVal); err != nil {
			return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "unable to read performance_schema.setup_consumers"}, err
		}
		key := strings.ToLower(name)
		enabled[key] = strings.EqualFold(enabledVal, "YES")
		present[key] = true
	}
	if err := rows.Err(); err != nil {
		return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "unable to read performance_schema.setup_consumers"}, err
	}

	table := ""
	fallback := ""
	switch {
	case enabled["events_statements_history_long"]:
		table = "events_statements_history_long"
		if enabled["events_statements_history"] {
			fallback = "events_statements_history"
		}
	case enabled["events_statements_history"]:
		table = "events_statements_history"
	default:
		return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "statement history consumers are disabled"}, nil
	}

	source, err := c.buildMySQLErrorSource(ctx, table)
	if err != nil {
		return source, err
	}
	if source.status != mysqlErrorAttrEnabled {
		return source, nil
	}
	source.fallbackTable = fallback
	return source, nil
}

func (c *Collector) buildMySQLErrorSource(ctx context.Context, table string) (mysqlErrorSource, error) {
	cols, err := c.fetchMySQLTableColumns(ctx, table)
	if err != nil {
		return mysqlErrorSource{status: mysqlErrorAttrNotSupported, reason: "unable to read history table columns"}, err
	}

	required := []string{"DIGEST", "MYSQL_ERRNO", "MESSAGE_TEXT", "RETURNED_SQLSTATE"}
	for _, key := range required {
		if !cols[key] {
			return mysqlErrorSource{status: mysqlErrorAttrNotSupported, reason: "required history columns are missing"}, nil
		}
	}

	return mysqlErrorSource{
		table:   table,
		columns: cols,
		status:  mysqlErrorAttrEnabled,
	}, nil
}

func (c *Collector) fetchMySQLTableColumns(ctx context.Context, table string) (map[string]bool, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, `
SELECT COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = 'performance_schema'
  AND TABLE_NAME = ?;`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[strings.ToUpper(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

func (c *Collector) fetchMySQLErrorRows(ctx context.Context, source mysqlErrorSource, digests []string, limit int) ([]mysqlErrorRow, error) {
	if source.status != mysqlErrorAttrEnabled {
		return nil, fmt.Errorf("error history not enabled")
	}

	selectCols := []string{"DIGEST", "MYSQL_ERRNO", "RETURNED_SQLSTATE", "MESSAGE_TEXT"}
	if source.columns["DIGEST_TEXT"] {
		selectCols = append(selectCols, "DIGEST_TEXT")
	}
	if source.columns["SCHEMA_NAME"] {
		selectCols = append(selectCols, "SCHEMA_NAME")
	}
	if source.columns["SQL_TEXT"] {
		selectCols = append(selectCols, "SQL_TEXT")
	}

	orderBy := ""
	switch {
	case source.columns["EVENT_ID"]:
		orderBy = "EVENT_ID DESC"
	case source.columns["TIMER_END"]:
		orderBy = "TIMER_END DESC"
	}

	var args []any
	var filters []string
	filters = append(filters, "MYSQL_ERRNO <> 0")
	if len(digests) > 0 {
		placeholders := make([]string, 0, len(digests))
		for _, digest := range digests {
			placeholders = append(placeholders, "?")
			args = append(args, digest)
		}
		filters = append(filters, fmt.Sprintf("DIGEST IN (%s)", strings.Join(placeholders, ",")))
	}

	query := fmt.Sprintf("SELECT %s FROM performance_schema.%s WHERE %s",
		strings.Join(selectCols, ", "),
		source.table,
		strings.Join(filters, " AND "),
	)
	if orderBy != "" {
		query = fmt.Sprintf("%s ORDER BY %s", query, orderBy)
	}
	if limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, limit)
	}

	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []mysqlErrorRow
	seen := make(map[string]bool)

	for rows.Next() {
		var (
			digest      sql.NullString
			errno       sql.NullInt64
			sqlState    sql.NullString
			message     sql.NullString
			digestText  sql.NullString
			schemaName  sql.NullString
			sqlText     sql.NullString
			scanTargets []any
		)

		scanTargets = append(scanTargets, &digest, &errno, &sqlState, &message)
		if source.columns["DIGEST_TEXT"] {
			scanTargets = append(scanTargets, &digestText)
		}
		if source.columns["SCHEMA_NAME"] {
			scanTargets = append(scanTargets, &schemaName)
		}
		if source.columns["SQL_TEXT"] {
			scanTargets = append(scanTargets, &sqlText)
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return nil, err
		}

		if !digest.Valid || strings.TrimSpace(digest.String) == "" {
			continue
		}
		if seen[digest.String] {
			continue
		}
		seen[digest.String] = true

		queryText := ""
		switch {
		case digestText.Valid && strings.TrimSpace(digestText.String) != "":
			queryText = digestText.String
		case sqlText.Valid && strings.TrimSpace(sqlText.String) != "":
			queryText = sqlText.String
		}

		var errNoPtr *int64
		if errno.Valid {
			val := errno.Int64
			errNoPtr = &val
		}

		row := mysqlErrorRow{
			Digest:      digest.String,
			Query:       queryText,
			Schema:      schemaName.String,
			ErrorNumber: errNoPtr,
			SQLState:    sqlState.String,
			Message:     message.String,
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
