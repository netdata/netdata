// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// makeModuleFuncHandler creates a function handler for a module that provides methods
func (m *Manager) makeModuleFuncHandler(moduleName string) func(functions.Function) {
	return func(fn functions.Function) {
		// Check for "info" request
		if slices.Contains(fn.Args, "info") {
			m.handleModuleFuncInfo(moduleName, fn)
			return
		}

		// Parse __method:xxx, __job:xxx, __sort:xxx from args AND payload (v3 protocol)
		method, jobName, sortBy := "", "", ""

		// First, try to parse from JSON payload (v3 protocol sends params here)
		// Frontend sends: {"selections": {"__method": ["top-queries"], ...}, "timeout": 120000}
		// We need to extract from "selections" sub-object, with arrays as values
		if len(fn.Payload) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(fn.Payload, &payload); err == nil {
				// Check for "selections" sub-object (v3 frontend format)
				if selections, ok := payload["selections"].(map[string]any); ok {
					method = extractParamValue(selections, "__method")
					jobName = extractParamValue(selections, "__job")
					sortBy = extractParamValue(selections, "__sort")
				} else {
					// Fallback: params at top level (legacy or direct API calls)
					method = extractParamValue(payload, "__method")
					jobName = extractParamValue(payload, "__job")
					sortBy = extractParamValue(payload, "__sort")
				}
			}
		}

		// Fallback: parse from args (v2 protocol or CLI)
		// Args override payload if both are present
		for _, arg := range fn.Args {
			switch {
			case strings.HasPrefix(arg, "__method:"):
				method = strings.TrimPrefix(arg, "__method:")
			case strings.HasPrefix(arg, "__job:"):
				jobName = strings.TrimPrefix(arg, "__job:")
			case strings.HasPrefix(arg, "__sort:"):
				sortBy = strings.TrimPrefix(arg, "__sort:")
			}
		}

		// Validate method selection
		methods := m.moduleFuncs.getMethods(moduleName)
		if method == "" {
			// No method yet, use nil for methodCfg - buildRequiredParams handles this
			m.respondErrorWithParams(fn, moduleName, nil, 400,
				"missing __method parameter, available: %v", methodIDs(methods))
			return
		}
		methodCfg := findMethod(methods, method)
		if methodCfg == nil {
			m.respondErrorWithParams(fn, moduleName, nil, 404,
				"unknown method '%s', available: %v", method, methodIDs(methods))
			return
		}

		// Validate job selection
		jobs := m.moduleFuncs.getJobNames(moduleName)
		if len(jobs) == 0 {
			m.respondErrorWithParams(fn, moduleName, methodCfg, 503,
				"no %s instances configured", moduleName)
			return
		}
		if jobName == "" {
			m.respondErrorWithParams(fn, moduleName, methodCfg, 400,
				"missing __job parameter, available: %v", jobs)
			return
		}
		// Get job WITH generation for race condition detection
		// The generation increments when a job is replaced (config reload)
		job, jobGen := m.moduleFuncs.getJobWithGeneration(moduleName, jobName)
		if job == nil {
			m.respondErrorWithParams(fn, moduleName, methodCfg, 404,
				"unknown job '%s', available: %v", jobName, jobs)
			return
		}

		// Validate and resolve sortBy
		// SECURITY: sortBy MUST be validated against allowed options to prevent SQL injection
		// The module uses sortBy to build ORDER BY clauses
		sortColumn := "" // The actual column name to use in SQL
		if sortBy == "" {
			// Use default sort option
			for _, opt := range methodCfg.SortOptions {
				if opt.Default {
					sortBy = opt.ID
					sortColumn = opt.Column // Safe column name
					break
				}
			}
			// Fallback to first option if no default
			if sortColumn == "" && len(methodCfg.SortOptions) > 0 {
				sortBy = methodCfg.SortOptions[0].ID
				sortColumn = methodCfg.SortOptions[0].Column
			}
		} else {
			// Validate user-provided sortBy against whitelist
			found := false
			for _, opt := range methodCfg.SortOptions {
				if opt.ID == sortBy {
					sortColumn = opt.Column // Map ID to safe column name
					found = true
					break
				}
			}
			if !found {
				sortIDs := make([]string, len(methodCfg.SortOptions))
				for i, opt := range methodCfg.SortOptions {
					sortIDs[i] = opt.ID
				}
				m.respondErrorWithParams(fn, moduleName, methodCfg, 400,
					"invalid __sort value '%s', allowed: %v", sortBy, sortIDs)
				return
			}
		}

		// Guard: if method has no sort options, sortColumn stays empty
		// This is valid for methods that return unsorted data or handle sorting internally
		// The handler will receive empty string and must handle it appropriately

		// Create context with timeout from function request
		// This ensures DB queries are cancelled if the function times out
		// NOTE: fn.Timeout is already a time.Duration (set by parser as seconds)
		// Do NOT multiply by time.Second again - that would create huge timeouts
		ctx, cancel := context.WithTimeout(context.Background(), fn.Timeout)
		defer cancel()

		// RACE CONDITION MITIGATION: Verify job is still running before handler
		// The job could be stopped between lookup and handler call
		if !job.IsRunning() {
			m.respondErrorWithParams(fn, moduleName, methodCfg, 503,
				"job '%s' is no longer running", jobName)
			return
		}

		// Get the creator for this module to call HandleMethod
		creator, ok := m.moduleFuncs.getCreator(moduleName)
		if !ok || creator.HandleMethod == nil {
			m.respondErrorWithParams(fn, moduleName, methodCfg, 500,
				"module '%s' does not implement HandleMethod", moduleName)
			return
		}

		// Route to the module's handler - get DATA ONLY response
		// Pass sortColumn (safe SQL column name), NOT the raw sortBy from user input
		dataResp := creator.HandleMethod(ctx, job, method, sortColumn)

		// RACE CONDITION MITIGATION: Verify job was not replaced during handler execution
		// If a config reload replaced this job while we were querying, the response
		// might contain stale data or the connection might have been closed
		if !m.moduleFuncs.verifyJobGeneration(moduleName, jobName, jobGen) {
			// Job was replaced during our request - the response may be unreliable
			// Return error to prompt client to retry with new job instance
			m.respondErrorWithParams(fn, moduleName, methodCfg, 503,
				"job '%s' was replaced during request, please retry", jobName)
			return
		}

		// Core injects required_params into the response before sending
		m.respondWithParams(fn, moduleName, method, dataResp)
	}
}

