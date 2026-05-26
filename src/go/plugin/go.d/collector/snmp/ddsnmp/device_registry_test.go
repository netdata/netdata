// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceRegistryDeviceByHostname(t *testing.T) {
	reg := &deviceRegistry{devices: make(map[string]DeviceConnectionInfo)}
	reg.Register("switch-a", DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		SysName:        "switch-a",
		ManualProfiles: []string{"profile-a"},
		VnodeLabels:    map[string]string{"site": "lab"},
	})

	dev, ok := reg.DeviceByHostname("::ffff:192.0.2.10")
	require.True(t, ok)
	require.Equal(t, "switch-a", dev.SysName)

	dev.ManualProfiles[0] = "changed"
	dev.VnodeLabels["site"] = "changed"

	again, ok := reg.DeviceByHostname("192.0.2.10")
	require.True(t, ok)
	require.Equal(t, []string{"profile-a"}, again.ManualProfiles)
	require.Equal(t, "lab", again.VnodeLabels["site"])
}

func TestDeviceRegistryDeviceByHostnameNoMatch(t *testing.T) {
	reg := &deviceRegistry{devices: make(map[string]DeviceConnectionInfo)}
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "switch-a.example.com"})

	_, ok := reg.DeviceByHostname("switch-b.example.com")
	require.False(t, ok)
}

func TestDeviceRegistryDeviceByHostnameMatchesDNSCaseInsensitive(t *testing.T) {
	reg := &deviceRegistry{devices: make(map[string]DeviceConnectionInfo)}
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "Switch-A.Example.COM"})

	_, ok := reg.DeviceByHostname("switch-a.example.com")
	require.True(t, ok)
}

func TestDeviceRegistryDeviceByHostnameIndexUpdatesOnRegisterAndUnregister(t *testing.T) {
	reg := &deviceRegistry{devices: make(map[string]DeviceConnectionInfo)}
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-a"})

	dev, ok := reg.DeviceByHostname("192.0.2.10")
	require.True(t, ok)
	require.Equal(t, "switch-a", dev.SysName)

	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.11", SysName: "switch-a-renumbered"})

	_, ok = reg.DeviceByHostname("192.0.2.10")
	require.False(t, ok)
	dev, ok = reg.DeviceByHostname("192.0.2.11")
	require.True(t, ok)
	require.Equal(t, "switch-a-renumbered", dev.SysName)

	reg.Unregister("switch-a")
	_, ok = reg.DeviceByHostname("192.0.2.11")
	require.False(t, ok)
}
