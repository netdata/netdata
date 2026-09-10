// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"reflect"
	"strings"
	"testing"
)

func TestManifestLoaderPreservesFields(t *testing.T) {
	const data = `[{"name":"case","proves":"contract","cloud":"green","fixed_by":"#1","components":["a","b"]}]`
	cases, err := loadManifest([]byte(data))
	if err != nil {
		t.Fatal(err)
	}

	want := ManifestCase{
		Proves:     "contract",
		Cloud:      "green",
		FixedBy:    "#1",
		Components: []string{"a", "b"},
	}
	if got := cases["case"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded manifest case = %#v, want %#v", got, want)
	}
}

func TestManifestLoaderRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid JSON", data: `{`, want: "decode query corpus manifest"},
		{name: "no contracts", data: `[]`, want: "has no contracts"},
		{name: "empty name", data: `[{"name":""}]`, want: "empty contract name"},
		{name: "duplicate name", data: `[{"name":"same"},{"name":"same"}]`, want: "repeats contract"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadManifest([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadManifest() error = %v, want text %q", err, tc.want)
			}
		})
	}
}

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

func TestContractLedgerRegistrationRemainsIncompleteUntilVerdict(t *testing.T) {
	cases := map[string]ManifestCase{
		"registered": {},
	}
	ledger := newContractLedger()
	ledger.register("registered", defaultContractComponent)

	summary := ledger.summarize(cases)
	if summary.evaluated != 0 || len(summary.broken) != 0 {
		t.Fatalf("before verdict: evaluated=%d broken=%v, want 0/none", summary.evaluated, summary.broken)
	}
	if len(summary.incomplete) != 1 || summary.incomplete[0] != "registered" {
		t.Fatalf("before verdict: incomplete=%v, want [registered]", summary.incomplete)
	}

	ledger.record("registered", defaultContractComponent, true, false)
	summary = ledger.summarize(cases)
	if summary.evaluated != 1 || len(summary.broken) != 0 || len(summary.incomplete) != 0 {
		t.Fatalf("after verdict: evaluated=%d broken=%v incomplete=%v, want 1/none/none",
			summary.evaluated, summary.broken, summary.incomplete)
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

func TestWeightsLimitContractsRequireEveryMethodOptions(t *testing.T) {
	for _, name := range []string{"W/limit-ranking", "W/limit-summaries"} {
		for _, method := range []string{"value", "volume", "ks2", "anomaly-rate"} {
			for _, options := range []string{"raw", "null2zero", "raw|anomaly-bit", "null2zero|anomaly-bit"} {
				missing := method + "/" + options
				t.Run(name+"/"+missing, func(t *testing.T) {
					ledger := newContractLedger()
					for _, component := range requiredContractComponents(manifest[name]) {
						if component != missing {
							ledger.record(name, component, true, false)
						}
					}
					summary := ledger.summarize(map[string]ManifestCase{name: manifest[name]})
					want := []string{name + "/" + missing}
					if summary.evaluated != 0 || !reflect.DeepEqual(summary.incomplete, want) {
						t.Fatalf("omitted %s: evaluated=%d incomplete=%v, want 0/%v",
							missing, summary.evaluated, summary.incomplete, want)
					}
				})
			}
		}
	}
}
