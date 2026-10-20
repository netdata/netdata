// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreparedFunctionFrameTransfersExactlyOnce(t *testing.T) {
	result, err := NewSealedResult(200, "application/json", []byte(`{"ok":true}`))
	require.NoError(t, err)
	frame, err := PrepareFrame("u-linear", result, 1)
	require.NoError(t, err)
	alias := frame
	var output bytes.Buffer
	owner, err := NewFrameOwner(&output)
	require.NoError(t, err)

	require.NoError(t, owner.Commit(frame))

	require.ErrorIs(t, owner.Commit(alias), ErrPreparedFrameConsumed)

	got := bytes.Count(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN"))
	require.EqualValues(t, 1, got)
}

func TestPreparedProtocolFrameCommitOrAbortIsLinear(t *testing.T) {
	tests := map[string]struct {
		commit bool
	}{
		"commit": {commit: true},
		"abort":  {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := PrepareProtocolFrame([]byte("HOST_DEFINE node\n\n"))
			require.NoError(t, err)
			alias := frame
			if !test.commit {
				require.NoError(t, frame.Abort())

				owner, err := NewFrameOwner(&bytes.Buffer{})
				require.NoError(t, err)

				require.ErrorIs(t, owner.CommitPreparedProtocolFrame(alias), ErrPreparedFrameConsumed)

				return
			}

			var output bytes.Buffer
			owner, err := NewFrameOwner(&output)
			require.NoError(t, err)

			require.NoError(t, owner.CommitPreparedProtocolFrame(frame))

			require.ErrorIs(t, alias.Abort(), ErrPreparedFrameConsumed)

			got := output.String()
			require.EqualValues(t, "HOST_DEFINE node\n\n", got)
		})
	}
}
