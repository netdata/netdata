// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

const (
	paramJob = "__job"
)

type methodParamResolver func(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error)

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

func (c *Controller) ExecuteFunction(functionName string, fn functions.Function) {
	moduleName, methodID, err := functions.SplitFunctionName(functionName)
	if err != nil {
		c.respondError(fn, 400, "%v", err)
		return
	}

	c.makeMethodFuncHandler(moduleName, methodID)(fn)
}

func (c *Controller) executeMethodRequest(in methodExecutionInput) {
	ctx, cancel := context.WithTimeout(c.baseContext(), in.fn.Timeout)
	defer cancel()

	if !in.job.IsRunning() {
		c.respondError(in.fn, 503, "job '%s' is no longer running", in.jobLabel)
		return
	}

	creator, ok := c.registry.getCreator(in.moduleName)
	if !ok || creator.MethodHandler == nil {
		c.respondError(in.fn, 500, "module '%s' does not implement MethodHandler", in.moduleName)
		return
	}

	handler := creator.MethodHandler(in.job)
	if handler == nil {
		c.respondError(in.fn, 500, "module '%s' returned nil handler for job '%s'", in.moduleName, in.jobName)
		return
	}

	methodParams, paramsFromJob, err := in.resolveParams(ctx, in.methodCfg, handler, in.methodID)
	if err != nil {
		c.respondError(in.fn, 503, "job '%s' cannot provide parameters: %v", in.jobLabel, err)
		return
	}

	if paramsFromJob {
		if err := validateParamValues(methodParams, in.argValues, in.payload, in.jobName); err != nil {
			c.respondError(in.fn, 400, "%v", err)
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

	if !c.registry.verifyJobGeneration(in.moduleName, in.jobName, in.jobGen) {
		c.respondError(in.fn, 503, "job '%s' was replaced during request, please retry", in.jobLabel)
		return
	}

	updateEvery := max(in.methodCfg.UpdateEvery, 1)

	in.respond(dataResp, methodParams, updateEvery)
}

func (c *Controller) makeMethodFuncHandler(moduleName, methodID string) func(functions.Function) {
	return func(fn functions.Function) {
		if slices.Contains(fn.Args, "info") {
			c.handleMethodFuncInfo(moduleName, methodID, fn)
			return
		}

		methodCfg, ok := c.registry.getMethod(moduleName, methodID)
		if !ok {
			c.respondError(fn, 404, "unknown method '%s' for module '%s'", methodID, moduleName)
			return
		}

		payload := parsePayload(fn.Payload)
		argValues := parseArgsParams(fn.Args)

		jobs := c.registry.getJobNames(moduleName)
		if len(jobs) == 0 {
			c.respondError(fn, 422, "no %s instances configured", moduleName)
			return
		}
		jobParam := buildJobParamConfig(jobs)
		jobValues := paramValues(argValues, payload, paramJob)
		if len(jobValues) > 1 {
			c.respondError(fn, 400, "parameter '%s' expects a single value", paramJob)
			return
		}
		resolvedJob := funcapi.ResolveParam(jobParam, jobValues)
		jobName := resolvedJob.GetOne()
		if len(jobValues) > 0 && jobValues[0] != jobName {
			c.respondError(fn, 404, "unknown job '%s', available: %v", jobValues[0], jobs)
			return
		}
		if jobName == "" {
			c.respondError(fn, 404, "no %s instances configured", moduleName)
			return
		}

		job, jobGen := c.registry.getJobWithGeneration(moduleName, jobName)
		if job == nil {
			c.respondError(fn, 404, "unknown job '%s', available: %v", jobName, jobs)
			return
		}

		c.executeMethodRequest(methodExecutionInput{
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
				return c.resolveMethodParamsForJob(ctx, methodCfg, job, handler, methodID)
			},
			augmentParams: func(resolvedParams funcapi.ResolvedParams) {
				resolvedParams[paramJob] = resolvedJob
			},
			respond: func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
				c.respondWithParams(fn, moduleName, dataResp, methodParams, updateEvery)
			},
		})
	}
}

func (c *Controller) handleMethodFuncInfo(moduleName, methodID string, fn functions.Function) {
	methodCfg, ok := c.registry.getMethod(moduleName, methodID)
	if !ok {
		c.respondError(fn, 404, "unknown method '%s' for module '%s'", methodID, moduleName)
		return
	}

	methodParams := methodCfg.RequiredParams
	help := methodCfg.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", moduleName, methodID)
	}

	updateEvery := max(methodCfg.UpdateEvery, 1)

	c.respondJSON(fn, map[string]any{
		"v":               3,
		"update_every":    updateEvery,
		"status":          200,
		"type":            "table",
		"has_history":     false,
		"help":            help,
		"accepted_params": buildAcceptedParams(methodParams),
		"required_params": c.buildRequiredParams(moduleName, methodParams),
	})
}

