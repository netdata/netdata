// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExcerptReadingsPreservesBaseDocumentPath(t *testing.T) {
	node := &Resource{
		Kind: "fan",
		Data: map[string]any{
			"PowerWatts": map[string]any{"Reading": 10, "Status": map[string]any{"Health": "OK"}},
		},
	}
	readings := excerptReadings(node)
	require.Len(t, readings, 1)
	require.Equal(t, "fan.PowerWatts.Reading", readings[0].Path)
	require.Equal(t, "power", readings[0].Role)
}
