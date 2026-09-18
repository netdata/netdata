// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedInfoBootstrapAndScopedMetadata(t *testing.T) {
	var calls atomic.Int64
	method := managedInfoTestMethod()
	h := newManagedInfoHarness(
		t,
		method,
		"shared",
		func(_ context.Context, job string, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
			calls.Add(1)
			if job == "alpha" {
				return funcapi.UnavailableResponse("source unavailable")
			}
			assert.Equal(t, "events", req.Method)
			assert.True(t, req.Info)
			assert.Equal(t, []string{"info"}, req.Args)
			assert.JSONEq(t, `{"selections":{"__job":["beta rack,1"]},"after":123}`, string(req.Payload))
			return &funcapi.FunctionResponse{
				Help: "Selected source",
				RequiredParams: []funcapi.ParamConfig{{
					ID: "service", Name: "Service", Options: []funcapi.ParamOption{{ID: "events", Name: "Events"}},
				}},
				Columns: map[string]any{},
				Data:    [][]any{{"info must omit data"}},
			}
		},
	)

	bootstrap := h.call(t, []string{"info"}, nil, 200)
	assert.JSONEq(t, `{
		"v":3,"update_every":60,"status":200,"type":"table","has_history":true,"help":"Event query",
		"accepted_params":["__job","format","after","before","query"],
		"required_params":[
			{"id":"__job","name":"Instance","help":"Select which instance to query","type":"select","unique_view":true,
			 "options":[{"id":"alpha","name":"alpha","defaultSelected":true},{"id":"beta rack,1","name":"beta rack,1"}]},
			{"id":"format","name":"Format","type":"select","options":[{"id":"text","name":"Text","defaultSelected":true}]}
		]}`, string(bootstrap))
	assert.Zero(t, calls.Load(), "bootstrap must not invoke the default source")
	assertFunctionInfoSchema(t, bootstrap, "info_response")

	scoped := h.call(t, []string{"info"}, []byte(`{"selections":{"__job":["beta rack,1"]},"after":123}`), 200)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(scoped, &decoded))
	assert.Equal(t, "Selected source", decoded["help"])
	assert.Equal(t, []any{"__job", "format", "service", "after", "before", "query"}, decoded["accepted_params"])
	assert.Len(t, decoded["required_params"], 3)
	assert.EqualValues(t, 1, calls.Load())
	assertFunctionInfoSchema(t, scoped, "info_response")
}

func TestManagedInfoHelp(t *testing.T) {
	for name, test := range map[string]struct {
		declared string
		response string
		info     bool
		want     string
	}{
		"declared help":    {declared: "Declared help", info: true, want: "Declared help"},
		"default help":     {info: true, want: "module events data function"},
		"handler override": {declared: "Declared help", response: "Source help", info: true, want: "Source help"},
		"data unchanged":   {declared: "Declared help"},
	} {
		t.Run(name, func(t *testing.T) {
			method := funcapi.FunctionConfig{
				ID:          "events",
				Help:        test.declared,
				RawRequest:  true,
				ManagedInfo: true,
			}
			h := newManagedInfoHarness(
				t,
				method,
				"agent",
				func(_ context.Context, _ string, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					return &funcapi.FunctionResponse{
						Help:           test.response,
						RequiredParams: []funcapi.ParamConfig{{ID: "service", Name: "Service"}},
					}
				},
			)
			var args []string
			if test.info {
				args = []string{"info"}
			}
			got := h.call(t, args, nil, 200)
			assert.JSONEq(t, fmt.Sprintf(`{
				"v":3,"update_every":1,"status":200,"type":"table","has_history":false,"help":%q,
				"accepted_params":["service"],"required_params":[{"id":"service","name":"Service","type":"select","options":[]}]
			}`, test.want), string(got))
		})
	}
}

func TestManagedInfoRoutingAndErrors(t *testing.T) {
	tests := map[string]struct {
		args    []string
		payload string
		status  int
		want    string
		calls   int64
	}{
		"selected source failure": {
			args:   []string{"info", "__job:alpha"},
			status: 503,
			want:   "source unavailable",
			calls:  1,
		},
		"unknown job": {
			args:    []string{"info"},
			payload: `{"selections":{"__job":["missing"]}}`,
			status:  404,
			want:    "unknown job",
		},
		"multiple jobs": {
			args:    []string{"info"},
			payload: `{"selections":{"__job":["alpha","beta rack,1"]}}`,
			status:  400,
			want:    "single value",
		},
		"repeated job arguments": {
			args:   []string{"info", "__job:missing", "__job:alpha"},
			status: 400,
			want:   "single value",
		},
		"repeated job arguments on data": {
			args:   []string{"__job:missing", "__job:alpha"},
			status: 400,
			want:   "single value",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			h := newManagedInfoHarness(
				t,
				managedInfoTestMethod(),
				"shared",
				func(_ context.Context, _ string, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					calls.Add(1)
					return funcapi.UnavailableResponse("source unavailable")
				},
			)
			got := h.call(t, test.args, []byte(test.payload), test.status)
			assert.Contains(t, string(got), test.want)
			assert.Equal(t, test.calls, calls.Load())
		})
	}
}

