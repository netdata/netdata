// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestTransactionProbeYieldsClaimsUntilPreparationResumes(t *testing.T) {
	kernel, run := newKernel(t)
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	probeDone := make(chan error, 1)

	probePlan := WorkPlan{
		Claims:              []string{"graph"},
		NoResponse:          true,
		YieldClaimOnPrepare: "graph",
		Transaction: &ResourceTransactionPlan{
			ID: "job-a",
			Prepare: func(
				ctx context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				workErr, claimErr := RunWithoutClaims(ctx, func(context.Context) error {
					close(probeStarted)
					<-releaseProbe
					return nil
				})
				if err := errors.Join(workErr, claimErr); err != nil {
					return nil, err
				}
				return &simpleCompositeChildTransaction{scope: scope, permit: permit}, nil
			},
		},
	}
	go func() {
		probeDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "probe",
				LaneKey: "job-a",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/probe",
			},
			probePlan,
		)
	}()
	select {
	case <-probeStarted:
	case err := <-probeDone:
		require.NoError(t, err)
		require.FailNow(t, "probe transaction completed before probing")
	case <-time.After(time.Second):
		require.FailNow(t, "probe did not start")
	}

	sameLaneApplied := make(chan struct{})
	sameLaneDone := make(chan error, 1)
	go func() {
		sameLaneDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "same-lane",
				LaneKey: "job-a",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/same-lane",
			},
			compositeTestPlan("job-a", []string{"graph"}, func() {
				close(sameLaneApplied)
			}),
		)
	}()

	otherApplied := make(chan struct{})
	otherPlan := compositeTestPlan("job-b", []string{"graph"}, func() {
		close(otherApplied)
	})
	otherCtx, cancelOther := context.WithTimeout(context.Background(), time.Second)
	defer cancelOther()
	require.NoError(t, kernel.SubmitPreparedAndWait(
		otherCtx,
		Request{
			UID:     "other",
			LaneKey: "job-b",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/other",
		},
		otherPlan,
	))
	select {
	case <-otherApplied:
	default:
		require.FailNow(t, "other transaction did not apply while probe was blocked")
	}
	select {
	case <-sameLaneApplied:
		require.FailNow(t, "same-resource transaction overlapped the active probe")
	default:
	}

	close(releaseProbe)
	select {
	case err := <-probeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "probe transaction did not resume")
	}
	select {
	case err := <-sameLaneDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "same-resource transaction did not resume after the probe")
	}

	stopCompositeTestKernel(t, kernel)
}

func TestCompositeProbeYieldsJobGraphClaimButRetainsDependencyClaim(t *testing.T) {
	kernel, run := newKernel(t)
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	parentDone := make(chan error, 1)

	childPlan := WorkPlan{
		Claims:              []string{"graph"},
		NoResponse:          true,
		YieldClaimOnPrepare: "graph",
		Transaction: &ResourceTransactionPlan{
			ID: "job",
			Prepare: func(
				ctx context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				workErr, claimErr := RunWithoutClaims(ctx, func(context.Context) error {
					close(probeStarted)
					<-releaseProbe
					return nil
				})
				if err := errors.Join(workErr, claimErr); err != nil {
					return nil, err
				}
				return &simpleCompositeChildTransaction{scope: scope, permit: permit}, nil
			},
		},
	}
	parentPlan := compositeParentTestPlan(
		"secret",
		[]string{"dependency", "graph"},
		func(ctx context.Context, commands CompositeCommandScope) error {
			return commands.SubmitPreparedAndWait(
				ctx,
				Request{
					UID:     "child",
					LaneKey: "job",
					Source:  lifecycle.SourceJobManager,
					Route:   "internal/test/child",
				},
				childPlan,
			)
		},
	)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "parent",
				LaneKey: "secret",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/parent",
			},
			parentPlan,
		)
	}()
	select {
	case <-probeStarted:
	case err := <-parentDone:
		require.NoError(t, err)
		require.FailNow(t, "parent completed before child probing")
	case <-time.After(time.Second):
		require.FailNow(t, "child probe did not start")
	}

	graphCtx, cancelGraph := context.WithTimeout(context.Background(), time.Second)
	defer cancelGraph()
	require.NoError(t, kernel.SubmitPreparedAndWait(
		graphCtx,
		Request{
			UID:     "graph-only",
			LaneKey: "other-job",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/graph-only",
		},
		compositeTestPlan("other-job", []string{"graph"}, nil),
	))

	dependencyDone := make(chan error, 1)
	go func() {
		dependencyDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "dependency-only",
				LaneKey: "other-secret",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/dependency-only",
			},
			compositeTestPlan("other-secret", []string{"dependency"}, nil),
		)
	}()
	select {
	case err := <-dependencyDone:
		require.NoError(t, err)
		require.FailNow(t, "retained dependency claim was released during child probe")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseProbe)
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "parent did not resume after child probe")
	}
	select {
	case err := <-dependencyDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "dependency claim did not release after parent completion")
	}

	stopCompositeTestKernel(t, kernel)
}

