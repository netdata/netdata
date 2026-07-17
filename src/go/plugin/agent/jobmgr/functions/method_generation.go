// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const functionJobParameter = "__job"

type methodGenerationKind uint8

const (
	methodGenerationAgent methodGenerationKind = iota + 1
	methodGenerationShared
	methodGenerationInstance
)

type methodGeneration struct {
	id       string
	module   string
	kind     methodGenerationKind
	creator  collectorapi.Creator
	methods  map[string]funcapi.FunctionConfig
	jobs     map[string]collectorapi.RuntimeJob
	handlers map[string]funcapi.MethodHandler

	cleanupOnce sync.Once
	cleanupErr  error
	done        chan struct{}
}

func newMethodGeneration(
	id string,
	module string,
	kind methodGenerationKind,
	creator collectorapi.Creator,
	methods []funcapi.FunctionConfig,
	jobs map[string]collectorapi.RuntimeJob,
) (*methodGeneration, error) {
	if id == "" || module == "" || creator.MethodHandler == nil || len(methods) == 0 {
		return nil, errors.New("jobmgr Function method generation: invalid construction")
	}
	generation := &methodGeneration{
		id: id, module: module, kind: kind, creator: creator,
		methods:  make(map[string]funcapi.FunctionConfig, len(methods)),
		jobs:     make(map[string]collectorapi.RuntimeJob, len(jobs)),
		handlers: make(map[string]funcapi.MethodHandler, max(1, len(jobs))),
		done:     make(chan struct{}),
	}
	for _, method := range methods {
		if method.ID == "" {
			return nil, errors.New("jobmgr Function method generation: empty method ID")
		}
		if _, exists := generation.methods[method.ID]; exists {
			return nil, errors.New("jobmgr Function method generation: duplicate method ID")
		}
		generation.methods[method.ID] = method
	}
	if kind == methodGenerationAgent {
		handler := creator.MethodHandler(nil)
		if handler == nil {
			return nil, errors.New("jobmgr Function method generation: nil agent handler")
		}
		generation.handlers[""] = handler
		return generation, nil
	}
	if len(jobs) == 0 {
		return nil, errors.New("jobmgr Function method generation: no runtime jobs")
	}
	for name, job := range jobs {
		if name == "" || job == nil || job.Name() != name || job.ModuleName() != module {
			return nil, errors.New("jobmgr Function method generation: invalid runtime job")
		}
		handler := creator.MethodHandler(job)
		if handler == nil {
			return nil, fmt.Errorf("jobmgr Function method generation: nil handler for job %q", name)
		}
		generation.jobs[name] = job
		generation.handlers[name] = handler
	}
	return generation, nil
}

func (generation *methodGeneration) declaration() *HandlerGenerationDeclaration {
	return &HandlerGenerationDeclaration{
		ID: generation.id, Handler: generation.handle, Cleanup: generation.cleanup,
	}
}

func (generation *methodGeneration) wait(ctx context.Context) error {
	if generation == nil {
		return nil
	}
	select {
	case <-generation.done:
		return generation.cleanupErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (generation *methodGeneration) cleanup(ctx context.Context) error {
	generation.cleanupOnce.Do(func() {
		names := make([]string, 0, len(generation.handlers))
		for name := range generation.handlers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			generation.cleanupErr = errors.Join(
				generation.cleanupErr,
				callMethodCleanup(ctx, generation.handlers[name]),
			)
		}
		close(generation.done)
	})
	return generation.cleanupErr
}

func callMethodCleanup(ctx context.Context, handler funcapi.MethodHandler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("jobmgr Function handler Cleanup panic: %v", recovered)
		}
	}()
	handler.Cleanup(ctx)
	return nil
}

