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