// handleModuleFuncInfo handles "info" requests for a module
func (m *Manager) handleModuleFuncInfo(moduleName string, fn functions.Function) {
	methods := m.moduleFuncs.getMethods(moduleName)

	// Use first method for default sort options
	var methodCfg *module.MethodConfig
	if len(methods) > 0 {
		methodCfg = &methods[0]
	}

	resp := map[string]any{
		"v":               3,
		"status":          200,
		"type":            "table",
		"has_history":     false,
		"help":            fmt.Sprintf("%s data functions", moduleName),
		"accepted_params": []string{"__method", "__job", "__sort"},
		"required_params": m.buildRequiredParams(moduleName, methodCfg, nil),
	}

	m.respondJSON(fn, resp)
}

// respondWithParams wraps the module's data response with current required_params
func (m *Manager) respondWithParams(fn functions.Function, moduleName, method string, dataResp *module.FunctionResponse) {
	// Nil guard: if module returns nil, treat as internal error
	if dataResp == nil {
		m.respondErrorWithParams(fn, moduleName, nil, 500, "internal error: module returned nil response")
		return
	}

	methods := m.moduleFuncs.getMethods(moduleName)
	methodCfg := findMethod(methods, method)

	// Build the full response with injected required_params
	// Use dynamic sort options from response if provided (reflects actual DB capabilities)
	resp := map[string]any{
		"v":           3,
		"status":      dataResp.Status,
		"type":        "table",
		"has_history": false,
		"help":        dataResp.Help,

		// ALWAYS include fresh required_params
		"accepted_params": []string{"__method", "__job", "__sort"},
		"required_params": m.buildRequiredParams(moduleName, methodCfg, dataResp.SortOptions),
	}

	// Only include data fields when present (avoid null values on errors)
	if dataResp.Columns != nil {
		resp["columns"] = dataResp.Columns
	}
	if dataResp.Data != nil {
		resp["data"] = dataResp.Data
	}
	if dataResp.DefaultSortColumn != "" {
		resp["default_sort_column"] = dataResp.DefaultSortColumn
	}

	// Add chart configuration if provided
	if len(dataResp.Charts) > 0 {
		resp["charts"] = dataResp.Charts
	}
	if len(dataResp.DefaultCharts) > 0 {
		resp["default_charts"] = dataResp.DefaultCharts
	}
	if len(dataResp.GroupBy) > 0 {
		resp["group_by"] = dataResp.GroupBy
	}

	if dataResp.Status != 200 {
		resp["errorMessage"] = dataResp.Message
	}

	m.respondJSON(fn, resp)
}

// respondErrorWithParams sends an error response that STILL includes required_params
// This is critical: even error responses must include selectors so the UI can show them
func (m *Manager) respondErrorWithParams(fn functions.Function, moduleName string, methodCfg *module.MethodConfig, status int, format string, args ...any) {
	resp := map[string]any{
		"v":             3,
		"status":        status,
		"type":          "table",
		"has_history":   false,
		"errorMessage": fmt.Sprintf(format, args...),

		// ALWAYS include required_params even in errors
		"accepted_params": []string{"__method", "__job", "__sort"},
		"required_params": m.buildRequiredParams(moduleName, methodCfg, nil),
	}
	m.respondJSON(fn, resp)
}