func (generation *methodGeneration) handle(
	ctx context.Context,
	input HandlerInput,
) (lifecycle.SealedResult, error) {
	method, ok := generation.methods[input.Method]
	if !ok {
		return functionErrorResult(404, "unknown method %q", input.Method)
	}
	if slices.Contains(input.Args, "info") && !method.RawRequest {
		return generation.infoResult(method)
	}
	jobName, job, handler, err := generation.resolveTarget(method, input)
	if err != nil {
		return functionResponseError(err)
	}
	if job != nil && !job.IsRunning() {
		return functionErrorResult(503, "job %q is not running", jobName)
	}
	if method.RawRequest {
		raw, ok := handler.(funcapi.RawMethodHandler)
		if !ok {
			return functionErrorResult(
				500,
				"module %q method %q requires raw request handling",
				generation.module,
				method.ID,
			)
		}
		response := raw.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method: method.ID, Info: slices.Contains(input.Args, "info"),
			Args:        append([]string(nil), input.Args...),
			Payload:     append([]byte(nil), input.Payload...),
			ContentType: input.ContentType, Timeout: input.Timeout,
			Permissions: input.Permissions, Source: input.CallerSource,
		})
		return generation.responseResult(method, nil, response)
	}

	params, err := handler.MethodParams(ctx, method.ID)
	if err != nil {
		return functionErrorResult(503, "method %q cannot provide parameters: %v", method.ID, err)
	}
	params = funcapi.MergeParamConfigs(method.RequiredParams, params)
	payload := parseMethodPayload(input.Payload)
	arguments := parseMethodArguments(input.Args)
	if err := validateMethodParamValues(params, arguments, payload, jobName); err != nil {
		return functionErrorResult(400, "%v", err)
	}
	values := make(map[string][]string, len(params))
	for _, param := range params {
		values[param.ID] = methodParamValues(arguments, payload, param.ID)
	}
	resolved := funcapi.ResolveParams(params, values)
	if generation.kind == methodGenerationShared &&
		generation.creator.InstancePolicy != collectorapi.InstancePolicySingle {
		resolved[functionJobParameter] = funcapi.ResolvedParam{IDs: []string{jobName}}
	}
	response := handler.Handle(ctx, method.ID, resolved)
	if job != nil && !job.IsRunning() {
		return functionErrorResult(503, "job %q stopped during request", jobName)
	}
	return generation.responseResult(method, params, response)
}

func (generation *methodGeneration) resolveTarget(
	method funcapi.FunctionConfig,
	input HandlerInput,
) (string, collectorapi.RuntimeJob, funcapi.MethodHandler, error) {
	switch generation.kind {
	case methodGenerationAgent:
		return "", nil, generation.handlers[""], nil
	case methodGenerationInstance:
		names := generation.availableJobNames(method.ID)
		if len(names) != 1 {
			return "", nil, nil, functionStatusError{
				status: 404, message: fmt.Sprintf("unknown function %q", method.ID),
			}
		}
		name := names[0]
		return name, generation.jobs[name], generation.handlers[name], nil
	case methodGenerationShared:
		names := generation.availableJobNames(method.ID)
		if len(names) == 0 {
			return "", nil, nil, functionStatusError{
				status: 404, message: fmt.Sprintf("no %s instances available", generation.module),
			}
		}
		name := names[0]
		if generation.creator.InstancePolicy != collectorapi.InstancePolicySingle {
			values := methodParamValues(
				parseMethodArguments(input.Args),
				parseMethodPayload(input.Payload),
				functionJobParameter,
			)
			if len(values) > 1 {
				return "", nil, nil, functionStatusError{
					status: 400, message: "parameter '__job' expects a single value",
				}
			}
			if len(values) == 1 {
				name = values[0]
			}
		}
		job := generation.jobs[name]
		if job == nil || !slices.Contains(names, name) {
			return "", nil, nil, functionStatusError{
				status: 404, message: fmt.Sprintf("unknown job %q, available: %v", name, names),
			}
		}
		return name, job, generation.handlers[name], nil
	}
	return "", nil, nil, errors.New("jobmgr Function method generation: invalid target")
}

