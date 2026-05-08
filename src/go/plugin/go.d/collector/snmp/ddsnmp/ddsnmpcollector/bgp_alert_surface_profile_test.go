// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_BGPAlertSurface_FromMatchedProfiles(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		profileFile string
		tables      map[string][]string
		virtuals    []string
		walks       func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU
		assertions  func(t *testing.T, metrics []ddsnmp.Metric)
	}{
		"Arista switch emits the common contract and accepted prefixes with a neighbor label derived from the row index": {
			sysObjectID: "1.3.6.1.4.1.30065.1.3011.7010.427.48",
			profileFile: "arista-switch.yaml",
			tables: map[string][]string{
				"aristaBgp4V2PeerTable":         {"aristaBgp4V2PeerAdminStatus", "aristaBgp4V2PeerState"},
				"aristaBgp4V2PeerCountersTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"aristaBgp4V2PrefixGaugesTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "7.1.4.192.0.2.12"
				prefixIdx := idx + ".1.128"
				return map[string][]gosnmp.SnmpPDU{
					"aristaBgp4V2PeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerState"), idx), 6),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_as"), idx), 65000),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "remote_as"), idx), 65001),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_identifier"), idx), gosnmp.IPAddress, "198.51.100.12"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.12"),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "peer_description"), idx), "upstream-a"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "aristaBgp4V2PeerTable", "local_address"), idx), gosnmp.OctetString, []byte{198, 51, 100, 12}),
					},
					"aristaBgp4V2PeerCountersTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerInUpdates"), idx), 16),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerOutUpdates"), idx), 17),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), idx), 2),
					},
					"aristaBgp4V2PrefixGaugesTable": {
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "aristaBgp4V2PrefixGaugesTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1300),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "7",
					"neighbor":         "192.0.2.12",
					"remote_as":        "65001",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor": "192.0.2.12",
				})
				assert.Equal(t, map[string]int64{"received": 16, "sent": 17}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor": "192.0.2.12",
				})
				assert.EqualValues(t, 2, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.12",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1300, accepted.Value)
			},
		},
		"Dell OS10 emits the common contract and accepted prefixes with a neighbor label derived from the row index": {
			sysObjectID: "1.3.6.1.4.1.674.11000.5000.100.2.1.1",
			profileFile: "dell-os10.yaml",
			tables: map[string][]string{
				"dell.os10bgp4V2PeerTable":         {"dell.os10bgp4V2PeerAdminStatus", "dell.os10bgp4V2PeerState"},
				"dell.os10bgp4V2PeerCountersTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
				"dell.os10bgp4V2PrefixGaugesTable": {"bgpPeerPrefixesAccepted"},
			},
			virtuals: []string{"bgpPeerAvailability", "bgpPeerUpdates"},
			walks: func(t *testing.T, p *ddsnmp.Profile) map[string][]gosnmp.SnmpPDU {
				idx := "9.1.4.192.0.2.13"
				prefixIdx := idx + ".1.128"
				return map[string][]gosnmp.SnmpPDU{
					"dell.os10bgp4V2PeerTable": {
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerTable", "dell.os10bgp4V2PeerAdminStatus"), idx), 2),
						createIntegerPDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerTable", "dell.os10bgp4V2PeerState"), idx), 6),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_as"), idx), 65010),
						createGauge32PDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "remote_as"), idx), 65011),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_identifier"), idx), gosnmp.IPAddress, "198.51.100.13"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.13"),
						createStringPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "peer_description"), idx), "upstream-b"),
						createPDU(oidWithIndex(requireMetricTagOID(t, p, "dell.os10bgp4V2PeerTable", "local_address"), idx), gosnmp.OctetString, []byte{198, 51, 100, 13}),
					},
					"dell.os10bgp4V2PeerCountersTable": {
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerInUpdates"), idx), 18),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerOutUpdates"), idx), 19),
						createCounter32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), idx), 6),
					},
					"dell.os10bgp4V2PrefixGaugesTable": {
						createGauge32PDU(oidWithIndex(requireSymbolOID(t, p, "dell.os10bgp4V2PrefixGaugesTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1400),
					},
				}
			},
			assertions: func(t *testing.T, metrics []ddsnmp.Metric) {
				availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
					"routing_instance": "9",
					"neighbor":         "192.0.2.13",
					"remote_as":        "65011",
				})
				assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

				updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
					"neighbor": "192.0.2.13",
				})
				assert.Equal(t, map[string]int64{"received": 18, "sent": 19}, updates.MultiValue)

				transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
					"neighbor": "192.0.2.13",
				})
				assert.EqualValues(t, 6, transitions.Value)

				accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
					"neighbor":                  "192.0.2.13",
					"address_family":            "ipv4",
					"subsequent_address_family": "vpn",
				})
				assert.EqualValues(t, 1400, accepted.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedProfileByFile(t, tc.sysObjectID, tc.profileFile)
			filterProfileForAlertSurface(profile, tc.tables, tc.virtuals)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			setProfileWalkExpectations(t, mockHandler, profile, tc.walks(t, profile))

			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: tc.sysObjectID,
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			tc.assertions(t, stripProfilePointers(results[0].Metrics))
		})
	}
}