// buildRequiredParams creates the required_params array with current job list
// dynamicSortOpts overrides static methodCfg.SortOptions when provided (for DB-specific capabilities)
func (m *Manager) buildRequiredParams(moduleName string, methodCfg *module.MethodConfig, dynamicSortOpts []module.SortOption) []map[string]any {
	methods := m.moduleFuncs.getMethods(moduleName)
	jobs := m.moduleFuncs.getJobNames(moduleName)

	// Build __job options (or "no instances" message)
	var jobOptions []map[string]any
	if len(jobs) == 0 {
		jobOptions = []map[string]any{
			{"id": "", "name": "(No instances configured)", "disabled": true},
		}
	} else {
		for i, j := range jobs {
			opt := map[string]any{"id": j, "name": j}
			if i == 0 {
				opt["defaultSelected"] = true
			}
			jobOptions = append(jobOptions, opt)
		}
	}

	// Build sort options - prefer dynamic options from module response (reflects actual DB capabilities)
	// Fall back to static methodCfg.SortOptions when dynamic options not provided
	var sortOptions []map[string]any
	if len(dynamicSortOpts) > 0 {
		// Use dynamic sort options from module (based on detected DB columns)
		sortOptions = buildSortOptions(dynamicSortOpts)
	} else if methodCfg != nil && len(methodCfg.SortOptions) > 0 {
		// Fall back to static sort options from method config
		sortOptions = buildSortOptions(methodCfg.SortOptions)
	} else {
		// Provide a placeholder when no method is selected or method has no sort options
		sortOptions = []map[string]any{
			{"id": "", "name": "(Select a method first)", "disabled": true},
		}
	}

	return []map[string]any{
		{
			"id":          "__method",
			"name":        "Method",
			"help":        "Select the operation to perform",
			"unique_view": true,
			"type":        "select",
			"options":     buildMethodOptions(methods),
		},
		{
			"id":          "__job",
			"name":        "Instance",
			"help":        "Select which database instance to query",
			"unique_view": true,
			"type":        "select",
			"options":     jobOptions,
		},
		{
			"id":          "__sort",
			"name":        "Filter By",
			"help":        "Select the primary sort column",
			"unique_view": true,
			"type":        "select",
			"options":     sortOptions,
		},
	}
}

// respondJSON sends a JSON response to the function request
// The HTTP status code is extracted from the "status" field in the response
func (m *Manager) respondJSON(fn functions.Function, resp map[string]any) {
	data, err := json.Marshal(resp)
	if err != nil {
		m.Errorf("failed to marshal function response: %v", err)
		return
	}

	// Extract status code from response for pluginsd protocol
	// Default to 200 if not present or not an int
	code := 200
	if status, ok := resp["status"]; ok {
		switch v := status.(type) {
		case int:
			code = v
		case int64:
			code = int(v)
		case float64:
			code = int(v)
		}
	}

	m.dyncfgApi.SendJSONWithCode(fn, string(data), code)
}

// extractParamValue extracts a parameter value from JSON payload
// Handles both string and array formats: "value" or ["value"]
func extractParamValue(payload map[string]any, key string) string {
	val, ok := payload[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// buildMethodOptions creates options for __method selector
func buildMethodOptions(methods []module.MethodConfig) []map[string]any {
	options := make([]map[string]any, 0, len(methods))
	for i, m := range methods {
		opt := map[string]any{
			"id":   m.ID,
			"name": m.Name,
		}
		if i == 0 {
			opt["defaultSelected"] = true // First method is default
		}
		options = append(options, opt)
	}
	return options
}

// buildSortOptions creates options for __sort selector with default fallback
func buildSortOptions(sortOpts []module.SortOption) []map[string]any {
	if len(sortOpts) == 0 {
		return nil
	}

	options := make([]map[string]any, 0, len(sortOpts))
	hasDefault := false

	// Check if any option has Default: true
	for _, s := range sortOpts {
		if s.Default {
			hasDefault = true
			break
		}
	}

	for i, s := range sortOpts {
		opt := map[string]any{
			"id":   s.ID,
			"name": s.Label,
			"sort": "descending", // Sort direction for this option (metrics typically sort desc)
		}
		// Set default: either the one marked as Default, or first option as fallback
		if s.Default || (!hasDefault && i == 0) {
			opt["defaultSelected"] = true
		}
		options = append(options, opt)
	}
	return options
}

// findMethod finds a method config by ID
func findMethod(methods []module.MethodConfig, id string) *module.MethodConfig {
	for i := range methods {
		if methods[i].ID == id {
			return &methods[i]
		}
	}
	return nil
}

// methodIDs extracts method IDs for error messages
func methodIDs(methods []module.MethodConfig) []string {
	ids := make([]string, len(methods))
	for i, m := range methods {
		ids[i] = m.ID
	}
	return ids
}
