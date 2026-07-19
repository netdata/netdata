// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
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
			apply:       true,
			wantEvents:  []string{"current-stop", "current-finalize", "successor-accept", "successor-publish", "after-apply"},
			wantPayload: `{"version":2}`,
		},
		"dispose keeps graph and current": {
			wantEvents:  []string{"successor-dispose"},
			wantPayload: `{"version":1}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			events := []string{}
			currentIdentity := lifecycle.ResourceIdentity{
				ID: "job", Generation: 1,
			}
			successorIdentity := lifecycle.ResourceIdentity{
				ID: "job", Generation: 2,
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
					Disposition: lifecycle.ResourceTransactionReplaced,
					Current:     current,
					Successor:   successor,
					Graph:       graph,
					Mutation:    mutation,
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

type transactionTestPreparedResource struct {
	identity lifecycle.ResourceIdentity
	ready    lifecycle.ReadyResource
	events   *[]string
}

func (ttpr *transactionTestPreparedResource) Identity() lifecycle.ResourceIdentity {
	return ttpr.identity
}

func (ttpr *transactionTestPreparedResource) AcceptStart(
	_ context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	*ttpr.events = append(*ttpr.events, "successor-accept")
	if expected != ttpr.identity.Generation {
		return nil, ErrJobGenerationMismatch
	}
	return ttpr.ready, nil
}

func (ttpr *transactionTestPreparedResource) Dispose(
	context.Context,
) error {
	*ttpr.events = append(*ttpr.events, "successor-dispose")
	return nil
}

type transactionTestReadyResource struct {
	identity lifecycle.ResourceIdentity
	prefix   string
	events   *[]string
}

func (ttrr *transactionTestReadyResource) Identity() lifecycle.ResourceIdentity {
	return ttrr.identity
}

func (ttrr *transactionTestReadyResource) Publish() error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-publish")
	return nil
}

func (ttrr *transactionTestReadyResource) AbortReady(
	context.Context,
) error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-abort")
	return nil
}

func (ttrr *transactionTestReadyResource) Stop(
	context.Context,
) error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-stop")
	return nil
}

func (ttrr *transactionTestReadyResource) Finalize() error {
	*ttrr.events = append(*ttrr.events, ttrr.prefix+"-finalize")
	return nil
}
