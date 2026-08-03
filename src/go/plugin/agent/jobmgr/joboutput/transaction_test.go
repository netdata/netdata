// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestPreparedResourceTransactionCommitsOrRestoresWholePostimage(t *testing.T) {
	tests := map[string]struct {
		apply       bool
		wantEvents  []string
		wantPayload string
	}{
		"apply graph and replacement": {
			apply: true,
			wantEvents: []string{
				"current-stop",
				"current-finalize",
				"successor-accept",
				"successor-publish",
				"after-apply",
			},
			wantPayload: `{"version":2}`,
		},
		"dispose keeps graph and current": {wantEvents: []string{"successor-dispose"}, wantPayload: `{"version":1}`},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			currentIdentity := lifecycle.ResourceIdentity{
				ID:         "job",
				Generation: 1,
			}
			successorIdentity := lifecycle.ResourceIdentity{
				ID:         "job",
				Generation: 2,
			}
			current := &transactionTestReadyResource{
				identity: currentIdentity,
				prefix:   "current",
				events:   &events,
			}
			successorReady := &transactionTestReadyResource{
				identity: successorIdentity,
				prefix:   "successor",
				events:   &events,
			}
			successor := &transactionTestPreparedResource{
				identity: successorIdentity,
				ready:    successorReady,
				events:   &events,
			}
			graph, err := dyncfg.NewGraph([]dyncfg.GraphConfig{{
				ID: "job", Module: "module", Name: "job",
				Payload: []byte(`{"version":1}`),
			}})
			require.NoError(t, err)
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: "job",
				Config: &dyncfg.GraphConfig{
					ID:      "job",
					Module:  "module",
					Name:    "job",
					Payload: []byte(`{"version":2}`),
				},
			}})
			require.NoError(t, err)
			result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"accepted":true}`))
			require.NoError(t, err)
			transaction, err := PrepareResourceTransaction(
				ResourceTransactionSpec{
					Scope: lifecycle.ResourceTransactionScope{
						ID:        "job",
						Current:   currentIdentity,
						Successor: successorIdentity,
					},
					Disposition:      lifecycle.ResourceTransactionReplaced,
					Current:          current,
					Successor:        successor,
					Graph:            graph,
					Mutation:         mutation,
					MutationPrepared: true,
					AfterApply: func() {
						events = append(events, "after-apply")
					},
					Result:  result,
					Cleanup: func() error { return nil },
				},
			)
			require.NoError(t, err)

			if test.apply {

				_, applyErr2 := transaction.Apply(context.Background())
				require.NoError(t, applyErr2)

			} else {
				restored, err := transaction.Dispose(context.Background())
				require.NoError(t, err)
				require.Same(t, current, restored)
			}
			record, ok := graph.Lookup("job")
			require.False(t, !ok || record.Payload() != test.wantPayload)
			require.Equal(t, test.wantEvents, events)

			_, applyErr := transaction.Apply(context.Background())
			require.Error(t, applyErr)

		})
	}
}

func TestPreparedResourceTransactionAbortsGraphMutationOnPrecommitFailure(t *testing.T) {
	tests := map[string]struct {
		configure func(*transactionTestReadyResource, *transactionTestPreparedResource, *transactionTestReadyResource, error)
		want      lifecycle.ResourceTransactionDisposition
		successor bool
	}{
		"current stop": {
			configure: func(
				current *transactionTestReadyResource,
				_ *transactionTestPreparedResource,
				_ *transactionTestReadyResource,
				failure error,
			) {
				current.stopErr = failure
			},
			want: lifecycle.ResourceTransactionUnchanged,
		},
		"current finalize": {
			configure: func(
				current *transactionTestReadyResource,
				_ *transactionTestPreparedResource,
				_ *transactionTestReadyResource,
				failure error,
			) {
				current.finalizeErr = failure
			},
			want: lifecycle.ResourceTransactionUnchanged,
		},
		"successor accept": {
			configure: func(
				_ *transactionTestReadyResource,
				successor *transactionTestPreparedResource,
				_ *transactionTestReadyResource,
				failure error,
			) {
				successor.acceptErr = failure
			},
			want:      lifecycle.ResourceTransactionReplaced,
			successor: true,
		},
		"successor publish": {
			configure: func(
				_ *transactionTestReadyResource,
				_ *transactionTestPreparedResource,
				successor *transactionTestReadyResource,
				failure error,
			) {
				successor.publishErr = failure
			},
			want:      lifecycle.ResourceTransactionReplaced,
			successor: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			failure := errors.New("precommit failed")
			currentIdentity := lifecycle.ResourceIdentity{
				ID:         "job",
				Generation: 1,
			}
			successorIdentity := lifecycle.ResourceIdentity{
				ID:         "job",
				Generation: 2,
			}
			current := &transactionTestReadyResource{
				identity: currentIdentity,
				prefix:   "current",
				events:   &events,
			}
			successorReady := &transactionTestReadyResource{
				identity: successorIdentity,
				prefix:   "successor",
				events:   &events,
			}
			successor := &transactionTestPreparedResource{
				identity: successorIdentity,
				ready:    successorReady,
				events:   &events,
			}
			test.configure(current, successor, successorReady, failure)
			graph, err := dyncfg.NewGraph([]dyncfg.GraphConfig{{
				ID: "job", Module: "module", Name: "job",
				Payload: []byte(`{"version":1}`),
			}})
			require.NoError(t, err)
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: "job",
				Config: &dyncfg.GraphConfig{
					ID:      "job",
					Module:  "module",
					Name:    "job",
					Payload: []byte(`{"version":2}`),
				},
			}})
			require.NoError(t, err)
			result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"accepted":true}`))
			require.NoError(t, err)
			transactionScope := lifecycle.ResourceTransactionScope{
				ID:        "job",
				Current:   currentIdentity,
				Successor: successorIdentity,
			}
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope:            transactionScope,
				Disposition:      lifecycle.ResourceTransactionReplaced,
				Current:          current,
				Successor:        successor,
				Graph:            graph,
				Mutation:         mutation,
				MutationPrepared: true,
				Result:           result,
				Cleanup:          func() error { return nil },
			})
			require.NoError(t, err)

			applied, err := transaction.Apply(context.Background())
			require.ErrorIs(t, err, failure)
			scope, disposition, owned := applied.Ownership()
			require.Equal(t, transactionScope, scope)
			require.Equal(t, test.want, disposition)
			if test.successor {
				require.Same(t, successorReady, owned)
			} else {
				require.Same(t, current, owned)
			}
			record, ok := graph.Lookup("job")
			require.True(t, ok)
			require.Equal(t, `{"version":1}`, record.Payload())

			next, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: "job",
				Config: &dyncfg.GraphConfig{
					ID:      "job",
					Module:  "module",
					Name:    "job",
					Payload: []byte(`{"version":3}`),
				},
			}})
			require.NoError(t, err)
			require.NoError(t, graph.Abort(next))
		})
	}
}

