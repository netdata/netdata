// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedBGPMetricsFromProfileMetrics(t *testing.T) {
	tests := map[string]struct {
		pm       *ddsnmp.ProfileMetrics
		validate func(t *testing.T, metrics []ddsnmp.Metric)
	}{
		"peer and peer-family rows become BGP chart metrics": {
			pm: &ddsnmp.ProfileMetrics{
				BGPRows: []ddsnmp.BGPRow{
					typedBGPPeerRow(),
					typedBGPPeerFamilyRow(),
				},
			},
			validate: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.1",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
				assert.NotContains(t, availability.Tags, "local_address")
				assert.NotContains(t, availability.Tags, "peer_description")
				assert.Equal(t, "198.51.100.1", availability.Tags["_local_address"])
				assert.Equal(t, "Transit Peer", availability.Tags["_peer_description"])

				updates := requireMetric(t, metrics, "bgp.peers.update_traffic", map[string]string{
					"routing_instance": "blue",
					"neighbor":         "192.0.2.1",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"received": 10, "sent": 20}, updates.MultiValue)

				routes := requireMetric(t, metrics, "bgp.peer_families.route_counts.current", map[string]string{
					"routing_instance":          "blue",
					"neighbor":                  "192.0.2.1",
					"remote_as":                 "65001",
					"address_family":            "ipv4",
					"subsequent_address_family": "unicast",
				})
				assert.Equal(t, map[string]int64{"accepted": 540336, "advertised": 540339}, routes.MultiValue)

				counts := requireMetric(t, metrics, "bgp.devices.peer_counts", nil)
				assert.Equal(t, map[string]int64{"configured": 1, "admin_enabled": 1, "established": 1}, counts.MultiValue)

				states := requireMetric(t, metrics, "bgp.devices.peer_states", nil)
				assert.Equal(t, map[string]int64{"established": 1}, states.MultiValue)
			},
		},
		"empty routing instance is normalized to default": {
			pm: &ddsnmp.ProfileMetrics{
				BGPRows: []ddsnmp.BGPRow{
					typedBGPPeerRowWithIdentity("typed-peer-default-vrf", "192.0.2.3", "65003", "", ddprofiledefinition.BGPPeerStateEstablished),
				},
			},
			validate: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
					"routing_instance": "default",
					"neighbor":         "192.0.2.3",
					"remote_as":        "65003",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
			},
		},
		"explicit device peer counts override auto peer-row count dimensions": {
			pm: &ddsnmp.ProfileMetrics{
				BGPRows: []ddsnmp.BGPRow{
					typedBGPPeerRow(),
					{
						OriginProfileID: "_vendor-bgp.yaml",
						Kind:            ddprofiledefinition.BGPRowKindDevice,
						StructuralID:    "typed-device-key",
						Device: ddsnmp.BGPDeviceCounts{
							Peers:         ddsnmp.BGPInt64{Has: true, Value: 12},
							InternalPeers: ddsnmp.BGPInt64{Has: true, Value: 4},
							ExternalPeers: ddsnmp.BGPInt64{Has: true, Value: 8},
						},
					},
				},
			},
			validate: func(t *testing.T, metrics []ddsnmp.Metric) {
				counts := requireMetric(t, metrics, "bgp.devices.peer_counts", nil)
				assert.Equal(t, map[string]int64{
					"configured":    12,
					"ibgp":          4,
					"ebgp":          8,
					"admin_enabled": 1,
					"established":   1,
				}, counts.MultiValue)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := typedBGPMetricsFromProfileMetrics([]*ddsnmp.ProfileMetrics{tc.pm})
			tc.validate(t, metrics)
		})
	}
}