func matchedProfileByFile(t *testing.T, sysObjectID, profileFile string) *ddsnmp.Profile {
	t.Helper()

	matched := ddsnmp.FindProfiles(sysObjectID, "", nil)
	for _, prof := range matched {
		if strings.HasSuffix(prof.SourceFile, profileFile) {
			return prof
		}
	}

	require.FailNowf(t, "missing profile", "expected %s for %s", profileFile, sysObjectID)
	return nil
}

func filterProfileForAlertSurface(profile *ddsnmp.Profile, keepTables map[string][]string, keepVirtuals []string) {
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.Topology = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = nil

	keepTableSymbols := make(map[string]map[string]struct{}, len(keepTables))
	for tableName, symbols := range keepTables {
		set := make(map[string]struct{}, len(symbols))
		for _, symbol := range symbols {
			set[symbol] = struct{}{}
		}
		keepTableSymbols[tableName] = set
	}

	filteredMetrics := make([]ddprofiledefinition.MetricsConfig, 0, len(keepTables))
	for _, metric := range profile.Definition.Metrics {
		if !metric.IsColumn() {
			continue
		}

		keepSymbols, ok := keepTableSymbols[metric.Table.Name]
		if !ok {
			continue
		}

		metric.Symbols = slicesDeleteFunc(metric.Symbols, func(sym ddprofiledefinition.SymbolConfig) bool {
			_, keep := keepSymbols[sym.Name]
			return !keep
		})
		if len(metric.Symbols) == 0 {
			continue
		}
		filteredMetrics = append(filteredMetrics, metric)
	}
	profile.Definition.Metrics = filteredMetrics

	profile.Definition.VirtualMetrics = slicesDeleteFunc(profile.Definition.VirtualMetrics, func(vm ddprofiledefinition.VirtualMetricConfig) bool {
		for _, keep := range keepVirtuals {
			if vm.Name == keep {
				return false
			}
		}
		return true
	})
}

func setProfileWalkExpectations(t *testing.T, mockHandler *snmpmock.MockHandler, profile *ddsnmp.Profile, walks map[string][]gosnmp.SnmpPDU) {
	t.Helper()

	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

	seen := make(map[string]bool)
	for _, metric := range profile.Definition.Metrics {
		if metric.Table.OID == "" || metric.Table.Name == "" || seen[metric.Table.Name] {
			continue
		}
		seen[metric.Table.Name] = true

		pdus, ok := walks[metric.Table.Name]
		require.Truef(t, ok, "missing mocked walk for table %s", metric.Table.Name)
		mockHandler.EXPECT().BulkWalkAll(metric.Table.OID).Return(pdus, nil)
	}
}

func requireSymbolOID(t *testing.T, profile *ddsnmp.Profile, tableName, symbolName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, sym := range metric.Symbols {
			if sym.Name == symbolName {
				return strings.Trim(sym.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing symbol", "missing symbol %s on table %s", symbolName, tableName)
	return ""
}

func requireMetricTagOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.Symbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing tag", "missing tag %s on table %s", tagName, tableName)
	return ""
}

func requireCrossTableTagOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.Symbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing cross-table tag", "missing cross-table tag %s on table %s", tagName, tableName)
	return ""
}

func requireCrossTableLookupOID(t *testing.T, profile *ddsnmp.Profile, tableName, tagName string) string {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		if metric.Table.Name != tableName {
			continue
		}
		for _, tag := range metric.MetricTags {
			if tag.Tag == tagName {
				return strings.Trim(tag.LookupSymbol.OID, ".")
			}
		}
	}

	require.FailNowf(t, "missing lookup tag", "missing lookup tag %s on table %s", tagName, tableName)
	return ""
}

func oidWithIndex(baseOID, index string) string {
	return strings.Trim(baseOID, ".") + "." + strings.Trim(index, ".")
}

func stripProfilePointers(metrics []ddsnmp.Metric) []ddsnmp.Metric {
	out := make([]ddsnmp.Metric, len(metrics))
	for i, metric := range metrics {
		metric.Profile = nil
		out[i] = metric
	}
	return out
}

func requireMetricWithTags(t *testing.T, metrics []ddsnmp.Metric, name string, subset map[string]string) ddsnmp.Metric {
	t.Helper()

	var candidates []map[string]string
	for _, metric := range metrics {
		if metric.Name != name {
			continue
		}
		candidates = append(candidates, metric.Tags)
		if tagsContain(metric.Tags, subset) {
			return metric
		}
	}

	require.FailNowf(t, "missing metric", "missing metric %s with tags %v; candidates: %v", name, subset, candidates)
	return ddsnmp.Metric{}
}

func tagsContain(tags map[string]string, subset map[string]string) bool {
	for k, v := range subset {
		if tags[k] != v {
			return false
		}
	}
	return true
}

func slicesDeleteFunc[T any](s []T, del func(T) bool) []T {
	out := s[:0]
	for _, v := range s {
		if del(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}
