// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"errors"
	"strings"
	"testing"
)

func TestInfrastructureAttribution(t *testing.T) {
	t.Run("setup failure before completion", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		t.Run("uncompleted setup", func(t *testing.T) {
			trackInfrastructureSetup(t, ledger, "fixture/setup")
		})

		assertInfrastructurePhases(t, ledger, []string{"fixture/setup"})
	})

	t.Run("query failure after clean setup is not infrastructure", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		failed := false
		tracker := newInfrastructureSetupTracker(
			ledger, "fixture/setup", func() bool { return failed })

		tracker.complete()
		failed = true
		tracker.finish()

		assertInfrastructurePhases(t, ledger, nil)
	})

	t.Run("setup completion observes an existing failure", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		tracker := newInfrastructureSetupTracker(
			ledger, "fixture/setup", func() bool { return true })

		tracker.complete()
		tracker.finish()

		assertInfrastructurePhases(t, ledger, []string{"fixture/setup"})
	})

	t.Run("skipped setup is not an infrastructure failure", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		t.Run("skipped setup", func(t *testing.T) {
			trackInfrastructureSetup(t, ledger, "fixture/setup")
			t.Skip("intentional setup skip")
		})

		assertInfrastructurePhases(t, ledger, nil)
	})

	t.Run("shutdown executes once and preserves its error", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		calls := 0
		if err := ledger.run("fixture/clean-shutdown", func() error {
			calls++
			return nil
		}); err != nil {
			t.Fatalf("clean shutdown: %v", err)
		}

		wantErr := errors.New("shutdown failed")
		gotErr := ledger.run("fixture/failed-shutdown", func() error {
			calls++
			return wantErr
		})
		if gotErr != wantErr {
			t.Fatalf("shutdown error = %v, want exact error %v", gotErr, wantErr)
		}
		if calls != 2 {
			t.Fatalf("shutdown calls = %d, want 2", calls)
		}
		assertInfrastructurePhases(t, ledger, []string{"fixture/failed-shutdown"})
	})

	t.Run("report is sorted and deduplicated", func(t *testing.T) {
		ledger := newInfrastructureLedger()
		ledger.record("fixture-b/shutdown")
		ledger.record("fixture-a/setup")
		ledger.record("fixture-b/shutdown")

		report, failed := ledger.summary()
		if !failed {
			t.Fatal("infrastructure summary reported no failure")
		}
		const want = "\nquery corpus infrastructure: 2 phase(s) FAILED\n" +
			"  FAILED  fixture-a/setup\n" +
			"  FAILED  fixture-b/shutdown\n" +
			"\nInfrastructure failures are not query-engine contract verdicts.\n"
		if report != want {
			t.Fatalf("infrastructure report:\n%s\nwant:\n%s", report, want)
		}
	})

	t.Run("query and infrastructure verdicts stay independent", func(t *testing.T) {
		contracts := newContractLedger()
		cases := map[string]ManifestCase{
			"broken-query": {},
			"not-run":      {},
		}
		contracts.record("broken-query", defaultContractComponent, false, false)
		contractReport, _ := formatContractSummary(contracts.summarize(cases), len(cases), true)

		infrastructure := newInfrastructureLedger()
		infrastructure.record("fixture/setup")
		infrastructureReport, _ := infrastructure.summary()
		report := contractReport + infrastructureReport

		for _, want := range []string{
			"BROKEN  broken-query",
			"NOT RUN  not-run",
			"FAILED  fixture/setup",
		} {
			if !strings.Contains(report, want) {
				t.Fatalf("combined report lacks %q:\n%s", want, report)
			}
		}
		for _, reject := range []string{
			"BROKEN  not-run",
			"BROKEN  fixture/setup",
		} {
			if strings.Contains(report, reject) {
				t.Fatalf("combined report falsely contains %q:\n%s", reject, report)
			}
		}
	})
}

func assertInfrastructurePhases(t *testing.T, ledger *infrastructureLedger, want []string) {
	t.Helper()

	got := ledger.phases()
	if len(got) != len(want) {
		t.Fatalf("infrastructure phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("infrastructure phases = %v, want %v", got, want)
		}
	}
}
