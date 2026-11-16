// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestLifecycleErrorSanitizerFailsClosed(t *testing.T) {
	source := errors.New("source failure")
	require.ErrorIs(t, sanitizeLifecycleError(nil, source), source)
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { return nil }, source), "discarded")
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { panic("failed") }, source), "panicked")
}

func TestLifecycleSanitizerPreservesRetryDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	source := collectorapi.WithRetryNotBefore(errors.New("synthetic secret"), deadline)
	for name, sanitize := range map[string]func(error) error{
		"replacement": func(error) error { return errors.New("redacted") },
		"discard":     func(error) error { return nil },
		"panic":       func(error) error { panic("untrusted diagnostic") },
	} {
		t.Run(name, func(t *testing.T) {
			safe := sanitizeLifecycleError(sanitize, source)
			require.Equal(t, deadline, collectorapi.RetryNotBefore(safe))
			require.NotContains(t, safe.Error(), "synthetic secret")
			require.False(t, errors.Is(safe, source))
			failure := newRunFailure(source, "error", sanitize)
			require.Equal(t, deadline, collectorapi.RetryNotBefore(failure))
			require.True(t, failure.Retryable())
		})
	}
}