func TestTypedBGPMetricsUseLogicalChartIdentity(t *testing.T) {
	tests := map[string]struct {
		profileMetrics []*ddsnmp.ProfileMetrics
		wantKey        string
	}{
		"same logical peer from different BGP rows keeps one public chart identity": {
			profileMetrics: []*ddsnmp.ProfileMetrics{
				{
					Source: "profile-a.yaml",
					BGPRows: []ddsnmp.BGPRow{
						typedBGPPeerRowWithIdentity("profile-a-peer", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished),
					},
				},
				{
					Source: "profile-b.yaml",
					BGPRows: []ddsnmp.BGPRow{
						func() ddsnmp.BGPRow {
							row := typedBGPPeerRowWithIdentity("profile-b-peer", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished)
							row.Descriptors.LocalAddress = "203.0.113.9"
							row.Descriptors.Description = "Backup Peer"
							return row
						}(),
					},
				},
			},
			wantKey: "bgp.peers.availability_192.0.2.1_65001_blue",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := typedBGPMetricsFromProfileMetrics(tc.profileMetrics)

			keys := make(map[string]struct{})
			var availabilityMetrics []ddsnmp.Metric
			for _, metric := range metrics {
				if metric.Name != "bgp.peers.availability" {
					continue
				}
				keys[tableMetricKey(metric)] = struct{}{}
				availabilityMetrics = append(availabilityMetrics, metric)
			}

			require.Len(t, availabilityMetrics, 2)
			assert.Equal(t, map[string]struct{}{tc.wantKey: {}}, keys)
			for _, metric := range availabilityMetrics {
				key := tableMetricKey(metric)
				assert.NotContains(t, key, "profile-")
				assert.NotContains(t, key, metric.Tags["_local_address"])
				assert.NotContains(t, key, metric.Tags["_peer_description"])
				assert.Contains(t, metric.Tags, "_local_address")
				assert.Contains(t, metric.Tags, "_peer_description")
			}
		})
	}
}

func requireMetric(t *testing.T, metrics []ddsnmp.Metric, name string, subset map[string]string) ddsnmp.Metric {
	t.Helper()
	for _, metric := range metrics {
		if metric.Name != name {
			continue
		}
		if len(subset) == 0 {
			return metric
		}
		matches := true
		for key, want := range subset {
			if metric.Tags[key] != want {
				matches = false
				break
			}
		}
		if matches {
			return metric
		}
	}
	require.Failf(t, "metric not found", "name=%q tags=%v metrics=%v", name, subset, metrics)
	return ddsnmp.Metric{}
}

func TestBGPPeerCache_UpdateRow(t *testing.T) {
	tests := map[string]struct {
		rows     []ddsnmp.BGPRow
		params   funcapi.ResolvedParams
		validate func(t *testing.T, resp *funcapi.FunctionResponse)
	}{
		"typed rows populate function cache without underscore tags": {
			rows: []ddsnmp.BGPRow{
				typedBGPPeerRow(),
				typedBGPPeerFamilyRow(),
			},
			params: resolveBGPPeerParams(map[string][]string{bgpPeersParamView: {bgpPeersViewAll}}),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				rows := responseRows(t, resp)
				require.Len(t, rows, 2)

				assert.Equal(t, "peer", rows[0][findBGPPeerColIdx("Scope")])
				assert.Equal(t, "192.0.2.1", rows[0][findBGPPeerColIdx("Neighbor")])
				assert.Equal(t, "198.51.100.1", rows[0][findBGPPeerColIdx("Local Address")])
				assert.Equal(t, "Transit Peer", rows[0][findBGPPeerColIdx("Peer Description")])
				assert.Equal(t, "enabled", rows[0][findBGPPeerColIdx("Admin Status")])
				assert.Equal(t, "established", rows[0][findBGPPeerColIdx("Connection State")])
				assert.Equal(t, "Cease (subcode 0)", rows[0][findBGPPeerColIdx("Last Error")])

				assert.Equal(t, "peer family", rows[1][findBGPPeerColIdx("Scope")])
				assert.Equal(t, "ipv4 unicast", rows[1][findBGPPeerColIdx("Family")])
				assert.Equal(t, "restart time wait", rows[1][findBGPPeerColIdx("GR State")])
				assert.Equal(t, "peer not ready", rows[1][findBGPPeerColIdx("Unavailability Reason")])
			},
		},
		"same peer identity from different profile rows keeps distinct cache rows": {
			rows: []ddsnmp.BGPRow{
				typedBGPPeerRowWithIdentity("profile-a-peer", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished),
				typedBGPPeerRowWithIdentity("profile-b-peer", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateActive),
			},
			params: resolveBGPPeerParams(nil),
			validate: func(t *testing.T, resp *funcapi.FunctionResponse) {
				assert.Equal(t, 200, resp.Status)
				rows := responseRows(t, resp)
				require.Len(t, rows, 2)

				stateByRowID := make(map[string]string, len(rows))
				for _, row := range rows {
					assert.Equal(t, "192.0.2.1", row[findBGPPeerColIdx("Neighbor")])
					assert.Equal(t, "65001", row[findBGPPeerColIdx("Remote AS")])
					rowID, ok := row[findBGPPeerColIdx("rowId")].(string)
					require.True(t, ok)
					state, ok := row[findBGPPeerColIdx("Connection State")].(string)
					require.True(t, ok)
					stateByRowID[rowID] = state
				}

				assert.Equal(t, map[string]string{
					"profile-a-peer": "established",
					"profile-b-peer": "active",
				}, stateByRowID)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newBGPPeerCache()
			cache.reset()
			for _, row := range tc.rows {
				cache.updateRow("_vendor-bgp.yaml", row)
			}
			cache.finalize()

			resp := newTestFuncBGPPeers(cache).Handle(context.Background(), bgpPeersMethodID, tc.params)
			tc.validate(t, resp)
		})
	}
}

