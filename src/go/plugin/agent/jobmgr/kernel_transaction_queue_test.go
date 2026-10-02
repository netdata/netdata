// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKernelQueuesTransactionWhileCurrentAuthorityIsInActiveApply(t *testing.T) {
	for _, disposition := range []string{"unchanged", "replaced", "removed"} {
		t.Run(disposition, func(t *testing.T) {
			kernel, run, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
			require.NoError(t, kernel.Start(context.Background()))
			require.NoError(t, run.OpenAdmission())
			t.Cleanup(func() {
				kernel.Stop()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, kernel.Wait(ctx))
				closeUIDLedger(t, uids)
			})
			request := func(uid string) Request {
				return Request{UID: uid, LaneKey: "resource", Source: lifecycle.SourceJobManager, Route: "internal/test"}
			}
			current := newKernelTestReadyResource("resource", nil, nil)
			permitPlan := lifecycle.NewJobLongLivedPlan()
			require.NoError(t, kernel.SubmitPreparedAndWait(t.Context(), request("install"), kernelTestInstallPlan("resource", permitPlan, current)))
			var expected lifecycle.ReadyResource = current
			plan := queuedUnchangedPlan(nil)
			var events []string
			switch disposition {
			case "replaced":
				successor := newKernelTestReadyResource("resource", nil, nil)
				var err error
				plan, err = (kernelTestTransactionPlanner{permitPlan: permitPlan, current: current, successor: successor, events: &events}).Plan(Request{LaneKey: "resource", Route: "replace"})
				require.NoError(t, err)
				plan.NoResponse = true
				expected = successor
			case "removed":
				plan = kernelTestRemovePlan("resource")
				expected = nil
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			prepare := plan.Transaction.Prepare
			plan.Transaction.Prepare = func(ctx context.Context, resource lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				transaction, err := prepare(ctx, resource, scope, permit)
				if err != nil {
					return nil, err
				}
				return &queuedGatedTransaction{PreparedResourceTransaction: transaction, entered: entered, release: release}, nil
			}
			activeDone := make(chan error, 1)
			go func() { activeDone <- kernel.SubmitPreparedAndWait(t.Context(), request("active"), plan) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("active transaction did not enter Apply")
			}
			observed := make(chan queuedCurrentObservation, 1)
			require.NoError(t, kernel.SubmitPrepared(t.Context(), request("callback"), queuedUnchangedPlan(observed)))
			select {
			case <-observed:
				t.Fatal("queued transaction prepared before active Apply settled")
			default:
			}
			close(release)
			require.NoError(t, <-activeDone)
			require.NoError(t, kernel.SubmitPreparedAndWait(t.Context(), request("barrier"), queuedUnchangedPlan(nil)))
			got := <-observed
			if expected == nil {
				require.Nil(t, got.resource)
				require.False(t, got.identity.Valid())
			} else {
				require.Same(t, expected, got.resource)
				require.Equal(t, expected.Identity(), got.identity)
			}
			require.NoError(t, run.DirtyCause())
		})
	}
}

type queuedCurrentObservation struct {
	resource lifecycle.ReadyResource
	identity lifecycle.ResourceIdentity
}

func queuedUnchangedPlan(observed chan<- queuedCurrentObservation) WorkPlan {
	return WorkPlan{NoResponse: true, Transaction: &ResourceTransactionPlan{
		ID: "resource",
		Prepare: func(_ context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, _ lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
			if observed != nil {
				observed <- queuedCurrentObservation{resource: current, identity: scope.Current}
			}
			return &queuedUnchangedTransaction{scope: scope, current: current}, nil
		},
	}}
}

type queuedUnchangedTransaction struct {
	scope   lifecycle.ResourceTransactionScope
	current lifecycle.ReadyResource
}

func (transaction *queuedUnchangedTransaction) Scope() lifecycle.ResourceTransactionScope {
	return transaction.scope
}
func (transaction *queuedUnchangedTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	return transaction.current, nil
}
func (transaction *queuedUnchangedTransaction) Apply(context.Context) (lifecycle.AppliedResourceTransaction, error) {
	result, err := lifecycle.NewSealedResult(204, "text/plain", nil)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(transaction.scope, lifecycle.ResourceTransactionUnchanged, transaction.current, result, func() error { return nil })
}

type queuedGatedTransaction struct {
	lifecycle.PreparedResourceTransaction
	entered chan<- struct{}
	release <-chan struct{}
}

func (transaction *queuedGatedTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	close(transaction.entered)
	<-transaction.release
	return transaction.PreparedResourceTransaction.Apply(ctx)
}
