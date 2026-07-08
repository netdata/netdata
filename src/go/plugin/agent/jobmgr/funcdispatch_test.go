// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type mockMethodHandler struct {
	paramsFunc func(ctx context.Context, method string) ([]funcapi.ParamConfig, error)
	handleFunc func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse
}

func (m *mockMethodHandler) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if m.paramsFunc != nil {
		return m.paramsFunc(ctx, method)
	}
	return nil, nil
}

func (m *mockMethodHandler) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, method, params)
	}
	return nil
}

func (m *mockMethodHandler) Cleanup(context.Context) {}

func TestExecuteFunction_ModuleMethodPaths(t *testing.T) {
	tests := map[string]struct {
		methods           []funcapi.FunctionConfig
		fnArgs            []string
		fnPayload         map[string]any
		generationRace    bool
		wantStatus        int
		wantHelp          string
		wantAccepted      []string
		wantRequiredIDs   []string
		wantDataValue     any
		wantResolvedJob   string
		wantErrorContains string
	}{
		"success with __job resolution": {
			methods:         []funcapi.FunctionConfig{{ID: "details", Help: "details help"}},
			fnArgs:          []string{"__job:job1"},
			wantStatus:      200,
			wantHelp:        "details help",
			wantAccepted:    []string{"__job"},
			wantRequiredIDs: []string{"__job"},
			wantDataValue:   "row",
			wantResolvedJob: "job1",
		},
		"info response includes module and method params": {
			methods: []funcapi.FunctionConfig{{
				ID:   "details",
				Help: "details help",
				RequiredParams: []funcapi.ParamConfig{{
					ID:        "scope",
					Name:      "Scope",
					Selection: funcapi.ParamSelect,
					Options:   []funcapi.ParamOption{{ID: "default", Name: "Default"}},
				}},
			}},
			fnArgs:          []string{"info"},
			wantStatus:      200,
			wantHelp:        "details help",
			wantAccepted:    []string{"__job", "scope"},
			wantRequiredIDs: []string{"__job", "scope"},
		},
		"generation race returns 503": {
			methods:           []funcapi.FunctionConfig{{ID: "details"}},
			fnArgs:            []string{"__job:job1"},
			generationRace:    true,
			wantStatus:        503,
			wantErrorContains: "replaced during request",
		},
		"explicit unknown __job in args returns 404": {
			methods:           []funcapi.FunctionConfig{{ID: "details"}},
			fnArgs:            []string{"__job:missing"},
			wantStatus:        404,
			wantErrorContains: "unknown job 'missing'",
		},
		"explicit unknown __job in payload returns 404": {
			methods:           []funcapi.FunctionConfig{{ID: "details"}},
			fnPayload:         map[string]any{"__job": "missing"},
			wantStatus:        404,
			wantErrorContains: "unknown job 'missing'",
		},
		"multiple __job values return 400": {
			methods:           []funcapi.FunctionConfig{{ID: "details"}},
			fnPayload:         map[string]any{"__job": []string{"job1", "job2"}},
			wantStatus:        400,
			wantErrorContains: "parameter '__job' expects a single value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writer := &jsonWriteCapture{}
			var gotJob string
			var mgr *Manager

			methodHandler := &mockMethodHandler{}
			if len(tc.fnArgs) == 0 || tc.fnArgs[0] != "info" {
				methodHandler.handleFunc = func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
					if tc.generationRace {
						mgr.funcCtl.OnJobStart(&lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"})
						return &funcapi.FunctionResponse{Status: 200, Help: "should be replaced"}
					}

					gotJob = params.GetOne("__job")
					return &funcapi.FunctionResponse{
						Status: 200,
						Help:   tc.wantHelp,
						Columns: map[string]any{
							"value": map[string]any{"name": "Value"},
						},
						Data: [][]any{{tc.wantDataValue}},
					}
				}
			}

			mgr = newModuleDispatchTestManager(t, nil, writer.write, methodHandler, tc.methods)
			var payload []byte
			if tc.fnPayload != nil {
				var err error
				payload, err = json.Marshal(tc.fnPayload)
				require.NoError(t, err)
			}
			mgr.ExecuteFunction("mod:details", functions.Function{
				UID:     "module-test",
				Timeout: time.Second,
				Args:    tc.fnArgs,
				Payload: payload,
			})

			resp := writer.requireResponse(t)
			assert.Equal(t, tc.wantStatus, writer.code)
			assert.Equal(t, float64(tc.wantStatus), resp["status"])
			if tc.wantErrorContains != "" {
				assert.Contains(t, resp["errorMessage"], tc.wantErrorContains)
				return
			}

			if tc.wantResolvedJob != "" {
				assert.Equal(t, tc.wantResolvedJob, gotJob)
			}
			if tc.wantHelp != "" {
				assert.Equal(t, tc.wantHelp, resp["help"])
			}
			assert.Equal(t, tc.wantAccepted, jsonArrayStrings(t, resp["accepted_params"]))

			required := jsonObjectArray(t, resp["required_params"])
			require.Len(t, required, len(tc.wantRequiredIDs))
			for i, id := range tc.wantRequiredIDs {
				assert.Equal(t, id, required[i]["id"])
			}
			if tc.wantDataValue != nil {
				assert.Equal(t, tc.wantDataValue, jsonNestedArrayValue(t, resp["data"], 0, 0))
			}
		})
	}
}

