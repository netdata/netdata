// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/stretchr/testify/require"
)

func TestTopologyObservationIdentityCanonicalHelpers(t *testing.T) {
	require.Equal(t, "edge-a", canonicalObservationHost(" Edge-A "))
	require.Equal(t, "00:11:22:33:44:55", canonicalObservationChassis("001122334455"))
	require.Equal(t, "chassis-a", canonicalObservationChassis("Chassis-A"))
	require.Equal(t, "00:11:22:33:44:55", canonicalObservationMAC("00-11-22-33-44-55"))
	require.Equal(t, "10.20.4.60", canonicalObservationIP("0A14043C"))
	require.Equal(t, "", canonicalObservationIP(""))
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
