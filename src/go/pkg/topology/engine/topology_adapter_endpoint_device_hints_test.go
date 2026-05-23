// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterManagedDeviceHints_ReturnsNilWhenNoManagedHintMatches(t *testing.T) {
	filtered := filterManagedDeviceHints(
		[]string{"ghost-switch", "unmanaged-switch"},
		map[string]struct{}{"managed-switch": {}},
	)

	require.Nil(t, filtered)
}
