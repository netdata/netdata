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

type testJobReadiness struct {
	name  string
	ready atomic.Bool
}

func (r *testJobReadiness) FunctionAvailable(id string) bool {
	return id == "always" || r.ready.Load() && (id == "shared" || id == r.name)
}

func newReadinessController(t *testing.T, creator collectorapi.Creator) (*Controller, *Catalog, *recordingPublicationPort) {
	t.Helper()
	creator.MethodHandler = func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &controllerTestHandler{} }
	modules := collectorapi.Registry{}
	modules.Register("module", creator)
	controller, catalog, err := newContainedControllerTest(t, 1, modules)
	require.NoError(t, err)
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&controllerTestMutationPort{catalog: catalog}, publication))
	require.NoError(t, controller.Activate())
	return controller, catalog, port
}

func prepareReadinessJob(t *testing.T, controller *Controller, name string) (*testJobReadiness, *JobHandle) {
	t.Helper()
	r := &testJobReadiness{name: name}
	job := &controllerTestJob{fullName: "module_" + name, module: "module", name: name, running: true, collector: r}
	handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handle.CloseAndDrain(context.Background())) })
	return r, handle
}

func TestSharedFunctionReadinessPublication(t *testing.T) {
	controller, catalog, port := newReadinessController(t, collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "shared", Aliases: []string{"alias"}}, {ID: "always"}, {ID: "first"}, {ID: "second"}}
		},
	})
	start := func(name string) *testJobReadiness {
		t.Helper()
		r, handle := prepareReadinessJob(t, controller, name)
		require.NoError(t, handle.Publish())
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

	// Each job also serves a distinct method. Its publication or withdrawal
	// confirms the poll committed when only the shared selector changes.
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
	controller, _, port := newReadinessController(t, collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "shared", Aliases: []string{"module:other"}}, {ID: "other"}}
		},
	})
	_, handle := prepareReadinessJob(t, controller, "job")
	require.ErrorContains(t, handle.Publish(), "public name")
	require.Empty(t, port.eventsSnapshot())
}

func TestInstanceFunctionReadinessPublication(t *testing.T) {
	controller, _, port := newReadinessController(t, collectorapi.Creator{
		InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: job.Name()}}
		},
	})
	first, firstHandle := prepareReadinessJob(t, controller, "first")
	_, secondHandle := prepareReadinessJob(t, controller, "second")
	require.NoError(t, firstHandle.Publish())
	require.NoError(t, secondHandle.Publish())
	require.Empty(t, port.eventsSnapshot())
	for i, ready := range []bool{true, false} {
		first.ready.Store(ready)
		require.EventuallyWithT(t, func(check *assert.CollectT) {
			require.NoError(check, controller.ReconcileModule(t.Context(), "module"))
			require.Len(check, port.eventsSnapshot(), i+1)
		}, time.Second, time.Millisecond)
	}
	require.Equal(t, []string{"publish:module:first", "withdraw:module:first"}, port.eventsSnapshot())
}

func TestOlderPollCannotRepublishWithdrawnFunction(t *testing.T) {
	controller, _, port := newReadinessController(t, collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "shared"}} },
	})
	r, handle := prepareReadinessJob(t, controller, "job")
	require.NoError(t, handle.Publish())
	r.ready.Store(true)
	poll := func() functionAvailabilityPoll {
		t.Helper()
		p, err := handle.bundle.startAvailabilityPoll()
		require.NoError(t, err)
		waitBundleContainmentTestValue(t, p.attempt.Released(), "availability poll release")
		return p
	}
	owner := functionOwner{name: "module"}
	controller.finishAvailabilityPoll(owner, poll())
	// Containment releases the worker identity before the controller applies its result.
	older := poll()
	r.ready.Store(false)
	controller.finishAvailabilityPoll(owner, poll())
	controller.finishAvailabilityPoll(owner, older)

	// A newer observation of the same state must also supersede an older result.
	r.ready.Store(true)
	older = poll()
	r.ready.Store(false)
	controller.finishAvailabilityPoll(owner, poll())
	controller.finishAvailabilityPoll(owner, older)
	require.Equal(t, []string{"publish:module:shared", "withdraw:module:shared"}, port.eventsSnapshot())
}
