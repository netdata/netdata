// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mysql/mysqlfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredInfoBootstrapAndScopedMetadata(t *testing.T) {
	var calls atomic.Int64
	method := funcapi.FunctionConfig{
		ID: "events",
		RequiredParams: []funcapi.ParamConfig{
			{ID: "format", Name: "Format", Options: []funcapi.ParamOption{{ID: "text", Name: "Text"}}},
			{ID: "__sort", Name: "Sort", Options: []funcapi.ParamOption{{ID: "static", Name: "Static"}}},
		},
	}
	h := newFunctionInfoHarness(t, method, "shared", func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
		return &structuredInfoHandler{
			t: t,
			params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
				calls.Add(1)
				return []funcapi.ParamConfig{
					{ID: "__sort", Name: "Sort", Options: []funcapi.ParamOption{{ID: job.Name(), Name: "Available"}}},
					{ID: "__job", Name: "Handler must not replace job selection"},
				}, nil
			},
		}
	})

	bootstrap := h.call(t, []string{"info"}, nil, 200)
	assert.Contains(t, string(bootstrap), `"id":"static"`)
	assert.Zero(t, calls.Load(), "unscoped info must not contact any source")
	assertFunctionInfoSchema(t, bootstrap, "info_response")

	for name, test := range map[string]struct {
		args    []string
		payload string
		job     string
	}{
		"argument": {args: []string{"info", "__job:alpha", "__sort:stale"}, job: "alpha"},
		"payload":  {args: []string{"info"}, payload: `{"selections":{"__job":["beta rack,1"],"__sort":["stale"]}}`, job: "beta rack,1"},
	} {
		t.Run(name, func(t *testing.T) {
			got := h.call(t, test.args, []byte(test.payload), 200)
			assert.JSONEq(t, fmt.Sprintf(`{
				"v":3,"update_every":1,"status":200,"type":"table","has_history":false,
				"help":"module events data function","accepted_params":["__job","format","__sort"],
				"required_params":[
					{"id":"__job","name":"Instance","help":"Select which instance to query","type":"select","unique_view":true,
					 "options":[{"id":"alpha","name":"alpha","defaultSelected":true},{"id":"beta rack,1","name":"beta rack,1"}]},
					{"id":"format","name":"Format","type":"select","options":[{"id":"text","name":"Text","defaultSelected":true}]},
					{"id":"__sort","name":"Sort","type":"select","options":[{"id":%q,"name":"Available","defaultSelected":true}]}
				]}`, test.job), string(got))
			assertFunctionInfoSchema(t, got, "info_response")
		})
	}
	assert.EqualValues(t, 2, calls.Load())
}

func TestStructuredInfoPreservesStaticModes(t *testing.T) {
	for _, mode := range []string{"agent", "instance", "single"} {
		t.Run(mode, func(t *testing.T) {
			h := newFunctionInfoHarness(t, funcapi.FunctionConfig{
				ID: "events",
			}, mode,
				func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &structuredInfoHandler{
						t: t,
					}
				})
			for _, args := range [][]string{{"info"}, {"info", "__job:unknown"}} {
				got := h.call(t, args, nil, 200)
				assert.JSONEq(t, `{"v":3,"update_every":1,"status":200,"type":"table","has_history":false,
					"help":"module events data function","accepted_params":[],"required_params":[]}`, string(got))
			}
		})
	}
}

func TestStructuredInfoNilParamsKeepsDeclarations(t *testing.T) {
	var calls atomic.Int64
	method := funcapi.FunctionConfig{
		ID: "events",
		RequiredParams: []funcapi.ParamConfig{
			{ID: "format", Name: "Format", Options: []funcapi.ParamOption{{ID: "text", Name: "Text"}}},
		},
	}
	h := newFunctionInfoHarness(t, method, "shared", func(collectorapi.RuntimeJob) funcapi.MethodHandler {
		return &structuredInfoHandler{
			t: t,
			params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
				calls.Add(1)
				return nil, nil
			},
		}
	})
	bootstrap := h.call(t, []string{"info"}, nil, 200)
	scoped := h.call(t, []string{"info", "__job:alpha"}, nil, 200)
	assert.JSONEq(t, string(bootstrap), string(scoped))
	assert.EqualValues(t, 1, calls.Load())
}

