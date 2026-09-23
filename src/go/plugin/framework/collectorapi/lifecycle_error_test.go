// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleErrorClassification(t *testing.T) {
	type classification struct {
		Message   string
		Cause     bool
		Code      int
		Retryable bool
	}
	tests := map[string]struct {
		classify func(error) error
		want     classification
	}{
		"permanent error is coded 422 and not retryable": {
			classify: PermanentError,
			want: classification{
				Message:   "check: unknown profile",
				Cause:     true,
				Code:      422,
				Retryable: false,
			},
		},
		"temporary error is coded 503 and retryable": {
			classify: TemporaryError,
			want: classification{
				Message:   "check: unknown profile",
				Cause:     true,
				Code:      503,
				Retryable: true,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, test.classify(nil))

			cause := errors.New("unknown profile")
			err := fmt.Errorf("check: %w", test.classify(cause))
			coded, ok := errors.AsType[dyncfg.CodedError](err)
			require.True(t, ok)

			assert.Equal(t, test.want, classification{
				Message:   err.Error(),
				Cause:     errors.Is(err, cause),
				Code:      coded.DyncfgCode(),
				Retryable: dyncfg.IsRetryableError(err),
			})
		})
	}
}