func (generation *methodGeneration) availableJobNames(methodID string) []string {
	names := make([]string, 0, len(generation.jobs))
	for name, job := range generation.jobs {
		if jobBackedFunctionAvailable(job, methodID) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func jobBackedFunctionAvailable(job collectorapi.RuntimeJob, methodID string) bool {
	if job == nil || !job.IsRunning() {
		return false
	}
	availability, ok := job.Collector().(collectorapi.FunctionAvailability)
	return !ok || availability == nil || availability.FunctionAvailable(methodID)
}

type functionStatusError struct {
	status  int
	message string
}

func (err functionStatusError) Error() string { return err.message }

func functionResponseError(err error) (lifecycle.SealedResult, error) {
	var status functionStatusError
	if errors.As(err, &status) {
		return functionErrorResult(status.status, "%s", status.message)
	}
	return lifecycle.SealedResult{}, err
}

func (generation *methodGeneration) infoResult(
	method funcapi.FunctionConfig,
) (lifecycle.SealedResult, error) {
	params := append([]funcapi.ParamConfig(nil), method.RequiredParams...)
	includeJob := generation.kind == methodGenerationShared &&
		generation.creator.InstancePolicy != collectorapi.InstancePolicySingle
	if includeJob {
		params = append([]funcapi.ParamConfig{buildFunctionJobParam(
			generation.availableJobNames(method.ID),
		)}, params...)
	}
	help := method.Help
	if help == "" {
		help = fmt.Sprintf("%s %s data function", generation.module, method.ID)
	}
	response := map[string]any{
		"v": 3, "update_every": max(method.UpdateEvery, 1), "status": 200,
		"type": functionResponseType("", method.ResponseType), "has_history": false,
		"help": help, "accepted_params": acceptedMethodParams(params),
		"required_params": requiredMethodParams(params),
	}
	if presentation := method.Presentation(); presentation != nil {
		response["presentation"] = presentation
	}
	return functionJSONResult(200, response)
}

func (generation *methodGeneration) responseResult(
	method funcapi.FunctionConfig,
	params []funcapi.ParamConfig,
	response *funcapi.FunctionResponse,
) (lifecycle.SealedResult, error) {
	if response == nil {
		return functionErrorResult(500, "module returned nil response")
	}
	if response.RawResponse != nil {
		return functionJSONResult(responseStatus(response.RawResponse, 200), response.RawResponse)
	}
	status := response.Status
	if status == 0 {
		status = 200
	}
	if status < 100 || status > 599 {
		return functionErrorResult(500, "module returned invalid status %d", status)
	}
	if status >= 400 {
		return functionErrorResult(status, "%s", response.Message)
	}
	if len(response.RequiredParams) != 0 {
		params = funcapi.MergeParamConfigs(params, response.RequiredParams)
	}
	if generation.kind == methodGenerationShared &&
		generation.creator.InstancePolicy != collectorapi.InstancePolicySingle {
		params = append([]funcapi.ParamConfig{
			buildFunctionJobParam(generation.availableJobNames(method.ID)),
		}, params...)
	}
	payload := map[string]any{
		"v": 3, "update_every": max(method.UpdateEvery, 1), "status": status,
		"type":        functionResponseType(response.ResponseType, method.ResponseType),
		"has_history": false, "help": response.Help,
		"accepted_params": acceptedMethodParams(params),
		"required_params": requiredMethodParams(params),
	}
	if response.Columns != nil {
		payload["columns"] = response.Columns
	}
	if response.Data != nil {
		payload["data"] = response.Data
	}
	if response.DefaultSortColumn != "" {
		payload["default_sort_column"] = response.DefaultSortColumn
	}
	if len(response.Charts) != 0 {
		payload["charts"] = response.Charts
	}
	if len(response.DefaultCharts) != 0 {
		payload["default_charts"] = response.DefaultCharts.Build()
	}
	if len(response.GroupBy) != 0 {
		payload["group_by"] = response.GroupBy
	}
	return functionJSONResult(status, payload)
}

func functionErrorResult(
	status int,
	format string,
	arguments ...any,
) (lifecycle.SealedResult, error) {
	return functionJSONResult(status, map[string]any{
		"status": status, "errorMessage": fmt.Sprintf(format, arguments...),
	})
}

func functionJSONResult(status int, payload any) (lifecycle.SealedResult, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	envelope, err := lifecycle.NewCompleteRawEnvelope(
		status,
		lifecycle.ReviewedPerformanceJSON,
		encoded,
	)
	if err != nil {
		return lifecycle.SealedResult{}, err
	}
	return lifecycle.SealFunctionResult(envelope)
}

func responseStatus(payload map[string]any, fallback int) int {
	switch value := payload["status"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return fallback
	}
}

func functionResponseType(responseType, methodType string) string {
	if responseType != "" {
		return responseType
	}
	if methodType != "" {
		return methodType
	}
	return "table"
}

func parseMethodPayload(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload
}

func parseMethodArguments(arguments []string) map[string][]string {
	params := make(map[string][]string)
	for _, argument := range arguments {
		if argument == "info" {
			continue
		}
		parts := strings.SplitN(argument, ":", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(argument, "=", 2)
		}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		params[parts[0]] = splitMethodCSV(parts[1])
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func splitMethodCSV(value string) []string {
	parts := strings.Split(value, ",")
	values := parts[:0]
	for _, part := range parts {
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func methodParamValues(
	arguments map[string][]string,
	payload map[string]any,
	key string,
) []string {
	if values := arguments[key]; len(values) != 0 {
		return values
	}
	if selections, ok := payload["selections"].(map[string]any); ok {
		if values := methodValueStrings(selections[key]); len(values) != 0 {
			return values
		}
	}
	return methodValueStrings(payload[key])
}

func methodValueStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			return []string{typed}
		}
	case []any:
		values := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok && text != "" {
				values = append(values, text)
			}
		}
		return values
	case []string:
		return slices.DeleteFunc(append([]string(nil), typed...), func(value string) bool {
			return value == ""
		})
	}
	return nil
}

func validateMethodParamValues(
	params []funcapi.ParamConfig,
	arguments map[string][]string,
	payload map[string]any,
	jobName string,
) error {
	for _, param := range params {
		values := methodParamValues(arguments, payload, param.ID)
		if len(values) == 0 {
			continue
		}
		if param.Selection == funcapi.ParamSelect && len(values) > 1 {
			return fmt.Errorf("parameter %q expects a single value", param.ID)
		}
		allowed := make(map[string]struct{}, len(param.Options))
		for _, option := range param.Options {
			if option.ID != "" && !option.Disabled {
				allowed[option.ID] = struct{}{}
			}
		}
		for _, value := range values {
			if _, ok := allowed[value]; !ok {
				if jobName == "" {
					return fmt.Errorf("parameter %q option %q is not supported", param.ID, value)
				}
				return fmt.Errorf(
					"parameter %q option %q is not supported by job %q",
					param.ID,
					value,
					jobName,
				)
			}
		}
	}
	return nil
}

func buildFunctionJobParam(jobs []string) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, max(1, len(jobs)))
	for index, job := range jobs {
		options = append(options, funcapi.ParamOption{
			ID: job, Name: job, Default: index == 0,
		})
	}
	if len(options) == 0 {
		options = append(options, funcapi.ParamOption{
			Name: "(No instances configured)", Disabled: true,
		})
	}
	return funcapi.ParamConfig{
		ID: functionJobParameter, Name: "Instance",
		Help:      "Select which database instance to query",
		Selection: funcapi.ParamSelect, Options: options, UniqueView: true,
	}
}

func acceptedMethodParams(params []funcapi.ParamConfig) []string {
	accepted := make([]string, 0, len(params))
	for _, param := range params {
		if !slices.Contains(accepted, param.ID) {
			accepted = append(accepted, param.ID)
		}
	}
	return accepted
}

func requiredMethodParams(params []funcapi.ParamConfig) []map[string]any {
	required := make([]map[string]any, 0, len(params))
	for _, param := range params {
		required = append(required, param.RequiredParam())
	}
	return required
}
