// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"net/netip"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/stretchr/testify/require"
)

func TestResolveDeviceActorType_MapsAndFallsBack(t *testing.T) {
	require.Equal(t, "switch", resolveDeviceActorType(map[string]string{"type": "switch"}))
	require.Equal(t, "access_point", resolveDeviceActorType(map[string]string{"type": "Wireless"}))
	require.Equal(t, "device", resolveDeviceActorType(map[string]string{"type": "unknown-kind"}))
	require.Equal(t, "device", resolveDeviceActorType(nil))
	require.True(t, IsDeviceActorType("switch"))
	require.True(t, IsDeviceActorType(" access_point "))
	require.False(t, IsDeviceActorType("unknown-kind"))
}

func TestAdjacencySideToEndpoint_FallsBackToRequestedPort(t *testing.T) {
	addr := netip.MustParseAddr("10.0.0.1")
	dev := model.Device{
		ID:        "switch-a",
		Hostname:  "switch-a",
		SysObject: "1.2.3.4",
		ChassisID: "00:11:22:33:44:55",
		Addresses: []netip.Addr{addr},
	}

	endpoint := adjacencySideToEndpoint(dev, "Gi0/9", nil, nil)
	require.Equal(t, []string{"00:11:22:33:44:55"}, endpoint.Match.ChassisIDs)
	require.Equal(t, []string{"00:11:22:33:44:55"}, endpoint.Match.MacAddresses)
	require.Equal(t, []string{"10.0.0.1"}, endpoint.Match.IPAddresses)
	require.Zero(t, endpoint.IfIndex)
	require.Equal(t, "Gi0/9", endpoint.IfName)
	require.Equal(t, "Gi0/9", endpoint.PortID)
	require.Equal(t, "switch-a", endpoint.SysName)
	require.Equal(t, "10.0.0.1", endpoint.ManagementIP)
	require.Empty(t, endpoint.IfDescr)
}