func TestClaimYieldAcknowledgementSettlesAfterEnqueueCancellation(t *testing.T) {
	tests := map[string]struct {
		setup func(*testing.T, *testCommandKernel) (*commandOperation, *commandOperation)
	}{
		"direct owner": {
			setup: func(t *testing.T, kernel *testCommandKernel) (*commandOperation, *commandOperation) {
				t.Helper()
				operation := activeClaimYieldTestOperation(t, kernel, 1, "direct", []string{"graph"})
				operation.plan = WorkPlan{
					YieldClaimOnPrepare: "graph",
					Transaction:         &ResourceTransactionPlan{ID: "direct", Prepare: unusedClaimYieldPrepare},
				}
				return operation, operation
			},
		},
		"composite owner": {
			setup: func(t *testing.T, kernel *testCommandKernel) (*commandOperation, *commandOperation) {
				t.Helper()
				parent := activeClaimYieldTestOperation(t, kernel, 1, "parent", []string{"dependency", "graph"})
				parent.plan = WorkPlan{
					Transaction: &ResourceTransactionPlan{
						ID:               "parent",
						PrepareComposite: unusedCompositeClaimYieldPrepare,
					},
				}
				parent.composite = newKernelCompositeScope(kernel.CommandKernel, parent)
				require.NoError(t, kernel.beginCompositeFence(parent))

				generation, err := lifecycle.NewOperation(2, "child", lifecycle.SourceJobManager, "child", true)
				require.NoError(t, err)
				child := &commandOperation{
					OperationGeneration: generation,
					plan: WorkPlan{
						YieldClaimOnPrepare: "graph",
						Transaction:         &ResourceTransactionPlan{ID: "child", Prepare: unusedClaimYieldPrepare},
					},
					parent:          parent,
					claimsInherited: true,
				}
				child.lane = &commandLane{active: child}
				parent.activeChild = child
				kernel.operations[child.UID] = child
				return parent, child
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, _ := newKernel(t)
			owner, borrower := test.setup(t, kernel)
			ctx, cancel := context.WithCancel(context.Background())
			released := make(chan error, 1)
			go func() {
				released <- (kernelClaimYieldLease{
					kernel:    kernel.CommandKernel,
					operation: borrower,
				}).release(ctx)
			}()
			require.Eventually(t, func() bool {
				return len(kernel.claimYields) == 1
			}, time.Second, time.Millisecond)
			cancel()
			select {
			case err := <-released:
				require.NoError(t, err)
				require.FailNow(t, "yield release returned before its queued acknowledgement")
			case <-time.After(20 * time.Millisecond):
			}
			kernel.serviceClaimYields(1)
			require.NoError(t, <-released)
			require.True(t, owner.claimsYielded)

			reacquired := make(chan error, 1)
			go func() {
				reacquired <- (kernelClaimYieldLease{
					kernel:    kernel.CommandKernel,
					operation: borrower,
				}).reacquire(context.Background())
			}()
			require.Eventually(t, func() bool {
				return len(kernel.claimYields) == 1
			}, time.Second, time.Millisecond)
			kernel.serviceClaimYields(1)
			require.NoError(t, <-reacquired)
			require.True(t, owner.claimsHeld)
			require.False(t, owner.claimsYielded)
		})
	}
}

func TestRunWithoutClaimsReacquiresAfterPanic(t *testing.T) {
	lease := &claimYieldPanicTestLease{}
	ctx := withClaimYieldLease(context.Background(), lease)
	require.PanicsWithValue(t, "probe panic", func() {
		_, _ = RunWithoutClaims(ctx, func(context.Context) error {
			panic("probe panic")
		})
	})
	require.EqualValues(t, 1, lease.releases)
	require.EqualValues(t, 1, lease.reacquires)
}