func TestBGPIntegration_PreservesFunctionCacheOnProfileBGPError(t *testing.T) {
	tests := map[string]struct {
		initial []*ddsnmp.ProfileMetrics
		failed  []*ddsnmp.ProfileMetrics
	}{
		"profile BGP failure marks existing function rows stale instead of deleting them": {
			initial: []*ddsnmp.ProfileMetrics{{
				Source:  "typed-bgp-profile.yaml",
				BGPRows: []ddsnmp.BGPRow{typedBGPPeerRow()},
			}},
			failed: []*ddsnmp.ProfileMetrics{{
				Source:          "typed-bgp-profile.yaml",
				BGPCollectError: errors.New("BGP table walk failed"),
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.sysInfo = &snmputils.SysInfo{}
			collr.enableBGPIntegration()
			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.initial}

			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp := newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)
			require.Len(t, responseRows(t, resp), 1)

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.failed}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp = newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)
			assert.Contains(t, resp.Help, "showing stale BGP rows")
			rows := responseRows(t, resp)
			require.Len(t, rows, 1)
			assert.Equal(t, true, rows[0][findBGPPeerColIdx("Stale")])
		})
	}
}

func TestBGPIntegration_RecoveryClearsStaleFunctionRows(t *testing.T) {
	tests := map[string]struct {
		initial   []*ddsnmp.ProfileMetrics
		failed    []*ddsnmp.ProfileMetrics
		recovered []*ddsnmp.ProfileMetrics
	}{
		"success then failure then success clears stale flag and help banner": {
			initial: []*ddsnmp.ProfileMetrics{{
				Source:  "typed-bgp-profile.yaml",
				BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-a", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished)},
			}},
			failed: []*ddsnmp.ProfileMetrics{{
				Source:          "typed-bgp-profile.yaml",
				BGPCollectError: errors.New("BGP table walk failed"),
			}},
			recovered: []*ddsnmp.ProfileMetrics{{
				Source:  "typed-bgp-profile.yaml",
				BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-a", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateActive)},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.sysInfo = &snmputils.SysInfo{}
			collr.enableBGPIntegration()

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.initial}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.failed}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp := newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)
			assert.Contains(t, resp.Help, "showing stale BGP rows")
			rows := responseRows(t, resp)
			require.Len(t, rows, 1)
			assert.Equal(t, true, rows[0][findBGPPeerColIdx("Stale")])

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.recovered}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp = newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)
			assert.NotContains(t, resp.Help, "stale BGP rows")
			rows = responseRows(t, resp)
			require.Len(t, rows, 1)
			assert.Equal(t, false, rows[0][findBGPPeerColIdx("Stale")])
			assert.Equal(t, "active", rows[0][findBGPPeerColIdx("Connection State")])
		})
	}
}

