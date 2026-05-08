// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFuncBGPPeers(cache *bgpPeerCache) *funcBGPPeers {
	return newFuncBGPPeers(cache)
}

func TestBGPLastErrorText(t *testing.T) {
	tests := map[string]struct {
		code     *int64
		subcode  *int64
		expected string
	}{
		"nil code": {
			expected: "",
		},
		"open bad bgp identifier": {
			code:     int64Ptr(2),
			subcode:  int64Ptr(3),
			expected: "OPEN Message Error - Bad BGP Identifier",
		},
		"unknown subcode": {
			code:     int64Ptr(7),
			subcode:  int64Ptr(99),
			expected: "ROUTE-REFRESH Message Error (subcode 99)",
		},
		"unknown code": {
			code:     int64Ptr(42),
			subcode:  int64Ptr(7),
			expected: "Unknown error code 42 / subcode 7",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, bgpLastErrorText(tc.code, tc.subcode))
		})
	}
}

func TestBGPPeerEntryKeyEscapesValues(t *testing.T) {
	keyA := bgpPeerEntryKey("peers", map[string]string{
		"neighbor":  "198.51.100.1|remote_as=65001",
		"remote_as": "65002",
	})
	keyB := bgpPeerEntryKey("peers", map[string]string{
		"neighbor":  "198.51.100.1",
		"remote_as": "65001|remote_as=65002",
	})

	assert.NotEqual(t, keyA, keyB)
}

func TestBGPPeerEntryKeyUsesStableIdentityTags(t *testing.T) {
	keyA := bgpPeerEntryKey("peers", map[string]string{
		"routing_instance": "blue",
		"neighbor":         "198.51.100.1",
		"remote_as":        "65001",
		"local_address":    "192.0.2.1",
		"peer_description": "Transit A",
	})
	keyB := bgpPeerEntryKey("peers", map[string]string{
		"routing_instance": "blue",
		"neighbor":         "198.51.100.1",
		"remote_as":        "65001",
		"local_address":    "192.0.2.2",
		"peer_description": "Transit B",
	})
	familyKey := bgpPeerEntryKey("peer_families", map[string]string{
		"routing_instance":          "blue",
		"neighbor":                  "198.51.100.1",
		"remote_as":                 "65001",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	otherFamilyKey := bgpPeerEntryKey("peer_families", map[string]string{
		"routing_instance":          "blue",
		"neighbor":                  "198.51.100.1",
		"remote_as":                 "65001",
		"address_family":            "ipv6",
		"subsequent_address_family": "unicast",
	})
	keyWithoutRoutingInstance := bgpPeerEntryKey("peers", map[string]string{
		"neighbor":  "198.51.100.1",
		"remote_as": "65001",
	})
	keyWithEmptyRoutingInstance := bgpPeerEntryKey("peers", map[string]string{
		"routing_instance": "",
		"neighbor":         "198.51.100.1",
		"remote_as":        "65001",
	})

	assert.Equal(t, keyA, keyB)
	assert.NotEqual(t, keyA, familyKey)
	assert.NotEqual(t, familyKey, otherFamilyKey)
	assert.NotEmpty(t, keyWithoutRoutingInstance)
	assert.Equal(t, keyWithoutRoutingInstance, keyWithEmptyRoutingInstance)
	assert.NotEqual(t, keyA, keyWithoutRoutingInstance)
	assert.Empty(t, bgpPeerEntryKey("peers", map[string]string{"remote_as": "65001"}))
	assert.Empty(t, bgpPeerEntryKey("peers", map[string]string{"neighbor": "198.51.100.1"}))
	assert.Empty(t, bgpPeerEntryKey("peer_families", map[string]string{
		"neighbor":                  "198.51.100.1",
		"remote_as":                 "65001",
		"address_family":            "ipv4",
		"subsequent_address_family": "",
	}))
}

func TestBGPAdminStatus(t *testing.T) {
	tests := map[string]struct {
		mv       map[string]int64
		expected string
	}{
		"enabled": {
			mv:       map[string]int64{"admin_enabled": 1},
			expected: "enabled",
		},
		"disabled": {
			mv:       map[string]int64{"admin_disabled": 1},
			expected: "disabled",
		},
		"disabled from explicit admin_enabled zero": {
			mv:       map[string]int64{"admin_enabled": 0},
			expected: "disabled",
		},
		"unknown when absent": {
			mv:       map[string]int64{"established": 1},
			expected: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, bgpAdminStatus(tc.mv))
		})
	}
}