func TestExecuteFunction_ModuleMethodPublicFunctionName(t *testing.T) {
	writer := &jsonWriteCapture{}
	var gotMethod string

	mgr := newModuleDispatchTestManager(t, nil, writer.write, &mockMethodHandler{
		handleFunc: func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
			gotMethod = method
			return &funcapi.FunctionResponse{Status: 200, Help: "trap logs"}
		},
	}, []funcapi.FunctionConfig{{
		ID:           "logs",
		FunctionName: "snmp:traps",
	}})

	mgr.ExecuteFunction("snmp:traps", functions.Function{
		UID:     "public-name",
		Timeout: time.Second,
	})

	resp := writer.requireResponse(t)
	assert.Equal(t, float64(200), resp["status"])
	assert.Equal(t, "trap logs", resp["help"])
	assert.Equal(t, "logs", gotMethod)
}

func TestExecuteFunction_AgentScopeModuleMethodDoesNotRequireRunningJob(t *testing.T) {
	tests := map[string]struct {
		functionName string
	}{
		"public function name": {
			functionName: "snmp:topology:snmp",
		},
		"alias": {
			functionName: "topology:snmp",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writer := &jsonWriteCapture{}
			var gotMethod string
			var gotJob collectorapi.RuntimeJob

			mgr := New(Config{
				PluginName:         testPluginName,
				FunctionJSONWriter: writer.write,
			})
			mgr.modules = collectorapi.Registry{
				"snmp_topology": collectorapi.Creator{
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{
							ID:           "topology:snmp",
							FunctionName: "snmp:topology:snmp",
							Aliases:      []string{"topology:snmp"},
							RequiredParams: []funcapi.ParamConfig{{
								ID:        "scope",
								Name:      "Scope",
								Selection: funcapi.ParamSelect,
								Options:   []funcapi.ParamOption{{ID: "all", Name: "All"}},
							}},
						}}
					},
					MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
						gotJob = job
						return &mockMethodHandler{
							handleFunc: func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
								gotMethod = method
								assert.Equal(t, "all", params.GetOne("scope"))
								assert.Empty(t, params.GetOne("__job"))
								return &funcapi.FunctionResponse{Status: 200, Help: "topology"}
							},
						}
					},
				},
			}
			mgr.funcCtl.RegisterModules(mgr.modules)

			mgr.ExecuteFunction(tc.functionName, functions.Function{
				UID:     "agent-scope-public-name",
				Timeout: time.Second,
				Args:    []string{"scope:all"},
			})

			resp := writer.requireResponse(t)
			assert.Equal(t, float64(200), resp["status"])
			assert.Equal(t, "topology", resp["help"])
			assert.Equal(t, "topology:snmp", gotMethod)
			assert.Nil(t, gotJob)
		})
	}
}

func TestExecuteFunction_AgentScopeModuleMethodValidationErrorOmitsJob(t *testing.T) {
	writer := &jsonWriteCapture{}

	mgr := New(Config{
		PluginName:         testPluginName,
		FunctionJSONWriter: writer.write,
	})
	mgr.modules = collectorapi.Registry{
		"mod": collectorapi.Creator{
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID: "logs",
					RequiredParams: []funcapi.ParamConfig{{
						ID:        "scope",
						Name:      "Scope",
						Selection: funcapi.ParamSelect,
						Options: []funcapi.ParamOption{
							{ID: "a", Name: "A"},
							{ID: "b", Name: "B"},
						},
					}},
				}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &mockMethodHandler{}
			},
		},
	}
	mgr.funcCtl.RegisterModules(mgr.modules)

	mgr.ExecuteFunction("mod:logs", functions.Function{
		UID:     "agent-scope-validation",
		Timeout: time.Second,
		Args:    []string{"scope:a,b"},
	})

	resp := writer.requireResponse(t)
	assert.Equal(t, float64(400), resp["status"])
	assert.Contains(t, resp["errorMessage"], "parameter 'scope' expects a single value")
}

