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
		"shutdown runs every root": {
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
	observed := make(map[shippedRootRun]struct{}, 12)
	for _, caseID := range caseIDs {
		runs, err := shippedRootRuns(caseID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			if _, ok := observed[run]; ok {
				t.Fatalf("duplicate shipped-root scenario: %+v", run)
			}
			observed[run] = struct{}{}
		}
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
			"shutdown",
		} {
			run := shippedRootRun{
				root: root, scenario: scenario,
			}
			if _, ok := observed[run]; !ok {
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
			published, withdrawn := observation.snapshot()
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
				func(publications, withdrawals int) bool {
					return publications == test.wantPublished &&
						withdrawals == test.wantWithdrawals
				},
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}
