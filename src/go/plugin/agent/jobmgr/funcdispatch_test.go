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
		methods           []funcapi.MethodConfig
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
			methods:         []funcapi.MethodConfig{{ID: "details", Help: "details help"}},
			fnArgs:          []string{"__job:job1"},
			wantStatus:      200,
			wantHelp:        "details help",
			wantAccepted:    []string{"__job"},
			wantRequiredIDs: []string{"__job"},
			wantDataValue:   "row",
			wantResolvedJob: "job1",
		},
		"info response includes module and method params": {
			methods: []funcapi.MethodConfig{{
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
			methods:           []funcapi.MethodConfig{{ID: "details"}},
			fnArgs:            []string{"__job:job1"},
			generationRace:    true,
			wantStatus:        503,
			wantErrorContains: "replaced during request",
		},
		"explicit unknown __job in args returns 404": {
			methods:           []funcapi.MethodConfig{{ID: "details"}},
			fnArgs:            []string{"__job:missing"},
			wantStatus:        404,
			wantErrorContains: "unknown job 'missing'",
		},
		"explicit unknown __job in payload returns 404": {
			methods:           []funcapi.MethodConfig{{ID: "details"}},
			fnPayload:         map[string]any{"__job": "missing"},
			wantStatus:        404,
			wantErrorContains: "unknown job 'missing'",
		},
		"multiple __job values return 400": {
			methods:           []funcapi.MethodConfig{{ID: "details"}},
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
			}, []funcapi.MethodConfig{{ID: "details"}})
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

func TestJobMethodRegisteredHandlerPaths(t *testing.T) {
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
			mgr := newJobMethodDispatchTestManager(t, fnReg, writer.write, &mockMethodHandler{
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
			}, []funcapi.MethodConfig{{ID: "job-details", Help: "job details help"}})

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
			mgr.stopRunningJob("mod_job1")
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

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), writer.write, handler, []funcapi.MethodConfig{{ID: "details"}})
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

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&firstOut))), nil, handler, []funcapi.MethodConfig{{ID: "details"}})
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

				mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), nil, handler, []funcapi.MethodConfig{{ID: "details"}})
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
			mgr := newModuleDispatchTestManager(t, dyncfg.NewResponder(netdataapi.New(safewriter.New(&responderOut))), nil, handler, []funcapi.MethodConfig{{ID: "details"}})
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
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "static-method"}}
		},
	}
	jobCreator := collectorapi.Creator{
		JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "job-method"}}
		},
	}

	mgr.modules = collectorapi.Registry{
		"staticmod": staticCreator,
		"jobmod":    jobCreator,
	}
	mgr.funcCtl.RegisterModules(mgr.modules)

	mgr.startRunningJob(&lockProbeJob{fullName: "staticmod_job1", moduleName: "staticmod", name: "job1"})
	mgr.startRunningJob(&lockProbeJob{fullName: "jobmod_job1", moduleName: "jobmod", name: "job1"})

	mgr.cleanup()

	unregistered := fnReg.unregisteredNames()
	assert.Contains(t, unregistered, "staticmod:static-method")
	assert.Contains(t, unregistered, "jobmod:job-method")
	assert.Less(
		t,
		fnReg.unregisteredIndex("staticmod:static-method"),
		fnReg.unregisteredIndex("jobmod:job-method"),
		"static module cleanup must run before per-job stop cleanup",
	)
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

func (r *capturingFunctionRegistry) unregisteredIndex(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, got := range r.unregistered {
		if got == name {
			return i
		}
	}
	return -1
}

func newModuleDispatchTestManager(
	t *testing.T,
	api *dyncfg.Responder,
	jsonWriter func([]byte, int),
	methodHandler funcapi.MethodHandler,
	methods []funcapi.MethodConfig,
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
		Methods: func() []funcapi.MethodConfig { return methods },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return methodHandler
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)
	mgr.funcCtl.OnJobStart(&lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"})

	return mgr
}

func newJobMethodDispatchTestManager(
	t *testing.T,
	fnReg FunctionRegistry,
	jsonWriter func([]byte, int),
	methodHandler funcapi.MethodHandler,
	methods []funcapi.MethodConfig,
) *Manager {
	t.Helper()

	mgr := New(Config{
		PluginName:         testPluginName,
		FnReg:              fnReg,
		FunctionJSONWriter: jsonWriter,
	})

	creator := collectorapi.Creator{
		JobMethods: func(_ collectorapi.RuntimeJob) []funcapi.MethodConfig { return methods },
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return methodHandler
		},
	}
	mgr.modules = collectorapi.Registry{"mod": creator}
	mgr.funcCtl.RegisterModules(mgr.modules)
	mgr.startRunningJob(&lockProbeJob{fullName: "mod_job1", moduleName: "mod", name: "job1"})

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
