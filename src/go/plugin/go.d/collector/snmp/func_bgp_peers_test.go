// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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

func TestBGPPeerEntryKey(t *testing.T) {
	tests := map[string]struct {
		scope      string
		tags       map[string]string
		otherScope string
		otherTags  map[string]string
		wantEqual  bool
		wantEmpty  bool
	}{
		"escapes delimiter-like values": {
			scope: "peers",
			tags: map[string]string{
				"neighbor":  "198.51.100.1|remote_as=65001",
				"remote_as": "65002",
			},
			otherScope: "peers",
			otherTags: map[string]string{
				"neighbor":  "198.51.100.1",
				"remote_as": "65001|remote_as=65002",
			},
			wantEqual: false,
		},
		"ignores peer descriptor tags": {
			scope: "peers",
			tags: map[string]string{
				"routing_instance": "blue",
				"neighbor":         "198.51.100.1",
				"remote_as":        "65001",
				"local_address":    "192.0.2.1",
				"peer_description": "Transit A",
			},
			otherScope: "peers",
			otherTags: map[string]string{
				"routing_instance": "blue",
				"neighbor":         "198.51.100.1",
				"remote_as":        "65001",
				"local_address":    "192.0.2.2",
				"peer_description": "Transit B",
			},
			wantEqual: true,
		},
		"scope participates in identity": {
			scope: "peers",
			tags: map[string]string{
				"routing_instance": "blue",
				"neighbor":         "198.51.100.1",
				"remote_as":        "65001",
			},
			otherScope: "peer_families",
			otherTags: map[string]string{
				"routing_instance":          "blue",
				"neighbor":                  "198.51.100.1",
				"remote_as":                 "65001",
				"address_family":            "ipv4",
				"subsequent_address_family": "unicast",
			},
			wantEqual: false,
		},
		"family address family participates in identity": {
			scope: "peer_families",
			tags: map[string]string{
				"routing_instance":          "blue",
				"neighbor":                  "198.51.100.1",
				"remote_as":                 "65001",
				"address_family":            "ipv4",
				"subsequent_address_family": "unicast",
			},
			otherScope: "peer_families",
			otherTags: map[string]string{
				"routing_instance":          "blue",
				"neighbor":                  "198.51.100.1",
				"remote_as":                 "65001",
				"address_family":            "ipv6",
				"subsequent_address_family": "unicast",
			},
			wantEqual: false,
		},
		"empty routing instance is same as missing": {
			scope: "peers",
			tags: map[string]string{
				"neighbor":  "198.51.100.1",
				"remote_as": "65001",
			},
			otherScope: "peers",
			otherTags: map[string]string{
				"routing_instance": "",
				"neighbor":         "198.51.100.1",
				"remote_as":        "65001",
			},
			wantEqual: true,
		},
		"routing instance partitions peers": {
			scope: "peers",
			tags: map[string]string{
				"routing_instance": "blue",
				"neighbor":         "198.51.100.1",
				"remote_as":        "65001",
			},
			otherScope: "peers",
			otherTags: map[string]string{
				"neighbor":  "198.51.100.1",
				"remote_as": "65001",
			},
			wantEqual: false,
		},
		"missing peer neighbor is empty": {
			scope:     "peers",
			tags:      map[string]string{"remote_as": "65001"},
			wantEmpty: true,
		},
		"missing peer remote AS is empty": {
			scope:     "peers",
			tags:      map[string]string{"neighbor": "198.51.100.1"},
			wantEmpty: true,
		},
		"missing peer-family SAFI is empty": {
			scope: "peer_families",
			tags: map[string]string{
				"neighbor":                  "198.51.100.1",
				"remote_as":                 "65001",
				"address_family":            "ipv4",
				"subsequent_address_family": "",
			},
			wantEmpty: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			key := bgpPeerEntryKey(tc.scope, tc.tags)
			if tc.wantEmpty {
				assert.Empty(t, key)
				return
			}

			require.NotEmpty(t, key)
			if tc.otherTags == nil {
				return
			}

			otherKey := bgpPeerEntryKey(tc.otherScope, tc.otherTags)
			require.NotEmpty(t, otherKey)
			if tc.wantEqual {
				assert.Equal(t, key, otherKey)
			} else {
				assert.NotEqual(t, key, otherKey)
			}
		})
	}
}

func TestBGPPeerCacheClearsZeroLastError(t *testing.T) {
	cache := newBGPPeerCache()

	row := minimalBGPPeerRow("peer-a", "198.51.100.1", "65001")
	row.LastError = ddsnmp.BGPLastError{
		Code:    ddsnmp.BGPInt64{Has: true, Value: 2},
		Subcode: ddsnmp.BGPInt64{Has: true, Value: 3},
	}

	cache.reset()
	cache.updateRow("test-profile.yaml", row)
	require.Len(t, cache.entries, 1)

	for _, entry := range cache.entries {
		require.NotNil(t, entry.lastErrorCode)
		require.NotNil(t, entry.lastErrorSubcode)
		assert.Equal(t, "OPEN Message Error - Bad BGP Identifier", bgpLastErrorDisplay(entry))
	}

	row.LastError = ddsnmp.BGPLastError{
		Code:    ddsnmp.BGPInt64{Has: true, Value: 0},
		Subcode: ddsnmp.BGPInt64{Has: true, Value: 0},
	}
	cache.reset()
	cache.updateRow("test-profile.yaml", row)
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

	row := minimalBGPPeerRow("peer-a", "198.51.100.1", "65001")
	row.Descriptors.LocalAddress = "192.0.2.1"
	row.Admin.Enabled = ddsnmp.BGPBool{Has: true, Value: true}
	row.LastError = ddsnmp.BGPLastError{
		Code:    ddsnmp.BGPInt64{Has: true, Value: 2},
		Subcode: ddsnmp.BGPInt64{Has: true, Value: 3},
	}
	row.Traffic.Updates = ddsnmp.BGPDirectional{
		Received: ddsnmp.BGPInt64{Has: true, Value: 10},
		Sent:     ddsnmp.BGPInt64{Has: true, Value: 20},
	}
	cache.reset()
	cache.updateRow("test-profile.yaml", row)
	cache.finalize()
	require.Len(t, cache.entries, 1)

	next := minimalBGPPeerRow("peer-a", "198.51.100.1", "65001")
	next.State = ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerStateEstablished}
	cache.reset()
	cache.updateRow("test-profile.yaml", next)
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

	peer := typedBGPPeerRow()
	peer.LastError = ddsnmp.BGPLastError{
		Code:    ddsnmp.BGPInt64{Has: true, Value: 2},
		Subcode: ddsnmp.BGPInt64{Has: true, Value: 3},
	}
	peer.Connection.LastReceivedUpdateAge = ddsnmp.BGPInt64{Has: true, Value: 45}
	family := typedBGPPeerFamilyRow()
	cache.updateRow("test-profile.yaml", peer)
	cache.updateRow("test-profile.yaml", family)
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

