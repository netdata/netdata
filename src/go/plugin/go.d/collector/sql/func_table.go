// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type funcTable struct {
	collector *Collector
}

func newFuncTable(c *Collector) *funcTable {
	return &funcTable{collector: c}
}

var _ funcapi.MethodHandler = (*funcTable)(nil)

// sqlJobMethods returns method configs for a specific SQL job.
// Each configured function becomes a separate method: "jobName:functionID"
// This results in functions like "sql:postgres_test:active-queries"
func sqlJobMethods(job module.RuntimeJob) []funcapi.MethodConfig {
	c, ok := job.Collector().(*Collector)
	if !ok || len(c.Config.Functions) == 0 {
		return nil
	}

	methods := make([]funcapi.MethodConfig, 0, len(c.Config.Functions))
	for _, fn := range c.Config.Functions {
		// Method ID format: "jobName:functionID" (e.g., "postgres_test:active-queries")
		// Full function name will be: "sql:postgres_test:active-queries"
		methodID := job.Name() + ":" + fn.ID

		methodName := fn.derivedName()
		help := fn.Description
		if help == "" {
			help = "Execute SQL query: " + fn.ID
		}

		methods = append(methods, funcapi.MethodConfig{
			ID:           methodID,
			Name:         methodName,
			Help:         help,
			UpdateEvery:  10,
			RequireCloud: true,
		})
	}

	return methods
}

func sqlMethodHandler(job module.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcTable
}

func (f *funcTable) Cleanup(context.Context) {
	// No-op: DB connection managed by collector
}

func (f *funcTable) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	// Each function is now a separate method endpoint, no __function selector needed
	return nil, nil
}

func (f *funcTable) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	// Check if collector is shutting down (fast path)
	if f.collector.dbCtx != nil && f.collector.dbCtx.Err() != nil {
		return funcapi.ErrorResponse(503, "collector is shutting down")
	}

	// Acquire read lock to prevent DB close during query
	f.collector.dbMu.RLock()
	defer f.collector.dbMu.RUnlock()

	if f.collector.db == nil {
		return funcapi.ErrorResponse(503, "database connection not initialized")
	}

	// Method format is "jobName:functionID" (e.g., "postgres_test:active-queries")
	// Extract the functionID part after the last colon
	functionID := method
	if idx := strings.LastIndex(method, ":"); idx != -1 {
		functionID = method[idx+1:]
	}

	funcCfg := f.findFunction(functionID)
	if funcCfg == nil {
		return funcapi.ErrorResponse(404, "unknown function: %s", functionID)
	}

	// Merge request context with collector's shutdown context
	// Query cancels if either request times out OR collector shuts down
	queryCtx, queryCancel := context.WithCancel(ctx)
	defer queryCancel()

	if f.collector.dbCtx != nil {
		stop := context.AfterFunc(f.collector.dbCtx, queryCancel)
		defer stop()
	}

	return f.executeFunction(queryCtx, funcCfg)
}

func (f *funcTable) findFunction(id string) *ConfigFunction {
	for i := range f.collector.Config.Functions {
		if f.collector.Config.Functions[i].ID == id {
			return &f.collector.Config.Functions[i]
		}
	}
	return nil
}

func (f *funcTable) executeFunction(ctx context.Context, cfg *ConfigFunction) *funcapi.FunctionResponse {
	timeout := f.collector.Timeout.Duration()
	if cfg.Timeout.Duration() > 0 {
		timeout = cfg.Timeout.Duration()
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rows, err := f.collector.db.QueryContext(queryCtx, cfg.Query)
	if err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timeout after %v", timeout)
		}
		return funcapi.ErrorResponse(500, "query failed: %v", err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return funcapi.ErrorResponse(500, "failed to get column types: %v", err)
	}

	sortDesc := cfg.DefaultSortDesc == nil || *cfg.DefaultSortDesc
	columns := f.buildColumnMetadata(colTypes, cfg.Columns, cfg.DefaultSort, sortDesc)

	limit := cfg.Limit
	if limit <= 0 {
		limit = defaultFunctionLimit
	}
	if limit > maxFunctionLimit {
		limit = maxFunctionLimit
	}

	data := make([][]any, 0, limit)
	for rows.Next() && len(data) < limit {
		row, err := f.scanRow(rows, len(colTypes))
		if err != nil {
			f.collector.Warningf("scan row failed: %v", err)
			continue
		}
		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timeout during iteration")
		}
		return funcapi.ErrorResponse(500, "row iteration failed: %v", err)
	}

	defaultSort := cfg.DefaultSort
	if defaultSort != "" {
		found := false
		for _, ct := range colTypes {
			if ct.Name() == defaultSort {
				found = true
				break
			}
		}
		if !found {
			f.collector.Warningf("function %q: default_sort column %q not in query results", cfg.ID, defaultSort)
			defaultSort = ""
		}
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              cfg.Description,
		Columns:           columns,
		Data:              data,
		DefaultSortColumn: defaultSort,
	}
}

