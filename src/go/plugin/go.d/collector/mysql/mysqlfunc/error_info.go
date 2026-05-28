// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/sqlquery"
)

const (
	mysqlErrorAttrEnabled      = "enabled"
	mysqlErrorAttrNotEnabled   = "not_enabled"
	mysqlErrorAttrNotSupported = "not_supported"
	mysqlErrorAttrNoData       = "no_data"
)

const errorInfoMethodID = "error-info"

// errorInfoColumn defines a column for the error-info function.
type errorInfoColumn struct {
	funcapi.ColumnMeta
	Value func(*mysqlErrorRow) any
}

func errorInfoColumnSet(cols []errorInfoColumn) funcapi.ColumnSet[errorInfoColumn] {
	return funcapi.Columns(cols, func(c errorInfoColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var errorInfoColumns = []errorInfoColumn{
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "digest",
			Tooltip:   "Normalized hash of the query for grouping similar statements",
			Type:      funcapi.FieldTypeString,
			Sortable:  true,
			Visible:   false,
			UniqueKey: true,
		},
		Value: func(r *mysqlErrorRow) any { return r.Digest },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "query",
			Tooltip:   "The SQL statement that caused the error",
			Type:      funcapi.FieldTypeString,
			Sortable:  true,
			Visible:   true,
			Sticky:    true,
			FullWidth: true,
		},
		Value: func(r *mysqlErrorRow) any { return r.Query },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "schema",
			Tooltip:  "Database/schema where the error occurred",
			Type:     funcapi.FieldTypeString,
			Sortable: true,
			Visible:  true,
		},
		Value: func(r *mysqlErrorRow) any { return r.Schema },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "errorNumber",
			Tooltip:   "MySQL error number (MYSQL_ERRNO)",
			Type:      funcapi.FieldTypeInteger,
			Sortable:  true,
			Visible:   true,
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *mysqlErrorRow) any {
			if r.ErrorNumber == nil {
				return nil
			}
			return *r.ErrorNumber
		},
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "sqlState",
			Tooltip:  "5-character SQLSTATE error code",
			Type:     funcapi.FieldTypeString,
			Sortable: true,
			Visible:  true,
		},
		Value: func(r *mysqlErrorRow) any { return r.SQLState },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "errorMessage",
			Tooltip:   "The error message text",
			Type:      funcapi.FieldTypeString,
			Sortable:  false,
			Visible:   true,
			FullWidth: true,
		},
		Value: func(r *mysqlErrorRow) any { return r.Message },
	},
}

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
	router *router
}

