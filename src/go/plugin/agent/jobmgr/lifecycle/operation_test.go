// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperationRequiredDeadlineStartIsNonterminal(t *testing.T) {
	operation, err := NewOperation(1, "uid", SourceFunction, "lane", true)
	require.NoError(t, err)

	require.NoError(t, operation.RequireDeadlineStart())

	require.False(t, operation.Child != ChildDeadlineStartPending || operation.CanDisposeTerminal())
	ref := TaskRef{Slot: 0, Generation: 1}

	require.NoError(t, operation.StartChild(ref))

	require.False(t, operation.Child != ChildExecuting || operation.Task != ref)
}

func TestOperationAbandonedDeadlineStartBecomesTerminal(t *testing.T) {
	operation, err := NewOperation(1, "uid", SourceFunction, "lane", true)
	require.NoError(t, err)

	require.NoError(t, operation.RequireDeadlineStart())

	require.NoError(t, operation.AbandonDeadlineStart())

	require.EqualValues(t, ChildAbandonedBeforeStart, operation.Child)

	require.NoError(t, operation.CommitResponse())

	require.True(t, operation.CanDisposeTerminal())
}

func TestOperationResponseCommitPrecedesDisposalAcknowledgement(
	t *testing.T,
) {
	operation, err := NewOperation(
		1,
		"uid",
		SourceFunction,
		"lane",
		true,
	)
	require.NoError(t, err)
	ref := TaskRef{Slot: 1, Generation: 1}

	require.NoError(t, operation.StartChild(ref))

	require.NoError(t, operation.ResultReady(ref, 1))

	require.NoError(t, operation.MarkResponsePending())

	require.NoError(t, operation.ActionPending(ref, 2))

	require.NoError(t, operation.ActionAcknowledged(ref, 2))

	require.NoError(t, operation.TerminationPending(ref, 3))

	require.NoError(t, operation.ChildExited(ref, 3))

	require.False(t, operation.CanDisposeTerminal())

	require.NoError(t, operation.CommitResponse())

	require.True(t, operation.CanDisposeTerminal())
}

func TestOperationResponseTerminalStatesRemainTerminalWhenPoisoned(t *testing.T) {
	tests := map[string]struct {
		response ResponseState
		want     ResponseState
	}{
		"open becomes poisoned": {
			response: ResponseOpen,
			want:     ResponsePoisoned,
		},
		"pending becomes poisoned": {
			response: ResponsePending,
			want:     ResponsePoisoned,
		},
		"committed remains committed": {
			response: ResponseCommitted,
			want:     ResponseCommitted,
		},
		"not required remains not required": {
			response: ResponseNotRequired,
			want:     ResponseNotRequired,
		},
		"poisoned remains poisoned": {
			response: ResponsePoisoned,
			want:     ResponsePoisoned,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			operation := &OperationGeneration{Response: test.response}

			operation.PoisonResponse()

			require.Equal(t, test.want, operation.Response)
		})
	}
}
