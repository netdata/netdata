// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetainedOwnershipClassificationPreservesCauseAndIdentity(t *testing.T) {
	sentinel := errors.New("ownership is ambiguous")
	retained := RetainOwnership(sentinel)

	require.ErrorIs(t, retained, sentinel)
	require.True(t, OwnershipRetained(retained))
	require.Same(t, retained, RetainOwnership(retained))
	require.Nil(t, RetainOwnership(nil))
	require.False(t, OwnershipRetained(sentinel))
}
