// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceStoreDevicesByHostname(t *testing.T) {
	reg := NewDeviceStore()
	reg.Register("switch-a", DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		SysName:        "switch-a",
		ManualProfiles: []string{"profile-a"},
		VnodeLabels:    map[string]string{"site": "lab"},
	})

	devices := reg.DevicesByHostname("::ffff:192.0.2.10")
	require.Len(t, devices, 1)
	dev := devices[0]
	require.Equal(t, "switch-a", dev.SysName)

	dev.ManualProfiles[0] = "changed"
	dev.VnodeLabels["site"] = "changed"

	again := reg.DevicesByHostname("192.0.2.10")
	require.Len(t, again, 1)
	require.Equal(t, []string{"profile-a"}, again[0].ManualProfiles)
	require.Equal(t, "lab", again[0].VnodeLabels["site"])
}

func TestDeviceStoreDevicesByHostnameNoMatch(t *testing.T) {
	reg := NewDeviceStore()
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "switch-a.example.com"})

	require.Empty(t, reg.DevicesByHostname("switch-b.example.com"))
}

func TestDeviceStoreDevicesByHostnameMatchesDNSCaseInsensitive(t *testing.T) {
	reg := NewDeviceStore()
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "Switch-A.Example.COM"})

	require.Len(t, reg.DevicesByHostname("switch-a.example.com"), 1)
}

func TestDeviceStoreDevicesByHostnameIndexUpdatesOnRegisterAndUnregister(t *testing.T) {
	reg := NewDeviceStore()
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-a"})

	devices := reg.DevicesByHostname("192.0.2.10")
	require.Len(t, devices, 1)
	dev := devices[0]
	require.Equal(t, "switch-a", dev.SysName)

	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.11", SysName: "switch-a-renumbered"})

	require.Empty(t, reg.DevicesByHostname("192.0.2.10"))
	devices = reg.DevicesByHostname("192.0.2.11")
	require.Len(t, devices, 1)
	dev = devices[0]
	require.Equal(t, "switch-a-renumbered", dev.SysName)

	reg.Unregister("switch-a")
	require.Empty(t, reg.DevicesByHostname("192.0.2.11"))
}

func TestDeviceStoreDevicesByHostnameReturnsAllMatches(t *testing.T) {
	reg := NewDeviceStore()
	reg.Register("switch-b", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-b"})
	reg.Register("switch-a", DeviceConnectionInfo{Hostname: "::ffff:192.0.2.10", SysName: "switch-a"})

	devices := reg.DevicesByHostname("192.0.2.10")
	require.Len(t, devices, 2)
	require.Equal(t, "switch-a", devices[0].SysName)
	require.Equal(t, "switch-b", devices[1].SysName)

	devices[0].SysName = "changed"
	again := reg.DevicesByHostname("192.0.2.10")
	require.Equal(t, "switch-a", again[0].SysName)
}
