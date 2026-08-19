// SPDX-License-Identifier: GPL-3.0-or-later

package netdataadapter

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryLookupProjectsUniqueDevice(t *testing.T) {
	store := ddsnmp.NewDeviceStore()
	store.Register("job:192.0.2.10", ddsnmp.DeviceConnectionInfo{
		Hostname:      "192.0.2.10",
		SysName:       "fallback.example.test",
		VnodeHostname: "device.example.test",
		Vendor:        "example-vendor",
		VnodeGUID:     "example-vnode",
	})

	lookup := RegistryLookup(store)
	require.NotNil(t, lookup)
	assert.Equal(t, 1, lookup("192.0.2.10").Matches)
	assert.Equal(t, "device.example.test", lookup("192.0.2.10").Hostname)
	assert.Equal(t, "example-vendor", lookup("192.0.2.10").Vendor)
	assert.Equal(t, "example-vnode", lookup("192.0.2.10").VnodeID)
}

func TestRegistryLookupProjectsAmbiguityWithoutDeviceFields(t *testing.T) {
	store := ddsnmp.NewDeviceStore()
	for _, job := range []string{"job-a", "job-b"} {
		store.Register(job+":192.0.2.10", ddsnmp.DeviceConnectionInfo{
			Hostname:  "192.0.2.10",
			SysName:   job,
			Vendor:    job,
			VnodeGUID: job,
		})
	}

	got := RegistryLookup(store)("192.0.2.10")
	assert.Equal(t, 2, got.Matches)
	assert.Empty(t, got.Hostname)
	assert.Empty(t, got.Vendor)
	assert.Empty(t, got.VnodeID)
}

func TestRegistryLookupTreatsUnknownVnodeHostnameAsUnresolved(t *testing.T) {
	store := ddsnmp.NewDeviceStore()
	store.Register("job:192.0.2.10", ddsnmp.DeviceConnectionInfo{
		Hostname:      "192.0.2.10",
		SysName:       "device.example.test",
		VnodeHostname: " UNKNOWN ",
	})

	assert.Equal(t, "device.example.test", RegistryLookup(store)("192.0.2.10").Hostname)
}

func TestTopologyResultProjectsAllFields(t *testing.T) {
	got := ProjectTopologyResult(&snmptopology.TrapTopologyEnrichment{
		DeviceStatus:    "matched",
		DeviceMethod:    "management_ip",
		DeviceMatches:   1,
		DeviceHostname:  "device.example.test",
		DeviceVendor:    "example-vendor",
		SourceVnodeID:   "example-vnode",
		InterfaceIndex:  "7",
		InterfaceStatus: "matched",
		Interface:       "Gi0/7",
		NeighborStatus:  "matched",
		Neighbors:       []string{"access-a", "access-b"},
	})

	assert.Equal(t, "matched", got.Status)
	assert.Equal(t, "management_ip", got.Method)
	assert.Equal(t, 1, got.Matches)
	assert.Equal(t, "device.example.test", got.Hostname)
	assert.Equal(t, "example-vendor", got.Vendor)
	assert.Equal(t, "example-vnode", got.VnodeID)
	assert.Equal(t, "7", got.InterfaceIndex)
	assert.Equal(t, "matched", got.InterfaceStatus)
	assert.Equal(t, "Gi0/7", got.InterfaceName)
	assert.Equal(t, "matched", got.NeighborStatus)
	assert.Equal(t, []string{"access-a", "access-b"}, got.NeighborNames)
}

func TestNilDependenciesProduceNilLookups(t *testing.T) {
	assert.Nil(t, RegistryLookup(nil))
	assert.Nil(t, TopologyLookup(nil))
	assert.Empty(t, ProjectTopologyResult(nil))
}
