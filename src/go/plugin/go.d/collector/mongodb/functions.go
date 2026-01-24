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

const (
	paramSort = "__sort"

	ftString    = funcapi.FieldTypeString
	ftInteger   = funcapi.FieldTypeInteger
	ftDuration  = funcapi.FieldTypeDuration
	ftTimestamp = funcapi.FieldTypeTimestamp

	trNumber   = funcapi.FieldTransformNumber
	trDuration = funcapi.FieldTransformDuration
	trDatetime = funcapi.FieldTransformDatetime
	trText     = funcapi.FieldTransformText

	visValue = funcapi.FieldVisualValue
	visBar   = funcapi.FieldVisualBar

	summarySum = funcapi.FieldSummarySum
	summaryMax = funcapi.FieldSummaryMax

	filterRange = funcapi.FieldFilterRange
	filterMulti = funcapi.FieldFilterMultiselect
)

// mongoColumnMeta defines metadata for a column in the response
type mongoColumnMeta struct {
	id             string                 // column ID in response (e.g., "execution_time")
	dbField        string                 // MongoDB document field name (e.g., "millis")
	name           string                 // display name (e.g., "Execution Time")
	colType        funcapi.FieldType      // column type: integer, duration, timestamp, string, bool
	visible        bool                   // default visibility
	sortable       bool                   // can be used for sorting
	fullWidth      bool                   // for query text columns
	wrap           bool                   // wrap text
	sticky         bool                   // sticky column
	filter         funcapi.FieldFilter    // filter type: range, multiselect, text
	visualization  funcapi.FieldVisual    // visualization type: value, bar
	summary        funcapi.FieldSummary   // summary type: sum, max, or empty
	transform      funcapi.FieldTransform // value transform: number, duration, datetime, text
	units          string                 // display units (e.g., "seconds")
	decimalPoints  int                    // decimal points for numeric display
	uniqueKey      bool                   // unique key column
	expandFilter   bool                   // default expanded filter
	isLabel        bool                   // available for group-by
	isPrimary      bool                   // primary label
	isMetric       bool                   // chartable metric
	chartGroup     string                 // chart group key
	chartTitle     string                 // chart title
	isDefaultChart bool                   // include in default charts
}