func TestBGPIntegration_MixedProfileBGPErrorRefreshesSuccessfulProfiles(t *testing.T) {
	tests := map[string]struct {
		initial []*ddsnmp.ProfileMetrics
		next    []*ddsnmp.ProfileMetrics
	}{
		"failed profile keeps stale rows while successful profile refreshes": {
			initial: []*ddsnmp.ProfileMetrics{
				{
					Source:  "profile-a.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-a", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished)},
				},
				{
					Source:  "profile-b.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-b", "192.0.2.2", "65002", "blue", ddprofiledefinition.BGPPeerStateEstablished)},
				},
			},
			next: []*ddsnmp.ProfileMetrics{
				{
					Source:          "profile-a.yaml",
					BGPCollectError: errors.New("BGP table walk failed"),
				},
				{
					Source:  "profile-b.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-b", "192.0.2.2", "65002", "blue", ddprofiledefinition.BGPPeerStateActive)},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.sysInfo = &snmputils.SysInfo{}
			collr.enableBGPIntegration()
			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.initial}

			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.next}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp := newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)
			assert.Contains(t, resp.Help, "showing stale BGP rows")

			byNeighbor := bgpPeerRowsByNeighbor(t, responseRows(t, resp))
			require.Len(t, byNeighbor, 2)

			assert.Equal(t, true, byNeighbor["192.0.2.1"][findBGPPeerColIdx("Stale")])
			assert.Equal(t, "established", byNeighbor["192.0.2.1"][findBGPPeerColIdx("Connection State")])
			assert.Equal(t, false, byNeighbor["192.0.2.2"][findBGPPeerColIdx("Stale")])
			assert.Equal(t, "active", byNeighbor["192.0.2.2"][findBGPPeerColIdx("Connection State")])
		})
	}
}

func TestBGPIntegration_ExpiredFailedProfileDoesNotKeepStaleRowsWithFreshProfiles(t *testing.T) {
	tests := map[string]struct {
		initial []*ddsnmp.ProfileMetrics
		next    []*ddsnmp.ProfileMetrics
	}{
		"expired failed profile row is omitted while successful profile refreshes": {
			initial: []*ddsnmp.ProfileMetrics{
				{
					Source:  "profile-a.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-a", "192.0.2.1", "65001", "blue", ddprofiledefinition.BGPPeerStateEstablished)},
				},
				{
					Source:  "profile-b.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-b", "192.0.2.2", "65002", "blue", ddprofiledefinition.BGPPeerStateEstablished)},
				},
			},
			next: []*ddsnmp.ProfileMetrics{
				{
					Source:          "profile-a.yaml",
					BGPCollectError: errors.New("BGP table walk failed"),
				},
				{
					Source:  "profile-b.yaml",
					BGPRows: []ddsnmp.BGPRow{typedBGPPeerRowWithIdentity("peer-b", "192.0.2.2", "65002", "blue", ddprofiledefinition.BGPPeerStateActive)},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.sysInfo = &snmputils.SysInfo{}
			collr.enableBGPIntegration()
			collr.bgp.setStaleAfter(time.Minute)
			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.initial}

			require.NoError(t, collr.collectSNMP(map[string]int64{}))
			markBGPCacheSourceLastUpdate(t, collr.bgp.peerCache, "profile-a.yaml", time.Now().Add(-2*time.Minute))

			collr.ddSnmpColl = &mockDdSnmpCollector{pms: tc.next}
			require.NoError(t, collr.collectSNMP(map[string]int64{}))

			resp := newTestFuncBGPPeers(collr.bgp.peerCache).Handle(context.Background(), bgpPeersMethodID, nil)
			assert.Equal(t, 200, resp.Status)

			byNeighbor := bgpPeerRowsByNeighbor(t, responseRows(t, resp))
			require.Len(t, byNeighbor, 1)
			require.NotContains(t, byNeighbor, "192.0.2.1")
			require.Contains(t, byNeighbor, "192.0.2.2")
			assert.Equal(t, false, byNeighbor["192.0.2.2"][findBGPPeerColIdx("Stale")])
			assert.Equal(t, "active", byNeighbor["192.0.2.2"][findBGPPeerColIdx("Connection State")])
			requireBGPCacheKeyAbsent(t, collr.bgp.peerCache, "peer-a")
		})
	}
}

func TestProfilesHaveBGP(t *testing.T) {
	tests := map[string]struct {
		profiles []*ddsnmp.Profile
		want     bool
	}{
		"typed BGP row enables integration": {
			profiles: []*ddsnmp.Profile{{
				Definition: &ddprofiledefinition.ProfileDefinition{
					BGP: []ddprofiledefinition.BGPConfig{{ID: "peer"}},
				},
			}},
			want: true,
		},
		"legacy BGP virtual metric does not enable integration": {
			profiles: []*ddsnmp.Profile{{
				Definition: &ddprofiledefinition.ProfileDefinition{
					VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{{Name: "bgpPeerAvailability"}},
				},
			}},
			want: false,
		},
		"legacy BGP raw metric does not enable integration": {
			profiles: []*ddsnmp.Profile{{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{{
						Symbols: []ddprofiledefinition.SymbolConfig{{Name: "bgpPeerState"}},
					}},
				},
			}},
			want: false,
		},
		"ordinary metrics do not enable integration": {
			profiles: []*ddsnmp.Profile{{
				Definition: &ddprofiledefinition.ProfileDefinition{
					Metrics: []ddprofiledefinition.MetricsConfig{{
						Symbol: ddprofiledefinition.SymbolConfig{Name: "sysUpTime"},
					}},
				},
			}},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, profilesHaveBGP(tc.profiles))
		})
	}
}