func TestPreparedResourceTransactionDoesNotSuppressRetirementAfterGraphCommit(t *testing.T) {
	var events []string
	identity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	successorReady := &transactionTestAcknowledgedResource{
		transactionTestReadyResource: &transactionTestReadyResource{
			identity: identity,
			prefix:   "successor",
			events:   &events,
		},
		acknowledgeErr: jobmgr.ErrProcessAttemptRetired,
	}
	successor := &transactionTestPreparedResource{
		identity: identity,
		ready:    successorReady,
		events:   &events,
	}
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	postimage := dyncfg.GraphConfig{ID: identity.ID, Module: "module", Name: "job"}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID:     identity.ID,
		Config: &postimage,
	}})
	require.NoError(t, err)
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	afterApply := false
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope: lifecycle.ResourceTransactionScope{
			ID:        identity.ID,
			Successor: identity,
		},
		Disposition:      lifecycle.ResourceTransactionInstalled,
		Successor:        successor,
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		AfterApply:       func() { afterApply = true },
		Result:           result,
		Cleanup:          func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())

	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)
	require.False(t, afterApply)
	record, ok := graph.Lookup(identity.ID)
	require.True(t, ok)
	require.Equal(t, postimage.Module, record.Module)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionInstalled, disposition)
	require.Same(t, successorReady, current)
}

