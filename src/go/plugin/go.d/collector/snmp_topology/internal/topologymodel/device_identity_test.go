// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTopologyObservationDeviceID_PrefersAgentScopedFallbacks(t *testing.T) {
	tests := map[string]struct {
		device Device
		want   string
	}{
		"agent-job-id": {
			device: Device{AgentJobID: "Job-1"},
			want:   "agent_job:job-1",
		},
		"netdata-host-id": {
			device: Device{NetdataHostID: "11111111-1111-1111-1111-111111111111"},
			want:   "agent:11111111-1111-1111-1111-111111111111",
		},
		"agent-id": {
			device: Device{AgentID: "22222222-2222-2222-2222-222222222222"},
			want:   "agent:22222222-2222-2222-2222-222222222222",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ObservationDeviceID(tc.device, ""))
		})
	}
}
