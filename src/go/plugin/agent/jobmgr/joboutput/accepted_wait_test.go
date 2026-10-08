// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

type activationTestSubmission struct {
	request jobmgr.Request
	plan    jobmgr.WorkPlan
	ack     chan error
}

type activationTestCommands struct {
	queue chan activationTestSubmission
	stop  chan struct{}
}

func (c *activationTestCommands) SubmitPrepared(ctx context.Context, request jobmgr.Request, plan jobmgr.WorkPlan) error {
	return c.submit(ctx, activationTestSubmission{request: request, plan: plan})
}
func (c *activationTestCommands) SubmitPreparedAndWait(ctx context.Context, request jobmgr.Request, plan jobmgr.WorkPlan) error {
	call := activationTestSubmission{request: request, plan: plan, ack: make(chan error, 1)}
	if err := c.submit(ctx, call); err != nil {
		return err
	}
	select {
	case err := <-call.ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stop:
		return &lifecycle.StoppingRejection{Generation: 9}
	}
}
func (c *activationTestCommands) submit(ctx context.Context, call activationTestSubmission) error {
	select {
	case c.queue <- call:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stop:
		return &lifecycle.StoppingRejection{Generation: 9}
	}
}
func (c *activationTestCommands) next(t *testing.T, route string) activationTestSubmission {
	t.Helper()
	select {
	case call := <-c.queue:
		require.Equal(t, route, call.request.Route)
		return call
	case <-time.After(time.Second):
		t.Fatalf("missing activation command %s", route)
		return activationTestSubmission{}
	}
}
func applyActivationTestSubmission(t *testing.T, call activationTestSubmission, current lifecycle.ReadyResource, generation uint64) lifecycle.ReadyResource {
	t.Helper()
	scope := lifecycle.ResourceTransactionScope{ID: call.plan.Transaction.ID}
	var permit lifecycle.LongLivedPermit
	if current != nil {
		scope.Current = current.Identity()
	}
	if call.plan.Transaction.AllocateSuccessor {
		scope.Successor = lifecycle.ResourceIdentity{ID: scope.ID, Generation: generation}
		permit, _ = issueTestJobPermit(t, scope.ID, generation)
	}
	prepared, err := call.plan.Transaction.Prepare(t.Context(), current, scope, permit)
	require.NoError(t, err)
	applied, err := prepared.Apply(t.Context())
	require.NoError(t, err)
	if call.ack != nil {
		call.ack <- nil
	}
	_, _, next := applied.Ownership()
	return next
}

func TestAcceptedActivationWaitsForVNodeWithoutOperationalRetry(t *testing.T) {
	for _, disable := range []bool{false, true} {
		t.Run(map[bool]string{false: "dependency arrives", true: "disabled before arrival"}[disable], func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() }}
			})
			var available atomic.Bool
			controller.factory.config.Vnode = func(name string) (jobruntime.VnodeSnapshot, bool) {
				if !available.Load() {
					return jobruntime.VnodeSnapshot{}, false
				}
				return jobruntime.VnodeSnapshot{Vnode: &vnodes.VirtualNode{Name: name, Hostname: "remote", GUID: "c61f5634-03c7-4b36-86ed-fd61000cd4d8"}, Revision: 1, MetadataRevision: 1}, true
			}
			commands := &activationTestCommands{queue: make(chan activationTestSubmission, 8), stop: make(chan struct{})}
			require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("background failure: %v", err) }))
			var current lifecycle.ReadyResource
			t.Cleanup(func() {
				close(commands.stop)
				controller.scheduler.StopBackgroundWorkers()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
				stopRuntimeTestResource(t, current)
			})
			config := acceptedActivationTestConfig(confgroup.TypeUser).Set("vnode", "remote").Set("autodetection_retry", 0)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)
			first := commands.next(t, "internal/jobs/accepted-activation")
			// Arrival between failed lookup and terminal acknowledgement must survive.
			if !disable {
				available.Store(true)
				controller.NotifyDependencyChanged("vnode", "remote")
			}
			current = applyActivationTestSubmission(t, first, nil, 2)
			require.Nil(t, current)
			record, _ := graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			require.False(t, runtimeTestHasRetry(controller, config.FullName()))
			if disable {
				prepared, err := controller.Prepare(t.Context(), DynCfgJobRequest{Args: []string{controller.externalID(config.FullName()), "disable"}}, nil, lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				_, err = prepared.Apply(t.Context())
				require.NoError(t, err)
				available.Store(true)
				controller.NotifyDependencyChanged("vnode", "remote")
				select {
				case call := <-commands.queue:
					t.Fatalf("disabled job resumed through %s", call.request.Route)
				case <-time.After(30 * time.Millisecond):
				}
				record, _ = graph.Lookup(config.FullName())
				require.Equal(t, dyncfg.StatusDisabled.String(), record.Status)
				require.False(t, controller.ActivationEnabled(config.FullName()))
				return
			}
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
			require.NotNil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
			record, _ = graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
		})
	}
}