func (f *funcTable) scanRow(rows *sql.Rows, numCols int) ([]any, error) {
	values := make([]any, numCols)
	ptrs := make([]any, numCols)
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	for i, v := range values {
		values[i] = normalizeValue(v)
	}
	return values, nil
}

func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		return string(val)
	case time.Time:
		return val.UnixMilli()
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case float32:
		return float64(val)
	default:
		return v
	}
}

// typeMapping maps database type names to funcapi field types.
// Covers MySQL, PostgreSQL (pgx), SQL Server, and Oracle drivers.
var typeMapping = map[string]funcapi.FieldType{
	// MySQL (uppercase)
	"INT":       funcapi.FieldTypeInteger,
	"BIGINT":    funcapi.FieldTypeInteger,
	"TINYINT":   funcapi.FieldTypeInteger,
	"SMALLINT":  funcapi.FieldTypeInteger,
	"MEDIUMINT": funcapi.FieldTypeInteger,
	"FLOAT":     funcapi.FieldTypeFloat,
	"DOUBLE":    funcapi.FieldTypeFloat,
	"DECIMAL":   funcapi.FieldTypeFloat,
	"VARCHAR":   funcapi.FieldTypeString,
	"CHAR":      funcapi.FieldTypeString,
	"TEXT":      funcapi.FieldTypeString,
	"DATETIME":  funcapi.FieldTypeTimestamp,
	"TIMESTAMP": funcapi.FieldTypeTimestamp,
	"DATE":      funcapi.FieldTypeTimestamp,
	"TIME":      funcapi.FieldTypeDuration,

	// PostgreSQL (pgx) - lowercase
	"int2":        funcapi.FieldTypeInteger,
	"int4":        funcapi.FieldTypeInteger,
	"int8":        funcapi.FieldTypeInteger,
	"smallint":    funcapi.FieldTypeInteger,
	"integer":     funcapi.FieldTypeInteger,
	"bigint":      funcapi.FieldTypeInteger,
	"float4":      funcapi.FieldTypeFloat,
	"float8":      funcapi.FieldTypeFloat,
	"numeric":     funcapi.FieldTypeFloat,
	"decimal":     funcapi.FieldTypeFloat,
	"varchar":     funcapi.FieldTypeString,
	"char":        funcapi.FieldTypeString,
	"text":        funcapi.FieldTypeString,
	"bpchar":      funcapi.FieldTypeString,
	"timestamp":   funcapi.FieldTypeTimestamp,
	"timestamptz": funcapi.FieldTypeTimestamp,
	"date":        funcapi.FieldTypeTimestamp,
	"bool":        funcapi.FieldTypeBoolean,
	"boolean":     funcapi.FieldTypeBoolean,
	"interval":    funcapi.FieldTypeDuration,

	// SQL Server
	"NVARCHAR":  funcapi.FieldTypeString,
	"NCHAR":     funcapi.FieldTypeString,
	"DATETIME2": funcapi.FieldTypeTimestamp,
	"BIT":       funcapi.FieldTypeBoolean,
	"REAL":      funcapi.FieldTypeFloat,

	// Oracle
	"VARCHAR2":      funcapi.FieldTypeString,
	"NVARCHAR2":     funcapi.FieldTypeString,
	"CLOB":          funcapi.FieldTypeString,
	"NUMBER":        funcapi.FieldTypeFloat, // Could be int, safer as float
	"BINARY_FLOAT":  funcapi.FieldTypeFloat,
	"BINARY_DOUBLE": funcapi.FieldTypeFloat,
}

