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

func TestObservationDeviceIDChassisNormalization(t *testing.T) {
	for name, tc := range map[string]struct {
		chassisID, chassisType, baseBridgeAddress string
		want                                      string
	}{
		"plain":                {chassisID: "switch-a", chassisType: "local", want: "local:switch-a"},
		"chassis whitespace":   {chassisID: " \tswitch-a\n", chassisType: "local", want: "local:switch-a"},
		"type whitespace":      {chassisID: "switch-a", chassisType: " local\t", want: "local:switch-a"},
		"internal whitespace":  {chassisID: " Switch A ", chassisType: "local", want: "local:Switch A"},
		"empty chassis":        {chassisType: "local", want: "sysname:switch-a"},
		"blank chassis":        {chassisID: " \t", chassisType: "local", want: "sysname:switch-a"},
		"bridge MAC preferred": {chassisID: " switch-a ", chassisType: "local", baseBridgeAddress: "02:00:00:00:00:01", want: "macAddress:02:00:00:00:00:01"},
	} {
		t.Run(name, func(t *testing.T) {
			device := Device{ChassisID: tc.chassisID, ChassisIDType: tc.chassisType, SysName: "switch-a"}
			require.Equal(t, tc.want, ObservationDeviceID(device, tc.baseBridgeAddress))
		})
	}
}