func TestStructuredInfoRoutingAndErrors(t *testing.T) {
	for name, test := range map[string]struct {
		args    []string
		payload string
		failure string
		status  int
		message string
		calls   int64
	}{
		"unknown job":               {args: []string{"info", "__job:missing"}, status: 404, message: "unknown job"},
		"multiple jobs":             {args: []string{"info"}, payload: `{"selections":{"__job":["alpha","beta rack,1"]}}`, status: 400, message: "single value"},
		"repeated job arguments":    {args: []string{"info", "__job:missing", "__job:alpha"}, status: 400, message: "single value"},
		"repeated equals arguments": {args: []string{"info", "__job=missing", "__job=alpha"}, status: 400, message: "single value"},
		"source failure":            {args: []string{"info", "__job:alpha"}, failure: "source", status: 503, message: "source unavailable", calls: 1},
		"panic":                     {args: []string{"info", "__job:alpha"}, failure: "panic", status: 503, message: "Function handler is unavailable", calls: 1},
		"job stopped":               {args: []string{"info", "__job:alpha"}, failure: "stop", status: 503, message: "stopped during request", calls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			h := newFunctionInfoHarness(
				t,
				funcapi.FunctionConfig{
					ID: "events",
				},
				"shared",
				func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &structuredInfoHandler{
						t: t,
						params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
							calls.Add(1)
							switch test.failure {
							case "panic":
								panic("metadata failed")
							case "source":
								return nil, errors.New("source unavailable")
							case "stop":
								job.(*controllerTestJob).running = false
							}
							return nil, nil
						},
					}
				},
			)
			got := h.call(t, test.args, []byte(test.payload), test.status)
			assert.Contains(t, string(got), test.message)
			assert.Equal(t, test.calls, calls.Load())
		})
	}
}

func TestStructuredInfoCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := make(chan struct{})
	h := newFunctionInfoHarness(
		t,
		funcapi.FunctionConfig{
			ID: "events",
		},
		"shared",
		func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &structuredInfoHandler{
				t: t,
				params: func(callCtx context.Context, _ string) ([]funcapi.ParamConfig, error) {
					cancel()
					<-callCtx.Done()
					close(observed)
					return nil, callCtx.Err()
				},
			}
		},
	)
	decision, err := h.catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "cancel-structured-info",
		Route: h.route,
		Args:  []string{"info", "__job:alpha"},
	})
	require.NoError(t, err)
	require.Zero(t, decision.Rejected)
	_, err = decision.Plan.Work(ctx)
	require.NoError(t, err)
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("metadata discovery did not receive cancellation")
	}
	cleanup, err := h.catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())
}

func TestStructuredInfoJobReload(t *testing.T) {
	var construction int
	h := newFunctionInfoHarness(
		t,
		funcapi.FunctionConfig{
			ID: "events",
		},
		"shared",
		func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			construction++
			option := fmt.Sprintf("source-%d", construction)
			return &structuredInfoHandler{
				t: t,
				params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
					return []funcapi.ParamConfig{
						{ID: "source", Name: "Source", Options: []funcapi.ParamOption{{ID: option, Name: option}}},
					}, nil
				},
			}
		},
	)
	got := h.call(t, []string{"info", "__job:alpha"}, nil, 200)
	assert.Contains(t, string(got), `"id":"source-1"`)
	require.NoError(t, h.handles["alpha"].CloseAndDrain(t.Context()))
	got = h.call(t, []string{"info"}, nil, 200)
	assert.NotContains(t, string(got), "alpha")
	h.call(t, []string{"info", "__job:alpha"}, nil, 404)
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
	got = h.call(t, []string{"info", "__job:alpha"}, nil, 200)
	assert.Contains(t, string(got), `"id":"source-3"`)
	assert.NotContains(t, string(got), `"id":"source-1"`)
}