func TestPreparedResourceTransactionReturnsRetainedPendingSuccessorOnFailure(t *testing.T) {
	startFailure := errors.New("successor start failed")
	abortFailure := errors.New("successor abort failed")
	tests := map[string]struct {
		disposition lifecycle.ResourceTransactionDisposition
		currentID   lifecycle.ResourceIdentity
		generation  uint64
	}{
		"installation": {
			disposition: lifecycle.ResourceTransactionInstalled,
			generation:  1,
		},
		"replacement": {
			disposition: lifecycle.ResourceTransactionReplaced,
			currentID:   lifecycle.ResourceIdentity{ID: "job", Generation: 1},
			generation:  2,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			var current lifecycle.ReadyResource
			if test.currentID.Valid() {
				current = &transactionTestReadyResource{
					identity: test.currentID,
					prefix:   "current",
					events:   &events,
				}
			}
			successorID := lifecycle.ResourceIdentity{ID: "job", Generation: test.generation}
			successor, releaseSuccessor := newRetainedTransactionSuccessor(
				t,
				successorID,
				startFailure,
				abortFailure,
			)
			defer releaseSuccessor()
			result, err := lifecycle.NewSealedResult(500, "application/json", nil)
			require.NoError(t, err)
			scope := lifecycle.ResourceTransactionScope{
				ID:        "job",
				Current:   test.currentID,
				Successor: successorID,
			}
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope:       scope,
				Disposition: test.disposition,
				Current:     current,
				Successor:   successor,
				Result:      result,
				Cleanup:     func() error { return nil },
			})
			require.NoError(t, err)

			applied, err := transaction.Apply(context.Background())

			require.ErrorIs(t, err, startFailure)
			require.ErrorIs(t, err, abortFailure)
			gotScope, gotDisposition, owned := applied.Ownership()
			require.Equal(t, scope, gotScope)
			require.Equal(t, test.disposition, gotDisposition)
			retained, ok := owned.(*JobGeneration)
			require.True(t, ok)
			require.Equal(t, JobRetained, retained.State())
		})
	}
}

func newRetainedTransactionSuccessor(
	t *testing.T,
	identity lifecycle.ResourceIdentity,
	startErr error,
	abortErr error,
) (PreparedJob, func()) {
	t.Helper()
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { return nil },
		finalCleanup:     func(context.Context) error { return nil },
		outputGate:       gate,
	}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	candidate.attach = func(
		lifecycle.ResourceIdentity,
		*stagedJobOwner,
	) (constructedJobAttachment, error) {
		if err := owner.BindAttachment(); err != nil {
			return constructedJobAttachment{}, err
		}
		attached := candidate
		attached.Runtime = transactionTestRuntime{
			startErr: startErr,
			abortErr: abortErr,
		}
		attached.processOwner = owner
		return constructedJobAttachment{resources: attached, transferred: true}, nil
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	prepared := PreparedJob{state: &preparedJobState{
		id:          identity.ID,
		generation:  identity.Generation,
		constructed: candidate,
		permit:      permit,
		owner:       owner,
	}}
	release := func() {
		owner.Detached()
		owner.Reject()
		<-owner.done
		require.NoError(t, permit.ReleaseExternal())
		require.NoError(t, permit.Return())
		require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	}
	return prepared, release
}

type transactionTestRuntime struct {
	startErr error
	abortErr error
}

func (ttr transactionTestRuntime) Start(context.Context) error {
	return ttr.startErr
}

func (ttr transactionTestRuntime) Abort(context.Context) error {
	return ttr.abortErr
}

func (transactionTestRuntime) Stop(context.Context) error {
	return nil
}

func (transactionTestRuntime) ReleaseAfterCleanup(context.Context) error {
	return nil
}

func TestPreparedResourceTransactionSettlesTargetRetirementBeforeAcceptance(t *testing.T) {
	identity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	postimage := dyncfg.GraphConfig{ID: identity.ID, Module: "module", Name: "job"}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID:     identity.ID,
		Config: &postimage,
	}})
	require.NoError(t, err)
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	events := []string{}
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope: lifecycle.ResourceTransactionScope{
			ID:        identity.ID,
			Successor: identity,
		},
		Disposition: lifecycle.ResourceTransactionInstalled,
		Successor: &transactionTestPreparedResource{
			identity:  identity,
			events:    &events,
			acceptErr: jobmgr.ErrProcessAttemptRetired,
		},
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		Result:           result,
		Cleanup:          func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	_, exists := graph.Lookup(identity.ID)
	require.False(t, exists)
}

