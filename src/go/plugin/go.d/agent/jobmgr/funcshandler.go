// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	paramMethod = "__method"
	paramJob    = "__job"
	paramSort   = "__sort"
)

// makeModuleFuncHandler creates a function handler for a module that provides methods
func (m *Manager) makeModuleFuncHandler(moduleName string) func(functions.Function) {
	return func(fn functions.Function) {
		// Check for "info" request
		if slices.Contains(fn.Args, "info") {
			m.handleModuleFuncInfo(moduleName, fn)
			return
		}

		methods := m.moduleFuncs.getMethods(moduleName)
		if len(methods) == 0 {
			m.respondError(fn, 404, "no methods available for %s", moduleName)
			return
		}

		payload := parsePayload(fn.Payload)
		argValues := parseArgsParams(fn.Args)

		methodParam := buildMethodParamConfig(methods)
		methodValues := paramValues(argValues, payload, paramMethod)
		resolvedMethod := funcapi.ResolveParam(methodParam, methodValues)
		method := resolvedMethod.GetOne()
		methodCfg := findMethod(methods, method)
		if methodCfg == nil {
			m.respondError(fn, 404, "unknown method '%s', available: %v", method, methodIDs(methods))
			return
		}

		jobs := m.moduleFuncs.getJobNames(moduleName)
		if len(jobs) == 0 {
			m.respondError(fn, 503, "no %s instances configured", moduleName)
			return
		}
		jobParam := buildJobParamConfig(jobs)
		jobValues := paramValues(argValues, payload, paramJob)
		resolvedJob := funcapi.ResolveParam(jobParam, jobValues)
		jobName := resolvedJob.GetOne()
		if jobName == "" {
			m.respondError(fn, 404, "no %s instances configured", moduleName)
			return
		}

		// Get job WITH generation for race condition detection
		// The generation increments when a job is replaced (config reload)
		job, jobGen := m.moduleFuncs.getJobWithGeneration(moduleName, jobName)
		if job == nil {
			m.respondError(fn, 404, "unknown job '%s', available: %v", jobName, jobs)
			return
		}

		// Resolve method-specific required params
		methodParamValues := make(map[string][]string, len(methodCfg.RequiredParams))
		for _, paramCfg := range methodCfg.RequiredParams {
			methodParamValues[paramCfg.ID] = paramValues(argValues, payload, paramCfg.ID)
		}
		resolvedParams := funcapi.ResolveParams(methodCfg.RequiredParams, methodParamValues)
		resolvedParams[paramMethod] = resolvedMethod
		resolvedParams[paramJob] = resolvedJob

		// Create context with timeout from function request
		// This ensures DB queries are cancelled if the function times out
		// NOTE: fn.Timeout is already a time.Duration (set by parser as seconds)
		// Do NOT multiply by time.Second again - that would create huge timeouts
		ctx, cancel := context.WithTimeout(context.Background(), fn.Timeout)
		defer cancel()

		// RACE CONDITION MITIGATION: Verify job is still running before handler
		// The job could be stopped between lookup and handler call
		if !job.IsRunning() {
			m.respondError(fn, 503, "job '%s' is no longer running", jobName)
			return
		}

		// Get the creator for this module to call HandleMethod
		creator, ok := m.moduleFuncs.getCreator(moduleName)
		if !ok || creator.HandleMethod == nil {
			m.respondError(fn, 500, "module '%s' does not implement HandleMethod", moduleName)
			return
		}

		// Route to the module's handler - get DATA ONLY response
		dataResp := creator.HandleMethod(ctx, job, method, resolvedParams)

		// RACE CONDITION MITIGATION: Verify job was not replaced during handler execution
		// If a config reload replaced this job while we were querying, the response
		// might contain stale data or the connection might have been closed
		if !m.moduleFuncs.verifyJobGeneration(moduleName, jobName, jobGen) {
			// Job was replaced during our request - the response may be unreliable
			// Return error to prompt client to retry with new job instance
			m.respondError(fn, 503, "job '%s' was replaced during request, please retry", jobName)
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
		"accepted_params": buildAcceptedParams(methodCfg, nil),
		"required_params": m.buildRequiredParams(moduleName, methodCfg, nil),
	}

	m.respondJSON(fn, resp)
}

// respondWithParams wraps the module's data response with current required_params
func (m *Manager) respondWithParams(fn functions.Function, moduleName, method string, dataResp *module.FunctionResponse) {
	// Nil guard: if module returns nil, treat as internal error
	if dataResp == nil {
		m.respondError(fn, 500, "internal error: module returned nil response")
		return
	}

	if dataResp.Status >= 400 {
		m.respondError(fn, dataResp.Status, "%s", dataResp.Message)
		return
	}

	methods := m.moduleFuncs.getMethods(moduleName)
	methodCfg := findMethod(methods, method)

	// Build the full response with injected required_params
	// Use dynamic sort options from response if provided (reflects actual DB capabilities)
	resp := map[string]any{
		"v":               3,
		"status":          dataResp.Status,
		"type":            "table",
		"has_history":     false,
		"help":            dataResp.Help,
		"accepted_params": buildAcceptedParams(methodCfg, dataResp.RequiredParams),
		"required_params": m.buildRequiredParams(moduleName, methodCfg, dataResp.RequiredParams),
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

	m.respondJSON(fn, resp)
}

// respondError sends a minimal error response (status + errorMessage).
func (m *Manager) respondError(fn functions.Function, status int, format string, args ...any) {
	resp := map[string]any{
		"status":       status,
		"errorMessage": fmt.Sprintf(format, args...),
	}
	m.respondJSON(fn, resp)
}

// buildRequiredParams creates the required_params array with current job list
// overrideParams overrides static methodCfg.RequiredParams when provided (for DB-specific capabilities)
func (m *Manager) buildRequiredParams(moduleName string, methodCfg *module.MethodConfig, overrideParams []funcapi.ParamConfig) []map[string]any {
	methods := m.moduleFuncs.getMethods(moduleName)
	jobs := m.moduleFuncs.getJobNames(moduleName)

	paramConfigs := []funcapi.ParamConfig{
		buildMethodParamConfig(methods),
		buildJobParamConfig(jobs),
	}
	if methodCfg != nil {
		methodParams := methodCfg.RequiredParams
		if len(overrideParams) > 0 {
			methodParams = funcapi.MergeParamConfigs(methodParams, overrideParams)
		}
		paramConfigs = append(paramConfigs, methodParams...)
	}

	required := make([]map[string]any, 0, len(paramConfigs))
	for _, cfg := range paramConfigs {
		required = append(required, cfg.RequiredParam())
	}
	return required
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

	if m.FunctionJSONWriter != nil {
		m.FunctionJSONWriter(data, code)
		return
	}

	m.dyncfgApi.SendJSONWithCode(fn, string(data), code)
}

func parsePayload(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func parseArgsParams(args []string) map[string][]string {
	if len(args) == 0 {
		return nil
	}
	params := make(map[string][]string)
	for _, arg := range args {
		if arg == "info" {
			continue
		}
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]
		if key == "" || value == "" {
			continue
		}
		params[key] = splitCSV(value)
	}
	return params
}

func paramValues(args map[string][]string, payload map[string]any, key string) []string {
	if args != nil {
		if vals := args[key]; len(vals) > 0 {
			return vals
		}
	}
	return extractParamValues(payload, key)
}

// extractParamValues extracts parameter values from payload, checking selections first.
func extractParamValues(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	if selections, ok := payload["selections"].(map[string]any); ok {
		if vals := extractValues(selections[key]); len(vals) > 0 {
			return vals
		}
	}
	return extractValues(payload[key])
}

func extractValues(val any) []string {
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		var out []string
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func splitCSV(value string) []string {
	if !strings.Contains(value, ",") {
		return []string{value}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func buildMethodParamConfig(methods []module.MethodConfig) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, len(methods))
	for i, m := range methods {
		opt := funcapi.ParamOption{
			ID:   m.ID,
			Name: m.Name,
		}
		if i == 0 {
			opt.Default = true
		}
		options = append(options, opt)
	}
	return funcapi.ParamConfig{
		ID:         paramMethod,
		Name:       "Method",
		Help:       "Select the operation to perform",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}

func buildJobParamConfig(jobs []string) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, len(jobs))
	if len(jobs) == 0 {
		options = append(options, funcapi.ParamOption{
			ID:       "",
			Name:     "(No instances configured)",
			Disabled: true,
		})
	} else {
		for i, j := range jobs {
			opt := funcapi.ParamOption{
				ID:   j,
				Name: j,
			}
			if i == 0 {
				opt.Default = true
			}
			options = append(options, opt)
		}
	}
	return funcapi.ParamConfig{
		ID:         paramJob,
		Name:       "Instance",
		Help:       "Select which database instance to query",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}

func buildAcceptedParams(methodCfg *module.MethodConfig, overrideParams []funcapi.ParamConfig) []string {
	accepted := []string{paramMethod, paramJob}
	if methodCfg == nil {
		return accepted
	}
	params := methodCfg.RequiredParams
	if len(overrideParams) > 0 {
		params = funcapi.MergeParamConfigs(params, overrideParams)
	}
	for _, p := range params {
		if !slices.Contains(accepted, p.ID) {
			accepted = append(accepted, p.ID)
		}
	}
	return accepted
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
