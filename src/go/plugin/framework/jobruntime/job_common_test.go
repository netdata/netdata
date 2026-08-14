// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLifecycleErrorSanitizerFailsClosed(t *testing.T) {
	source := errors.New("source failure")
	require.ErrorIs(t, sanitizeLifecycleError(nil, source), source)
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { return nil }, source), "discarded")
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { panic("failed") }, source), "panicked")
}