func TestBGPPeerCacheUsesStableIdentityAndMutableTags(t *testing.T) {
	cache := newBGPPeerCache()
	cache.reset()

	cache.updateEntry(ddsnmp.Metric{
		Name:    "bgp.peers.availability",
		IsTable: true,
		Tags: map[string]string{
			"routing_instance":  "blue",
			"neighbor":          "198.51.100.1",
			"remote_as":         "65001",
			"_local_address":    "192.0.2.1",
			"_peer_description": "Transit A",
		},
		MultiValue: map[string]int64{"admin_enabled": 0},
	})
	cache.updateEntry(ddsnmp.Metric{
		Name:    "bgp.peers.connection_state",
		IsTable: true,
		Tags: map[string]string{
			"routing_instance":  "blue",
			"neighbor":          "198.51.100.1",
			"remote_as":         "65001",
			"_local_address":    "192.0.2.2",
			"_peer_description": "Transit B",
		},
		MultiValue: map[string]int64{"established": 1},
	})
	cache.finalize()

	require.Len(t, cache.entries, 1)
	var entry *bgpPeerEntry
	for _, got := range cache.entries {
		entry = got
	}
	require.NotNil(t, entry)

	assert.Equal(t, "disabled", entry.adminStatus)
	assert.Equal(t, "established", entry.state)
	assert.Equal(t, "192.0.2.2", entry.tags["local_address"])
	assert.Equal(t, "Transit B", entry.tags["peer_description"])
}

func TestMergeBGPPeerEntryTagsPrefersUnprefixedWithinSample(t *testing.T) {
	entry := &bgpPeerEntry{tags: make(map[string]string)}

	for i := 0; i < 100; i++ {
		entry.tags = make(map[string]string)
		mergeBGPPeerEntryTags(entry, map[string]string{
			"local_address":  "192.0.2.10",
			"_local_address": "192.0.2.20",
		})
		assert.Equal(t, "192.0.2.10", entry.tags["local_address"])
	}

	mergeBGPPeerEntryTags(entry, map[string]string{
		"_local_address": "192.0.2.30",
	})
	assert.Equal(t, "192.0.2.30", entry.tags["local_address"])
}

func TestBGPPeerCacheClearsZeroLastError(t *testing.T) {
	cache := newBGPPeerCache()
	cache.reset()
	tags := map[string]string{
		"neighbor":  "198.51.100.1",
		"remote_as": "65001",
	}

	cache.updateEntry(ddsnmp.Metric{
		Name:       "bgp.peers.last_error",
		IsTable:    true,
		Tags:       tags,
		MultiValue: map[string]int64{"code": 2, "subcode": 3},
	})
	require.Len(t, cache.entries, 1)

	for _, entry := range cache.entries {
		require.NotNil(t, entry.lastErrorCode)
		require.NotNil(t, entry.lastErrorSubcode)
		assert.Equal(t, "OPEN Message Error - Bad BGP Identifier", bgpLastErrorDisplay(entry))
	}

	cache.updateEntry(ddsnmp.Metric{
		Name:       "bgp.peers.last_error",
		IsTable:    true,
		Tags:       tags,
		MultiValue: map[string]int64{"code": 0, "subcode": 0},
	})
	require.Len(t, cache.entries, 1)

	for _, entry := range cache.entries {
		assert.Nil(t, entry.lastErrorCode)
		assert.Nil(t, entry.lastErrorSubcode)
		assert.Empty(t, entry.lastErrorText)
		assert.Empty(t, bgpLastErrorDisplay(entry))
	}
}