func TestFuncBGPPeers_HandleStaleAndFilteredRows(t *testing.T) {
	tests := map[string]struct {
		prepare  func(cache *bgpPeerCache)
		params   funcapi.ResolvedParams
		validate func(t *testing.T, resp *funcapi.FunctionResponse)
	}{
		"recent failure keeps stale rows visible": {
			prepare: func(cache *bgpPeerCache) {
				cache.setStaleAfter(time.Hour)
				cache.updateRow("test-profile.yaml", typedBGPPeerRow())
				cache.finalize()
				cache.markCollectFailed(errors.New("walk failed"))
			},
			params: resolveBGPPeerParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Contains(t, resp.Help, "showing stale BGP rows")
				rows := responseRows(t, resp)
				require.Len(t, rows, 1)
				assert.Equal(t, true, rows[0][findBGPPeerColIdx("Stale")])
			},
		},
		"expired failure returns unavailable": {
			prepare: func(cache *bgpPeerCache) {
				cache.setStaleAfter(time.Minute)
				cache.updateRow("test-profile.yaml", typedBGPPeerRow())
				cache.finalize()
				cache.mu.Lock()
				cache.lastUpdate = time.Now().Add(-2 * time.Minute)
				cache.lastFailure = time.Now()
				cache.lastError = "walk failed"
				cache.mu.Unlock()
			},
			params: resolveBGPPeerParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 503, resp.Status)
				assert.Contains(t, resp.Message, "last successful BGP collection")
			},
		},
		"filtered empty view returns an empty table": {
			prepare: func(cache *bgpPeerCache) {
				cache.updateRow("test-profile.yaml", typedBGPPeerRow())
				cache.finalize()
			},
			params: resolveBGPPeerParams(map[string][]string{bgpPeersParamView: {bgpPeersViewFamilies}}),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				assert.Empty(t, responseRows(t, resp))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newBGPPeerCache()
			tc.prepare(cache)

			resp := newTestFuncBGPPeers(cache).Handle(context.Background(), bgpPeersMethodID, tc.params)
			tc.validate(t, resp)
		})
	}
}

func TestCollector_BGPFunctionHandlerIsAlwaysRegistered(t *testing.T) {
	tests := map[string]struct {
		prepare    func(c *Collector)
		wantStatus int
		wantBGP    bool
	}{
		"new collector has unavailable BGP handler": {
			wantStatus: 503,
		},
		"enabled BGP integration registers handler": {
			prepare: func(c *Collector) {
				c.enableBGPIntegration()
			},
			wantStatus: 503,
			wantBGP:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if tc.prepare != nil {
				tc.prepare(collr)
			}

			assert.Equal(t, tc.wantBGP, collr.bgp != nil)

			params, err := collr.funcRouter.MethodParams(context.Background(), bgpPeersMethodID)
			require.NoError(t, err)
			require.NotEmpty(t, params)

			resp := collr.funcRouter.Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, tc.wantStatus, resp.Status)
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
		row := typedBGPPeerRow()
		row.LastError = ddsnmp.BGPLastError{
			Code:    ddsnmp.BGPInt64{Has: true, Value: 2},
			Subcode: ddsnmp.BGPInt64{Has: true, Value: 3},
		}
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{{
			Source:  "test",
			BGPRows: []ddsnmp.BGPRow{row},
		}}}
	}

	require.NoError(t, collr.Init(context.Background()))
	_ = collr.Check(context.Background())

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	assert.Contains(t, mx, "snmp_bgp_peers_availability_192_0_2_1_65001_blue_established")
	assert.Contains(t, mx, "snmp_bgp_peers_availability_192_0_2_1_65001_blue_admin_enabled")

	chart := collr.Charts().Get("snmp_bgp_peers_availability_192_0_2_1_65001_blue")
	require.NotNil(t, chart)
	assert.Equal(t, "snmp.bgp.peers.availability", chart.Ctx)
	assert.Equal(t, "198.51.100.1", chartLabels(chart)["local_address"])
	assert.Equal(t, "Transit Peer", chartLabels(chart)["peer_description"])

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

func minimalBGPPeerRow(structuralID, neighbor, remoteAS string) ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		OriginProfileID: "test-profile.yaml",
		Kind:            ddprofiledefinition.BGPRowKindPeer,
		StructuralID:    structuralID,
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: "default",
			Neighbor:        neighbor,
			RemoteAS:        remoteAS,
		},
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
