// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"strings"
	"testing"
)

func TestContractLedgerDeduplicatesAndKeepsFailuresSticky(t *testing.T) {
	cases := map[string]ManifestCase{
		"single": {},
	}
	ledger := newContractLedger()

	ledger.record("single", defaultContractComponent, true, false)
	ledger.record("single", defaultContractComponent, false, false)
	ledger.record("single", defaultContractComponent, true, false)

	summary := ledger.summarize(cases)
	if summary.evaluated != 1 {
		t.Fatalf("evaluated=%d, want 1", summary.evaluated)
	}
	if len(summary.broken) != 1 || summary.broken[0] != "single" {
		t.Fatalf("broken=%v, want [single]", summary.broken)
	}
	if len(summary.incomplete) != 0 {
		t.Fatalf("incomplete=%v, want none", summary.incomplete)
	}
}

func TestContractLedgerRequiresEveryComponent(t *testing.T) {
	cases := map[string]ManifestCase{
		"composite": {Components: []string{"first", "second"}},
	}
	ledger := newContractLedger()

	ledger.record("composite", "first", true, false)
	summary := ledger.summarize(cases)
	if summary.evaluated != 0 {
		t.Fatalf("evaluated=%d, want 0", summary.evaluated)
	}
	if len(summary.incomplete) != 1 || summary.incomplete[0] != "composite/second" {
		t.Fatalf("incomplete=%v, want [composite/second]", summary.incomplete)
	}

	ledger.record("composite", "second", true, false)
	summary = ledger.summarize(cases)
	if summary.evaluated != 1 || len(summary.incomplete) != 0 {
		t.Fatalf("after second component: evaluated=%d incomplete=%v", summary.evaluated, summary.incomplete)
	}
}

func TestContractLedgerDoesNotCountSkippedScopeAsEvaluated(t *testing.T) {
	cases := map[string]ManifestCase{
		"skipped": {},
	}
	ledger := newContractLedger()
	ledger.record("skipped", defaultContractComponent, true, true)

	summary := ledger.summarize(cases)
	if summary.evaluated != 0 {
		t.Fatalf("evaluated=%d, want 0", summary.evaluated)
	}
	if len(summary.broken) != 0 {
		t.Fatalf("broken=%v, want none", summary.broken)
	}
	if len(summary.incomplete) != 1 || summary.incomplete[0] != "skipped" {
		t.Fatalf("incomplete=%v, want [skipped]", summary.incomplete)
	}
}

func TestContractLedgerKeepsFailureBeforeSkip(t *testing.T) {
	cases := map[string]ManifestCase{
		"failed-then-skipped": {},
	}
	ledger := newContractLedger()
	ledger.record("failed-then-skipped", defaultContractComponent, false, true)

	summary := ledger.summarize(cases)
	if summary.evaluated != 1 {
		t.Fatalf("evaluated=%d, want 1", summary.evaluated)
	}
	if len(summary.broken) != 1 || summary.broken[0] != "failed-then-skipped" {
		t.Fatalf("broken=%v, want [failed-then-skipped]", summary.broken)
	}
	if len(summary.incomplete) != 0 {
		t.Fatalf("incomplete=%v, want none", summary.incomplete)
	}
}

func TestIncompleteContractSummaryNeverClaimsAllHold(t *testing.T) {
	summary := contractRunSummary{
		evaluated:  1,
		incomplete: []string{"not-run"},
	}

	report, complete := formatContractSummary(summary, 2, true)
	if complete {
		t.Fatal("incomplete summary reported complete")
	}
	if strings.Contains(report, "all 2 contracts hold") {
		t.Fatalf("incomplete summary claimed all contracts hold:\n%s", report)
	}
	if !strings.Contains(report, "1 of 2 contracts fully evaluated") ||
		!strings.Contains(report, "NOT RUN  not-run") {
		t.Fatalf("incomplete summary lacks coverage evidence:\n%s", report)
	}
}

func TestManifestComponentsAreValid(t *testing.T) {
	for name, mc := range manifest {
		seen := map[string]bool{}
		for _, component := range mc.Components {
			if component == "" {
				t.Errorf("%s has an empty component", name)
			}
			if component == defaultContractComponent {
				t.Errorf("%s uses reserved component name %q", name, component)
			}
			if seen[component] {
				t.Errorf("%s repeats component %q", name, component)
			}
			seen[component] = true
		}
	}
}
