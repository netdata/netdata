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
	paramJob = "__job"
)

// makeMethodFuncHandler creates a function handler for a module+method function (module:method).
func (m *Manager) makeMethodFuncHandler(moduleName, methodID string) func(functions.Function) {
	return func(fn functions.Function) {
		// Check for "info" request
		if slices.Contains(fn.Args, "info") {
			m.handleMethodFuncInfo(moduleName, methodID, fn)
			return
		}

		methodCfg, ok := m.moduleFuncs.getMethod(moduleName, methodID)
		if !ok {
			m.respondError(fn, 404, "unknown method '%s' for module '%s'", methodID, moduleName)
			return
		}

		payload := parsePayload(fn.Payload)
		argValues := parseArgsParams(fn.Args)

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

		// Resolve method-specific required params (job-aware)
		methodParams, paramsFromJob, err := m.resolveMethodParamsForJob(ctx, moduleName, methodID, methodCfg, job, creator)
		if err != nil {
			m.respondError(fn, 503, "job '%s' cannot provide parameters: %v", jobName, err)
			return
		}

		// Validate provided param values when job-specific options are available
		if paramsFromJob {
			if err := validateParamValues(methodParams, argValues, payload, jobName); err != nil {
				m.respondError(fn, 400, "%v", err)
				return
			}
		}

		methodParamValues := make(map[string][]string, len(methodParams))
		for _, paramCfg := range methodParams {
			methodParamValues[paramCfg.ID] = paramValues(argValues, payload, paramCfg.ID)
		}
		resolvedParams := funcapi.ResolveParams(methodParams, methodParamValues)
		resolvedParams[paramJob] = resolvedJob

		// Route to the module's handler - get DATA ONLY response
		dataResp := creator.HandleMethod(ctx, job, methodID, resolvedParams)

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
		m.respondWithParams(fn, moduleName, dataResp, methodParams)
	}
}

// handleMethodFuncInfo handles "info" requests for a module:method function
func (m *Manager) handleMethodFuncInfo(moduleName, methodID string, fn functions.Function) {
	methodCfg, ok := m.moduleFuncs.getMethod(moduleName, methodID)
	if !ok {
		m.respondError(fn, 404, "unknown method '%s' for module '%s'", methodID, moduleName)
		return
	}

	methodParams := m.unionMethodParams(moduleName, methodID, methodCfg, fn)
	help := methodCfg.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", moduleName, methodID)
	}

	updateEvery := 1
	if methodCfg.UpdateEvery > 1 {
		updateEvery = methodCfg.UpdateEvery
	}

	resp := map[string]any{
		"v":               3,
		"update_every":    updateEvery,
		"status":          200,
		"type":            "table",
		"has_history":     false,
		"help":            help,
		"accepted_params": buildAcceptedParams(methodParams),
		"required_params": m.buildRequiredParams(moduleName, methodParams),
	}

	m.respondJSON(fn, resp)
}

