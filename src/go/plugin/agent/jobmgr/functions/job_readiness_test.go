// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"slices"
	"sync"
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

	require.Empty(t, port.eventsSnapshot())
	require.NoError(t, controller.ReconcileModule(t.Context(), "module"))
	require.Eventually(t, func() bool {
		return slices.Contains(port.eventsSnapshot(), "publish:module:always")
	}, time.Second, time.Millisecond)
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

// Count observations and hold one job in its callback after staging.
type passReadiness struct {
	ready atomic.Bool
	polls atomic.Int32
	wait  chan struct{}
}

func (r *passReadiness) FunctionAvailable(string) bool {
	r.polls.Add(1)
	if r.wait != nil {
		<-r.wait
	}
	return r.ready.Load()
}

func TestModuleReadinessPassCannotBeOvertaken(t *testing.T) {
	controller, _, port := newReadinessController(t, collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "shared"}} },
	})
	start := func(name string) *passReadiness {
		t.Helper()
		r := &passReadiness{}
		job := &controllerTestJob{fullName: "module_" + name, module: "module", name: name, running: true, collector: r}
		handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, handle.CloseAndDrain(context.Background())) })
		require.NoError(t, handle.Publish())
		return r
	}
	fast, slow := start("fast"), start("slow")
	slow.wait = make(chan struct{})
	t.Cleanup(func() { close(slow.wait) })
	fast.ready.Store(true)
	require.NoError(t, controller.ReconcileModule(t.Context(), "module"))
	identity := func(name string) jobmgr.ProcessAttemptIdentity {
		return jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionPoll,
			Key:       jobFunctionAttemptKey(1, "module_"+name),
			Resource:  candidateFunctionResource("module_" + name),
		}
	}
	waitForFast := func() {
		t.Helper()
		if done, ok := controller.attempts.ProcessAttemptReleased(identity("fast")); ok {
			waitBundleContainmentTestValue(t, done, "fast worker release")
		}
	}
	waitForFast()
	require.EqualValues(t, 1, fast.polls.Load())
	fast.ready.Store(false)
	require.NoError(t, controller.ReconcileModule(t.Context(), "module"))
	waitForFast()
	require.EqualValues(t, 1, fast.polls.Load())
	// Logical settlement releases the pass even while slow's physical callback
	// remains blocked. The next tick can still withdraw fast's last readiness.
	require.True(t, controller.attempts.CutProcessAttempt(identity("slow"), jobmgr.ErrProcessAttemptDeadline))
	require.Eventually(t, func() bool { return len(port.eventsSnapshot()) == 1 }, time.Second, time.Millisecond)
	require.NoError(t, controller.ReconcileModule(t.Context(), "module"))
	require.Eventually(t, func() bool { return len(port.eventsSnapshot()) == 2 }, time.Second, time.Millisecond)
	require.Equal(t, []string{"publish:module:shared", "withdraw:module:shared"}, port.eventsSnapshot())
}

func TestExitedJobCannotPublishFromPendingReadinessPass(t *testing.T) {
	controller, _, port := newReadinessController(t, collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "exiting", Aliases: []string{"alias"}}, {ID: "sibling"}}
		},
	})
	r := &passReadiness{wait: make(chan struct{})}
	r.ready.Store(true)
	job := &controllerTestJob{fullName: "module_exiting", module: "module", name: "exiting", running: true, collector: r}
	handle, err := prepareControllerTestJob(t, controller, lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}, job)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handle.CloseAndDrain(context.Background())) })
	release := sync.OnceFunc(func() { close(r.wait) })
	t.Cleanup(release)
	require.NoError(t, handle.Publish())
	sibling, siblingHandle := prepareReadinessJob(t, controller, "sibling")
	require.NoError(t, siblingHandle.Publish())
	sibling.ready.Store(true)

	require.NoError(t, controller.ReconcileModule(t.Context(), "module"))
	require.Eventually(t, func() bool { return r.polls.Load() != 0 }, time.Second, time.Millisecond)
	// A completed run clears IsRunning before its queued failure command detaches the handle.
	job.running = false
	release()
	require.Eventually(t, func() bool {
		return slices.Contains(port.eventsSnapshot(), "publish:module:sibling")
	}, time.Second, time.Millisecond)
	require.Equal(t, []string{"publish:module:sibling"}, port.eventsSnapshot())
}
