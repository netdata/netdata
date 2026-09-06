// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeviceStoreLifecyclePrecedesTopologyRegistration(t *testing.T) {
	store := NewDeviceStore()
	store.RegisterJob("switch-a", DeviceLifecycleInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: "2c",
	})

	beforeCut := time.Now()
	cut := store.LifecycleCut()
	afterCut := time.Now()
	require.NotZero(t, cut.Sequence)
	require.False(t, cut.CapturedAt.Before(beforeCut))
	require.False(t, cut.CapturedAt.After(afterCut))
	require.Len(t, cut.Entries, 1)
	entry := cut.Entries[0]
	require.NotZero(t, entry.RegistrationID)
	require.Equal(t, "192.0.2.10", entry.Info.Hostname)
	require.Equal(t, 161, entry.Info.Port)
	require.Equal(t, "2c", entry.Info.SNMPVersion)
	require.Equal(t, DeviceLifecyclePhaseUnknown, entry.LastCompleted.Phase)
	require.Equal(t, DeviceLifecycleOutcomeUnknown, entry.LastCompleted.Outcome)
	require.False(t, entry.TopologyReady)

	completedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	store.RecordJobLifecycle("switch-a", DeviceLifecycleStatus{
		Phase:       DeviceLifecyclePhaseInit,
		Outcome:     DeviceLifecycleOutcomeFailed,
		CompletedAt: completedAt,
	})

	cut = store.LifecycleCut()
	require.Len(t, cut.Entries, 1)
	require.Equal(t, entry.RegistrationID, cut.Entries[0].RegistrationID)
	require.Equal(t, DeviceLifecyclePhaseInit, cut.Entries[0].LastCompleted.Phase)
	require.Equal(t, DeviceLifecycleOutcomeFailed, cut.Entries[0].LastCompleted.Outcome)
	require.Equal(t, completedAt, cut.Entries[0].LastCompleted.CompletedAt)
	require.False(t, cut.Entries[0].TopologyReady)

	store.Register("switch-a", DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: "2c",
		Community:   "must-not-appear-in-lifecycle",
		V3AuthKey:   "must-not-appear-in-lifecycle",
		V3PrivKey:   "must-not-appear-in-lifecycle",
	})

	require.Len(t, store.Entries(), 1)
	require.Equal(t, entry.RegistrationID, store.Entries()[0].RegistrationID)
	cut = store.LifecycleCut()
	require.Len(t, cut.Entries, 1)
	require.Equal(t, entry.RegistrationID, cut.Entries[0].RegistrationID)
	require.True(t, cut.Entries[0].TopologyReady)
	require.Equal(t, DeviceLifecycleInfo{
		Hostname:    "192.0.2.10",
		Port:        161,
		SNMPVersion: "2c",
	}, cut.Entries[0].Info)
}

