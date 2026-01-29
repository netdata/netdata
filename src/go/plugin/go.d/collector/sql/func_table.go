// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// sqlJobMethods returns the method config for a specific SQL job.
// The method ID is the job name, so each job registers as "sql:jobname".
func sqlJobMethods(job *module.Job) []funcapi.MethodConfig {
	c, ok := job.Module().(*Collector)
	if !ok || len(c.Config.Functions) == 0 {
		return nil
	}

	// Build method name from job name (e.g., "postgres_test" → "Postgres Test")
	methodName := deriveNameFromID(job.Name())

	// Build help text from first function's description or default
	help := "Execute a configured SQL query and return results as a table"
	if len(c.Config.Functions) > 0 && c.Config.Functions[0].Description != "" {
		help = c.Config.Functions[0].Description
	}

	return []funcapi.MethodConfig{{
		ID:          job.Name(),
		Name:        methodName,
		Help:        help,
		UpdateEvery: 10,
	}}
}

func sqlMethodHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcTable
}

func (f *funcTable) Cleanup(context.Context) {
	// No-op: DB connection managed by collector
}

func (f *funcTable) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.collector.db == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	if len(f.collector.Config.Functions) == 0 {
		return nil, fmt.Errorf("no functions configured")
	}

	options := make([]funcapi.ParamOption, len(f.collector.Config.Functions))
	for i, fn := range f.collector.Config.Functions {
		options[i] = funcapi.ParamOption{
			ID:      fn.ID,
			Name:    fn.derivedName(),
			Default: i == 0,
		}
	}

	return []funcapi.ParamConfig{{
		ID:        "__function",
		Name:      "Function",
		Selection: funcapi.ParamSelect,
		Options:   options,
	}}, nil
}

func (f *funcTable) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.collector.db == nil {
		return funcapi.ErrorResponse(503, "database connection not initialized")
	}

	functionID := params.GetOne("__function")
	funcCfg := f.findFunction(functionID)
	if funcCfg == nil {
		return funcapi.ErrorResponse(404, "unknown function: %s", functionID)
	}
	return f.executeFunction(ctx, funcCfg)
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
