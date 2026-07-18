//go:build !windows

package jobmgrtest

import (
	"context"
	"testing"
)

func TestShippedRootCaseMapIsPredicateExact(t *testing.T) {
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

func TestShippedRootScenarioMatrixHasExactRootScenarioProduct(t *testing.T) {
	observed := shippedRootMatrixRuns()
	if len(observed) != 12 {
		t.Fatalf("shipped-root executions=%d, want 12", len(observed))
	}
	seen := make(map[shippedRootRun]struct{}, len(observed))
	for _, run := range observed {
		if _, exists := seen[run]; exists {
			t.Fatalf("duplicate shipped-root scenario: %+v", run)
		}
		seen[run] = struct{}{}
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
			if _, exists := seen[run]; !exists {
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
