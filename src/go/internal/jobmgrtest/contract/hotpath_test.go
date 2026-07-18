package contract

import (
	"strings"
	"testing"
)

func TestBMM002HotpathContractIsExact(t *testing.T) {
	const requiredOwners = 31
	gates := BMM002HotpathGates()
	if len(gates) != requiredOwners {
		t.Fatalf(
			"hot-path owners=%d, want %d",
			len(gates),
			requiredOwners,
		)
	}
	owners := make(map[string]struct{}, len(gates))
	benchmarks := make(map[string]string, len(gates))
	for _, gate := range gates {
		if gate.OwnerID == "" ||
			!strings.HasPrefix(gate.Package, "./plugin/") ||
			len(gate.Tests) == 0 ||
			!strings.HasPrefix(gate.Benchmark, "Benchmark") {
			t.Fatalf("invalid hot-path gate: %#v", gate)
		}
		if _, exists := owners[gate.OwnerID]; exists {
			t.Fatalf("duplicate hot-path owner %s", gate.OwnerID)
		}
		owners[gate.OwnerID] = struct{}{}
		if owner, exists := benchmarks[gate.Benchmark]; exists {
			t.Fatalf(
				"benchmark %s is shared by %s and %s",
				gate.Benchmark,
				owner,
				gate.OwnerID,
			)
		}
		benchmarks[gate.Benchmark] = gate.OwnerID
		for _, test := range gate.Tests {
			if !strings.HasPrefix(test, "Test") {
				t.Fatalf(
					"owner %s has invalid test %q",
					gate.OwnerID,
					test,
				)
			}
		}
	}
	for _, futureOwner := range []string{
		"B-O-00027",
		"B-O-00028",
		"B-O-00029",
		"B-O-00030",
		"B-O-00032",
		"B-O-00033",
	} {
		if _, exists := owners[futureOwner]; exists {
			t.Fatalf(
				"future checkpoint owner %s entered B-M-002",
				futureOwner,
			)
		}
	}
}
