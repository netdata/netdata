// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	maxQueryTextLength     = 4096
	defaultTopQueriesLimit = 500
	topQueriesHelpText     = "Top queries from MongoDB Profiler (system.profile). " +
		"WARNING: Query text may contain unmasked literals (potential PII). " +
		"Requires profiling enabled on target databases (db.setProfilingLevel)."
)

const paramSort = "__sort"

// mongoColumn defines metadata for a MongoDB profile column.
// Embeds funcapi.ColumnMeta for UI rendering and adds MongoDB-specific fields.
type mongoColumn struct {
	funcapi.ColumnMeta

	// DBField is the MongoDB document field name (e.g., "millis")
	DBField string
	// IsSortOption indicates whether this column can be used for sorting in params
	IsSortOption bool
}

// mongoColumnSet creates a ColumnSet from a slice of mongoColumn.
func mongoColumnSet(cols []mongoColumn) funcapi.ColumnSet[mongoColumn] {
	return funcapi.Columns(cols, func(c mongoColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

// mongoAllColumns defines all available columns from system.profile.
// Ordered by display priority (index).
var mongoAllColumns = []mongoColumn{
	// Core fields (visible by default)
	{ColumnMeta: funcapi.ColumnMeta{Name: "timestamp", Tooltip: "Timestamp", Type: funcapi.FieldTypeTimestamp, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummaryMax, Transform: funcapi.FieldTransformDatetime, UniqueKey: true}, DBField: "ts", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "namespace", Tooltip: "Namespace", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Sticky: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, ExpandFilter: true, GroupBy: &funcapi.GroupByOptions{IsDefault: true}}, DBField: "ns"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "operation", Tooltip: "Operation", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}, DBField: "op"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, FullWidth: true, Wrap: true, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "command"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "execution_time", Tooltip: "Execution Time", Type: funcapi.FieldTypeDuration, Units: "seconds", Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformDuration, DecimalPoints: 3, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time", IsDefault: true}}, DBField: "millis", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "docs_examined", Tooltip: "Docs Examined", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "docsExamined", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "keys_examined", Tooltip: "Keys Examined", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "keysExamined", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "docs_returned", Tooltip: "Docs Returned", Type: funcapi.FieldTypeInteger, Visible: true, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "nreturned", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "plan_summary", Tooltip: "Plan Summary", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "planSummary"},

	// Secondary fields
	{ColumnMeta: funcapi.ColumnMeta{Name: "client", Tooltip: "Client", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}, DBField: "client"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "user", Tooltip: "User", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}, DBField: "user"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "docs_deleted", Tooltip: "Docs Deleted", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "ndeleted", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "docs_inserted", Tooltip: "Docs Inserted", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "ninserted", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "docs_modified", Tooltip: "Docs Modified", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Docs", Title: "Documents"}}, DBField: "nModified", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "response_length", Tooltip: "Response Length", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Response", Title: "Response Size"}}, DBField: "responseLength", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "num_yield", Tooltip: "Num Yield", Type: funcapi.FieldTypeInteger, Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualValue, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformNumber, Chart: &funcapi.ChartOptions{Group: "Yield", Title: "Yield"}}, DBField: "numYield", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "app_name", Tooltip: "App Name", Type: funcapi.FieldTypeString, Visible: true, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText, GroupBy: &funcapi.GroupByOptions{}}, DBField: "appName"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cursor_exhausted", Tooltip: "Cursor Exhausted", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "cursorExhausted"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "has_sort_stage", Tooltip: "Has Sort Stage", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "hasSortStage"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "uses_disk", Tooltip: "Uses Disk", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "usedDisk"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "from_multi_planner", Tooltip: "From Multi Planner", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "fromMultiPlanner"},
	{ColumnMeta: funcapi.ColumnMeta{Name: "replanned", Tooltip: "Replanned", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "replanned"},

	// Version-specific fields (hidden by default)
	{ColumnMeta: funcapi.ColumnMeta{Name: "query_hash", Tooltip: "Query Hash", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "queryHash"},            // 4.2+
	{ColumnMeta: funcapi.ColumnMeta{Name: "plan_cache_key", Tooltip: "Plan Cache Key", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "planCacheKey"}, // 4.2+
	{ColumnMeta: funcapi.ColumnMeta{Name: "planning_time", Tooltip: "Planning Time", Type: funcapi.FieldTypeDuration, Units: "seconds", Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformDuration, DecimalPoints: 3, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, DBField: "planningTimeMicros", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "cpu_time", Tooltip: "CPU Time", Type: funcapi.FieldTypeDuration, Units: "seconds", Visible: false, Sortable: true, Filter: funcapi.FieldFilterRange, Visualization: funcapi.FieldVisualBar, Summary: funcapi.FieldSummarySum, Transform: funcapi.FieldTransformDuration, DecimalPoints: 3, Chart: &funcapi.ChartOptions{Group: "Time", Title: "Execution Time"}}, DBField: "cpuNanos", IsSortOption: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "query_framework", Tooltip: "Query Framework", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "queryFramework"},   // 7.0+
	{ColumnMeta: funcapi.ColumnMeta{Name: "query_shape_hash", Tooltip: "Query Shape Hash", Type: funcapi.FieldTypeString, Visible: false, Sortable: false, Filter: funcapi.FieldFilterMultiselect, Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText}, DBField: "queryShapeHash"}, // 8.0+
}

// optionalDuration converts an optional int64 pointer to float64 seconds, returning nil if nil.
func optionalDuration(v *int64, divisor float64) any {
	if v == nil {
		return nil
	}
	return float64(*v) / divisor
}

// optionalBool converts a bool pointer to a display value.
func optionalBool(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return "Yes"
	}
	return "No"
}

// buildMongoSortParam builds sort parameter from available columns.
func buildMongoSortParam(cols []mongoColumn) funcapi.ParamConfig {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.IsSortOption {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.Name,
			Column: col.DBField,
			Name:   fmt.Sprintf("Top queries by %s", col.Name),
			Sort:   &sortDir,
		}
		if col.Name == "execution_time" {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}

	return funcapi.ParamConfig{
		ID:         paramSort,
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    sortOptions,
		UniqueView: true,
	}
}

// mongoMethods returns the available function methods for MongoDB.
func mongoMethods() []module.MethodConfig {
	return []module.MethodConfig{{
		UpdateEvery:  10,
		ID:           "top-queries",
		Name:         "Top Queries",
		Help:         topQueriesHelpText,
		RequireCloud: true,
		RequiredParams: []funcapi.ParamConfig{
			buildMongoSortParam(mongoAllColumns),
		},
	}}
}

func mongoMethodParams(ctx context.Context, job *module.Job, method string) ([]funcapi.ParamConfig, error) {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return nil, fmt.Errorf("invalid module type")
	}
	if collector.conn == nil {
		return nil, fmt.Errorf("collector is still initializing")
	}
	switch method {
	case "top-queries":
		return collector.topQueriesParams(ctx)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// mongoHandleMethod handles function requests for MongoDB
func mongoHandleMethod(ctx context.Context, job *module.Job, method string, params funcapi.ResolvedParams) *module.FunctionResponse {
	collector, ok := job.Module().(*Collector)
	if !ok {
		return &module.FunctionResponse{Status: 500, Message: "internal error: invalid module type"}
	}

	// Check if collector is initialized
	if collector.conn == nil {
		return &module.FunctionResponse{
			Status:  503,
			Message: "collector is still initializing, please retry in a few seconds",
		}
	}

	switch method {
	case "top-queries":
		// Check if function is enabled
		if !collector.Config.GetTopQueriesFunctionEnabled() {
			return &module.FunctionResponse{
				Status:  403,
				Message: "Top Queries function has been disabled in configuration. Set 'top_queries_function_enabled: true' to enable.",
			}
		}
		return collector.collectTopQueries(ctx, params.Column(paramSort))
	default:
		return &module.FunctionResponse{Status: 404, Message: fmt.Sprintf("unknown method: %s", method)}
	}
}

// profileDocument represents a document from system.profile
type profileDocument struct {
	// Core fields (always present)
	Timestamp   time.Time `bson:"ts"`
	Op          string    `bson:"op"`
	Ns          string    `bson:"ns"`
	Command     bson.M    `bson:"command"`
	Millis      int64     `bson:"millis"`
	PlanSummary string    `bson:"planSummary"`

	// Common fields
	DocsExamined   int64  `bson:"docsExamined"`
	KeysExamined   int64  `bson:"keysExamined"`
	Nreturned      int64  `bson:"nreturned"`
	Client         string `bson:"client"`
	User           string `bson:"user"`
	Ndeleted       int64  `bson:"ndeleted"`
	Ninserted      int64  `bson:"ninserted"`
	NModified      int64  `bson:"nModified"`
	ResponseLength int64  `bson:"responseLength"`
	NumYield       int64  `bson:"numYield"`
	AppName        string `bson:"appName"`

	// Boolean fields (pointers for nil detection)
	CursorExhausted  *bool `bson:"cursorExhausted"`
	HasSortStage     *bool `bson:"hasSortStage"`
	UsedDisk         *bool `bson:"usedDisk"`
	FromMultiPlanner *bool `bson:"fromMultiPlanner"`
	Replanned        *bool `bson:"replanned"`

	// Version-specific fields
	QueryHash          string `bson:"queryHash"`          // 4.2+
	PlanCacheKey       string `bson:"planCacheKey"`       // 4.2+
	PlanningTimeMicros *int64 `bson:"planningTimeMicros"` // 6.2+
	CpuNanos           *int64 `bson:"cpuNanos"`           // 6.3+ Linux only
	QueryFramework     string `bson:"queryFramework"`     // 7.0+
	QueryShapeHash     string `bson:"queryShapeHash"`     // 8.0+
}

// detectMongoProfileFields detects available fields in system.profile using double-checked locking
func (c *Collector) detectMongoProfileFields(ctx context.Context, databases []string) (map[string]bool, error) {
	// Fast path: return cached
	c.topQueriesColsMu.RLock()
	if c.topQueriesCols != nil {
		cols := c.topQueriesCols
		c.topQueriesColsMu.RUnlock()
		return cols, nil
	}
	c.topQueriesColsMu.RUnlock()

	// Slow path: detect and cache
	c.topQueriesColsMu.Lock()
	defer c.topQueriesColsMu.Unlock()

	// Double-check after acquiring write lock
	if c.topQueriesCols != nil {
		return c.topQueriesCols, nil
	}

	client, ok := c.conn.(*mongoClient)
	if !ok || client == nil || client.client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	available := make(map[string]bool)

	// Always include core fields that are guaranteed to exist
	coreFields := []string{"ts", "op", "ns", "command", "millis"}
	for _, f := range coreFields {
		available[f] = true
	}

	// Sample documents from system.profile to detect available fields
	for _, dbName := range databases {
		queryCtx, cancel := context.WithTimeout(ctx, client.timeout)

		collection := client.client.Database(dbName).Collection("system.profile")
		opts := options.FindOne().SetSort(bson.D{{Key: "$natural", Value: -1}})

		var doc bson.M
		err := collection.FindOne(queryCtx, bson.M{}, opts).Decode(&doc)
		cancel()

		if err != nil {
			continue // No documents or profiling disabled
		}

		// Add all fields found in this document
		for field := range doc {
			available[field] = true
		}
	}

	c.topQueriesCols = available
	return available, nil
}

// buildAvailableMongoColumns returns columns that are available based on detected fields.
func buildAvailableMongoColumns(available map[string]bool) []mongoColumn {
	var result []mongoColumn
	for _, col := range mongoAllColumns {
		// command field maps to query column, always include
		if col.DBField == "command" || available[col.DBField] {
			result = append(result, col)
		}
	}
	return result
}

// collectTopQueries queries system.profile for top queries across all databases.
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Get limit from config
	limit := c.Config.TopQueriesLimit
	if limit <= 0 {
		limit = defaultTopQueriesLimit
	}

	// Build valid sort columns map from metadata
	validSortCols := make(map[string]bool)
	for _, col := range mongoAllColumns {
		if col.IsSortOption {
			validSortCols[col.DBField] = true
		}
	}
	if !validSortCols[sortColumn] {
		sortColumn = "millis" // safe default
	}

	// Get list of databases to query
	databases, err := c.topQueriesDatabases()
	if err != nil {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to list databases: %v", err),
		}
	}
	filteredDBs := databases

	// Detect available fields (with caching)
	availableFields, err := c.detectMongoProfileFields(ctx, filteredDBs)
	if err != nil {
		c.Debugf("failed to detect profile fields: %v", err)
	}
	availableCols := buildAvailableMongoColumns(availableFields)
	cs := mongoColumnSet(availableCols)
	sortParam := buildMongoSortParam(availableCols)

	// Query system.profile from each database
	var allDocs []profileDocument
	var profilingDisabledDBs []string
	var failedDBs []string
	var successfulDBs int

	for _, dbName := range filteredDBs {
		docs, enabled, err := c.querySystemProfile(ctx, dbName, sortColumn, limit)
		if err != nil {
			// Check for timeout (parent or child context)
			if ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
				return &module.FunctionResponse{Status: 504, Message: "query timed out"}
			}
			c.Debugf("failed to query system.profile in %s: %v", dbName, err)
			failedDBs = append(failedDBs, dbName)
			continue
		}

		if !enabled {
			profilingDisabledDBs = append(profilingDisabledDBs, dbName)
			continue
		}

		successfulDBs++
		allDocs = append(allDocs, docs...)
	}

	// Check if all databases failed with errors (not just profiling disabled)
	if successfulDBs == 0 && len(failedDBs) > 0 && len(profilingDisabledDBs) == 0 {
		return &module.FunctionResponse{
			Status:  500,
			Message: fmt.Sprintf("failed to query all databases: %v", failedDBs),
		}
	}

	// Check if profiling is disabled everywhere (no successful queries, no errors, only disabled)
	if len(allDocs) == 0 && len(profilingDisabledDBs) > 0 && len(failedDBs) == 0 {
		return &module.FunctionResponse{
			Status: 503,
			Message: fmt.Sprintf(
				"Database profiling is disabled. Enable it with: db.setProfilingLevel(1, {slowms: 100}). "+
					"Disabled databases: %v", profilingDisabledDBs),
		}
	}

	// Check if we have a mix of failures and disabled profiling (no successful queries at all)
	if successfulDBs == 0 && (len(failedDBs) > 0 || len(profilingDisabledDBs) > 0) {
		msg := "No databases could be queried successfully."
		if len(failedDBs) > 0 {
			msg += fmt.Sprintf(" Failed: %v.", failedDBs)
		}
		if len(profilingDisabledDBs) > 0 {
			msg += fmt.Sprintf(" Profiling disabled: %v.", profilingDisabledDBs)
		}
		return &module.FunctionResponse{
			Status:  503,
			Message: msg,
		}
	}

	// Build empty response structure
	emptyResponse := &module.FunctionResponse{
		Status:            200,
		Message:           "No slow queries found. Profiling may be disabled or no queries exceeded the slowms threshold.",
		Help:              topQueriesHelpText,
		Columns:           cs.BuildColumns(),
		Data:              [][]any{},
		DefaultSortColumn: "execution_time",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}

	if len(allDocs) == 0 {
		return emptyResponse
	}

	// Sort all documents by the requested column
	sortProfileDocuments(allDocs, sortColumn)

	// Apply limit
	if len(allDocs) > limit {
		allDocs = allDocs[:limit]
	}

	// Convert to response format: [][]any (array of arrays, ordered by column index)
	data := make([][]any, 0, len(allDocs))
	for _, doc := range allDocs {
		row := make([]any, len(availableCols))

		// Fill each column based on available columns
		for i, col := range availableCols {
			switch col.Name {
			case "timestamp":
				row[i] = doc.Timestamp.Format(time.RFC3339Nano)
			case "namespace":
				row[i] = doc.Ns
			case "operation":
				row[i] = doc.Op
			case "query":
				cmdJSON, err := json.Marshal(doc.Command)
				if err != nil {
					cmdJSON = []byte("{}")
				}
				row[i] = strmutil.TruncateText(string(cmdJSON), maxQueryTextLength)
			case "execution_time":
				row[i] = float64(doc.Millis) / 1000.0 // ms to seconds
			case "docs_examined":
				row[i] = doc.DocsExamined
			case "keys_examined":
				row[i] = doc.KeysExamined
			case "docs_returned":
				row[i] = doc.Nreturned
			case "plan_summary":
				row[i] = doc.PlanSummary
			case "client":
				row[i] = doc.Client
			case "user":
				row[i] = doc.User
			case "docs_deleted":
				row[i] = doc.Ndeleted
			case "docs_inserted":
				row[i] = doc.Ninserted
			case "docs_modified":
				row[i] = doc.NModified
			case "response_length":
				row[i] = doc.ResponseLength
			case "num_yield":
				row[i] = doc.NumYield
			case "app_name":
				row[i] = doc.AppName
			case "cursor_exhausted":
				row[i] = optionalBool(doc.CursorExhausted)
			case "has_sort_stage":
				row[i] = optionalBool(doc.HasSortStage)
			case "uses_disk":
				row[i] = optionalBool(doc.UsedDisk)
			case "from_multi_planner":
				row[i] = optionalBool(doc.FromMultiPlanner)
			case "replanned":
				row[i] = optionalBool(doc.Replanned)
			case "query_hash":
				row[i] = doc.QueryHash
			case "plan_cache_key":
				row[i] = doc.PlanCacheKey
			case "planning_time":
				row[i] = optionalDuration(doc.PlanningTimeMicros, 1000000.0) // us to seconds
			case "cpu_time":
				row[i] = optionalDuration(doc.CpuNanos, 1000000000.0) // ns to seconds
			case "query_framework":
				row[i] = doc.QueryFramework
			case "query_shape_hash":
				row[i] = doc.QueryShapeHash
			default:
				row[i] = nil
			}
		}

		data = append(data, row)
	}

	return &module.FunctionResponse{
		Status:            200,
		Help:              topQueriesHelpText,
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "execution_time",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            cs.BuildCharts(),
		DefaultCharts:     cs.BuildDefaultCharts(),
		GroupBy:           cs.BuildGroupBy(),
	}
}

