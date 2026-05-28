// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalKeyHelpers_DeduplicateAndNormalizeDeterministically(t *testing.T) {
	require.Equal(t,
		"10.20.4.60,alpha",
		canonicalIPListKey([]string{"alpha", "10.20.4.60", "0A14043C", "ALPHA"}),
	)
	require.Equal(t,
		"00:11:22:33:44:55,10.20.4.60,chassis-a",
		canonicalHardwareListKey([]string{"chassis-a", "00:11:22:33:44:55", "0A14043C", "001122334455"}),
	)
	require.Equal(t,
		"edge-a,edge-b",
		canonicalStringListKey([]string{"edge-b", " Edge-A ", "edge-a"}),
	)
}