func TestPreparedResourceTransactionSettlesTargetRetirementAfterPublication(t *testing.T) {
	applied, applyErr, graphConfigExists := applyTargetRetirementTransaction(t, nil)
	require.NoError(t, applyErr)
	scope, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionScope{
		ID:        "job",
		Successor: lifecycle.ResourceIdentity{ID: "job", Generation: 1},
	}, scope)
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	require.False(t, graphConfigExists)
}

func TestPreparedResourceTransactionDoesNotHideTargetRetirementSettlementFailure(t *testing.T) {
	settlementErr := errors.New("detach failed")
	applied, applyErr, graphConfigExists := applyTargetRetirementTransaction(t, settlementErr)
	require.ErrorIs(t, applyErr, settlementErr)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionInstalled, disposition)
	require.NotNil(t, current)
	require.False(t, graphConfigExists)
}

func applyTargetRetirementTransaction(
	t *testing.T,
	detachErr error,
) (lifecycle.AppliedResourceTransaction, error, bool) {
	t.Helper()
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	job := newBlockingStopManagedJob()
	stopReleased := false
	var owner *stagedJobOwner
	releaseStop := func() {
		if !stopReleased {
			close(job.stopRelease)
			stopReleased = true
		}
	}
	defer func() {
		if owner != nil {
			owner.Detached()
			owner.Reject()
		}
		releaseStop()
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	}()
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)

	var cleanups int
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { cleanups++; return nil },
		candidateJob:     job,
		outputGate:       gate,
	}
	identity := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
	owner = newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	require.NoError(t, owner.Promote(t.Context()))
	attached, err := newProcessManagedJob(
		JobVariantV1,
		job,
		identity,
		newTestScheduler(t),
		candidate.CollectorCleanup,
		owner,
	)
	require.NoError(t, err)
	attached.outputGate = gate
	attached.Handlers = &retireTargetOnPublishHandler{
		attempts:  attempts,
		target:    1,
		retired:   owner.retire,
		detachErr: detachErr,
	}
	require.NoError(t, owner.AdoptAttachment(attached))

	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	accepted, err := owner.AcceptResources(permit)
	require.NoError(t, err)
	generation := &JobGeneration{
		resources:    accepted,
		permit:       permit,
		stopDone:     make(chan struct{}),
		ID:           identity.ID,
		Generation:   identity.Generation,
		state:        JobAllocated,
		owner:        owner,
		processOwner: owner,
	}
	require.NoError(t, generation.Start(context.Background()))

	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	postimage := dyncfg.GraphConfig{
		ID: identity.ID, Module: job.ModuleName(), Name: job.Name(),
	}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
		ID:     identity.ID,
		Config: &postimage,
	}})
	require.NoError(t, err)
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	events := []string{}
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope: lifecycle.ResourceTransactionScope{
			ID:        identity.ID,
			Successor: identity,
		},
		Disposition: lifecycle.ResourceTransactionInstalled,
		Successor: &transactionTestPreparedResource{
			identity: identity,
			ready:    generation,
			events:   &events,
		},
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		Result:           result,
		Cleanup:          func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	_, exists := graph.Lookup(identity.ID)
	if generation.State() == JobRetained {
		owner.Detached()
		owner.Reject()
		require.NoError(t, permit.ReleaseExternal())
		require.NoError(t, permit.Return())
	}

	releaseStop()
	<-owner.done
	require.Equal(t, 1, cleanups)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	return applied, err, exists
}

type retireTargetOnPublishHandler struct {
	attempts  interface{ CutTarget(uint64) int }
	target    uint64
	retired   <-chan struct{}
	detachErr error
}

func (handler *retireTargetOnPublishHandler) Publish() error {
	_ = handler.attempts.CutTarget(handler.target)
	<-handler.retired
	return nil
}

func (*retireTargetOnPublishHandler) CloseAndDrain(context.Context) error { return nil }
func (handler *retireTargetOnPublishHandler) Detach(context.Context) error {
	return handler.detachErr
}
func (*retireTargetOnPublishHandler) Finalize(context.Context) error { return nil }