func TestExecuteFunction_ContextBehavior(t *testing.T) {
	tests := map[string]struct {
		managerCtx      context.Context
		wantMarker      string
		wantMarkerSet   bool
		wantHasDeadline bool
	}{
		"uses background fallback before manager context is set": {
			wantHasDeadline: true,
		},
		"uses manager context when available": {
			managerCtx:      context.WithValue(context.Background(), dispatchContextKey("marker"), "manager"),
			wantMarker:      "manager",
			wantMarkerSet:   true,
			wantHasDeadline: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writer := &jsonWriteCapture{}
			mgr := newModuleDispatchTestManager(t, nil, writer.write, &mockMethodHandler{
				handleFunc: func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
					require.NotNil(t, ctx)
					_, hasDeadline := ctx.Deadline()
					assert.Equal(t, tc.wantHasDeadline, hasDeadline)
					gotMarker := ctx.Value(dispatchContextKey("marker"))
					if tc.wantMarkerSet {
						assert.Equal(t, tc.wantMarker, gotMarker)
					} else {
						assert.Nil(t, gotMarker)
					}
					return &funcapi.FunctionResponse{Status: 200}
				},
			}, []funcapi.FunctionConfig{{ID: "details"}})
			if tc.managerCtx != nil {
				mgr.funcCtl.Init(tc.managerCtx)
			}

			mgr.ExecuteFunction("mod:details", functions.Function{
				UID:     "module-context",
				Timeout: time.Second,
				Args:    []string{"__job:job1"},
			})

			resp := writer.requireResponse(t)
			assert.Equal(t, float64(200), resp["status"])
		})
	}
}

func TestInstanceFunctionRegisteredHandlerPaths(t *testing.T) {
	tests := map[string]struct {
		fnArgs          []string
		wantRequiredLen int
		wantDataValue   any
	}{
		"success path omits __job": {
			wantRequiredLen: 0,
			wantDataValue:   "job1",
		},
		"info path omits __job": {
			fnArgs:          []string{"info"},
			wantRequiredLen: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writer := &jsonWriteCapture{}
			fnReg := newCapturingFunctionRegistry()
			mgr := newInstanceFunctionDispatchTestManager(t, fnReg, writer.write, &mockMethodHandler{
				handleFunc: func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
					return &funcapi.FunctionResponse{
						Status: 200,
						Help:   "job details help",
						Columns: map[string]any{
							"value": map[string]any{"name": "Value"},
						},
						Data: [][]any{{"job1"}},
					}
				},
			}, []funcapi.FunctionConfig{{ID: "job-details", Help: "job details help"}})

			handler := fnReg.requireHandler(t, "mod:job-details")
			handler(functions.Function{
				UID:     "job-handler",
				Timeout: time.Second,
				Args:    tc.fnArgs,
			})

			resp := writer.requireResponse(t)
			assert.Equal(t, float64(200), resp["status"])
			assert.NotContains(t, jsonArrayStrings(t, resp["accepted_params"]), "__job")
			assert.Len(t, jsonObjectArray(t, resp["required_params"]), tc.wantRequiredLen)
			if tc.wantDataValue != nil {
				assert.Equal(t, tc.wantDataValue, jsonNestedArrayValue(t, resp["data"], 0, 0))
			}
			mgr.stopRunningJob(context.Background(), "mod_job1")
		})
	}
}