func TestDeviceStoreReplaceJobCommitsOneIncarnationTransition(t *testing.T) {
	store := NewDeviceStore()
	store.RegisterJob("old-config", DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	store.Register("old-config", DeviceConnectionInfo{Hostname: "192.0.2.10"})
	oldCut := store.LifecycleCut()
	status := DeviceLifecycleStatus{
		Phase:       DeviceLifecyclePhaseInit,
		Outcome:     DeviceLifecycleOutcomeFailed,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}

	store.ReplaceJob("old-config", "new-config", DeviceLifecycleInfo{
		Hostname:    "192.0.2.20",
		Port:        1161,
		SNMPVersion: "3",
	}, status, nil)

	newCut := store.LifecycleCut()
	require.Equal(t, oldCut.Sequence+1, newCut.Sequence)
	require.Len(t, newCut.Entries, 1)
	require.Greater(t, newCut.Entries[0].RegistrationID, oldCut.Entries[0].RegistrationID)
	require.Equal(t, "192.0.2.20", newCut.Entries[0].Info.Hostname)
	require.Equal(t, status, newCut.Entries[0].LastCompleted)
	require.False(t, newCut.Entries[0].TopologyReady)
	require.Empty(t, store.Entries())
}

func TestDeviceStoreLifecycleCleanupAndReincarnation(t *testing.T) {
	store := NewDeviceStore()
	store.RegisterJob("switch-a", DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	first := store.LifecycleCut()
	require.Len(t, first.Entries, 1)

	store.Unregister("switch-a")
	require.Empty(t, store.LifecycleCut().Entries)
	require.Empty(t, store.Entries())

	store.RegisterJob("switch-a", DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	second := store.LifecycleCut()
	require.Len(t, second.Entries, 1)
	require.Greater(t, second.Entries[0].RegistrationID, first.Entries[0].RegistrationID)
}

func TestDeviceStoreLifecycleCutIsSortedAndIndependent(t *testing.T) {
	store := NewDeviceStore()
	store.RegisterJob("switch-b", DeviceLifecycleInfo{Hostname: "switch-b"})
	store.RegisterJob("switch-a", DeviceLifecycleInfo{Hostname: "switch-a"})

	cut := store.LifecycleCut()
	require.Len(t, cut.Entries, 2)
	require.Less(t, cut.Entries[0].RegistrationID, cut.Entries[1].RegistrationID)

	cut.Entries[0].Info.Hostname = "changed"
	again := store.LifecycleCut()
	require.Equal(t, "switch-b", again.Entries[0].Info.Hostname)
}

func BenchmarkDeviceStoreRecordJobLifecycle(b *testing.B) {
	store := NewDeviceStore()
	store.RegisterJob("switch-a", DeviceLifecycleInfo{Hostname: "192.0.2.10"})
	status := DeviceLifecycleStatus{
		Phase:       DeviceLifecyclePhaseCollect,
		Outcome:     DeviceLifecycleOutcomeSuccess,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		store.RecordJobLifecycle("switch-a", status)
	}
}

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
	require.NotZero(t, entries[0].RegistrationID)
	require.Equal(t, []string{"profile-a"}, entries[0].Info.ManualProfiles)
	require.Equal(t, "lab", entries[0].Info.VnodeLabels["site"])
}

func TestDeviceStoreEntriesAreSortedByRegistrationID(t *testing.T) {
	store := NewDeviceStore()
	store.Register("job-b", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-b"})
	store.Register("job-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "switch-a"})

	entries := store.Entries()
	require.Len(t, entries, 2)
	require.Less(t, entries[0].RegistrationID, entries[1].RegistrationID)
	require.Equal(t, "switch-b", entries[0].Info.SysName)
	require.Equal(t, "switch-a", entries[1].Info.SysName)
}

func TestDeviceStoreRegistrationIdentityChangesOnlyAfterUnregister(t *testing.T) {
	store := NewDeviceStore()
	store.Register("owner-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "initial"})

	entries := store.Entries()
	require.Len(t, entries, 1)
	initialIdentity := entries[0].RegistrationID

	store.Register("owner-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "updated"})
	entries = store.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, initialIdentity, entries[0].RegistrationID, "live metadata updates must retain the registration identity")

	store.Unregister("owner-a")
	store.Register("owner-a", DeviceConnectionInfo{Hostname: "192.0.2.10", SysName: "replacement"})
	entries = store.Entries()
	require.Len(t, entries, 1)
	require.Greater(t, entries[0].RegistrationID, initialIdentity, "replacement registrations must receive a new identity")
}

func TestDeviceWriterCannotReviveRemovedOrReplaceSuccessor(t *testing.T) {
	store := NewDeviceStore()
	first := store.ReplaceJob("", "same-config", DeviceLifecycleInfo{Hostname: "first"}, DeviceLifecycleStatus{}, nil)
	first.UpdateDevice(DeviceConnectionInfo{Hostname: "first"})
	require.Len(t, store.Entries(), 1)
	successor := store.ReplaceJob("same-config", "same-config", DeviceLifecycleInfo{Hostname: "successor"}, DeviceLifecycleStatus{}, nil)
	first.UpdateDevice(DeviceConnectionInfo{Hostname: "retired"})
	first.RecordLifecycle(DeviceLifecycleStatus{Phase: DeviceLifecyclePhaseCollect, Outcome: DeviceLifecycleOutcomeFailed})
	require.Empty(t, store.Entries())
	require.Equal(t, "successor", store.LifecycleCut().Entries[0].Info.Hostname)
	require.Equal(t, DeviceLifecyclePhaseUnknown, store.LifecycleCut().Entries[0].LastCompleted.Phase)
	successor.UpdateDevice(DeviceConnectionInfo{Hostname: "successor"})
	require.Equal(t, "successor", store.Entries()[0].Info.Hostname)
	store.Unregister("same-config")
	successor.UpdateDevice(DeviceConnectionInfo{Hostname: "retired"})
	successor.RecordLifecycle(DeviceLifecycleStatus{Phase: DeviceLifecyclePhaseCollect})
	require.Empty(t, store.Entries())
	require.Empty(t, store.LifecycleCut().Entries)
}

func BenchmarkDeviceWriterLifecycle(b *testing.B) {
	for _, count := range []int{1, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			store := NewDeviceStore()
			var writer *DeviceWriter
			for i := range count {
				writer = store.ReplaceJob("", strconv.Itoa(i), DeviceLifecycleInfo{Hostname: "switch.example"}, DeviceLifecycleStatus{}, nil)
			}
			status := DeviceLifecycleStatus{Phase: DeviceLifecyclePhaseCollect, Outcome: DeviceLifecycleOutcomeSuccess}
			b.ReportAllocs()
			for b.Loop() {
				writer.RecordLifecycle(status)
			}
		})
	}
}
