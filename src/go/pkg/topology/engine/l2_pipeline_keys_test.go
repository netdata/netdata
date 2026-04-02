// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdjacencyKey_NormalizesProtocolAndEndpointWhitespace(t *testing.T) {
	raw := Adjacency{
		Protocol:   " LLDP ",
		SourceID:   " sw1 ",
		SourcePort: " Gi0/1 ",
		TargetID:   " sw2 ",
		TargetPort: " Gi0/2 ",
	}

	normalized := Adjacency{
		Protocol:   "lldp",
		SourceID:   "sw1",
		SourcePort: "Gi0/1",
		TargetID:   "sw2",
		TargetPort: "Gi0/2",
	}

	require.Equal(t, adjacencyKey(normalized), adjacencyKey(raw))
}
