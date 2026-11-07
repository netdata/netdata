// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/ownedjobs"
	"github.com/stretchr/testify/require"
)

func TestOwnedJobsDisableRevokesFunctionsBeforeNativeLaneDrains(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	service := ownerTestService{registries: make(chan ownedjobs.Registry, 1)}
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}}
	jobs.OwnerOnlyModules = map[string]bool{"module": true}
	jobs.OwnedJobServices = []ownedjobs.Service{service}
	invocationEntered := make(chan context.Context, 1)
	invocationRelease, laneRelease := make(chan struct{}), make(chan struct{})
	var invocationOnce, laneOnce sync.Once
	var oldCalls, siblingCalls, oldCleanups atomic.Int32
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation: 1, ShutdownTimeout: 5 * time.Second, UIDs: uids, Frames: frames,
		Modules: collectorapi.Registry{"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
				collector.CleanupFunc = func(context.Context) {
					if collector.Config.OptionStr == "old-child" {
						oldCleanups.Add(1)
					}
				}
				return collector
			},
			Config: func() any { return &collectorapi.MockConfiguration{} },
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "read", RawRequest: true}}
			},
			MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &processFunctionTestHandler{handle: func(ctx context.Context, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					if job.Name() == "job-000" {
						oldCalls.Add(1)
						invocationEntered <- ctx
						<-invocationRelease
					} else {
						siblingCalls.Add(1)
					}
					return &funcapi.FunctionResponse{Status: 200}
				}}
			},
		}},
		Jobs: jobs, Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		invocationOnce.Do(func() { close(invocationRelease) })
		laneOnce.Do(func() { close(laneRelease) })
		generation.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, generation.Wait(ctx))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	var registry ownedjobs.Registry
	select {
	case registry = <-service.registries:
	case <-time.After(time.Second):
		t.Fatal("parent service did not bind")
	}
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	children[0].Config.Set("option_str", "old-child")
	applyOwned(t, owner, 1, children)
	before := waitOwnedRunning(t, owner, 2)
	output.waitContains(t, `FUNCTION GLOBAL "module:read"`)
	submit := func(uid, name string) {
		t.Helper()
		require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
			UID: uid, Source: lifecycle.SourceFunction, Route: "module:read",
			Args: []string{"__job:" + name},
		}))
	}
	waitResult := func(uid string, code int) {
		t.Helper()
		output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", uid, code))
	}
	submit("sibling-before", "job-001")
	waitResult("sibling-before", 200)
	submit("old-admitted", "job-000")
	var admittedContext context.Context
	select {
	case admittedContext = <-invocationEntered:
	case <-time.After(time.Second):
		t.Fatal("native job-backed Function was not admitted")
	}

	// Hold only this child's native lane. The graph claim is released so parent
	// adoption can revoke Function admission before detach or collector cleanup.
	laneEntered := make(chan struct{})
	id := children[0].Config.FullName()
	require.NoError(t, generation.kernel.SubmitPrepared(t.Context(), jobmgr.Request{
		UID: "owned-function-held-lane", LaneKey: id, Source: lifecycle.SourceJobManager, Route: "test/held-lane",
	}, jobmgr.WorkPlan{
		Claims: []string{joboutput.DynCfgJobGraphClaim}, NoResponse: true,
		YieldClaimOnPrepare: joboutput.DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{ID: id, Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
			workErr, claimErr := jobmgr.RunWithoutClaims(ctx, func(ctx context.Context) error {
				close(laneEntered)
				select {
				case <-laneRelease:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			if err := errors.Join(workErr, claimErr); err != nil {
				return nil, err
			}
			result, err := lifecycle.NewSealedResult(204, "application/json", nil)
			if err != nil {
				return nil, err
			}
			return joboutput.PrepareNoopResourceTransaction(scope, current, permit, result, func() error { return nil }, nil)
		}},
	}))
	select {
	case <-laneEntered:
	case <-time.After(time.Second):
		t.Fatal("native child lane did not yield its graph claim")
	}
	children[0].Enabled = false
	applyOwned(t, owner, 2, children)
	submit("old-after-disable", "job-000")
	waitResult("old-after-disable", 404)
	submit("sibling-after", "job-001")
	waitResult("sibling-after", 200)
	require.EqualValues(t, 1, oldCalls.Load(), "revoked Function must not enter the old handler")
	require.EqualValues(t, 2, siblingCalls.Load())
	require.Zero(t, oldCleanups.Load(), "rejection must precede physical cleanup")
	require.NoError(t, admittedContext.Err(), "logical revocation lets admitted Function work drain")
	require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN old-admitted ")
	after := owner.Status()
	require.Equal(t, before[1].Desired, after[1].Desired)
	require.Equal(t, before[1].Generation, after[1].Generation)
	invocationOnce.Do(func() { close(invocationRelease) })
	waitResult("old-admitted", 200)
	laneOnce.Do(func() { close(laneRelease) })
	require.Eventually(t, func() bool {
		states := owner.Status()
		return states[0].ConfigStatus == "disabled" && states[0].Applied == states[0].Desired && oldCleanups.Load() == 1
	}, 3*time.Second, time.Millisecond)
	require.NoError(t, generation.run.DirtyCause())
}