func (c *Controller) buildRequiredParams(moduleName string, methodParams []funcapi.ParamConfig) []map[string]any {
	jobs := c.registry.getJobNames(moduleName)

	paramConfigs := []funcapi.ParamConfig{buildJobParamConfig(jobs)}
	paramConfigs = append(paramConfigs, methodParams...)

	required := make([]map[string]any, 0, len(paramConfigs))
	for _, cfg := range paramConfigs {
		required = append(required, cfg.RequiredParam())
	}
	return required
}

func (c *Controller) resolveMethodParamsForJob(ctx context.Context, methodCfg *funcapi.MethodConfig, job collectorapi.RuntimeJob, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error) {
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
		for _, value := range values {
			if !allowed[value] {
				return fmt.Errorf("parameter '%s' option '%s' is not supported by job '%s'", cfg.ID, value, jobName)
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
		key, value := parts[0], parts[1]
		if key == "" || value == "" {
			continue
		}
		params[key] = splitCSV(value)
	}
	return params
}

func paramValues(args map[string][]string, payload map[string]any, key string) []string {
	if args != nil {
		if values := args[key]; len(values) > 0 {
			return values
		}
	}
	return extractParamValues(payload, key)
}

func extractParamValues(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	if selections, ok := payload["selections"].(map[string]any); ok {
		if values := extractValues(selections[key]); len(values) > 0 {
			return values
		}
	}
	return extractValues(payload[key])
}

func extractValues(value any) []string {
	switch current := value.(type) {
	case string:
		if current == "" {
			return nil
		}
		return []string{current}
	case []any:
		var out []string
		for _, item := range current {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		var out []string
		for _, item := range current {
			if item != "" {
				out = append(out, item)
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
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
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
		for i, job := range jobs {
			option := funcapi.ParamOption{
				ID:   job,
				Name: job,
			}
			if i == 0 {
				option.Default = true
			}
			options = append(options, option)
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
	for _, param := range methodParams {
		if !slices.Contains(accepted, param.ID) {
			accepted = append(accepted, param.ID)
		}
	}
	return accepted
}

func (c *Controller) makeJobMethodFuncHandler(moduleName, jobName, methodID string) func(functions.Function) {
	return func(fn functions.Function) {
		if slices.Contains(fn.Args, "info") {
			c.handleJobMethodFuncInfo(moduleName, jobName, methodID, fn)
			return
		}

		methodCfg, ok := c.registry.getJobMethod(moduleName, jobName, methodID)
		if !ok {
			c.respondError(fn, 404, "unknown method '%s' for job '%s:%s'", methodID, moduleName, jobName)
			return
		}

		payload := parsePayload(fn.Payload)
		argValues := parseArgsParams(fn.Args)

		job, jobGen := c.registry.getJobWithGeneration(moduleName, jobName)
		if job == nil {
			c.respondError(fn, 503, "job '%s:%s' is not running", moduleName, jobName)
			return
		}

		c.executeMethodRequest(methodExecutionInput{
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
				return c.resolveJobMethodParams(ctx, methodCfg, handler, methodID)
			},
			respond: func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int) {
				c.respondJobMethodWithParams(fn, dataResp, methodParams, updateEvery)
			},
		})
	}
}

func (c *Controller) handleJobMethodFuncInfo(moduleName, jobName, methodID string, fn functions.Function) {
	methodCfg, ok := c.registry.getJobMethod(moduleName, jobName, methodID)
	if !ok {
		c.respondError(fn, 404, "unknown method '%s' for job '%s:%s'", methodID, moduleName, jobName)
		return
	}

	methodParams := methodCfg.RequiredParams
	help := methodCfg.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", moduleName, methodID)
	}

	updateEvery := max(methodCfg.UpdateEvery, 1)

	c.respondJSON(fn, map[string]any{
		"v":               3,
		"update_every":    updateEvery,
		"status":          200,
		"type":            "table",
		"has_history":     false,
		"help":            help,
		"accepted_params": buildJobMethodAcceptedParams(methodParams),
		"required_params": buildJobMethodRequiredParams(methodParams),
	})
}

func (c *Controller) resolveJobMethodParams(ctx context.Context, methodCfg *funcapi.MethodConfig, handler funcapi.MethodHandler, methodID string) ([]funcapi.ParamConfig, bool, error) {
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

func buildJobMethodAcceptedParams(methodParams []funcapi.ParamConfig) []string {
	accepted := make([]string, 0, len(methodParams))
	for _, param := range methodParams {
		if !slices.Contains(accepted, param.ID) {
			accepted = append(accepted, param.ID)
		}
	}
	return accepted
}

func buildJobMethodRequiredParams(methodParams []funcapi.ParamConfig) []map[string]any {
	required := make([]map[string]any, 0, len(methodParams))
	for _, cfg := range methodParams {
		required = append(required, cfg.RequiredParam())
	}
	return required
}
