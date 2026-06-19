// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestTopologyObservationIdentityCanonicalHelpers(t *testing.T) {
	tests := map[string]struct {
		normalize func(string) string
		in        string
		want      string
	}{
		"host":              {normalize: canonicalObservationHost, in: " Edge-A ", want: "edge-a"},
		"chassis-mac":       {normalize: canonicalObservationChassis, in: "001122334455", want: "00:11:22:33:44:55"},
		"chassis-text":      {normalize: canonicalObservationChassis, in: "Chassis-A", want: "chassis-a"},
		"mac":               {normalize: canonicalObservationMAC, in: "00-11-22-33-44-55", want: "00:11:22:33:44:55"},
		"ip-hex":            {normalize: canonicalObservationIP, in: "0A14043C", want: "10.20.4.60"},
		"ip-empty-rejected": {normalize: canonicalObservationIP, in: "", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.normalize(tc.in))
		})
	}
}

func TestTopologyObservationIdentityResolver_AssignsDeterministicFallbackIDs(t *testing.T) {
	resolver := newTopologyObservationIdentityResolver(topologyengine.L2Observation{
		DeviceID:     "local-device",
		Hostname:     "sw-a",
		ChassisID:    "00:11:22:33:44:55",
		ManagementIP: "10.0.0.1",
	})

	first := resolver.resolve(nil, "", "", "")
	second := resolver.resolve(nil, "", "", "")

	require.Equal(t, "remote-device-1", first)
	require.Equal(t, "remote-device-2", second)
}

func TestTopologyObservationIdentityResolver_ReusesFallbackIdentityAcrossCanonicalSignals(t *testing.T) {
	resolver := newTopologyObservationIdentityResolver(topologyengine.L2Observation{
		DeviceID:     "local-device",
		Hostname:     "sw-a",
		ChassisID:    "00:11:22:33:44:55",
		ManagementIP: "10.0.0.1",
	})

	id := resolver.resolve([]string{"Edge-A"}, "chassis-a", "local", "10.20.4.60")

	require.Equal(t, id, resolver.resolve([]string{" edge-a "}, "", "", ""))
	require.Equal(t, id, resolver.resolve(nil, "CHASSIS-A", "local", ""))
	require.Equal(t, id, resolver.resolve(nil, "", "", "10.20.4.60"))
}

func TestTopologyObservationIdentityResolver_RegistersAliasesWhenExistingIdentityIsReused(t *testing.T) {
	resolver := newTopologyObservationIdentityResolver(topologyengine.L2Observation{
		DeviceID:     "local-device",
		Hostname:     "sw-a",
		ChassisID:    "00:11:22:33:44:55",
		ManagementIP: "10.0.0.1",
	})

	id := resolver.resolve(nil, "", "", "10.20.4.60")
	require.Equal(t, id, resolver.resolve([]string{"Edge-A"}, "", "", "10.20.4.60"))
	require.Equal(t, id, resolver.resolve([]string{" edge-a "}, "", "", ""))
}