func TestPreparedResourceTransactionCommitsBusyFallbackAfterIncumbentRemoval(t *testing.T) {
	var events []string
	currentIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 2}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	successor := &transactionTestPreparedResource{
		identity:  successorIdentity,
		events:    &events,
		acceptErr: jobmgr.ErrProcessAttemptBusy,
	}
	graph, err := dyncfg.NewGraph([]dyncfg.GraphConfig{{
		ID: "job", Module: "module", Name: "job", Status: dyncfg.StatusRunning.String(),
		Payload: []byte(`{"version":1}`),
	}})
	require.NoError(t, err)
	running := dyncfg.GraphConfig{
		ID: "job", Module: "module", Name: "job", Status: dyncfg.StatusRunning.String(),
		Payload: []byte(`{"version":2}`),
	}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{ID: "job", Config: &running}})
	require.NoError(t, err)
	failed := running
	failed.Status = dyncfg.StatusFailed.String()
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	busyResult, err := lifecycle.NewSealedResult(503, "application/json", nil)
	require.NoError(t, err)
	transactionScope := lifecycle.ResourceTransactionScope{
		ID:        "job",
		Current:   currentIdentity,
		Successor: successorIdentity,
	}
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope:            transactionScope,
		Disposition:      lifecycle.ResourceTransactionReplaced,
		Current:          current,
		Successor:        successor,
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		ActivationBusyFallback: &ResourceActivationFallback{
			Change: dyncfg.GraphChange{ID: "job", Config: &failed},
			AfterGraphCommit: func() {
				events = append(events, "fallback-graph-commit")
			},
			AfterApply: func() {
				events = append(events, "fallback-after-apply")
			},
			Result:  busyResult,
			Cleanup: func() error { return nil },
		},
		Result:  result,
		Cleanup: func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	scope, disposition, owned := applied.Ownership()
	require.Equal(t, transactionScope, scope)
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, owned)
	require.Equal(t, 503, applied.ResultStatus())
	record, ok := graph.Lookup("job")
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.Equal(t, `{"version":2}`, record.Payload())
	require.Equal(t, []string{
		"current-stop",
		"current-finalize",
		"successor-accept",
		"fallback-graph-commit",
		"fallback-after-apply",
	}, events)

	next := failed
	next.Status = dyncfg.StatusRunning.String()
	nextMutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{ID: "job", Config: &next}})
	require.NoError(t, err)
	require.NoError(t, graph.Abort(nextMutation))
}

func TestPreparedResourceTransactionCommitsQuarantineFallbackWithoutPending(t *testing.T) {
	var events []string
	currentIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 2}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	successor := &transactionTestPreparedResource{
		identity: successorIdentity,
		events:   &events,
		acceptErr: errors.Join(
			jobmgr.ErrProcessAttemptBusy,
			jobmgr.ErrProcessAttemptQuarantined,
		),
	}
	graph, err := dyncfg.NewGraph([]dyncfg.GraphConfig{{
		ID: "job", Module: "module", Name: "job", Status: dyncfg.StatusRunning.String(),
		Payload: []byte(`{"version":1}`),
	}})
	require.NoError(t, err)
	running := dyncfg.GraphConfig{
		ID: "job", Module: "module", Name: "job", Status: dyncfg.StatusRunning.String(),
		Payload: []byte(`{"version":2}`),
	}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{ID: "job", Config: &running}})
	require.NoError(t, err)
	failed := running
	failed.Status = dyncfg.StatusFailed.String()
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	quarantinedResult, err := lifecycle.NewSealedResult(503, "application/json", nil)
	require.NoError(t, err)
	transactionScope := lifecycle.ResourceTransactionScope{
		ID:        "job",
		Current:   currentIdentity,
		Successor: successorIdentity,
	}
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope:            transactionScope,
		Disposition:      lifecycle.ResourceTransactionReplaced,
		Current:          current,
		Successor:        successor,
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		ActivationBusyFallback: &ResourceActivationFallback{
			Change: dyncfg.GraphChange{ID: "job", Config: &failed},
			AfterApply: func() {
				events = append(events, "busy-pending")
			},
			Result:  result,
			Cleanup: func() error { return nil },
		},
		ActivationQuarantinedFallback: &ResourceActivationFallback{
			Change: dyncfg.GraphChange{ID: "job", Config: &failed},
			AfterGraphCommit: func() {
				events = append(events, "fallback-graph-commit")
			},
			AfterApply: func() {
				events = append(events, "quarantine-settled")
			},
			Result:  quarantinedResult,
			Cleanup: func() error { return nil },
		},
		Result:  result,
		Cleanup: func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	scope, disposition, owned := applied.Ownership()
	require.Equal(t, transactionScope, scope)
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, owned)
	require.Equal(t, 503, applied.ResultStatus())
	record, ok := graph.Lookup("job")
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.Equal(t, `{"version":2}`, record.Payload())
	require.Equal(t, []string{
		"current-stop",
		"current-finalize",
		"successor-accept",
		"fallback-graph-commit",
		"quarantine-settled",
	}, events)
	require.NotContains(t, events, "busy-pending")
}