func TestFunctionDispatch_ResponsePaths(t *testing.T) {
	tests := map[string]struct {
		useJSONWriter      bool
		rebindResponder    bool
		nilRebindResponder bool
		marshalFail        bool
		wantWriterStatus   int
		wantResponderUID   string
		wantResponderJSON  string
		wantFirstUID       string
		wantSecondUID      string
	}{
		"JSONWriter takes precedence when configured": {
			useJSONWriter:    true,
			wantWriterStatus: 200,
			wantResponderUID: "writer-first",
		},
		"responder fallback is used when JSONWriter is nil": {
			wantResponderUID:  "responder-fallback",
			wantResponderJSON: "\"status\":200",
		},
		"responder rebinding updates only the responder-backed path": {
			rebindResponder: true,
			wantFirstUID:    "before-rebind",
			wantSecondUID:   "after-rebind",
		},
		"nil responder rebinding preserves the current responder-backed path": {
			nilRebindResponder: true,
			wantFirstUID:       "before-nil-rebind",
			wantSecondUID:      "after-nil-rebind",
		},
		"marshal failure falls back to JSONWriter with 500": {
			useJSONWriter:    true,
			marshalFail:      true,
			wantWriterStatus: 500,
			wantResponderUID: "writer-marshal-fail",
		},
		"marshal failure falls back to responder with 500": {
			marshalFail:       true,
			wantResponderUID:  "responder-marshal-fail",
			wantResponderJSON: "\"status\":500",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &mockMethodHandler{
				handleFunc: func(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
					if tc.marshalFail {
						return &funcapi.FunctionResponse{
							Status: 200,
							Data:   [][]any{{make(chan int)}},
						}
					}
					return &funcapi.FunctionResponse{Status: 200}
				},
			}

			if tc.useJSONWriter {
				writer := &jsonWriteCapture{}
				var responderOut bytes.Buffer

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), writer.write, handler, []funcapi.FunctionConfig{{ID: "details"}})
				mgr.ExecuteFunction("mod:details", functions.Function{
					UID:     tc.wantResponderUID,
					Timeout: time.Second,
					Args:    []string{"__job:job1"},
				})

				resp := writer.requireResponse(t)
				assert.Equal(t, float64(tc.wantWriterStatus), resp["status"])
				if tc.marshalFail {
					assert.Contains(t, resp["errorMessage"], "failed to encode response")
				}
				assert.NotContains(t, responderOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantResponderUID)
				return
			}

			if tc.rebindResponder {
				var firstOut bytes.Buffer
				var secondOut bytes.Buffer

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&firstOut))), nil, handler, []funcapi.FunctionConfig{{ID: "details"}})
				mgr.ExecuteFunction("mod:details", functions.Function{
					UID:     tc.wantFirstUID,
					Timeout: time.Second,
					Args:    []string{"__job:job1"},
				})

				mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&secondOut))))
				mgr.ExecuteFunction("mod:details", functions.Function{
					UID:     tc.wantSecondUID,
					Timeout: time.Second,
					Args:    []string{"__job:job1"},
				})

				assert.Contains(t, firstOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantFirstUID)
				assert.NotContains(t, firstOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantSecondUID)
				assert.Contains(t, secondOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantSecondUID)
				return
			}

			if tc.nilRebindResponder {
				var responderOut bytes.Buffer

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), nil, handler, []funcapi.FunctionConfig{{ID: "details"}})
				mgr.ExecuteFunction("mod:details", functions.Function{
					UID:     tc.wantFirstUID,
					Timeout: time.Second,
					Args:    []string{"__job:job1"},
				})

				mgr.SetDyncfgResponder(nil)
				mgr.ExecuteFunction("mod:details", functions.Function{
					UID:     tc.wantSecondUID,
					Timeout: time.Second,
					Args:    []string{"__job:job1"},
				})

				assert.Contains(t, responderOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantFirstUID)
				assert.Contains(t, responderOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantSecondUID)
				return
			}

			var responderOut bytes.Buffer
			mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), nil, handler, []funcapi.FunctionConfig{{ID: "details"}})
			mgr.ExecuteFunction("mod:details", functions.Function{
				UID:     tc.wantResponderUID,
				Timeout: time.Second,
				Args:    []string{"__job:job1"},
			})

			assert.Contains(t, responderOut.String(), "FUNCTION_RESULT_BEGIN "+tc.wantResponderUID)
			assert.Contains(t, responderOut.String(), tc.wantResponderJSON)
			if tc.marshalFail {
				assert.Contains(t, responderOut.String(), "failed to encode response")
			}
		})
	}
}

func TestCleanup_UnregistersStaticFunctionsBeforeStoppingJobs(t *testing.T) {
	fnReg := newCapturingFunctionRegistry()
	mgr := New(Config{PluginName: testPluginName, FnReg: fnReg})

	staticCreator := collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "static-method"}}
		},
	}
	jobCreator := collectorapi.Creator{
		InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "job-method"}}
		},
	}

	mgr.modules = collectorapi.Registry{
		"staticmod": staticCreator,
		"jobmod":    jobCreator,
	}
	mgr.funcCtl.RegisterModules(mgr.modules)

	mgr.startRunningJob(&lockProbeJob{fullName: "staticmod_job1", moduleName: "staticmod", name: "job1"})
	mgr.publishRunningJobFunctions("staticmod_job1")
	mgr.funcCtl.ReconcileModuleMethods("staticmod")
	mgr.startRunningJob(&lockProbeJob{fullName: "jobmod_job1", moduleName: "jobmod", name: "job1"})
	mgr.publishRunningJobFunctions("jobmod_job1")
	mgr.funcCtl.ReconcileModuleMethods("jobmod")

	mgr.cleanup()

	unregistered := fnReg.unregisteredNames()
	assert.Contains(t, unregistered, "staticmod:static-method")
	assert.Contains(t, unregistered, "jobmod:job-method")
}

