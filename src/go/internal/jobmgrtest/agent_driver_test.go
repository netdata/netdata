package jobmgrtest

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

func TestAgentPredicateRegistryMatchesContract(t *testing.T) {
	cases, err := contract.BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	want := make(map[string]struct{})
	for _, productionCase := range cases {
		if productionCase.Suite == contract.SuiteAgent &&
			productionCase.Proof != contract.ProofComponent {
			want[productionCase.ID] = struct{}{}
		}
	}
	if len(agentRuntimePredicates) != len(want) {
		t.Fatalf(
			"Agent predicates=%d, want %d",
			len(agentRuntimePredicates),
			len(want),
		)
	}
	for predicate := range want {
		if agentRuntimePredicates[predicate] == nil {
			t.Fatalf("Agent predicate %s is absent", predicate)
		}
	}
	for predicate := range agentRuntimePredicates {
		if _, exists := want[predicate]; !exists {
			t.Fatalf("unexpected Agent predicate %s", predicate)
		}
	}
}

func TestAgentDriverUsesCaseExactPublicLifecycle(t *testing.T) {
	tests := map[string]struct {
		predicate string
	}{
		"held Start acknowledgement": {
			predicate: "F01.1",
		},
		"Function invalid JSON": {
			predicate: "F14.5",
		},
		"concurrent same-route Functions": {
			predicate: "F08.2",
		},
	}
	driver := &AgentDriver{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			if err := driver.Run(ctx, test.predicate); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentRuntimeOutputFaultModes(t *testing.T) {
	tests := map[string]injectedWriteMode{
		"short nil":      injectedShortNil,
		"short error":    injectedShortError,
		"pre-byte error": injectedPreByteError,
		"EPIPE":          injectedEPIPE,
	}
	for name, mode := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				3*time.Second,
			)
			defer cancel()
			if err := runAgentOutputFaultMode(
				ctx,
				outputFaultRuntime,
				mode,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}
