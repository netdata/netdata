// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const (
	paramJob = "__job"
)

type methodParamResolver func(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error)

type methodResponseWriter func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int)

type methodExecutionInput struct {
	fn            functions.Function
	moduleName    string
	jobName       string
	jobLabel      string
	methodID      string
	methodCfg     *funcapi.MethodConfig
	job           collectorapi.RuntimeJob
	jobGen        uint64
	payload       map[string]any
	argValues     map[string][]string
	resolveParams methodParamResolver
	augmentParams func(funcapi.ResolvedParams)
	respond       methodResponseWriter
}

// executeMethodRequest runs the common method execution pipeline for both
// module-level and job-bound method handlers.
func (m *Manager) executeMethodRequest(in methodExecutionInput) {
	ctx, cancel := context.WithTimeout(m.baseContext(), in.fn.Timeout)
	defer cancel()

	if !in.job.IsRunning() {
		m.respondError(in.fn, 503, "job '%s' is no longer running", in.jobLabel)
		return
	}

	creator, ok := m.moduleFuncs.getCreator(in.moduleName)
	if !ok || creator.MethodHandler == nil {
		m.respondError(in.fn, 500, "module '%s' does not implement MethodHandler", in.moduleName)
		return
	}

	handler := creator.MethodHandler(in.job)
	if handler == nil {
		m.respondError(in.fn, 500, "module '%s' returned nil handler for job '%s'", in.moduleName, in.jobName)
		return
	}

	methodParams, paramsFromJob, err := in.resolveParams(ctx, in.methodCfg, handler, in.methodID)
	if err != nil {
		m.respondError(in.fn, 503, "job '%s' cannot provide parameters: %v", in.jobLabel, err)
		return
	}

	if paramsFromJob {
		if err := validateParamValues(methodParams, in.argValues, in.payload, in.jobName); err != nil {
			m.respondError(in.fn, 400, "%v", err)
			return
		}
	}

	methodParamValues := make(map[string][]string, len(methodParams))
	for _, paramCfg := range methodParams {
		methodParamValues[paramCfg.ID] = paramValues(in.argValues, in.payload, paramCfg.ID)
	}
	resolvedParams := funcapi.ResolveParams(methodParams, methodParamValues)
	if in.augmentParams != nil {
		in.augmentParams(resolvedParams)
	}

	dataResp := handler.Handle(ctx, in.methodID, resolvedParams)

	if !m.moduleFuncs.verifyJobGeneration(in.moduleName, in.jobName, in.jobGen) {
		m.respondError(in.fn, 503, "job '%s' was replaced during request, please retry", in.jobLabel)
		return
	}

	updateEvery := 1
	if in.methodCfg.UpdateEvery > 1 {
		updateEvery = in.methodCfg.UpdateEvery
	}

	in.respond(dataResp, methodParams, updateEvery)
}

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

		m.executeMethodRequest(methodExecutionInput{
			fn:         fn,
			moduleName: moduleName,
			jobName:    jobName,
			jobLabel:   jobName,
			methodID:   methodID,
			methodCfg:  methodCfg,
			job:        job,
			jobGen:     jobGen,
			payload:    payload,
			argValues:  argValues,
			resolveParams: func(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error) {
				return m.resolveMethodParamsForJob(ctx, moduleName, methodID, methodCfg, job, handler)
			},
			augmentParams: func(resolvedParams funcapi.ResolvedParams) {
				resolvedParams[paramJob] = resolvedJob
			},
			respond: func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
				m.respondWithParams(fn, moduleName, dataResp, methodParams, updateEvery)
			},
		})
	}
}