func TestManagedInfoBoundMethods(t *testing.T) {
	for _, mode := range []string{"agent", "instance", "single"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			h := newManagedInfoHarness(
				t,
				managedInfoTestMethod(),
				mode,
				func(_ context.Context, _ string, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					calls.Add(1)
					assert.True(t, req.Info)
					return &funcapi.FunctionResponse{
						Help: "Bound source",
					}
				},
			)
			got := h.call(t, []string{"info"}, nil, 200)
			assert.NotContains(t, string(got), "__job")
			assert.Contains(t, string(got), "Bound source")
			assert.EqualValues(t, 1, calls.Load())
			assertFunctionInfoSchema(t, got, "info_response")
		})
	}
}

func TestManagedInfoDataComposition(t *testing.T) {
	method := managedInfoTestMethod()
	method.RequiredParams = append(method.RequiredParams, funcapi.ParamConfig{
		ID:   "__job",
		Name: "Wrong job list",
	})
	h := newManagedInfoHarness(
		t,
		method,
		"shared",
		func(_ context.Context, job string, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
			assert.Equal(t, "beta rack,1", job)
			assert.False(t, req.Info)
			assert.JSONEq(t, `{"selections":{"__job":["beta rack,1"]},"query":"fan","before":456}`, string(req.Payload))
			return &funcapi.FunctionResponse{
				Help: "Query result",
				RequiredParams: []funcapi.ParamConfig{
					{ID: "format", Name: "Format", Options: []funcapi.ParamOption{{ID: "full", Name: "Full"}}},
					{ID: "__job", Name: "Also wrong"},
				},
				Columns: map[string]any{"message": map[string]any{"index": 0, "name": "Message", "type": "string"}},
				Data:    [][]any{{"fan event"}},
			}
		},
	)
	got := h.call(t, nil, []byte(`{"selections":{"__job":["beta rack,1"]},"query":"fan","before":456}`), 200)
	assert.JSONEq(t, `{
		"v":3,"update_every":60,"status":200,"type":"table","has_history":true,"help":"Query result",
		"accepted_params":["__job","format","after","before","query"],
		"required_params":[
			{"id":"__job","name":"Instance","help":"Select which instance to query","type":"select","unique_view":true,
			 "options":[{"id":"alpha","name":"alpha","defaultSelected":true},{"id":"beta rack,1","name":"beta rack,1"}]},
			{"id":"format","name":"Format","type":"select","options":[{"id":"full","name":"Full","defaultSelected":true}]}
		],"columns":{"message":{"index":0,"name":"Message","type":"string"}},"data":[["fan event"]]
	}`, string(got))
	assertFunctionInfoSchema(t, got, "data_response")
}

func TestManagedInfoExistingModes(t *testing.T) {
	for name, raw := range map[string]bool{"structured info": false, "raw info passthrough": true} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			method := funcapi.FunctionConfig{
				ID:         "events",
				RawRequest: raw,
			}
			h := newManagedInfoHarness(
				t,
				method,
				"agent",
				func(_ context.Context, _ string, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					calls.Add(1)
					assert.True(t, req.Info)
					return funcapi.RawResponse(map[string]any{"status": 200, "sdk": "unchanged"})
				},
			)
			got := h.call(t, []string{"info"}, nil, 200)
			if raw {
				assert.JSONEq(t, `{"status":200,"sdk":"unchanged"}`, string(got))
				assert.EqualValues(t, 1, calls.Load())
			} else {
				assert.JSONEq(t, `{"v":3,"update_every":1,"status":200,"type":"table","has_history":false,
					"help":"module events data function","accepted_params":[],"required_params":[]}`, string(got))
				assert.Zero(t, calls.Load())
			}
		})
	}
}