func (c *Collector) topQueriesDatabases() ([]string, error) {
	databases, err := c.conn.listDatabaseNames()
	if err != nil {
		return nil, err
	}

	var filteredDBs []string
	for _, dbName := range databases {
		if dbName == "admin" || dbName == "local" || dbName == "config" {
			continue
		}
		if c.dbSelector != nil && !c.dbSelector.MatchString(dbName) {
			continue
		}
		filteredDBs = append(filteredDBs, dbName)
	}

	return filteredDBs, nil
}

func (c *Collector) topQueriesParams(ctx context.Context) ([]funcapi.ParamConfig, error) {
	if !c.Config.GetTopQueriesFunctionEnabled() {
		return nil, fmt.Errorf("top queries function disabled")
	}

	databases, err := c.topQueriesDatabases()
	if err != nil {
		return nil, err
	}

	availableFields, err := c.detectMongoProfileFields(ctx, databases)
	if err != nil {
		return nil, err
	}

	availableCols := buildAvailableMongoColumns(availableFields)
	sortParam := buildMongoSortParam(availableCols)
	return []funcapi.ParamConfig{sortParam}, nil
}

// querySystemProfile queries the system.profile collection for a specific database
func (c *Collector) querySystemProfile(ctx context.Context, dbName, sortColumn string, limit int) ([]profileDocument, bool, error) {
	client, ok := c.conn.(*mongoClient)
	if !ok || client == nil || client.client == nil {
		return nil, false, fmt.Errorf("client not initialized")
	}

	// Set timeout for the query
	queryCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	// Check if profiling is enabled for this database
	var profilingStatus struct {
		Was int `bson:"was"`
	}
	err := client.client.Database(dbName).RunCommand(queryCtx, bson.D{{Key: "profile", Value: -1}}).Decode(&profilingStatus)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check profiling status: %w", err)
	}

	if profilingStatus.Was == 0 {
		return nil, false, nil // Profiling disabled
	}

	// Query system.profile
	collection := client.client.Database(dbName).Collection("system.profile")

	// Build sort order (descending for all except timestamp which can be either)
	sortOrder := -1 // descending by default
	sortField := sortColumn

	findOpts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortOrder}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(queryCtx, bson.M{}, findOpts)
	if err != nil {
		return nil, true, fmt.Errorf("find failed: %w", err)
	}
	defer cursor.Close(queryCtx)

	var docs []profileDocument
	if err := cursor.All(queryCtx, &docs); err != nil {
		return nil, true, fmt.Errorf("cursor.All failed: %w", err)
	}

	return docs, true, nil
}

