// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestInferTopologyVendorFromMatch(t *testing.T) {
	match := graph.Match{
		MacAddresses: []string{
			"28:6f:b9:00:00:22",
			"08:ea:44:11:22:33",
		},
		ChassisIDs: []string{
			"28:6f:b9:00:00:22",
			"08ea.4411.2233",
		},
	}

	vendor, prefix := inferTopologyVendorFromMatch(match)

	require.Equal(t, "Extreme Networks Headquarters", vendor)
	require.Equal(t, "08EA44", prefix)
}
