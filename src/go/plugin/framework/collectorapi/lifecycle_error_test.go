// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleErrorClassification(t *testing.T) {
	type classification struct {
		Message string
		Cause   bool
		Class   LifecycleErrorClass
	}
	cause := errors.New("unknown profile")
	tests := map[string]struct {
		err  error
		want classification
	}{
		"unclassified error": {
			err: fmt.Errorf("check: %w", cause),
			want: classification{
				Message: "check: unknown profile",
				Cause:   true,
				Class:   LifecycleErrorUnclassified,
			},
		},
		"permanent error": {
			err: fmt.Errorf("check: %w", PermanentError(cause)),
			want: classification{
				Message: "check: unknown profile",
				Cause:   true,
				Class:   LifecycleErrorPermanent,
			},
		},
		"temporary error": {
			err: fmt.Errorf("check: %w", TemporaryError(cause)),
			want: classification{
				Message: "check: unknown profile",
				Cause:   true,
				Class:   LifecycleErrorTemporary,
			},
		},
		"permanent wins over temporary in a joined tree": {
			err: errors.Join(TemporaryError(errors.New("dependency not ready")), PermanentError(cause)),
			want: classification{
				Message: "dependency not ready\nunknown profile",
				Cause:   true,
				Class:   LifecycleErrorPermanent,
			},
		},
		"permanent wins over an enclosing temporary": {
			err: TemporaryError(fmt.Errorf("check: %w", PermanentError(cause))),
			want: classification{
				Message: "check: unknown profile",
				Cause:   true,
				Class:   LifecycleErrorPermanent,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, classification{
				Message: test.err.Error(),
				Cause:   errors.Is(test.err, cause),
				Class:   ClassifyLifecycleError(test.err),
			})
		})
	}
}

func TestLifecycleErrorConstructorsKeepNil(t *testing.T) {
	require.NoError(t, PermanentError(nil))
	require.NoError(t, TemporaryError(nil))
	require.Equal(t, LifecycleErrorUnclassified, ClassifyLifecycleError(nil))
}