// handleMethodFuncInfo handles "info" requests for a module:method function
func (m *Manager) handleMethodFuncInfo(moduleName, methodID string, fn functions.Function) {
	methodCfg, ok := m.moduleFuncs.getMethod(moduleName, methodID)
	if !ok {
		m.respondError(fn, 404, "unknown method '%s' for module '%s'", methodID, moduleName)
		return
	}

	// Use static params for info. Actual requests return job-specific params in the response.
	methodParams := methodCfg.RequiredParams
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
func (m *Manager) respondWithParams(fn functions.Function, moduleName string, dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
	m.respondMethodDataWithParams(
		fn,
		dataResp,
		methodParams,
		updateEvery,
		buildAcceptedParams,
		func(params []funcapi.ParamConfig) []map[string]any {
			return m.buildRequiredParams(moduleName, params)
		},
	)
}

func (m *Manager) respondMethodDataWithParams(
	fn functions.Function,
	dataResp *funcapi.FunctionResponse,
	methodParams []funcapi.ParamConfig,
	updateEvery int,
	buildAccepted func([]funcapi.ParamConfig) []string,
	buildRequired func([]funcapi.ParamConfig) []map[string]any,
) {
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

	resp := map[string]any{
		"v":               3,
		"update_every":    updateEvery,
		"status":          dataResp.Status,
		"type":            "table",
		"has_history":     false,
		"help":            dataResp.Help,
		"accepted_params": buildAccepted(paramsForResponse),
		"required_params": buildRequired(paramsForResponse),
	}

	if dataResp.Columns != nil {
		resp["columns"] = dataResp.Columns
	}
	if dataResp.Data != nil {
		resp["data"] = dataResp.Data
	}
	if dataResp.DefaultSortColumn != "" {
		resp["default_sort_column"] = dataResp.DefaultSortColumn
	}
	if len(dataResp.Charts) > 0 {
		resp["charts"] = dataResp.Charts
	}
	if len(dataResp.DefaultCharts) > 0 {
		resp["default_charts"] = dataResp.DefaultCharts.Build()
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

func (m *Manager) resolveMethodParamsForJob(ctx context.Context, moduleName, methodID string, methodCfg *funcapi.MethodConfig, job collectorapi.RuntimeJob, handler funcapi.MethodHandler) ([]funcapi.ParamConfig, bool, error) {
	methodParams := methodCfg.RequiredParams

	jobParams, err := handler.MethodParams(ctx, methodID)
	if err != nil {
		return nil, false, err
	}
	if len(jobParams) == 0 {
		return methodParams, true, nil
	}

	return funcapi.MergeParamConfigs(methodParams, jobParams), true, nil
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

	if m.functionJSONWriter != nil {
		m.functionJSONWriter(data, code)
		return
	}

	m.dyncfgApi.SendJSONWithCode(dyncfg.NewFunction(fn), string(data), code)
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

// makeJobMethodFuncHandler creates a function handler for a job-specific method.
// Unlike makeMethodFuncHandler, this handler routes directly to a specific job
// without needing the __job parameter (the job is known from the function name).
func (m *Manager) makeJobMethodFuncHandler(moduleName, jobName, methodID string) func(functions.Function) {
	return func(fn functions.Function) {
		// Check for "info" request
		if slices.Contains(fn.Args, "info") {
			m.handleJobMethodFuncInfo(moduleName, jobName, methodID, fn)
			return
		}

		methodCfg, ok := m.moduleFuncs.getJobMethod(moduleName, jobName, methodID)
		if !ok {
			m.respondError(fn, 404, "unknown method '%s' for job '%s:%s'", methodID, moduleName, jobName)
			return
		}

		payload := parsePayload(fn.Payload)
		argValues := parseArgsParams(fn.Args)

		// Get job WITH generation for race condition detection
		job, jobGen := m.moduleFuncs.getJobWithGeneration(moduleName, jobName)
		if job == nil {
			m.respondError(fn, 503, "job '%s:%s' is not running", moduleName, jobName)
			return
		}

		m.executeMethodRequest(methodExecutionInput{
			fn:         fn,
			moduleName: moduleName,
			jobName:    jobName,
			jobLabel:   fmt.Sprintf("%s:%s", moduleName, jobName),
			methodID:   methodID,
			methodCfg:  methodCfg,
			job:        job,
			jobGen:     jobGen,
			payload:    payload,
			argValues:  argValues,
			resolveParams: func(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error) {
				return m.resolveJobMethodParams(ctx, methodCfg, handler, methodID)
			},
			respond: func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
				m.respondJobMethodWithParams(fn, dataResp, methodParams, updateEvery)
			},
		})
	}
}

// handleJobMethodFuncInfo handles "info" requests for a job-specific method
func (m *Manager) handleJobMethodFuncInfo(moduleName, jobName, methodID string, fn functions.Function) {
	methodCfg, ok := m.moduleFuncs.getJobMethod(moduleName, jobName, methodID)
	if !ok {
		m.respondError(fn, 404, "unknown method '%s' for job '%s:%s'", methodID, moduleName, jobName)
		return
	}

	methodParams := methodCfg.RequiredParams
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
		"accepted_params": buildJobMethodAcceptedParams(methodParams),
		"required_params": buildJobMethodRequiredParams(methodParams),
	}

	m.respondJSON(fn, resp)
}

// resolveJobMethodParams resolves method parameters for a job-specific method
func (m *Manager) resolveJobMethodParams(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error) {
	methodParams := methodCfg.RequiredParams

	jobParams, err := handler.MethodParams(ctx, methodID)
	if err != nil {
		return nil, false, err
	}
	if len(jobParams) == 0 {
		return methodParams, true, nil
	}

	return funcapi.MergeParamConfigs(methodParams, jobParams), true, nil
}

// respondJobMethodWithParams wraps the module's data response for job-specific methods
func (m *Manager) respondJobMethodWithParams(fn functions.Function, dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
	m.respondMethodDataWithParams(
		fn,
		dataResp,
		methodParams,
		updateEvery,
		buildJobMethodAcceptedParams,
		buildJobMethodRequiredParams,
	)
}

// buildJobMethodAcceptedParams creates accepted_params for job-specific methods (no __job)
func buildJobMethodAcceptedParams(methodParams []funcapi.ParamConfig) []string {
	accepted := make([]string, 0, len(methodParams))
	for _, p := range methodParams {
		if !slices.Contains(accepted, p.ID) {
			accepted = append(accepted, p.ID)
		}
	}
	return accepted
}

// buildJobMethodRequiredParams creates required_params for job-specific methods (no __job)
func buildJobMethodRequiredParams(methodParams []funcapi.ParamConfig) []map[string]any {
	required := make([]map[string]any, 0, len(methodParams))
	for _, cfg := range methodParams {
		required = append(required, cfg.RequiredParam())
	}
	return required
}
