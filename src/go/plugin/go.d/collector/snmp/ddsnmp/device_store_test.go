// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceStoreDevicesByHostname(t *testing.T) {
	store := NewDeviceStore()
	store.Register("switch-a", DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		SysName:        "switch-a",
		ManualProfiles: []string{"profile-a"},
		VnodeLabels:    map[string]string{"site": "lab"},
	})

	devices := store.DevicesByHostname("::ffff:192.0.2.10")
	require.Len(t, devices, 1)
	dev := devices[0]
	require.Equal(t, "switch-a", dev.SysName)

	dev.ManualProfiles[0] = "changed"
	dev.VnodeLabels["site"] = "changed"

	again := store.DevicesByHostname("192.0.2.10")
	require.Len(t, again, 1)
	require.Equal(t, []string{"profile-a"}, again[0].ManualProfiles)
	require.Equal(t, "lab", again[0].VnodeLabels["site"])
}

func TestDeviceStoreDevicesByHostnameNoMatch(t *testing.T) {
	store := NewDeviceStore()
	store.Register("switch-a", DeviceConnectionInfo{Hostname: "switch-a.example.com"})

	require.Empty(t, store.DevicesByHostname("switch-b.example.com"))
}

func TestDeviceStoreDevicesByHostnameMatchesDNSCaseInsensitive(t *testing.T) {
	store := NewDeviceStore()
	store.Register("switch-a", DeviceConnectionInfo{Hostname: "Switch-A.Example.COM"})

	require.Len(t, store.DevicesByHostname("switch-a.example.com"), 1)
}

func TestDeviceStoreDevicesByHostnameIndexUpdatesOnRegisterAndUnregister(t *testing.T) {
	store := NewDeviceStore()
	store.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-a"})

	devices := store.DevicesByHostname("192.0.2.10")
	require.Len(t, devices, 1)
	dev := devices[0]
	require.Equal(t, "switch-a", dev.SysName)

	store.Register("switch-a", DeviceConnectionInfo{Hostname: "192.0.2.11", SysName: "switch-a-renumbered"})

	require.Empty(t, store.DevicesByHostname("192.0.2.10"))
	devices = store.DevicesByHostname("192.0.2.11")
	require.Len(t, devices, 1)
	dev = devices[0]
	require.Equal(t, "switch-a-renumbered", dev.SysName)

	store.Unregister("switch-a")
	require.Empty(t, store.DevicesByHostname("192.0.2.11"))
}

func TestDeviceStoreDevicesByHostnameReturnsAllMatches(t *testing.T) {
	store := NewDeviceStore()
	store.Register("switch-b", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-b"})
	store.Register("switch-a", DeviceConnectionInfo{Hostname: "::ffff:192.0.2.10", SysName: "switch-a"})

	devices := store.DevicesByHostname("192.0.2.10")
	require.Len(t, devices, 2)
	require.Equal(t, "switch-a", devices[0].SysName)
	require.Equal(t, "switch-b", devices[1].SysName)

	devices[0].SysName = "changed"
	again := store.DevicesByHostname("192.0.2.10")
	require.Equal(t, "switch-a", again[0].SysName)
}

func TestDeviceStoreRegisterClonesReferenceFields(t *testing.T) {
	store := NewDeviceStore()
	info := DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		ManualProfiles: []string{"profile-a"},
		VnodeLabels:    map[string]string{"site": "lab"},
	}

	store.Register("switch-a", info)
	info.ManualProfiles[0] = "changed"
	info.VnodeLabels["site"] = "changed"

	entries := store.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "switch-a", entries[0].Key)
	require.Equal(t, []string{"profile-a"}, entries[0].Info.ManualProfiles)
	require.Equal(t, "lab", entries[0].Info.VnodeLabels["site"])
}

func TestDeviceStoreEntriesAreSortedByRegistrationKey(t *testing.T) {
	store := NewDeviceStore()
	store.Register("job-b", DeviceConnectionInfo{Hostname: "192.0.2.10"})
	store.Register("job-a", DeviceConnectionInfo{Hostname: "192.0.2.10"})

	entries := store.Entries()
	require.Len(t, entries, 2)
	require.Equal(t, "job-a", entries[0].Key)
	require.Equal(t, "job-b", entries[1].Key)
}
