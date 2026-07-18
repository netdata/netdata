//go:build !windows

package jobmgrtest

import (
	"context"
	"testing"
)

func TestShippedRootCaseMapContainsTwelveScenarios(t *testing.T) {
	tests := map[string]struct {
		caseID string
		want   int
	}{
		"blocked-reader HUP runs every root": {
			caseID: "F06.1",
			want:   3,
		},
		"one terminal root": {
			caseID: "F24.20-b-godplugin-terminal",
			want:   1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runs, err := shippedRootRuns(test.caseID)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != test.want {
				t.Fatalf(
					"case %s runs=%d, want %d",
					test.caseID,
					len(runs),
					test.want,
				)
			}
		})
	}
}

func TestShippedRootClosureHasExactRootScenarioProduct(t *testing.T) {
	caseIDs := []string{
		"F06.1",
		"F24.20-b-godplugin-terminal",
		"F24.21-b-godplugin-nonterminal",
		"F24.22-b-godplugin-hup",
		"F24.23-b-ibmdplugin-terminal",
		"F24.24-b-ibmdplugin-nonterminal",
		"F24.25-b-ibmdplugin-hup",
		"F24.26-b-scriptsdplugin-terminal",
		"F24.27-b-scriptsdplugin-nonterminal",
		"F24.28-b-scriptsdplugin-hup",
	}
	var observed []shippedRootRun
	for _, caseID := range caseIDs {
		runs, err := shippedRootRuns(caseID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			observed = append(observed, run)
		}
	}
	if len(observed) != 12 {
		t.Fatalf("shipped-root executions=%d, want 12", len(observed))
	}
	for _, root := range []string{
		"godplugin",
		"ibmdplugin",
		"scriptsdplugin",
	} {
		for _, scenario := range []string{
			"terminal",
			"all-pipe",
			"repeated-hup",
		} {
			run := shippedRootRun{
				root: root, scenario: scenario,
			}
			found := false
			for _, candidate := range observed {
				if candidate == run {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing shipped-root scenario: %+v", run)
			}
		}
	}
}

func TestRootProtocolObservationPreservesChunkBoundaries(t *testing.T) {
	tests := map[string]struct {
		chunks          []string
		wantPublished   int
		wantWithdrawals int
	}{
		"split publication and withdrawal": {
			chunks: []string{
				`FUNCTION GLOB`,
				"AL \"config\" 30 \"help\" \"tags\" 0xFFFF 1 1\n\n" +
					`FUNCTION_DEL GLOBAL "con`,
				"fig\"\n\n",
			},
			wantPublished:   1,
			wantWithdrawals: 1,
		},
		"other routes do not count": {
			chunks: []string{
				"FUNCTION GLOBAL \"other\" 30 \"help\" \"tags\" 0xFFFF 1 1\n\n",
				"FUNCTION_DEL GLOBAL \"other\"\n\n",
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observation := newRootProtocolObservation()
			for _, chunk := range test.chunks {
				if err := observation.observe(
					[]byte(chunk),
					0,
				); err != nil {
					t.Fatal(err)
				}
			}
			published, withdrawn, _ := observation.snapshot()
			if published != test.wantPublished ||
				withdrawn != test.wantWithdrawals {
				t.Fatalf(
					"lifecycle=%d/%d, want %d/%d",
					published,
					withdrawn,
					test.wantPublished,
					test.wantWithdrawals,
				)
			}
			if err := observation.wait(
				context.Background(),
				func(publications, withdrawals, _ int) bool {
					return publications == test.wantPublished &&
						withdrawals == test.wantWithdrawals
				},
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}
