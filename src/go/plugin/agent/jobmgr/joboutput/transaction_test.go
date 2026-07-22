// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"

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
			currentIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
			successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 2}
			current := &transactionTestReadyResource{identity: currentIdentity, prefix: "current", events: &events}
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
					ID: "job", Module: "module", Name: "job",
					Payload: []byte(`{"version":2}`),
				},
			}})
			require.NoError(t, err)
			result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"accepted":true}`))
			require.NoError(t, err)
			transaction, err := PrepareResourceTransaction(
				ResourceTransactionSpec{
					Scope: lifecycle.ResourceTransactionScope{
						ID:      "job",
						Current: currentIdentity, Successor: successorIdentity,
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
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			failure := errors.New("precommit failed")
			currentIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
			successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 2}
			current := &transactionTestReadyResource{identity: currentIdentity, prefix: "current", events: &events}
			successorReady := &transactionTestReadyResource{
				identity: successorIdentity, prefix: "successor", events: &events,
			}
			successor := &transactionTestPreparedResource{
				identity: successorIdentity, ready: successorReady, events: &events,
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
					ID: "job", Module: "module", Name: "job",
					Payload: []byte(`{"version":2}`),
				},
			}})
			require.NoError(t, err)
			result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"accepted":true}`))
			require.NoError(t, err)
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope: lifecycle.ResourceTransactionScope{
					ID: "job", Current: currentIdentity, Successor: successorIdentity,
				},
				Disposition: lifecycle.ResourceTransactionReplaced,
				Current:     current, Successor: successor,
				Graph: graph, Mutation: mutation, MutationPrepared: true,
				Result: result, Cleanup: func() error { return nil },
			})
			require.NoError(t, err)

			_, err = transaction.Apply(context.Background())
			require.ErrorIs(t, err, failure)
			record, ok := graph.Lookup("job")
			require.True(t, ok)
			require.Equal(t, `{"version":1}`, record.Payload())

			next, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: "job",
				Config: &dyncfg.GraphConfig{
					ID: "job", Module: "module", Name: "job",
					Payload: []byte(`{"version":3}`),
				},
			}})
			require.NoError(t, err)
			require.NoError(t, graph.Abort(next))
		})
	}
}

func TestPreparedResourceTransactionAbortsGraphMutationOnPanic(t *testing.T) {
	successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	events := []string{}
	successor := &transactionTestPreparedResource{
		identity:    successorIdentity,
		events:      &events,
		acceptPanic: "accept panic",
	}
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	change := dyncfg.GraphChange{ID: "job", Config: &dyncfg.GraphConfig{ID: "job", Module: "module", Name: "job"}}
	mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{change})
	require.NoError(t, err)
	result, err := lifecycle.NewSealedResult(200, "application/json", nil)
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
		Scope:       lifecycle.ResourceTransactionScope{ID: "job", Successor: successorIdentity},
		Disposition: lifecycle.ResourceTransactionInstalled,
		Successor:   successor,
		Graph:       graph, Mutation: mutation, MutationPrepared: true,
		Result: result, Cleanup: func() error { return nil },
	})
	require.NoError(t, err)

	require.Panics(t, func() {
		_, _ = transaction.Apply(context.Background())
	})

	next, err := graph.PrepareMutation([]dyncfg.GraphChange{change})
	require.NoError(t, err)
	require.NoError(t, graph.Abort(next))
}

func TestPreparedResourceTransactionSettlesBeforeFailureResolution(t *testing.T) {
	var events []string
	successorIdentity := lifecycle.ResourceIdentity{ID: "job", Generation: 1}
	successor := &transactionTestPreparedResource{
		identity:  successorIdentity,
		events:    &events,
		acceptErr: &autoDetectionFailure{cause: errors.New("autodetection failed")},
	}
	result, err := lifecycle.NewSealedResult(422, "application/json", []byte(`{"accepted":false}`))
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(
		ResourceTransactionSpec{
			Scope:       lifecycle.ResourceTransactionScope{ID: "job", Successor: successorIdentity},
			Disposition: lifecycle.ResourceTransactionInstalled,
			Successor:   successor,
			AfterApply: func() {
				events = append(events, "settle")
			},
			Result:  result,
			Cleanup: func() error { return nil },
			SuccessorFailure: func(*autoDetectionFailure) (SuccessorFailureResolution, error) {
				return SuccessorFailureResolution{
					Result:  result,
					Cleanup: func() error { return nil },
					AfterApply: func() {
						events = append(events, "resolve")
					},
				}, nil
			},
		},
	)
	require.NoError(t, err)

	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"successor-accept", "settle", "resolve"}, events)
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
		return nil, ttpr.acceptErr
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
