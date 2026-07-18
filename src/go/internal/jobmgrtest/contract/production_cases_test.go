package contract

import (
	"strings"
	"testing"
)

func TestBMM002CaseClosureIsExactAndCodenameFree(t *testing.T) {
	cases, err := BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != BMM002RequiredCases {
		t.Fatalf("B-M-002 case count=%d, want %d", len(cases), BMM002RequiredCases)
	}
	wantSuites := map[RuntimeSuite]int{
		SuiteAgent: 70, SuiteProcess: 5, SuiteShippedRoot: 10,
		SuiteCollectorBoundary: 4, SuiteResolver: 1,
	}
	gotSuites := make(map[RuntimeSuite]int, len(wantSuites))
	for _, productionCase := range cases {
		gotSuites[productionCase.Suite]++
		if strings.Contains(productionCase.ID, "poc") ||
			strings.Contains(productionCase.ID, "postgres") ||
			strings.Contains(productionCase.ID, "cato") ||
			strings.Contains(productionCase.ID, "snmp") {
			t.Fatalf("stale implementation-specific case remains: %s", productionCase.ID)
		}
	}
	for suite, want := range wantSuites {
		if got := gotSuites[suite]; got != want {
			t.Fatalf("suite %s cases=%d, want %d", suite, got, want)
		}
	}
}

func TestBMM002CaseClosureRecordsHonestProofKinds(t *testing.T) {
	cases, err := BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	proofs := make(map[string]ProofKind, len(cases))
	for _, productionCase := range cases {
		proofs[productionCase.ID] = productionCase.Proof
	}
	tests := map[string]struct {
		caseID string
		want   ProofKind
	}{
		"component census": {
			caseID: "F03.1",
			want:   ProofComponent,
		},
		"combined lifecycle": {
			caseID: "F02.1-v2-nested-cleanup-boundary",
			want:   ProofCombined,
		},
		"public root": {
			caseID: "F24.20-b-godplugin-terminal",
			want:   ProofRuntime,
		},
		"complexity": {
			caseID: "F24.16-b-shutdown-complexity",
			want:   ProofComponent,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := proofs[test.caseID]; got != test.want {
				t.Fatalf("case %s proof=%s, want %s", test.caseID, got, test.want)
			}
		})
	}
}

func TestBMM002CaseClosureDigestIsStable(t *testing.T) {
	digest, err := BMM002CaseSHA256()
	if err != nil {
		t.Fatal(err)
	}
	const want = "037983e4e77dce8209b1964b33908f51aef0f73ed9250648da9cd2b6cc166bf1"
	if digest != want {
		t.Fatalf("B-M-002 case digest=%s", digest)
	}
}

func TestBMM002CaseEvidenceIsComplete(t *testing.T) {
	cases, err := BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	for _, productionCase := range cases {
		evidence, err := EvidenceFor(productionCase)
		if err != nil {
			t.Fatalf("case %s: %v", productionCase.ID, err)
		}
		if productionCase.Proof == ProofComponent &&
			evidence.RuntimeScenario != "" {
			t.Fatalf(
				"component-only case %s has runtime scenario %q",
				productionCase.ID,
				evidence.RuntimeScenario,
			)
		}
		if productionCase.Proof == ProofRuntime &&
			len(evidence.Components) != 0 {
			t.Fatalf(
				"runtime-only case %s has component proofs",
				productionCase.ID,
			)
		}
	}
}

func TestBMM002PublicBoundaryCasesHaveDistinctRuntimeScenarios(t *testing.T) {
	cases, err := BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	scenarios := make(map[string]string, len(cases))
	for _, productionCase := range cases {
		evidence, err := EvidenceFor(productionCase)
		if err != nil {
			t.Fatal(err)
		}
		scenarios[productionCase.ID] = evidence.RuntimeScenario
	}
	tests := map[string]struct {
		caseID string
		want   string
	}{
		"acquired collector abort": {
			caseID: "F04.2",
			want:   "collector-acquired-abort",
		},
		"published Function abort": {
			caseID: "F04.3",
			want:   "function-publication-abort",
		},
		"header": {
			caseID: "F14.1",
			want:   "function-header-boundaries",
		},
		"body": {
			caseID: "F14.2",
			want:   "function-body-boundaries",
		},
		"raw payload": {
			caseID: "F14.3",
			want:   "function-raw-payload",
		},
		"timeout": {
			caseID: "F14.4",
			want:   "function-timeout-boundaries",
		},
		"invalid JSON": {
			caseID: "F14.5",
			want:   "function-invalid-json",
		},
		"large result": {
			caseID: "F14.13",
			want:   "function-result-boundaries",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := scenarios[test.caseID]; got != test.want {
				t.Fatalf(
					"case %s scenario=%q, want %q",
					test.caseID,
					got,
					test.want,
				)
			}
		})
	}
}