func newFuncErrorInfo(r *router) *funcErrorInfo {
	return &funcErrorInfo{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcErrorInfo)(nil)

func (f *funcErrorInfo) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.cfg.errorInfoDisabled() {
		return nil, fmt.Errorf("error-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (f *funcErrorInfo) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if _, err := f.router.deps.DB(); err != nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	queryCtx, cancel := context.WithTimeout(ctx, f.router.cfg.errorInfoTimeout())
	defer cancel()
	return f.collectData(queryCtx)
}

func (f *funcErrorInfo) Cleanup(ctx context.Context) {}

func (f *funcErrorInfo) collectData(ctx context.Context) *funcapi.FunctionResponse {
	if f.router.cfg.errorInfoDisabled() {
		return funcapi.UnavailableResponse("error-info function has been disabled in configuration")
	}

	available, err := isPerformanceSchemaEnabled(ctx, f.router.deps)
	if err != nil {
		return &funcapi.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to check performance_schema availability: %v", err),
		}
	}
	if !available {
		return &funcapi.FunctionResponse{Status: 503, Message: "performance_schema is not enabled"}
	}

	source, err := detectMySQLErrorHistorySource(ctx, f.router.deps, f.router.cfg)
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

	limit := f.router.cfg.topQueriesLimit()

	rows, err := fetchMySQLErrorRows(ctx, f.router.deps, f.router.cfg, source, nil, limit)
	if err != nil {
		return &funcapi.FunctionResponse{Status: 500, Message: fmt.Sprintf("error-info query failed: %v", err)}
	}
	if len(rows) == 0 && source.fallbackTable != "" {
		fallback, ferr := buildMySQLErrorSource(ctx, f.router.deps, f.router.cfg, source.fallbackTable)
		if ferr == nil && fallback.status == mysqlErrorAttrEnabled {
			fallbackRows, ferr := fetchMySQLErrorRows(ctx, f.router.deps, f.router.cfg, fallback, nil, limit)
			if ferr == nil && len(fallbackRows) > 0 {
				rows = fallbackRows
			}
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
		Help:              "Recent SQL errors from performance_schema statement history tables",
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "errorNumber",
	}
}

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

func collectMySQLErrorDetailsForDigests(ctx context.Context, deps Deps, cfg FunctionsConfig, log *logger.Logger, digests []string) (string, map[string]mysqlErrorRow) {
	source, err := detectMySQLErrorHistorySource(ctx, deps, cfg)
	if err != nil {
		log.Debugf("error attribution: %v", err)
		return mysqlErrorAttrNotEnabled, nil
	}
	if source.status != mysqlErrorAttrEnabled {
		return source.status, nil
	}

	rows, err := fetchMySQLErrorRows(ctx, deps, cfg, source, digests, len(digests))
	if err != nil {
		log.Debugf("error attribution query failed: %v", err)
		return mysqlErrorAttrNotEnabled, nil
	}
	if len(rows) == 0 && source.fallbackTable != "" {
		fallback, ferr := buildMySQLErrorSource(ctx, deps, cfg, source.fallbackTable)
		if ferr == nil && fallback.status == mysqlErrorAttrEnabled {
			fallbackRows, ferr := fetchMySQLErrorRows(ctx, deps, cfg, fallback, digests, len(digests))
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

func detectMySQLErrorHistorySource(ctx context.Context, deps Deps, cfg FunctionsConfig) (mysqlErrorSource, error) {
	qctx, cancel := context.WithTimeout(ctx, cfg.collectorTimeout())
	defer cancel()

	db, err := deps.DB()
	if err != nil {
		return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "collector database is not ready"}, err
	}
	rows, err := db.QueryContext(qctx, `
SELECT NAME, ENABLED
FROM performance_schema.setup_consumers
WHERE NAME IN ('events_statements_history_long','events_statements_history','events_statements_current');
`)
	if err != nil {
		return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "unable to read performance_schema.setup_consumers"}, err
	}
	defer rows.Close()

	enabled := map[string]bool{}
	for rows.Next() {
		var name, enabledVal string
		if err := rows.Scan(&name, &enabledVal); err != nil {
			return mysqlErrorSource{status: mysqlErrorAttrNotEnabled, reason: "unable to read performance_schema.setup_consumers"}, err
		}
		key := strings.ToLower(name)
		enabled[key] = strings.EqualFold(enabledVal, "YES")
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

	source, err := buildMySQLErrorSource(ctx, deps, cfg, table)
	if err != nil {
		return source, err
	}
	if source.status != mysqlErrorAttrEnabled {
		return source, nil
	}
	source.fallbackTable = fallback
	return source, nil
}

func buildMySQLErrorSource(ctx context.Context, deps Deps, cfg FunctionsConfig, table string) (mysqlErrorSource, error) {
	cols, err := fetchMySQLTableColumns(ctx, deps, cfg, table)
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

func fetchMySQLTableColumns(ctx context.Context, deps Deps, cfg FunctionsConfig, table string) (map[string]bool, error) {
	qctx, cancel := context.WithTimeout(ctx, cfg.collectorTimeout())
	defer cancel()

	db, err := deps.DB()
	if err != nil {
		return nil, err
	}
	return sqlquery.FetchTableColumns(
		qctx,
		db,
		"performance_schema",
		table,
		sqlquery.PlaceholderQuestion,
		strings.ToUpper,
	)
}

func fetchMySQLErrorRows(ctx context.Context, deps Deps, cfg FunctionsConfig, source mysqlErrorSource, digests []string, limit int) ([]mysqlErrorRow, error) {
	if source.status != mysqlErrorAttrEnabled {
		return nil, fmt.Errorf("error history not enabled")
	}

	query, args, schemaCol := buildMySQLErrorQuery(source, digests, limit)

	qctx, cancel := context.WithTimeout(ctx, cfg.collectorTimeout())
	defer cancel()

	db, err := deps.DB()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(qctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMySQLErrorRows(rows, source, schemaCol)
}

func buildMySQLErrorQuery(source mysqlErrorSource, digests []string, limit int) (string, []any, string) {
	selectCols := []string{"DIGEST", "MYSQL_ERRNO", "RETURNED_SQLSTATE", "MESSAGE_TEXT"}
	if source.columns["DIGEST_TEXT"] {
		selectCols = append(selectCols, "DIGEST_TEXT")
	}

	schemaCol := ""
	if source.columns["SCHEMA_NAME"] {
		schemaCol = "SCHEMA_NAME"
	} else if source.columns["CURRENT_SCHEMA"] {
		schemaCol = "CURRENT_SCHEMA"
	}
	if schemaCol != "" {
		selectCols = append(selectCols, schemaCol)
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

	filters := []string{"MYSQL_ERRNO <> 0"}
	args := make([]any, 0, len(digests))
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
	return query, args, schemaCol
}

func scanMySQLErrorRows(rows *sql.Rows, source mysqlErrorSource, schemaCol string) ([]mysqlErrorRow, error) {
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
		if schemaCol != "" {
			scanTargets = append(scanTargets, &schemaName)
		}
		if source.columns["SQL_TEXT"] {
			scanTargets = append(scanTargets, &sqlText)
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return nil, err
		}

		digestKey := ""
		if digest.Valid && strings.TrimSpace(digest.String) != "" {
			digestKey = digest.String
		} else if sqlText.Valid && strings.TrimSpace(sqlText.String) != "" {
			// Generate synthetic digest from SQL_TEXT + MYSQL_ERRNO when DIGEST is NULL.
			// This happens for statements that fail during parsing (syntax errors, etc.).
			digestKey = generateSyntheticDigest(sqlText.String, errno.Int64)
		} else {
			continue
		}

		if seen[digestKey] {
			continue
		}
		seen[digestKey] = true

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
			Digest:      digestKey,
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

// generateSyntheticDigest creates a digest-like identifier for error rows
// where the real DIGEST is NULL (e.g., syntax errors that fail during parsing).
// It combines SQL_TEXT and MYSQL_ERRNO to provide meaningful deduplication.
func generateSyntheticDigest(sqlText string, errno int64) string {
	h := md5.New()
	h.Write([]byte(sqlText))
	h.Write([]byte("_"))
	h.Write([]byte(strconv.FormatInt(errno, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
