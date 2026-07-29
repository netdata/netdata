// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainsOnlyErrorLeavesFailsClosed(t *testing.T) {
	unexpected := errors.New("unexpected")
	tests := map[string]struct {
		err  error
		want bool
	}{
		"nil": {},
		"single allowed": {
			err:  ErrProcessAttemptRetired,
			want: true,
		},
		"wrapped allowed": {
			err:  fmt.Errorf("wrapped: %w", ErrProcessAttemptStopped),
			want: true,
		},
		"joined allowed": {
			err: errors.Join(
				ErrProcessAttemptRetired,
				fmt.Errorf("wrapped: %w", ErrProcessAttemptStopped),
			),
			want: true,
		},
		"mixed": {
			err: errors.Join(ErrProcessAttemptRetired, unexpected),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(
				t,
				test.want,
				ContainsOnlyErrorLeaves(
					test.err,
					ErrProcessAttemptRetired,
					ErrProcessAttemptStopped,
				),
			)
		})
	}
}

func TestContainsOnlyErrorLeavesBoundsTotalTraversal(t *testing.T) {
	unwraps := 0
	var tree error = ErrProcessAttemptRetired
	for range 6 {
		tree = sharedProcessAttemptErrorTree{
			child:   tree,
			unwraps: &unwraps,
		}
	}

	require.False(t, ContainsOnlyErrorLeaves(tree, ErrProcessAttemptRetired))
	require.LessOrEqual(t, unwraps, 32)
}

type sharedProcessAttemptErrorTree struct {
	child   error
	unwraps *int
}

func (sharedProcessAttemptErrorTree) Error() string {
	return "shared process attempt error tree"
}

func (tree sharedProcessAttemptErrorTree) Unwrap() []error {
	(*tree.unwraps)++
	return []error{tree.child, tree.child}
}