type dispatchContextKey string

type jsonWriteCapture struct {
	calls int
	code  int
	raw   []byte
}

func (c *jsonWriteCapture) write(payload []byte, code int) {
	c.calls++
	c.code = code
	c.raw = append([]byte(nil), payload...)
}

func (c *jsonWriteCapture) requireResponse(t *testing.T) map[string]any {
	t.Helper()
	require.Equal(t, 1, c.calls, "expected exactly one JSON writer call")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(c.raw, &resp))
	return resp
}

type capturingFunctionRegistry struct {
	mu           sync.Mutex
	handlers     map[string]func(functions.Function)
	unregistered []string
}

func newCapturingFunctionRegistry() *capturingFunctionRegistry {
	return &capturingFunctionRegistry{
		handlers: make(map[string]func(functions.Function)),
	}
}

func (r *capturingFunctionRegistry) Register(name string, fn func(functions.Function)) {
	r.mu.Lock()
	r.handlers[name] = fn
	r.mu.Unlock()
}

func (r *capturingFunctionRegistry) Unregister(name string) {
	r.mu.Lock()
	r.unregistered = append(r.unregistered, name)
	delete(r.handlers, name)
	r.mu.Unlock()
}

func (r *capturingFunctionRegistry) RegisterPrefix(string, string, func(functions.Function)) {}
func (r *capturingFunctionRegistry) UnregisterPrefix(string, string)                         {}

func (r *capturingFunctionRegistry) requireHandler(t *testing.T, name string) func(functions.Function) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()

	handler, ok := r.handlers[name]
	require.True(t, ok, "handler %q was not registered", name)
	return handler
}

func (r *capturingFunctionRegistry) unregisteredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.unregistered))
	copy(out, r.unregistered)
	return out
}

func newModuleDispatchTestManager(
	t *testing.T,
	api *dyncfg.Responder,
	jsonWriter func([]byte, int),
	methodHandler funcapi.MethodHandler,
	methods []funcapi.FunctionConfig,
) *Manager {
	t.Helper()

	mgr := New(Config{
		PluginName:         testPluginName,
		FunctionJSONWriter: jsonWriter,
	})
	if api != nil {
		mgr.SetDyncfgResponder(api)
	}

	creator := collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig { return methods },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return methodHandler
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)
	mgr.funcCtl.OnJobStart(&lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"})
	mgr.funcCtl.ReconcileModuleMethods("mod")

	return mgr
}

func newInstanceFunctionDispatchTestManager(
	t *testing.T,
	fnReg FunctionRegistry,
	jsonWriter func([]byte, int),
	methodHandler funcapi.MethodHandler,
	methods []funcapi.FunctionConfig,
) *Manager {
	t.Helper()

	mgr := New(Config{
		PluginName:         testPluginName,
		FnReg:              fnReg,
		FunctionJSONWriter: jsonWriter,
	})

	creator := collectorapi.Creator{
		InstanceFunctions: func(_ collectorapi.RuntimeJob) []funcapi.FunctionConfig { return methods },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return methodHandler
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)
	mgr.startRunningJob(&lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"})
	mgr.publishRunningJobFunctions("mod_job1") // publication happens at commit, not inside the start
	mgr.funcCtl.ReconcileModuleMethods("mod")

	return mgr
}

func jsonArrayStrings(t *testing.T, raw any) []string {
	t.Helper()

	items, ok := raw.([]any)
	require.True(t, ok, "expected []any, got %T", raw)

	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		require.True(t, ok, "expected string item, got %T", item)
		out = append(out, s)
	}
	return out
}

func jsonObjectArray(t *testing.T, raw any) []map[string]any {
	t.Helper()

	items, ok := raw.([]any)
	require.True(t, ok, "expected []any, got %T", raw)

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		require.True(t, ok, "expected map[string]any item, got %T", item)
		out = append(out, obj)
	}
	return out
}

func jsonNestedArrayValue(t *testing.T, raw any, row, col int) any {
	t.Helper()

	rows, ok := raw.([]any)
	require.True(t, ok, "expected []any rows, got %T", raw)
	require.Len(t, rows, row+1)

	cols, ok := rows[row].([]any)
	require.True(t, ok, "expected []any columns, got %T", rows[row])
	require.Len(t, cols, col+1)

	return cols[col]
}