// mongoAllColumns defines all available columns from system.profile
// Ordered by display priority (index)
var mongoAllColumns = []mongoColumnMeta{
	// Core fields (visible by default)
	{id: "timestamp", dbField: "ts", name: "Timestamp", colType: ftTimestamp, visible: true, sortable: true, filter: filterRange, visualization: visValue, summary: summaryMax, transform: trDatetime, uniqueKey: true},
	{id: "namespace", dbField: "ns", name: "Namespace", colType: ftString, visible: true, sortable: false, sticky: true, filter: filterMulti, visualization: visValue, transform: trText, expandFilter: true, isLabel: true, isPrimary: true},
	{id: "operation", dbField: "op", name: "Operation", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "query", dbField: "command", name: "Query", colType: ftString, visible: true, sortable: false, fullWidth: true, wrap: true, filter: filterMulti, visualization: visValue, transform: trText},
	{id: "execution_time", dbField: "millis", name: "Execution Time", colType: ftDuration, visible: true, sortable: true, filter: filterRange, visualization: visBar, summary: summarySum, transform: trDuration, units: "seconds", decimalPoints: 3, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time", isDefaultChart: true},
	{id: "docs_examined", dbField: "docsExamined", name: "Docs Examined", colType: ftInteger, visible: true, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "keys_examined", dbField: "keysExamined", name: "Keys Examined", colType: ftInteger, visible: true, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "docs_returned", dbField: "nreturned", name: "Docs Returned", colType: ftInteger, visible: true, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "plan_summary", dbField: "planSummary", name: "Plan Summary", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},

	// Secondary fields
	{id: "client", dbField: "client", name: "Client", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "user", dbField: "user", name: "User", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "docs_deleted", dbField: "ndeleted", name: "Docs Deleted", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "docs_inserted", dbField: "ninserted", name: "Docs Inserted", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "docs_modified", dbField: "nModified", name: "Docs Modified", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Docs", chartTitle: "Documents"},
	{id: "response_length", dbField: "responseLength", name: "Response Length", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Response", chartTitle: "Response Size"},
	{id: "num_yield", dbField: "numYield", name: "Num Yield", colType: ftInteger, visible: false, sortable: true, filter: filterRange, visualization: visValue, summary: summarySum, transform: trNumber, isMetric: true, chartGroup: "Yield", chartTitle: "Yield"},
	{id: "app_name", dbField: "appName", name: "App Name", colType: ftString, visible: true, sortable: false, filter: filterMulti, visualization: visValue, transform: trText, isLabel: true},
	{id: "cursor_exhausted", dbField: "cursorExhausted", name: "Cursor Exhausted", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
	{id: "has_sort_stage", dbField: "hasSortStage", name: "Has Sort Stage", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
	{id: "uses_disk", dbField: "usedDisk", name: "Uses Disk", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
	{id: "from_multi_planner", dbField: "fromMultiPlanner", name: "From Multi Planner", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},
	{id: "replanned", dbField: "replanned", name: "Replanned", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},

	// Version-specific fields (hidden by default)
	{id: "query_hash", dbField: "queryHash", name: "Query Hash", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},            // 4.2+
	{id: "plan_cache_key", dbField: "planCacheKey", name: "Plan Cache Key", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText}, // 4.2+
	{id: "planning_time", dbField: "planningTimeMicros", name: "Planning Time", colType: ftDuration, visible: false, sortable: true, filter: filterRange, visualization: visBar, summary: summarySum, transform: trDuration, units: "seconds", decimalPoints: 3, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "cpu_time", dbField: "cpuNanos", name: "CPU Time", colType: ftDuration, visible: false, sortable: true, filter: filterRange, visualization: visBar, summary: summarySum, transform: trDuration, units: "seconds", decimalPoints: 3, isMetric: true, chartGroup: "Time", chartTitle: "Execution Time"},
	{id: "query_framework", dbField: "queryFramework", name: "Query Framework", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText},   // 7.0+
	{id: "query_shape_hash", dbField: "queryShapeHash", name: "Query Shape Hash", colType: ftString, visible: false, sortable: false, filter: filterMulti, visualization: visValue, transform: trText}, // 8.0+
}

// optionalDuration converts an optional int64 pointer to float64 seconds, returning nil if nil
func optionalDuration(v *int64, divisor float64) any {
	if v == nil {
		return nil
	}
	return float64(*v) / divisor
}

// optionalBool converts a bool pointer to a display value
func optionalBool(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return "Yes"
	}
	return "No"
}

// topQueriesCharts returns the chart configuration for top queries responses
func topQueriesCharts(cols []mongoColumnMeta) map[string]module.ChartConfig {
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
		cfg.Columns = append(cfg.Columns, col.id)
		charts[col.chartGroup] = cfg
	}
	return charts
}

// topQueriesDefaultCharts returns the default chart configuration
func topQueriesDefaultCharts(cols []mongoColumnMeta) [][]string {
	label := primaryMongoLabel(cols)
	if label == "" {
		return nil
	}
	chartGroups := defaultMongoChartGroups(cols)
	out := make([][]string, 0, len(chartGroups))
	for _, group := range chartGroups {
		out = append(out, []string{group, label})
	}
	return out
}

// topQueriesGroupBy returns the group by configuration for top queries responses
func topQueriesGroupBy(cols []mongoColumnMeta) map[string]module.GroupByConfig {
	groupBy := make(map[string]module.GroupByConfig)
	for _, col := range cols {
		if !col.isLabel {
			continue
		}
		groupBy[col.id] = module.GroupByConfig{
			Name:    "Group by " + col.name,
			Columns: []string{col.id},
		}
	}
	return groupBy
}

func primaryMongoLabel(cols []mongoColumnMeta) string {
	for _, col := range cols {
		if col.isPrimary {
			return col.id
		}
	}
	for _, col := range cols {
		if col.isLabel {
			return col.id
		}
	}
	return ""
}

func defaultMongoChartGroups(cols []mongoColumnMeta) []string {
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

func buildMongoSortParam(cols []mongoColumnMeta) funcapi.ParamConfig {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.sortable {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.id,
			Column: col.dbField,
			Name:   fmt.Sprintf("Top queries by %s", col.name),
			Sort:   &sortDir,
		}
		if col.id == "execution_time" {
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

// mongoMethods returns the available function methods for MongoDB
func mongoMethods() []module.MethodConfig {
	// Build sort options from column metadata
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range mongoAllColumns {
		if !col.sortable {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.id,
			Column: col.dbField,
			Name:   fmt.Sprintf("Top queries by %s", col.name),
			Sort:   &sortDir,
		}
		if col.id == "execution_time" {
			opt.Default = true
		}
		sortOptions = append(sortOptions, opt)
	}

	return []module.MethodConfig{{
		UpdateEvery:  10,
		ID:           "top-queries",
		Name:         "Top Queries",
		Help:         topQueriesHelpText,
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

// buildAvailableMongoColumns returns columns that are available based on detected fields
func buildAvailableMongoColumns(available map[string]bool) []mongoColumnMeta {
	var result []mongoColumnMeta
	for _, col := range mongoAllColumns {
		// command field maps to query column, always include
		if col.dbField == "command" || available[col.dbField] {
			result = append(result, col)
		}
	}
	return result
}

// buildMongoColumnsFromMeta builds the Columns map for FunctionResponse
func buildMongoColumnsFromMeta(cols []mongoColumnMeta) map[string]any {
	result := make(map[string]any)
	for i, col := range cols {
		sortDir := funcapi.FieldSortDescending
		if col.colType == ftString {
			sortDir = funcapi.FieldSortAscending
		}
		colDef := funcapi.Column{
			Index:                 i,
			Name:                  col.name,
			Type:                  col.colType,
			Units:                 col.units,
			Visualization:         col.visualization,
			Sort:                  sortDir,
			Sortable:              col.sortable,
			Sticky:                col.sticky,
			Summary:               col.summary,
			Filter:                col.filter,
			FullWidth:             col.fullWidth,
			Wrap:                  col.wrap,
			DefaultExpandedFilter: col.expandFilter,
			UniqueKey:             col.uniqueKey,
			Visible:               col.visible,
			ValueOptions: funcapi.ValueOptions{
				Transform:     col.transform,
				DecimalPoints: col.decimalPoints,
				DefaultValue:  nil,
			},
		}

		result[col.id] = colDef.BuildColumn()
	}
	return result
}

// collectTopQueries queries system.profile for top queries across all databases
func (c *Collector) collectTopQueries(ctx context.Context, sortColumn string) *module.FunctionResponse {
	// Get limit from config
	limit := c.Config.TopQueriesLimit
	if limit <= 0 {
		limit = defaultTopQueriesLimit
	}

	// Build valid sort columns map from metadata
	validSortCols := make(map[string]bool)
	for _, col := range mongoAllColumns {
		if col.sortable {
			validSortCols[col.dbField] = true
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
	columns := buildMongoColumnsFromMeta(availableCols)
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
		Columns:           columns,
		Data:              [][]any{},
		DefaultSortColumn: "execution_time",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            topQueriesCharts(availableCols),
		DefaultCharts:     topQueriesDefaultCharts(availableCols),
		GroupBy:           topQueriesGroupBy(availableCols),
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
			switch col.id {
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
		Columns:           columns,
		Data:              data,
		DefaultSortColumn: "execution_time",
		RequiredParams:    []funcapi.ParamConfig{sortParam},
		Charts:            topQueriesCharts(availableCols),
		DefaultCharts:     topQueriesDefaultCharts(availableCols),
		GroupBy:           topQueriesGroupBy(availableCols),
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
