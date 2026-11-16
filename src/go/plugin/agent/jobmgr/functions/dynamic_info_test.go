// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
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

func dynamicInfoTestMethod() funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:          "events",
		DynamicInfo: true,
		RequiredParams: []funcapi.ParamConfig{
			{ID: "format", Name: "Format", Options: []funcapi.ParamOption{{ID: "text", Name: "Text"}}},
			{ID: "source", Name: "Source", Options: []funcapi.ParamOption{{ID: "static", Name: "Static"}}},
		},
	}
}

func TestDynamicInfoBoundMetadata(t *testing.T) {
	for _, mode := range []string{"agent", "instance", "single"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), mode,
				func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &structuredInfoHandler{t: t, params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
						calls.Add(1)
						return []funcapi.ParamConfig{
							{ID: "source", Name: "Source", Options: []funcapi.ParamOption{{ID: "cached", Name: "Cached"}}},
						}, nil
					}}
				})
			got := h.call(t, []string{"info"}, []byte(`{"selections":{"source":["stale"]}}`), 200)
			assert.EqualValues(t, 1, calls.Load())
			assert.Contains(t, string(got), `"id":"cached"`)
			assert.Contains(t, string(got), `"id":"text"`)
			assert.NotContains(t, string(got), `"id":"static"`)
			assert.NotContains(t, string(got), `"__job"`)
			assert.NotContains(t, string(got), `"data"`)
			assertFunctionInfoSchema(t, got, "info_response")
		})
	}
}

func TestDynamicInfoPreservesUnselectedSharedBootstrap(t *testing.T) {
	var calls atomic.Int64
	h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), "shared",
		func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &structuredInfoHandler{t: t, params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
				calls.Add(1)
				return nil, nil
			}}
		})
	bootstrap := h.call(t, []string{"info"}, nil, 200)
	assert.Zero(t, calls.Load())
	assert.Contains(t, string(bootstrap), `"id":"static"`)
	assert.Contains(t, string(bootstrap), `"id":"__job"`)
	scoped := h.call(t, []string{"info", "__job:alpha"}, nil, 200)
	assert.EqualValues(t, 1, calls.Load())
	assert.JSONEq(t, string(bootstrap), string(scoped))
}

func TestDynamicInfoBoundUnavailableBootstrap(t *testing.T) {
	for _, mode := range []string{"instance", "single"} {
		for _, unavailable := range []string{"stopped", "unavailable"} {
			t.Run(mode+"/"+unavailable, func(t *testing.T) {
				var job *controllerTestJob
				h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), mode,
					func(runtime collectorapi.RuntimeJob) funcapi.MethodHandler {
						job = runtime.(*controllerTestJob)
						return &structuredInfoHandler{t: t}
					})
				// The published route can outlive a stopped job until reconciliation.
				if unavailable == "stopped" {
					job.running = false
				} else {
					h.handles[job.Name()].bundle.commitAvailability(map[string]bool{"events": false})
				}
				got := h.call(t, []string{"info"}, nil, 200)
				assert.Contains(t, string(got), `"id":"static"`)
				assertFunctionInfoSchema(t, got, "info_response")
				h.call(t, nil, nil, 404)
			})
		}
	}
}

func TestDynamicInfoBoundJobReload(t *testing.T) {
	var construction int
	h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), "single",
		func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			construction++
			option := "old"
			if construction > 1 {
				option = "new"
			}
			return &structuredInfoHandler{t: t, params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
				return []funcapi.ParamConfig{
					{ID: "source", Name: "Source", Options: []funcapi.ParamOption{{ID: option, Name: option}}},
				}, nil
			}}
		})
	got := h.call(t, []string{"info"}, nil, 200)
	assert.Contains(t, string(got), `"id":"old"`)
	require.NoError(t, h.handles["module"].CloseAndDrain(t.Context()))
	job := &controllerTestJob{fullName: "module_module", module: "module", name: "module", running: true}
	handle, err := prepareControllerTestJob(t, h.controller,
		lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 2}, job)
	require.NoError(t, err)
	require.NoError(t, handle.Publish())
	got = h.call(t, []string{"info"}, nil, 200)
	assert.Contains(t, string(got), `"id":"new"`)
	assert.NotContains(t, string(got), `"id":"old"`)
}

func TestDynamicInfoBoundErrors(t *testing.T) {
	for name, failure := range map[string]struct {
		message string
		params  func(collectorapi.RuntimeJob) error
	}{
		"error": {message: "metadata unavailable", params: func(collectorapi.RuntimeJob) error {
			return errors.New("metadata unavailable")
		}},
		"panic": {message: "Function handler is unavailable", params: func(collectorapi.RuntimeJob) error {
			panic("metadata failed")
		}},
		"stop": {message: "stopped during request", params: func(job collectorapi.RuntimeJob) error {
			job.(*controllerTestJob).running = false
			return nil
		}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), "single",
				func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &structuredInfoHandler{t: t, params: func(context.Context, string) ([]funcapi.ParamConfig, error) {
						return nil, failure.params(job)
					}}
				})
			got := h.call(t, []string{"info"}, nil, 503)
			assert.Contains(t, string(got), failure.message)
		})
	}
}

func TestDynamicInfoBoundCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := make(chan struct{})
	h := newFunctionInfoHarness(t, dynamicInfoTestMethod(), "single",
		func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &structuredInfoHandler{t: t, params: func(callCtx context.Context, _ string) ([]funcapi.ParamConfig, error) {
				cancel()
				<-callCtx.Done()
				close(observed)
				return nil, callCtx.Err()
			}}
		})
	decision, err := h.catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "cancel-bound-info", Route: h.route, Args: []string{"info"},
	})
	require.NoError(t, err)
	require.Zero(t, decision.Rejected)
	_, err = decision.Plan.Work(ctx)
	require.NoError(t, err)
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("bound metadata discovery did not receive cancellation")
	}
	cleanup, err := h.catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())
}
