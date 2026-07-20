//go:build !windows

package jobmgrtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
				require.NoError(t, observation.observe([]byte(chunk), 0))
			}
			published, withdrawn, _ := observation.snapshot()
			require.Equal(t, test.wantPublished, published)
			require.Equal(t, test.wantWithdrawals, withdrawn)
			require.NoError(t, observation.wait(
				context.Background(),
				func(publications, withdrawals, _ int) bool {
					return publications == test.wantPublished &&
						withdrawals == test.wantWithdrawals
				},
			))
		})
	}
}
