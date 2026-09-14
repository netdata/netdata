// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicError(t *testing.T) {
	cause := errors.New("private [REDACTED_SECRET] cause")
	err := NewPublicError("configured endpoint is unavailable", cause)

	require.Equal(t, "configured endpoint is unavailable", err.Error())
	require.NotContains(t, err.Error(), "[REDACTED_SECRET]")
	require.ErrorIs(t, err, cause)

	message, ok := PublicMessage(fmt.Errorf("outer operation: %w", err))
	require.True(t, ok)
	require.Equal(t, "configured endpoint is unavailable", message)

	message, ok = PublicMessage(cause)
	require.False(t, ok)
	require.Empty(t, message)
}

func TestPublicErrorFallback(t *testing.T) {
	cause := errors.New("private cause")
	err := NewPublicError(" ", cause)

	require.Equal(t, publicErrorFallback, err.Error())
	require.ErrorIs(t, err, cause)
	require.NoError(t, NewPublicError("unused", nil))
}