func TestBGPPeerCacheResetClearsStaleFieldsBetweenPolls(t *testing.T) {
	cache := newBGPPeerCache()
	tags := map[string]string{
		"neighbor":       "198.51.100.1",
		"remote_as":      "65001",
		"_local_address": "192.0.2.1",
	}

	cache.reset()
	for _, metric := range []ddsnmp.Metric{
		{Name: "bgp.peers.availability", IsTable: true, Tags: tags, MultiValue: map[string]int64{"admin_enabled": 1}},
		{Name: "bgp.peers.last_error", IsTable: true, Tags: tags, MultiValue: map[string]int64{"code": 2, "subcode": 3}},
		{Name: "bgp.peers.update_traffic", IsTable: true, Tags: tags, MultiValue: map[string]int64{"received": 10, "sent": 20}},
	} {
		cache.updateEntry(metric)
	}
	cache.finalize()
	require.Len(t, cache.entries, 1)

	cache.reset()
	cache.updateEntry(ddsnmp.Metric{
		Name:       "bgp.peers.connection_state",
		IsTable:    true,
		Tags:       map[string]string{"neighbor": "198.51.100.1", "remote_as": "65001"},
		MultiValue: map[string]int64{"established": 1},
	})
	cache.finalize()
	require.Len(t, cache.entries, 1)

	for _, entry := range cache.entries {
		assert.Equal(t, "established", entry.state)
		assert.Empty(t, entry.adminStatus)
		assert.Nil(t, entry.lastErrorCode)
		assert.Nil(t, entry.lastErrorSubcode)
		assert.Empty(t, entry.lastErrorText)
		assert.Nil(t, entry.updateCounts)
		assert.Equal(t, "198.51.100.1", entry.tags["neighbor"])
		assert.Equal(t, "65001", entry.tags["remote_as"])
		assert.NotContains(t, entry.tags, "local_address")
	}
}