// sortProfileDocuments sorts documents in place by the specified column (descending)
func sortProfileDocuments(docs []profileDocument, sortColumn string) {
	sort.Slice(docs, func(i, j int) bool {
		switch sortColumn {
		case "millis":
			return docs[i].Millis > docs[j].Millis
		case "docsExamined":
			return docs[i].DocsExamined > docs[j].DocsExamined
		case "keysExamined":
			return docs[i].KeysExamined > docs[j].KeysExamined
		case "nreturned":
			return docs[i].Nreturned > docs[j].Nreturned
		case "ts":
			return docs[i].Timestamp.After(docs[j].Timestamp)
		case "ndeleted":
			return docs[i].Ndeleted > docs[j].Ndeleted
		case "ninserted":
			return docs[i].Ninserted > docs[j].Ninserted
		case "nModified":
			return docs[i].NModified > docs[j].NModified
		case "responseLength":
			return docs[i].ResponseLength > docs[j].ResponseLength
		case "numYield":
			return docs[i].NumYield > docs[j].NumYield
		case "planningTimeMicros":
			vi := int64(0)
			vj := int64(0)
			if docs[i].PlanningTimeMicros != nil {
				vi = *docs[i].PlanningTimeMicros
			}
			if docs[j].PlanningTimeMicros != nil {
				vj = *docs[j].PlanningTimeMicros
			}
			return vi > vj
		case "cpuNanos":
			vi := int64(0)
			vj := int64(0)
			if docs[i].CpuNanos != nil {
				vi = *docs[i].CpuNanos
			}
			if docs[j].CpuNanos != nil {
				vj = *docs[j].CpuNanos
			}
			return vi > vj
		default:
			return docs[i].Millis > docs[j].Millis
		}
	})
}
