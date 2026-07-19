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
	expectedOwners := map[string]struct{}{
		"B-O-00001": {}, "B-O-00002": {}, "B-O-00003": {},
		"B-O-00004": {}, "B-O-00005": {}, "B-O-00006": {},
		"B-O-00007": {}, "B-O-00008": {}, "B-O-00009": {},
		"B-O-00010": {}, "B-O-00011": {}, "B-O-00012": {},
		"B-O-00013": {}, "B-O-00014": {}, "B-O-00015": {},
		"B-O-00016": {}, "B-O-00017": {}, "B-O-00018": {},
		"B-O-00019": {}, "B-O-00020": {}, "B-O-00021": {},
		"B-O-00022": {}, "B-O-00023": {}, "B-O-00024": {},
		"B-O-00025": {}, "B-O-00026": {}, "B-O-00031": {},
		"B-O-00034": {}, "B-O-00035": {}, "B-O-00036": {},
		"B-O-00037": {},
	}
	for owner := range owners {
		if _, ok := expectedOwners[owner]; !ok {
			t.Fatalf("unexpected hot-path owner %s", owner)
		}
	}
	for owner := range expectedOwners {
		if _, ok := owners[owner]; !ok {
			t.Fatalf("approved hot-path owner %s is absent", owner)
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

func TestBMM002HotpathEnvelopeIsClosedByEveryOwnerTest(t *testing.T) {
	cases, err := BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	var target ProductionCase
	for _, productionCase := range cases {
		if productionCase.ID == "F24.14-b-hotpath-envelope" {
			target = productionCase
			break
		}
	}
	if target.ID == "" {
		t.Fatal("hot-path envelope case is absent")
	}
	evidence, err := EvidenceFor(target)
	if err != nil {
		t.Fatal(err)
	}
	observed := make(map[ComponentProof]struct{}, len(evidence.Components))
	for _, proof := range evidence.Components {
		observed[proof] = struct{}{}
	}
	for _, proof := range BMM002HotpathProofs() {
		if _, exists := observed[proof]; !exists {
			t.Fatalf(
				"hot-path envelope lacks deterministic proof %+v",
				proof,
			)
		}
	}
}

func TestBMM003HotpathContractAddsOnlySecretOwners(t *testing.T) {
	gates := BMM003HotpathGates()
	if len(gates) != 35 {
		t.Fatalf("hot-path owners=%d, want 35", len(gates))
	}
	owners := make(map[string]HotpathGate, len(gates))
	for _, gate := range gates {
		if _, exists := owners[gate.OwnerID]; exists {
			t.Fatalf("duplicate hot-path owner %s", gate.OwnerID)
		}
		owners[gate.OwnerID] = gate
	}
	expected := map[string]string{
		"B-O-00027": "./plugin/agent/secrets/secretstore",
		"B-O-00028": "./plugin/agent/secrets/secretstore",
		"B-O-00029": "./plugin/agent/jobmgr/secrets",
		"B-O-00030": "./plugin/agent/jobmgr/secrets",
	}
	for owner, pkg := range expected {
		gate, ok := owners[owner]
		if !ok {
			t.Fatalf("B-M-003 owner %s is absent", owner)
		}
		if gate.Package != pkg {
			t.Fatalf(
				"B-M-003 owner %s package=%q want=%q",
				owner,
				gate.Package,
				pkg,
			)
		}
	}
	for _, futureOwner := range []string{
		"B-O-00032",
		"B-O-00033",
	} {
		if _, exists := owners[futureOwner]; exists {
			t.Fatalf(
				"future checkpoint owner %s entered B-M-003",
				futureOwner,
			)
		}
	}
	if len(BMM003HotpathProofs()) <= len(BMM002HotpathProofs()) {
		t.Fatal("B-M-003 hot-path proofs did not grow cumulatively")
	}
}