func TestFuncBGPPeersHandle(t *testing.T) {
	cache := newBGPPeerCache()
	cache.reset()

	peerTags := map[string]string{
		"routing_instance":  "blue",
		"neighbor":          "192.0.2.1",
		"_local_address":    "198.51.100.1",
		"remote_as":         "65001",
		"_peer_description": "Transit Peer",
	}
	familyTags := map[string]string{
		"routing_instance":          "blue",
		"neighbor":                  "192.0.2.1",
		"_local_address":            "198.51.100.1",
		"remote_as":                 "65001",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
		"_address_family_name":      "ipv4 unicast",
	}

	for _, metric := range []ddsnmp.Metric{
		{Name: "bgp.peers.availability", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"admin_enabled": 1, "established": 1}},
		{Name: "bgp.peers.connection_state", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"established": 1}},
		{Name: "bgp.peers.previous_connection_state", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"idle": 1}},
		{Name: "bgp.peers.established_uptime", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"uptime": 1234}},
		{Name: "bgp.peers.last_received_update_age", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"age": 45}},
		{Name: "bgp.peers.update_traffic", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"received": 10, "sent": 20}},
		{Name: "bgp.peers.last_error", IsTable: true, Tags: peerTags, MultiValue: map[string]int64{"code": 2, "subcode": 3}},
		{Name: "bgp.peer_families.route_counts.current", IsTable: true, Tags: familyTags, MultiValue: map[string]int64{"accepted": 540336, "advertised": 540339}},
		{Name: "bgp.peer_families.graceful_restart_state", IsTable: true, Tags: familyTags, MultiValue: map[string]int64{"restart_time_wait": 1}},
		{Name: "bgp.peer_families.unavailability_reason", IsTable: true, Tags: familyTags, MultiValue: map[string]int64{"peer_not_ready": 1}},
	} {
		cache.updateEntry(metric)
	}
	cache.finalize()

	tests := map[string]struct {
		params   funcapi.ResolvedParams
		validate func(t *testing.T, resp *funcapi.FunctionResponse)
	}{
		"peers view returns peer row with human-readable last error": {
			params: resolveBGPPeerParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Equal(t, "Neighbor", resp.DefaultSortColumn)

				rowIDCol, ok := resp.Columns["rowId"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, false, rowIDCol["visible"])
				assert.Equal(t, true, rowIDCol["unique_key"])

				rows := responseRows(t, resp)
				require.Len(t, rows, 1)

				assert.Equal(t, "peer", rows[0][findBGPPeerColIdx("Scope")])
				assert.Equal(t, "192.0.2.1", rows[0][findBGPPeerColIdx("Neighbor")])
				assert.Equal(t, "198.51.100.1", rows[0][findBGPPeerColIdx("Local Address")])
				assert.Equal(t, "Transit Peer", rows[0][findBGPPeerColIdx("Peer Description")])
				assert.Equal(t, "enabled", rows[0][findBGPPeerColIdx("Admin Status")])
				assert.Equal(t, "OPEN Message Error - Bad BGP Identifier", rows[0][findBGPPeerColIdx("Last Error")])
				assert.Equal(t, "idle", rows[0][findBGPPeerColIdx("Previous State")])
			},
		},
		"all view returns peer and peer-family rows": {
			params: resolveBGPPeerParams(map[string][]string{bgpPeersParamView: {bgpPeersViewAll}}),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				rows := responseRows(t, resp)
				require.Len(t, rows, 2)
				assert.NotEqual(t, rows[0][findBGPPeerColIdx("rowId")], rows[1][findBGPPeerColIdx("rowId")])
				assert.Equal(t, "ipv4 unicast", rows[1][findBGPPeerColIdx("Family")])
				assert.Equal(t, "restart time wait", rows[1][findBGPPeerColIdx("GR State")])
				assert.Equal(t, "peer not ready", rows[1][findBGPPeerColIdx("Unavailability Reason")])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := newTestFuncBGPPeers(cache).Handle(context.Background(), bgpPeersMethodID, tc.params)
			tc.validate(t, resp)
		})
	}
}

func TestCollectSNMP_HidesBGPDiagnosticsButKeepsFunctionCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSNMP := snmpmock.NewMockHandler(ctrl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{{
			Source: "test",
			Metrics: []ddsnmp.Metric{
				{
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
					Profile:    &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
				},
				{
					Name:    "bgpPeerLastErrorCode",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   2,
					Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
				},
				{
					Name:    "bgpPeerLastErrorSubcode",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   3,
					Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
				},
			},
		}}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	for key := range mx {
		assert.NotContains(t, key, "last_error")
	}

	require.NotNil(t, collr.bgp)
	require.NotNil(t, collr.bgp.peerCache)
	require.Len(t, collr.bgp.peerCache.entries, 1)
	for _, entry := range collr.bgp.peerCache.entries {
		assert.Equal(t, "OPEN Message Error - Bad BGP Identifier", entry.lastErrorText)
	}
}

func findBGPPeerColIdx(name string) int {
	for i, col := range bgpPeerColumns {
		if col.Name == name {
			return i
		}
	}
	return -1
}

func resolveBGPPeerParams(values map[string][]string) funcapi.ResolvedParams {
	f := newTestFuncBGPPeers(newBGPPeerCache())
	params, err := f.MethodParams(context.Background(), bgpPeersMethodID)
	if err != nil {
		return nil
	}
	return funcapi.ResolveParams(params, values)
}

func responseRows(t *testing.T, resp *funcapi.FunctionResponse) [][]any {
	t.Helper()
	rows, ok := resp.Data.([][]any)
	require.True(t, ok)
	return rows
}