func TestCompositeClaimWaitsForSameResourceProbeReacquisition(t *testing.T) {
	kernel, run := newKernel(t)
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)

	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	probeDone := make(chan error, 1)
	go func() {
		probeDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "probe",
				LaneKey: "job",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/probe",
			},
			WorkPlan{
				Claims:              []string{"graph"},
				NoResponse:          true,
				YieldClaimOnPrepare: "graph",
				Transaction: &ResourceTransactionPlan{
					ID: "job",
					Prepare: func(
						ctx context.Context,
						_ lifecycle.ReadyResource,
						scope lifecycle.ResourceTransactionScope,
						permit lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						workErr, claimErr := RunWithoutClaims(ctx, func(context.Context) error {
							close(probeStarted)
							<-releaseProbe
							return nil
						})
						if err := errors.Join(workErr, claimErr); err != nil {
							return nil, err
						}
						return &simpleCompositeChildTransaction{scope: scope, permit: permit}, nil
					},
				},
			},
		)
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "probe did not yield its graph claim")
	}

	parentEntered := make(chan struct{})
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "parent",
				LaneKey: "secret",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/secret-parent",
			},
			compositeParentTestPlan(
				"secret",
				[]string{"dependency", "graph"},
				func(ctx context.Context, commands CompositeCommandScope) error {
					close(parentEntered)
					return commands.SubmitPreparedAndWait(
						ctx,
						Request{
							UID:     "child",
							LaneKey: "job",
							Source:  lifecycle.SourceJobManager,
							Route:   "internal/test/secret-child",
						},
						compositeTestPlan("job", []string{"graph"}, nil),
					)
				},
			),
		)
	}()

	select {
	case <-parentEntered:
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseProbe)
	select {
	case err := <-probeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "probe could not reacquire the graph claim")
	}
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "composite did not settle after the probe")
	}

	stopCompositeTestKernel(t, kernel)
}

func TestUnrelatedCompositeSettlesWhileResourceProbeIsYielded(t *testing.T) {
	kernel, run := newKernel(t)
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)

	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	probeDone := make(chan error, 1)
	go func() {
		probeDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "probe",
				LaneKey: "job-a",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/probe",
			},
			WorkPlan{
				Claims:              []string{"graph"},
				NoResponse:          true,
				YieldClaimOnPrepare: "graph",
				Transaction: &ResourceTransactionPlan{
					ID: "job-a",
					Prepare: func(
						ctx context.Context,
						_ lifecycle.ReadyResource,
						scope lifecycle.ResourceTransactionScope,
						permit lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						workErr, claimErr := RunWithoutClaims(ctx, func(context.Context) error {
							close(probeStarted)
							<-releaseProbe
							return nil
						})
						if err := errors.Join(workErr, claimErr); err != nil {
							return nil, err
						}
						return &simpleCompositeChildTransaction{scope: scope, permit: permit}, nil
					},
				},
			},
		)
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "probe did not yield its graph claim")
	}

	parentPlan := compositeParentTestPlan(
		"unrelated-secret",
		[]string{"dependency", "graph"},
		nil,
	)
	parentPlan.Transaction.CompositeChildLaneConflict = func(string) bool { return false }
	parentCtx, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	require.NoError(t, kernel.SubmitPreparedAndWait(
		parentCtx,
		Request{
			UID:     "unrelated-parent",
			LaneKey: "unrelated-secret",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/unrelated-parent",
		},
		parentPlan,
	))

	close(releaseProbe)
	require.NoError(t, <-probeDone)
	stopCompositeTestKernel(t, kernel)
}

func activeClaimYieldTestOperation(
	t *testing.T,
	kernel *testCommandKernel,
	id lifecycle.OperationID,
	uid string,
	claims []string,
) *commandOperation {
	t.Helper()
	operation := claimTestOperation(t, kernel.claims, id, uid, claims)
	operation.lane = &commandLane{active: operation}
	kernel.operations[operation.UID] = operation
	granted, err := kernel.claims.acquire(operation)
	require.NoError(t, err)
	require.True(t, granted)
	return operation
}

func unusedClaimYieldPrepare(
	context.Context,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	return nil, errors.New("unused test preparation")
}

func unusedCompositeClaimYieldPrepare(
	context.Context,
	lifecycle.ReadyResource,
	lifecycle.ResourceTransactionScope,
	lifecycle.LongLivedPermit,
) (PreparedCompositeResourceTransaction, error) {
	return nil, errors.New("unused test preparation")
}

type claimYieldPanicTestLease struct {
	releases   int
	reacquires int
}

func (lease *claimYieldPanicTestLease) release(context.Context) error {
	lease.releases++
	return nil
}

func (lease *claimYieldPanicTestLease) reacquire(context.Context) error {
	lease.reacquires++
	return nil
}