// respondWithParams wraps the module's data response with current required_params
func (m *Manager) respondWithParams(fn functions.Function, moduleName string, dataResp *module.FunctionResponse, methodParams []funcapi.ParamConfig) {
	// Nil guard: if module returns nil, treat as internal error
	if dataResp == nil {
		m.respondError(fn, 500, "internal error: module returned nil response")
		return
	}

	if dataResp.Status >= 400 {
		m.respondError(fn, dataResp.Status, "%s", dataResp.Message)
		return
	}

	paramsForResponse := methodParams
	if len(dataResp.RequiredParams) > 0 {
		paramsForResponse = funcapi.MergeParamConfigs(paramsForResponse, dataResp.RequiredParams)
	}

	// Build the full response with injected required_params
	// Use dynamic sort options from response if provided (reflects actual DB capabilities)
	resp := map[string]any{
		"v":               3,
		"status":          dataResp.Status,
		"type":            "table",
		"has_history":     false,
		"help":            dataResp.Help,
		"accepted_params": buildAcceptedParams(paramsForResponse),
		"required_params": m.buildRequiredParams(moduleName, paramsForResponse),
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
func (m *Manager) buildRequiredParams(moduleName string, methodParams []funcapi.ParamConfig) []map[string]any {
	jobs := m.moduleFuncs.getJobNames(moduleName)

	paramConfigs := []funcapi.ParamConfig{
		buildJobParamConfig(jobs),
	}
	paramConfigs = append(paramConfigs, methodParams...)

	required := make([]map[string]any, 0, len(paramConfigs))
	for _, cfg := range paramConfigs {
		required = append(required, cfg.RequiredParam())
	}
	return required
}

func (m *Manager) resolveMethodParamsForJob(ctx context.Context, moduleName, methodID string, methodCfg *module.MethodConfig, job *module.Job, creator module.Creator) ([]funcapi.ParamConfig, bool, error) {
	methodParams := methodCfg.RequiredParams
	if creator.MethodParams == nil {
		return methodParams, false, nil
	}

	jobParams, err := creator.MethodParams(ctx, job, methodID)
	if err != nil {
		return nil, false, err
	}
	if len(jobParams) == 0 {
		return methodParams, true, nil
	}

	return funcapi.MergeParamConfigs(methodParams, jobParams), true, nil
}

func (m *Manager) unionMethodParams(moduleName, methodID string, methodCfg *module.MethodConfig, fn functions.Function) []funcapi.ParamConfig {
	baseParams := methodCfg.RequiredParams

	creator, ok := m.moduleFuncs.getCreator(moduleName)
	if !ok || creator.MethodParams == nil {
		return baseParams
	}

	jobs := m.moduleFuncs.getJobNames(moduleName)
	if len(jobs) == 0 {
		return baseParams
	}

	ctx, cancel := context.WithTimeout(context.Background(), fn.Timeout)
	defer cancel()

	union := []funcapi.ParamConfig{}
	for _, jobName := range jobs {
		job, ok := m.moduleFuncs.getJob(moduleName, jobName)
		if !ok || job == nil {
			continue
		}
		params, err := creator.MethodParams(ctx, job, methodID)
		if err != nil {
			m.Debugf("method params unavailable for %s:%s job '%s': %v", moduleName, methodID, jobName, err)
			continue
		}
		if len(params) == 0 {
			continue
		}
		union = mergeParamConfigsUnion(union, params)
	}
	if len(union) == 0 {
		return baseParams
	}

	out := make([]funcapi.ParamConfig, 0, len(baseParams)+len(union))
	baseIndex := make(map[string]bool, len(baseParams))
	unionIndex := make(map[string]int, len(union))
	for i, cfg := range union {
		if cfg.ID != "" {
			unionIndex[cfg.ID] = i
		}
	}

	for _, cfg := range baseParams {
		if cfg.ID != "" {
			baseIndex[cfg.ID] = true
		}
		if i, ok := unionIndex[cfg.ID]; ok {
			out = append(out, mergeParamConfigMetadata(cfg, union[i]))
			continue
		}
		out = append(out, cfg)
	}

	for _, cfg := range union {
		if cfg.ID == "" || baseIndex[cfg.ID] {
			continue
		}
		out = append(out, cfg)
	}
	return out
}

func mergeParamConfigMetadata(base, add funcapi.ParamConfig) funcapi.ParamConfig {
	out := add
	if out.Name == "" {
		out.Name = base.Name
	}
	if out.Help == "" {
		out.Help = base.Help
	}
	if base.UniqueView {
		out.UniqueView = true
	}
	return out
}

func validateParamValues(methodParams []funcapi.ParamConfig, argValues map[string][]string, payload map[string]any, jobName string) error {
	for _, cfg := range methodParams {
		values := paramValues(argValues, payload, cfg.ID)
		if len(values) == 0 {
			continue
		}
		if cfg.Selection == funcapi.ParamSelect && len(values) > 1 {
			return fmt.Errorf("parameter '%s' expects a single value for job '%s'", cfg.ID, jobName)
		}
		allowed := allowedOptions(cfg.Options)
		for _, val := range values {
			if !allowed[val] {
				return fmt.Errorf("parameter '%s' option '%s' is not supported by job '%s'", cfg.ID, val, jobName)
			}
		}
	}
	return nil
}

func allowedOptions(options []funcapi.ParamOption) map[string]bool {
	allowed := make(map[string]bool, len(options))
	for _, opt := range options {
		if opt.ID == "" || opt.Disabled {
			continue
		}
		allowed[opt.ID] = true
	}
	return allowed
}

func mergeParamConfigsUnion(base, add []funcapi.ParamConfig) []funcapi.ParamConfig {
	if len(add) == 0 {
		return base
	}

	out := make([]funcapi.ParamConfig, len(base))
	copy(out, base)

	index := make(map[string]int, len(out))
	for i, cfg := range out {
		if cfg.ID != "" {
			index[cfg.ID] = i
		}
	}

	for _, cfg := range add {
		if cfg.ID == "" {
			continue
		}
		if i, ok := index[cfg.ID]; ok {
			out[i] = mergeParamConfigOptions(out[i], cfg)
			continue
		}
		out = append(out, cfg)
		index[cfg.ID] = len(out) - 1
	}
	return out
}

func mergeParamConfigOptions(base, add funcapi.ParamConfig) funcapi.ParamConfig {
	out := base
	if out.Name == "" {
		out.Name = add.Name
	}
	if out.Help == "" {
		out.Help = add.Help
	}
	if out.Selection == funcapi.ParamSelect && add.Selection == funcapi.ParamMultiSelect {
		out.Selection = add.Selection
	}
	if add.UniqueView {
		out.UniqueView = true
	}

	options := make([]funcapi.ParamOption, len(out.Options))
	copy(options, out.Options)

	optIndex := make(map[string]int, len(options))
	for i, opt := range options {
		if opt.ID != "" {
			optIndex[opt.ID] = i
		}
	}

	hasDefault := false
	for _, opt := range options {
		if opt.Default {
			hasDefault = true
			break
		}
	}

	for _, opt := range add.Options {
		if opt.ID == "" {
			continue
		}
		if i, ok := optIndex[opt.ID]; ok {
			merged := options[i]
			if merged.Name == "" {
				merged.Name = opt.Name
			}
			if merged.Sort == nil && opt.Sort != nil {
				merged.Sort = opt.Sort
			}
			if merged.Column == "" {
				merged.Column = opt.Column
			}
			if opt.Default && !hasDefault {
				merged.Default = true
				hasDefault = true
			}
			// Disabled should remain false if any job supports the option.
			merged.Disabled = merged.Disabled && opt.Disabled
			options[i] = merged
			continue
		}

		if opt.Default && hasDefault {
			opt.Default = false
		}
		options = append(options, opt)
		optIndex[opt.ID] = len(options) - 1
		if opt.Default {
			hasDefault = true
		}
	}

	out.Options = options
	return out
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

func buildAcceptedParams(methodParams []funcapi.ParamConfig) []string {
	accepted := []string{paramJob}
	for _, p := range methodParams {
		if !slices.Contains(accepted, p.ID) {
			accepted = append(accepted, p.ID)
		}
	}
	return accepted
}