func TestManagedInfoJobReload(t *testing.T) {
	h := newManagedInfoHarness(
		t,
		managedInfoTestMethod(),
		"shared",
		func(_ context.Context, _ string, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
			return funcapi.UnavailableResponse("source unavailable")
		},
	)
	require.NoError(t, h.handles["alpha"].CloseAndDrain(t.Context()))
	got := h.call(t, []string{"info"}, nil, 200)
	assert.NotContains(t, string(got), "alpha")
	assert.Contains(t, string(got), "beta rack,1")
	got = h.call(t, []string{"info", "__job:alpha"}, nil, 404)
	assert.Contains(t, string(got), "unknown job")
	job := &controllerTestJob{
		fullName: "module_alpha",
		module:   "module",
		name:     "alpha",
		running:  true,
	}
	handle, err := prepareControllerTestJob(
		t,
		h.controller,
		lifecycle.ResourceIdentity{
			ID:         job.FullName(),
			Generation: 2,
		},
		job,
	)
	require.NoError(t, err)
	require.NoError(t, handle.Publish())
	got = h.call(t, []string{"info"}, nil, 200)
	assert.Contains(t, string(got), "alpha")
	got = h.call(t, []string{"info", "__job:alpha"}, nil, 503)
	assert.Contains(t, string(got), "source unavailable")
}

func TestManagedInfoCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := make(chan struct{})
	h := newManagedInfoHarness(
		t,
		managedInfoTestMethod(),
		"shared",
		func(callCtx context.Context, _ string, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
			assert.True(t, req.Info)
			cancel()
			<-callCtx.Done()
			close(observed)
			return funcapi.UnavailableResponse("query canceled")
		},
	)
	decision, err := h.catalog.ResolveAndAcquire(
		jobmgr.FunctionLookup{
			UID:   "cancel-info",
			Route: h.route,
			Args:  []string{"info", "__job:alpha"},
		},
	)
	require.NoError(t, err)
	require.Zero(t, decision.Rejected)
	_, err = decision.Plan.Work(ctx)
	require.NoError(t, err)
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("selected info handler did not receive cancellation")
	}
	cleanup, err := h.catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())
}

func TestManagedInfoRawResponsePassthrough(t *testing.T) {
	h := newManagedInfoHarness(
		t,
		managedInfoTestMethod(),
		"shared",
		func(_ context.Context, _ string, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
			return funcapi.RawResponse(map[string]any{"status": 202, "sdk": "unchanged"})
		},
	)
	for _, args := range [][]string{{"info", "__job:alpha"}, {"__job:alpha"}} {
		got := h.call(t, args, nil, 202)
		assert.JSONEq(t, `{"status":202,"sdk":"unchanged"}`, string(got))
	}
}

func TestFunctionConfigRequiresRawRequest(t *testing.T) {
	for name, test := range map[string]struct {
		method funcapi.FunctionConfig
		want   string
	}{
		"managed info": {
			method: funcapi.FunctionConfig{
				ID:          "events",
				ManagedInfo: true,
			},
			want: "ManagedInfo requires RawRequest",
		},
		"extra accepted inputs": {
			method: funcapi.FunctionConfig{
				ID:             "events",
				AcceptedParams: []string{"after", "query"},
			},
			want: "AcceptedParams requires RawRequest",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := newContainedControllerTest(t, 1, collectorapi.Registry{
				"module": {
					AgentFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{test.method} },
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						t.Error("invalid declarations must be rejected before constructing a handler")
						return &managedInfoHandler{
							t: t,
						}
					},
				},
			})
			require.ErrorContains(t, err, test.want)
		})
	}
}

func managedInfoTestMethod() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             "events",
		UpdateEvery:    60,
		Help:           "Event query",
		RawRequest:     true,
		ManagedInfo:    true,
		HasHistory:     true,
		AcceptedParams: []string{"after", "before", "query", "format", "after"},
		RequiredParams: []funcapi.ParamConfig{
			{ID: "format", Name: "Format", Options: []funcapi.ParamOption{{ID: "text", Name: "Text"}}},
		},
	}
}

type managedInfoHandler struct {
	t      *testing.T
	handle func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
}

func (h *managedInfoHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	h.t.Error("info/raw handling must not call MethodParams")
	return nil, nil
}

func (h *managedInfoHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	h.t.Error("raw handling must not call Handle")
	return funcapi.InternalErrorResponse("unexpected structured call")
}

func (h *managedInfoHandler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	return h.handle(ctx, req)
}

func (*managedInfoHandler) Cleanup(context.Context) {}

func newManagedInfoHarness(
	t *testing.T,
	method funcapi.FunctionConfig,
	mode string,
	handle func(context.Context, string, funcapi.RawMethodRequest) *funcapi.FunctionResponse,
) *functionInfoHarness {
	t.Helper()
	return newFunctionInfoHarness(t, method, mode, func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
		name := ""
		if job != nil {
			name = job.Name()
		}
		return &managedInfoHandler{
			t: t,
			handle: func(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
				return handle(ctx, name, req)
			},
		}
	})
}
