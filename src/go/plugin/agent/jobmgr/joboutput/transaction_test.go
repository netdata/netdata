// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"reflect"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
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
			if err != nil {
				t.Fatal(err)
			}
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID: "job",
				Config: &dyncfg.GraphConfig{
					ID: "job", Module: "module", Name: "job",
					Payload: []byte(`{"version":2}`),
				},
			}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := lifecycle.NewSealedResult(
				200,
				"application/json",
				[]byte(`{"accepted":true}`),
			)
			if err != nil {
				t.Fatal(err)
			}
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
			if err != nil {
				t.Fatal(err)
			}

			if test.apply {
				if _, err := transaction.Apply(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				restored, err := transaction.Dispose(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if restored != current {
					t.Fatal("disposal did not return the exact current resource")
				}
			}
			record, ok := graph.Lookup("job")
			if !ok || record.Payload() != test.wantPayload {
				t.Fatalf(
					"graph record=%#v ok=%v, want payload %s",
					record,
					ok,
					test.wantPayload,
				)
			}
			if !reflect.DeepEqual(events, test.wantEvents) {
				t.Fatalf("events=%v, want %v", events, test.wantEvents)
			}
			if _, err := transaction.Apply(context.Background()); err == nil {
				t.Fatal("consumed transaction applied twice")
			}
		})
	}
}

type transactionTestPreparedResource struct {
	identity lifecycle.ResourceIdentity
	ready    lifecycle.ReadyResource
	events   *[]string
}

func (resource *transactionTestPreparedResource) Identity() lifecycle.ResourceIdentity {
	return resource.identity
}

func (resource *transactionTestPreparedResource) AcceptStart(
	_ context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	*resource.events = append(*resource.events, "successor-accept")
	if expected != resource.identity.Generation {
		return nil, ErrJobGenerationMismatch
	}
	return resource.ready, nil
}

func (resource *transactionTestPreparedResource) Dispose(
	context.Context,
) error {
	*resource.events = append(*resource.events, "successor-dispose")
	return nil
}

type transactionTestReadyResource struct {
	identity lifecycle.ResourceIdentity
	prefix   string
	events   *[]string
}

func (resource *transactionTestReadyResource) Identity() lifecycle.ResourceIdentity {
	return resource.identity
}

func (resource *transactionTestReadyResource) Publish() error {
	*resource.events = append(
		*resource.events,
		resource.prefix+"-publish",
	)
	return nil
}

func (resource *transactionTestReadyResource) AbortReady(
	context.Context,
) error {
	*resource.events = append(
		*resource.events,
		resource.prefix+"-abort",
	)
	return nil
}

func (resource *transactionTestReadyResource) Stop(
	context.Context,
) error {
	*resource.events = append(
		*resource.events,
		resource.prefix+"-stop",
	)
	return nil
}

func (resource *transactionTestReadyResource) Finalize() error {
	*resource.events = append(
		*resource.events,
		resource.prefix+"-finalize",
	)
	return nil
}
