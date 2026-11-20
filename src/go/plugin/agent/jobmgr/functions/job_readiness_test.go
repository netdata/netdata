// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
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

type testJobReadiness struct{ ready atomic.Bool }

func (r *testJobReadiness) FunctionAvailable(id string) bool {
	return id == "always" || r.ready.Load()
}

func TestJobFunctionReadinessPublication(t *testing.T) {
	controller, catalog, err := newContainedControllerTest(t, 1, collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "shared", Aliases: []string{"alias"}}, {ID: "always"}}
			},
			InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: job.Name()}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &controllerTestHandler{} },
		},
	})
	require.NoError(t, err)
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&controllerTestMutationPort{catalog: catalog}, publication))
	require.NoError(t, controller.Activate())

	start := func(name string) *testJobReadiness {
		t.Helper()
		r := &testJobReadiness{}
		job := &controllerTestJob{fullName: "module_" + name, module: "module", name: name, running: true, collector: r}
		handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
		require.NoError(t, err)
		require.NoError(t, handle.Publish())
		t.Cleanup(func() { require.NoError(t, handle.CloseAndDrain(context.Background())) })
		return r
	}
	info := &functionInfoHarness{catalog: catalog, route: "alias"}
	first := start("first")
	second := start("second")
	absent := func(route string) {
		t.Helper()
		result, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "probe", Route: route})
		require.NoError(t, err)
		require.NotZero(t, result.Rejected)
	}

	require.Contains(t, port.eventsSnapshot(), "publish:module:always")
	for _, route := range []string{"alias", "module:shared", "module:first", "module:second"} {
		require.NotContains(t, port.eventsSnapshot(), "publish:"+route)
		absent(route)
	}

	// Each job's instance Function witnesses its own poll committing, even
	// when changing the job selector leaves the shared route published.
	transition := func(r *testJobReadiness, ready bool, want ...string) {
		t.Helper()
		before := len(port.eventsSnapshot())
		r.ready.Store(ready)
		require.EventuallyWithT(t, func(check *assert.CollectT) {
			require.NoError(check, controller.ReconcileModule(t.Context(), "module"))
			require.GreaterOrEqual(check, len(port.eventsSnapshot()), before+len(want))
		}, time.Second, time.Millisecond)
		require.ElementsMatch(t, want, port.eventsSnapshot()[before:])
	}
	transition(first, true, "publish:alias", "publish:module:shared", "publish:module:first")
	transition(second, true, "publish:module:second")
	transition(first, false, "withdraw:module:first")
	metadata := string(info.call(t, []string{"info"}, nil, 200))
	require.Contains(t, metadata, `"id":"second"`)
	require.NotContains(t, metadata, `"id":"first"`)
	transition(second, false, "withdraw:alias", "withdraw:module:shared", "withdraw:module:second")
	absent("module:shared")
	absent("alias")
	transition(first, true, "publish:alias", "publish:module:shared", "publish:module:first")
}

func TestUnavailableJobFunctionReservesItsNames(t *testing.T) {
	controller, catalog, err := newContainedControllerTest(t, 1, collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "shared", Aliases: []string{"module:instance"}}}
			},
			InstanceFunctions: func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "instance"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &controllerTestHandler{} },
		},
	})
	require.NoError(t, err)
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&controllerTestMutationPort{catalog: catalog}, publication))
	require.NoError(t, controller.Activate())
	job := &controllerTestJob{fullName: "module_job", module: "module", name: "job", running: true, collector: &testJobReadiness{}}
	handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handle.CloseAndDrain(context.Background())) })
	require.ErrorContains(t, handle.Publish(), "public name")
	require.Empty(t, port.eventsSnapshot())
}