func typedBGPPeerRow() ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		OriginProfileID: "_vendor-bgp.yaml",
		Kind:            ddprofiledefinition.BGPRowKindPeer,
		StructuralID:    "typed-peer-key",
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: "blue",
			Neighbor:        "192.0.2.1",
			RemoteAS:        "65001",
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress: "198.51.100.1",
			Description:  "Transit Peer",
		},
		Admin: ddsnmp.BGPAdmin{
			Enabled: ddsnmp.BGPBool{Has: true, Value: true},
		},
		State: ddsnmp.BGPState{
			Has:   true,
			State: ddprofiledefinition.BGPPeerStateEstablished,
		},
		Previous: ddsnmp.BGPState{
			Has:   true,
			State: ddprofiledefinition.BGPPeerStateIdle,
		},
		Connection: ddsnmp.BGPConnection{
			EstablishedUptime: ddsnmp.BGPInt64{Has: true, Value: 1234},
		},
		Traffic: ddsnmp.BGPTraffic{
			Updates: ddsnmp.BGPDirectional{
				Received: ddsnmp.BGPInt64{Has: true, Value: 10},
				Sent:     ddsnmp.BGPInt64{Has: true, Value: 20},
			},
		},
		LastError: ddsnmp.BGPLastError{
			Code:    ddsnmp.BGPInt64{Has: true, Value: 6},
			Subcode: ddsnmp.BGPInt64{Has: true, Value: 0},
		},
	}
}

func typedBGPPeerRowWithIdentity(structuralID, neighbor, remoteAS, routingInstance string, state ddprofiledefinition.BGPPeerState) ddsnmp.BGPRow {
	row := typedBGPPeerRow()
	row.StructuralID = structuralID
	row.Identity.Neighbor = neighbor
	row.Identity.RemoteAS = remoteAS
	row.Identity.RoutingInstance = routingInstance
	row.State.State = state
	row.Tags = nil
	return row
}

func bgpPeerRowsByNeighbor(t *testing.T, rows [][]any) map[string][]any {
	t.Helper()
	result := make(map[string][]any, len(rows))
	for _, row := range rows {
		neighbor, ok := row[findBGPPeerColIdx("Neighbor")].(string)
		require.True(t, ok)
		result[neighbor] = row
	}
	return result
}

func markBGPCacheSourceLastUpdate(t *testing.T, cache *bgpPeerCache, source string, lastUpdate time.Time) {
	t.Helper()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, entry := range cache.entries {
		if entry.source == source {
			entry.lastUpdate = lastUpdate
			return
		}
	}
	t.Fatalf("BGP cache source %q not found", source)
}

func requireBGPCacheKeyAbsent(t *testing.T, cache *bgpPeerCache, key string) {
	t.Helper()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	require.NotContains(t, cache.entries, key)
}

func typedBGPPeerFamilyRow() ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		OriginProfileID: "_vendor-bgp.yaml",
		Kind:            ddprofiledefinition.BGPRowKindPeerFamily,
		StructuralID:    "typed-peer-family-key",
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance:         "blue",
			Neighbor:                "192.0.2.1",
			RemoteAS:                "65001",
			AddressFamily:           ddprofiledefinition.BGPAddressFamilyIPv4,
			SubsequentAddressFamily: ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
		},
		Routes: ddsnmp.BGPRoutes{
			Current: ddsnmp.BGPRouteCounters{
				Accepted:   ddsnmp.BGPInt64{Has: true, Value: 540336},
				Advertised: ddsnmp.BGPInt64{Has: true, Value: 540339},
			},
		},
		Restart: ddsnmp.BGPGracefulRestart{
			State: ddsnmp.BGPText{Has: true, Value: "restart_time_wait"},
		},
		Reasons: ddsnmp.BGPReasons{
			Unavailability: ddsnmp.BGPText{Has: true, Value: "peer_not_ready"},
		},
	}
}