func TestStructuredInfoPreservesDataValidation(t *testing.T) {
	var paramsCalls, dataCalls atomic.Int64
	h := newFunctionInfoHarness(
		t,
		funcapi.FunctionConfig{
			ID: "events",
		},
		"shared",
		func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &structuredInfoHandler{
				t: t,
				params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
					paramsCalls.Add(1)
					return []funcapi.ParamConfig{
						{
							ID:      "__sort",
							Name:    "Sort",
							Options: []funcapi.ParamOption{{ID: job.Name(), Name: "Available"}},
						},
					}, nil
				},
				handle: func(_ context.Context, _ string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
					dataCalls.Add(1)
					return &funcapi.FunctionResponse{
						Data: [][]any{{job.Name(), params.Column("__sort")}},
					}
				},
			}
		},
	)
	// The existing UI requests unscoped info, then sends selectors on data calls.
	h.call(t, []string{"info"}, nil, 200)
	assert.Zero(t, paramsCalls.Load())
	got := h.call(t, []string{"__job:alpha", "__sort:alpha"}, nil, 200)
	assert.Contains(t, string(got), `"data":[["alpha","alpha"]]`)
	got = h.call(t, []string{"__job:alpha", "__sort:stale"}, nil, 400)
	assert.Contains(t, string(got), "not supported by job")
	got = h.call(t, []string{"__job:alpha", "__sort:stale", "__sort:alpha"}, nil, 400)
	assert.Contains(t, string(got), "single value")
	assert.EqualValues(t, 3, paramsCalls.Load())
	assert.EqualValues(t, 1, dataCalls.Load())
}

func TestStructuredInfoMySQLColumnsByJob(t *testing.T) {
	handlers := make(map[string]funcapi.MethodHandler)
	for _, job := range []string{"alpha", "beta rack,1"} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectQuery("SELECT @@performance_schema").
			WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow("ON"))
		columns := sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("COUNT_STAR").AddRow("SUM_TIMER_WAIT")
		if job == "alpha" {
			columns.AddRow("SUM_CPU_TIME")
		}
		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = \\? AND TABLE_NAME = \\?").
			WithArgs("performance_schema", "events_statements_summary_by_digest").
			WillReturnRows(columns)
		t.Cleanup(func() { assert.NoError(t, mock.ExpectationsWereMet()) })
		handlers[job] = mysqlfunc.NewRouter(mysqlInfoDeps{
			db: db,
		}, logger.New(), mysqlfunc.FunctionsConfig{})
	}
	var method funcapi.FunctionConfig
	for _, candidate := range mysqlfunc.Methods() {
		if candidate.ID == "top-queries" {
			method = candidate
		}
	}
	require.NotEmpty(t, method.ID)
	h := newFunctionInfoHarness(t, method, "shared", func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
		return handlers[job.Name()]
	})
	bootstrap := h.call(t, []string{"info"}, nil, 200)
	assert.Contains(t, string(bootstrap), `"id":"cpuTime"`, "old clients retain static declarations")
	for _, job := range []string{"alpha", "beta rack,1"} {
		payload, err := json.Marshal(
			map[string]any{"selections": map[string][]string{"__job": {job}, "__sort": {"cpuTime"}}},
		)
		require.NoError(t, err)
		got := h.call(t, []string{"info"}, payload, 200)
		var response struct {
			Params []struct {
				ID      string `json:"id"`
				Options []struct {
					ID string `json:"id"`
				} `json:"options"`
			} `json:"required_params"`
		}
		require.NoError(t, json.Unmarshal(got, &response))
		var sorts []string
		for _, param := range response.Params {
			if param.ID == "__sort" {
				for _, option := range param.Options {
					sorts = append(sorts, option.ID)
				}
			}
		}
		want := []string{"calls", "totalTime"}
		if job == "alpha" {
			want = append(want, "cpuTime")
		}
		assert.Equal(t, want, sorts, job)
		assertFunctionInfoSchema(t, got, "info_response")
	}
}

type structuredInfoHandler struct {
	t      *testing.T
	params func(context.Context, string) ([]funcapi.ParamConfig, error)
	handle func(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse
}

func (h *structuredInfoHandler) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if h.params == nil {
		h.t.Error("static info must not call MethodParams")
		return nil, nil
	}
	return h.params(ctx, method)
}

func (h *structuredInfoHandler) Handle(
	ctx context.Context,
	method string,
	params funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	if h.handle == nil {
		h.t.Error("info must not call the data handler")
		return funcapi.InternalErrorResponse("unexpected data call")
	}
	return h.handle(ctx, method, params)
}

func (*structuredInfoHandler) Cleanup(context.Context) {}

type mysqlInfoDeps struct{ db *sql.DB }

func (d mysqlInfoDeps) DB() (mysqlfunc.Queryer, error) { return d.db, nil }
