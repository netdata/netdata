// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTopologyObservationDeviceID_PrefersAgentScopedFallbacks(t *testing.T) {
	require.Equal(t, "agent_job:job-1", ensureTopologyObservationDeviceID(topologyDevice{AgentJobID: "Job-1"}, ""))
	require.Equal(t, "agent:11111111-1111-1111-1111-111111111111", ensureTopologyObservationDeviceID(topologyDevice{
		NetdataHostID: "11111111-1111-1111-1111-111111111111",
	}, ""))
	require.Equal(t, "agent:22222222-2222-2222-2222-222222222222", ensureTopologyObservationDeviceID(topologyDevice{
		AgentID: "22222222-2222-2222-2222-222222222222",
	}, ""))
}
