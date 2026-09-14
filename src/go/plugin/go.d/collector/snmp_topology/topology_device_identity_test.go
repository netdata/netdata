// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/stretchr/testify/require"
)

func TestTopologyProfileChassisIdentityNormalization(t *testing.T) {
	for name, tc := range map[string]struct {
		chassisID string
		metadata  bool
	}{
		"plain metadata":  {chassisID: "switch-a", metadata: true},
		"padded metadata": {chassisID: " \tswitch-a\n", metadata: true},
		"plain tag":       {chassisID: "switch-a"},
		"padded tag":      {chassisID: " \tswitch-a\n"},
	} {
		t.Run(name, func(t *testing.T) {
			builder := newTopologyBuilderFromSemanticInput(
				topologydiag.DeviceInput{Hostname: "192.0.2.1"}, nil, topologyScenarioCollectedAt, time.Minute,
			)
			profile := &ddsnmp.ProfileMetrics{}
			if tc.metadata {
				profile.DeviceMetadata = map[string]ddsnmp.MetaTag{
					tagLldpLocChassisID:        {Value: tc.chassisID},
					tagLldpLocChassisIDSubtype: {Value: "7"},
				}
			} else {
				profile.Tags = map[string]string{
					tagLldpLocChassisID:        tc.chassisID,
					tagLldpLocChassisIDSubtype: "7",
				}
			}
			builder.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{profile})
			observation, ok := builder.buildObservationSnapshot()
			require.True(t, ok)
			require.Equal(t, "local:switch-a", observation.LocalDeviceID)
			require.Len(t, observation.L2Observations, 1)
			require.Equal(t, "switch-a", observation.L2Observations[0].ChassisID)
			require.Equal(t, observation.LocalDeviceID, observation.L2Observations[0].DeviceID)
		})
	}
}