func inferType(dbTypeName string) funcapi.FieldType {
	if t, ok := typeMapping[dbTypeName]; ok {
		return t
	}
	if t, ok := typeMapping[strings.ToUpper(dbTypeName)]; ok {
		return t
	}
	if t, ok := typeMapping[strings.ToLower(dbTypeName)]; ok {
		return t
	}
	return funcapi.FieldTypeString
}

func parseFieldType(s string) funcapi.FieldType {
	switch strings.ToLower(s) {
	case "integer":
		return funcapi.FieldTypeInteger
	case "float":
		return funcapi.FieldTypeFloat
	case "boolean":
		return funcapi.FieldTypeBoolean
	case "duration":
		return funcapi.FieldTypeDuration
	case "timestamp":
		return funcapi.FieldTypeTimestamp
	default:
		return funcapi.FieldTypeString
	}
}

func deriveTransform(fieldType funcapi.FieldType) funcapi.FieldTransform {
	switch fieldType {
	case funcapi.FieldTypeInteger, funcapi.FieldTypeFloat:
		return funcapi.FieldTransformNumber
	case funcapi.FieldTypeDuration:
		return funcapi.FieldTransformDuration
	case funcapi.FieldTypeTimestamp:
		return funcapi.FieldTransformDatetime
	default:
		return funcapi.FieldTransformNone
	}
}

func deriveFilterSummary(fieldType funcapi.FieldType) (funcapi.FieldFilter, funcapi.FieldSummary) {
	switch fieldType {
	case funcapi.FieldTypeInteger, funcapi.FieldTypeFloat, funcapi.FieldTypeDuration:
		return funcapi.FieldFilterRange, funcapi.FieldSummarySum
	case funcapi.FieldTypeTimestamp:
		return funcapi.FieldFilterRange, funcapi.FieldSummaryMax
	case funcapi.FieldTypeBoolean:
		return funcapi.FieldFilterMultiselect, funcapi.FieldSummaryCount
	default:
		return funcapi.FieldFilterMultiselect, funcapi.FieldSummaryCount
	}
}

// buildColumnMetadata constructs column definitions from SQL query results.
//
// This uses funcapi.Column directly instead of funcapi.ColumnMeta because:
// - Columns are discovered dynamically at runtime from query results
// - There are no static, pre-defined columns to embed ColumnMeta into
// - ColumnMeta is designed for collectors with static columns and custom Value extractors
func (f *funcTable) buildColumnMetadata(colTypes []*sql.ColumnType, overrides map[string]ConfigFuncColumn, defaultSort string, sortDesc bool) map[string]any {
	columns := make(map[string]any, len(colTypes))

	for i, ct := range colTypes {
		colName := ct.Name()

		fieldType := inferType(ct.DatabaseTypeName())

		override, hasOverride := overrides[colName]
		if hasOverride && override.Type != "" {
			fieldType = parseFieldType(override.Type)
		}

		transform := deriveTransform(fieldType)
		filter, summary := deriveFilterSummary(fieldType)

		visible := true
		if hasOverride && override.Visible != nil {
			visible = *override.Visible
		}

		sortable := true
		if hasOverride && override.Sortable != nil {
			sortable = *override.Sortable
		}

		sort := funcapi.FieldSortAscending
		if colName == defaultSort && sortDesc {
			sort = funcapi.FieldSortDescending
		}

		units := ""
		if hasOverride {
			units = override.Units
		}

		tooltip := colName
		if hasOverride && override.Tooltip != "" {
			tooltip = override.Tooltip
		}

		col := funcapi.Column{
			Index:    i,
			Name:     tooltip,
			Type:     fieldType,
			Units:    units,
			Visible:  visible,
			Sortable: sortable,
			Sort:     sort,
			Filter:   filter,
			Summary:  summary,
			ValueOptions: funcapi.ValueOptions{
				Transform: transform,
			},
		}
		columns[colName] = col.BuildColumn()
	}

	return columns
}