func TestPreparedResourceTransactionAbortsGraphMutationOnPanic(t *testing.T) {
	successorIdentity := lifecycle.ResourceIdentity{
		ID:         "job",
		Generation: 1,
	}
	events := []string{}
	successor := &transactionTestPreparedResource{
		identity:    successorIdentity,
		events:      &events,
		acceptPanic: "accept panic",
	}
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	change := dyncfg.GraphChange{
		ID: "job",
		Config: &dyncfg.GraphConfig{
			ID:     "job",
			Module: "module",
			Name:   "job",
		},
	}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{change})
	require.NoError(t, err)
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope: lifecycle.ResourceTransactionScope{
			ID:        "job",
			Successor: successorIdentity,
		},
		Disposition:      lifecycle.ResourceTransactionInstalled,
		Successor:        successor,
		Graph:            graph,
		Mutation:         mutation,
		MutationPrepared: true,
		Result:           result,
		Cleanup:          func() error { return nil },
	})
	require.NoError(t, err)

	applied, err := transaction.Apply(context.Background())
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
	scope, disposition, owned := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionScope{ID: "job", Successor: successorIdentity}, scope)
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, owned)

	next, err := graph.PrepareMutation([]dyncfg.GraphChange{change})
	require.NoError(t, err)
	require.NoError(t, graph.Abort(next))
}

type transactionTestPreparedResource struct {
	identity    lifecycle.ResourceIdentity
	ready       lifecycle.ReadyResource
	events      *[]string
	acceptErr   error
	acceptPanic any
	disposeErr  error
}

func (ttpr *transactionTestPreparedResource) Identity() lifecycle.ResourceIdentity {
	return ttpr.identity
}

func (ttpr *transactionTestPreparedResource) AcceptStart(
	_ context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	*ttpr.events = append(*ttpr.events, "successor-accept")
	if ttpr.acceptPanic != nil {
		panic(ttpr.acceptPanic)
	}
	if ttpr.acceptErr != nil {
		return ttpr.ready, ttpr.acceptErr
	}
	if expected != ttpr.identity.Generation {
		return nil, ErrJobGenerationMismatch
	}
	return ttpr.ready, nil
}

func (ttpr *transactionTestPreparedResource) Dispose(context.Context) error {
	*ttpr.events = append(*ttpr.events, "successor-dispose")
	return ttpr.disposeErr
}

type transactionTestReadyResource struct {
	identity    lifecycle.ResourceIdentity
	prefix      string
	events      *[]string
	publishErr  error
	stopErr     error
	finalizeErr error
}

func (ttrr *transactionTestReadyResource) Identity() lifecycle.ResourceIdentity {
	return ttrr.identity
}

func (ttrr *transactionTestReadyResource) Publish() error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-publish")
	return ttrr.publishErr
}

func (ttrr *transactionTestReadyResource) AbortReady(context.Context) error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-abort")
	return nil
}

func (ttrr *transactionTestReadyResource) Stop(context.Context) error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-stop")
	return ttrr.stopErr
}

func (ttrr *transactionTestReadyResource) Finalize() error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-finalize")
	return ttrr.finalizeErr
}

type transactionTestAcknowledgedResource struct {
	*transactionTestReadyResource
	acknowledgeErr error
}

func (resource *transactionTestAcknowledgedResource) acknowledgeInstallation() error {
	return resource.acknowledgeErr
}
