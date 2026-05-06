// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildFDBCandidates_KeepsDistinctVLANScopedCandidates(t *testing.T) {
	candidates := buildFDBCandidates([]FDBObservation{
		{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "7",
			Status:     "learned",
			VLANID:     "100",
		},
		{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "7",
			Status:     "learned",
			VLANID:     "100\x00vlan:200",
		},
	}, nil)

	require.Len(t, candidates, 2)
	require.Equal(t, "100", candidates[0].vlanID)
	require.Equal(t, "100\x00vlan:200", candidates[1].vlanID)
}

func TestBuildFDBCandidates_DoesNotReintroduceDuplicateEndpoint(t *testing.T) {
	candidates := buildFDBCandidates([]FDBObservation{
		{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "1",
			Status:     "learned",
		},
		{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "2",
			Status:     "learned",
		},
		{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "3",
			Status:     "learned",
		},
	}, map[string]int{
		"1": 1,
		"2": 2,
		"3": 3,
	})

	require.Empty(t, candidates)
}
