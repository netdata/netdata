// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestTopologyProductionPath_UniFiCapabilityProfiles(t *testing.T) {
	tests := map[string]struct {
		device      ddsnmp.DeviceConnectionInfo
		pdus        []gosnmp.SnmpPDU
		wantKinds   []ddsnmp.TopologyKind
		absentKinds []ddsnmp.TopologyKind
	}{
		"access point collects legacy ARP only": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.20",
				SysObjectID: "1.3.6.1.4.1.41112",
				SysDescr:    "Ubiquiti UniFi U6 Mesh",
			},
			pdus:        topologyUniFiAPPDUs(),
			wantKinds:   []ddsnmp.TopologyKind{ddsnmp.KindArpLegacyEntry},
			absentKinds: []ddsnmp.TopologyKind{ddsnmp.KindQbridgeFdbEntry, ddsnmp.KindFdbEntry, ddsnmp.KindStpPort, ddsnmp.KindLldpRem},
		},
		"switch collects Q-BRIDGE interface and legacy ARP": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.10",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysDescr:    "Linux UBNT UniFi Switch",
			},
			pdus:        topologyUniFiSwitchPDUs(),
			wantKinds:   []ddsnmp.TopologyKind{ddsnmp.KindIfName, ddsnmp.KindQbridgeFdbEntry, ddsnmp.KindQbridgeVlanEntry, ddsnmp.KindArpLegacyEntry},
			absentKinds: []ddsnmp.TopologyKind{ddsnmp.KindFdbEntry, ddsnmp.KindStpPort, ddsnmp.KindLldpRem},
		},
		"gateway collects modern and legacy ARP only": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.1",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysDescr:    "Linux Router 6.6.43-ui-ipq9574",
			},
			pdus:        topologyUniFiGatewayPDUs(),
			wantKinds:   []ddsnmp.TopologyKind{ddsnmp.KindArpEntry, ddsnmp.KindArpLegacyEntry},
			absentKinds: []ddsnmp.TopologyKind{ddsnmp.KindQbridgeFdbEntry, ddsnmp.KindFdbEntry, ddsnmp.KindStpPort, ddsnmp.KindLldpRem},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := collectTopologyProfileMetrics(t, tc.device, tc.pdus)
			kinds := collectedTopologyKinds(metrics)
			for _, kind := range tc.wantKinds {
				assert.Contains(t, kinds, kind)
			}
			for _, kind := range tc.absentKinds {
				assert.NotContains(t, kinds, kind)
			}
		})
	}
}

func TestTopologyProductionPath_UniFiQBridgeRendersDefaultManagedFabric(t *testing.T) {
	switchDevice := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
		SysName:     "unifi-switch",
		SysDescr:    "Linux UBNT UniFi Switch",
	}
	switchMetrics := collectTopologyProfileMetrics(t, switchDevice, topologyUniFiSwitchPDUs())
	switchCache := newTestTopologyCache(switchDevice)
	switchCache.updateTopologyProfileTags(switchMetrics)
	switchCache.ingestTopologyProfileMetrics(switchMetrics)
	switchCache.finalize()

	switchObservation := switchCache.buildEngineObservation(switchCache.localDevice)
	require.Len(t, switchObservation.FDBEntries, 2)
	for _, entry := range switchObservation.FDBEntries {
		assert.Equal(t, "7", entry.BridgePort)
		assert.Zero(t, entry.IfIndex, "an unmapped bridge base-port must remain outside the ifIndex namespace")
		assert.Equal(t, "20", entry.VLANID)
	}
	require.NotEmpty(t, switchObservation.ARPNDEntries)

	gatewayDevice := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.1",
		SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
		SysName:     "unifi-gateway",
		SysDescr:    "Linux Router 6.6.43-ui-ipq9574",
	}
	gatewayMetrics := collectTopologyProfileMetrics(t, gatewayDevice, topologyUniFiGatewayPDUs())
	gatewayCache := newTestTopologyCache(gatewayDevice)
	gatewayCache.ingestTopologyProfileMetrics(gatewayMetrics)
	gatewayCache.finalize()
	apDevice := ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.20",
		SysObjectID: "1.3.6.1.4.1.41112",
		SysName:     "unifi-ap",
		SysDescr:    "Ubiquiti UniFi U6 Mesh",
	}
	apMetrics := collectTopologyProfileMetrics(t, apDevice, topologyUniFiAPPDUs())
	apCache := newTestTopologyCache(apDevice)
	apCache.ingestTopologyProfileMetrics(apMetrics)
	apCache.finalize()

	registry := newTopologyRegistry()
	publishTestTopologyBuilder(registry, switchCache)
	publishTestTopologyBuilder(registry, gatewayCache)
	publishTestTopologyBuilder(registry, apCache)
	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok)
	assert.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "bridge"), 1)
	assert.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "fdb"), 1)
}

func TestTopologyProductionPath_UniFiActorsUseProfileIdentity(t *testing.T) {
	tests := map[string]struct {
		device            ddsnmp.DeviceConnectionInfo
		pdus              []gosnmp.SnmpPDU
		wantProfileVendor bool
		wantProfileModel  string
		wantVendor        string
		wantModel         string
	}{
		"access point prefers dedicated model OID": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.20",
				SysObjectID: "1.3.6.1.4.1.41112",
				SysName:     "unifi-ap",
				SysDescr:    "device model UAP-AC-LR 6.8.2.15592",
				Vendor:      "Unknown",
				Model:       "UniFi UAP-FlexHD",
			},
			pdus:              append(topologyUniFiAPPDUs(), topologySysDescrPDU("device model UAP-AC-LR 6.8.2.15592")),
			wantProfileVendor: true,
			wantProfileModel:  "UniFi U6-Mesh",
			wantVendor:        "Ubiquiti",
			wantModel:         "UniFi U6-Mesh",
		},
		"U6 Mesh access point falls back to sysDescr": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.21",
				SysObjectID: "1.3.6.1.4.1.41112",
				SysName:     "unifi-u6-mesh",
				SysDescr:    "Ubiquiti UniFi U6-Mesh 6.8.2.15592",
				Vendor:      "Unknown",
				Model:       "Unknown",
			},
			pdus: []gosnmp.SnmpPDU{
				topologyOctetStringPDU("1.3.6.1.4.1.41112.1.6.3.3.0", ""),
				topologySysDescrPDU("Ubiquiti UniFi U6-Mesh 6.8.2.15592"),
			},
			wantProfileVendor: true,
			wantProfileModel:  "UniFi U6-Mesh",
			wantVendor:        "Ubiquiti",
			wantModel:         "UniFi U6-Mesh",
		},
		"UAP AC LR access point falls back to sysDescr": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.22",
				SysObjectID: "1.3.6.1.4.1.41112",
				SysName:     "unifi-uap-ac-lr",
				SysDescr:    "device model UAP-AC-LR 6.8.2.15592",
				Vendor:      "Unknown",
				Model:       "Unknown",
			},
			pdus:              []gosnmp.SnmpPDU{topologySysDescrPDU("device model UAP-AC-LR 6.8.2.15592")},
			wantProfileVendor: true,
			wantProfileModel:  "UniFi UAP-AC-LR",
			wantVendor:        "Ubiquiti",
			wantModel:         "UniFi UAP-AC-LR",
		},
		"UDR7 resolves explicit generic Net-SNMP identity": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.2",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysName:     "unifi-udr7",
				SysDescr:    "Linux appliance UniFi UDR7 5.1.19 ipq5322",
				Vendor:      "net-snmp",
				Model:       "Linux",
			},
			pdus:              []gosnmp.SnmpPDU{topologySysDescrPDU("Linux appliance UniFi UDR7 5.1.19 ipq5322")},
			wantProfileVendor: true,
			wantProfileModel:  "UniFi UDR7",
			wantVendor:        "Ubiquiti",
			wantModel:         "UniFi UDR7",
		},
		"U6 Mesh Pro access point falls back to the longest matching model": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.23",
				SysObjectID: "1.3.6.1.4.1.41112",
				SysName:     "unifi-u6-mesh-pro",
				SysDescr:    "Ubiquiti UniFi U6-Mesh-Pro 6.8.2",
				Vendor:      "Unknown",
				Model:       "Unknown",
			},
			pdus:              []gosnmp.SnmpPDU{topologySysDescrPDU("Ubiquiti UniFi U6-Mesh-Pro 6.8.2")},
			wantProfileVendor: true,
			wantProfileModel:  "UniFi U6-Mesh-Pro",
			wantVendor:        "Ubiquiti",
			wantModel:         "UniFi U6-Mesh-Pro",
		},
		"switch overrides generic Net-SNMP vendor": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.10",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysName:     "unifi-switch",
				SysDescr:    "Linux UBNT 4.4.153 mips",
				Vendor:      "net-snmp",
				Model:       "Linux",
			},
			pdus:              append(topologyUniFiSwitchPDUs(), topologySysDescrPDU("Linux UBNT 4.4.153 mips")),
			wantProfileVendor: true,
			wantVendor:        "Ubiquiti",
			wantModel:         "Linux",
		},
		"gateway overrides generic Net-SNMP vendor": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.1",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysName:     "unifi-gateway",
				SysDescr:    "Linux Router 6.6.43-ui-ipq9574 aarch64",
				Vendor:      "net-snmp",
				Model:       "Linux",
			},
			pdus:              append(topologyUniFiGatewayPDUs(), topologySysDescrPDU("Linux Router 6.6.43-ui-ipq9574 aarch64")),
			wantProfileVendor: true,
			wantVendor:        "Ubiquiti",
			wantModel:         "Linux",
		},
		"unrelated generic Net-SNMP host keeps fallback identity": {
			device: ddsnmp.DeviceConnectionInfo{
				Hostname:    "192.0.2.30",
				SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
				SysName:     "linux-host",
				SysDescr:    "Linux generic Net-SNMP agent",
				Vendor:      "net-snmp",
				Model:       "Linux",
			},
			pdus:       []gosnmp.SnmpPDU{topologySysDescrPDU("Linux generic Net-SNMP agent")},
			wantVendor: "net-snmp",
			wantModel:  "Linux",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			device := tc.device
			mainMetrics := collectMainProfileMetrics(t, device, tc.pdus)
			metadata := make(map[string]ddsnmp.MetaTag)
			for _, pm := range mainMetrics {
				ddsnmp.MergeDeviceIdentityMetadata(metadata, pm.DeviceMetadata)
			}
			if tc.wantProfileVendor {
				assert.Equal(t, ddsnmp.MetaTag{Value: "Ubiquiti", IsExactMatch: true}, metadata["vendor"])
			} else {
				assert.NotContains(t, metadata, "vendor")
			}
			if tc.wantProfileModel != "" {
				assert.Equal(t, ddsnmp.MetaTag{Value: tc.wantProfileModel, IsExactMatch: true}, metadata["model"])
			} else {
				assert.NotContains(t, metadata, "model")
			}
			device.Vendor, device.Model = ddsnmp.ResolveDeviceIdentity(device.Vendor, device.Model, metadata, nil)

			metrics := collectTopologyProfileMetrics(t, device, tc.pdus)
			cache := newTestTopologyCache(device)
			cache.updateTopologyProfileTags(metrics)
			cache.ingestTopologyProfileMetrics(metrics)
			cache.finalize()

			registry := newTopologyRegistry()
			publishTestTopologyBuilder(registry, cache)
			data, ok := snapshotTopologyRegistryForTest(registry)
			require.True(t, ok)

			actor := findDeviceActorBySysName(data, device.SysName)
			require.NotNil(t, actor)
			assert.Equal(t, tc.wantVendor, actor.Detail.SNMP.Vendor)
			assert.Equal(t, tc.wantModel, actor.Detail.SNMP.Model)
		})
	}
}

func collectMainProfileMetrics(
	t *testing.T,
	device ddsnmp.DeviceConnectionInfo,
	pdus []gosnmp.SnmpPDU,
) []*ddsnmp.ProfileMetrics {
	t.Helper()

	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID: device.SysObjectID,
		SysDescr:    device.SysDescr,
	}).Project(ddsnmp.ConsumerMetrics, ddsnmp.ConsumerLicensing, ddsnmp.ConsumerBGP).Profiles()
	require.NotEmpty(t, profiles)
	metrics, err := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  newTopologySNMPHandler(pdus),
		Profiles:    profiles,
		Log:         newTestSNMPTopologyCollector().Logger,
		SysObjectID: device.SysObjectID,
	}).Collect()
	require.NoError(t, err)
	return metrics
}

func collectTopologyProfileMetrics(
	t *testing.T,
	device ddsnmp.DeviceConnectionInfo,
	pdus []gosnmp.SnmpPDU,
) []*ddsnmp.ProfileMetrics {
	t.Helper()

	profiles := (&Collector{}).findTopologyProfiles(device)
	require.NotEmpty(t, profiles)
	metrics, err := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  newTopologySNMPHandler(pdus),
		Profiles:    profiles,
		Log:         newTestSNMPTopologyCollector().Logger,
		SysObjectID: device.SysObjectID,
	}).Collect()
	require.NoError(t, err)
	return metrics
}

func collectedTopologyKinds(metrics []*ddsnmp.ProfileMetrics) map[ddsnmp.TopologyKind]struct{} {
	kinds := make(map[ddsnmp.TopologyKind]struct{})
	for _, profileMetrics := range metrics {
		for _, metric := range profileMetrics.TopologyMetrics {
			kinds[metric.TopologyKind] = struct{}{}
		}
	}
	return kinds
}

func topologyUniFiSwitchPDUs() []gosnmp.SnmpPDU {
	return []gosnmp.SnmpPDU{
		// The bridge identity is present, while dot1dBasePortIfIndex is intentionally absent.
		topologyOctetPDU("1.3.6.1.2.1.17.1.1.0", 0x02, 0, 0, 0, 0, 0x10),
		topologyOctetStringPDU("1.3.6.1.2.1.31.1.1.1.1.7", "Port 7"),
		topologyIntegerPDU("1.3.6.1.2.1.31.1.1.1.15.7", 1000),
		topologyOctetStringPDU("1.3.6.1.2.1.2.2.1.2.7", "Port 7"),
		topologyIntegerPDU("1.3.6.1.2.1.2.2.1.7.7", 1),
		topologyIntegerPDU("1.3.6.1.2.1.2.2.1.8.7", 1),
		topologyIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.2.100.2.0.0.0.0.1", 7),
		topologyIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.3.100.2.0.0.0.0.1", 3),
		topologyIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.2.100.2.0.0.0.0.2", 7),
		topologyIntegerPDU("1.3.6.1.2.1.17.7.1.2.2.1.3.100.2.0.0.0.0.2", 3),
		topologyIntegerPDU("1.3.6.1.2.1.17.7.1.4.2.1.3.0.20", 100),
		topologyOctetPDU("1.3.6.1.2.1.4.22.1.2.7.192.0.2.1", 0x02, 0, 0, 0, 0, 1),
		topologyIntegerPDU("1.3.6.1.2.1.4.22.1.4.7.192.0.2.1", 3),
		topologyOctetPDU("1.3.6.1.2.1.4.22.1.2.7.192.0.2.20", 0x02, 0, 0, 0, 0, 2),
		topologyIntegerPDU("1.3.6.1.2.1.4.22.1.4.7.192.0.2.20", 3),
	}
}

func topologyUniFiAPPDUs() []gosnmp.SnmpPDU {
	return []gosnmp.SnmpPDU{
		topologyOctetStringPDU("1.3.6.1.4.1.41112.1.6.3.3.0", "U6-Mesh"),
		topologyOctetPDU("1.3.6.1.2.1.4.22.1.2.17.192.0.2.1", 0x02, 0, 0, 0, 0, 1),
		topologyIntegerPDU("1.3.6.1.2.1.4.22.1.4.17.192.0.2.1", 3),
	}
}

func topologyUniFiGatewayPDUs() []gosnmp.SnmpPDU {
	return []gosnmp.SnmpPDU{
		topologyOctetPDU("1.3.6.1.2.1.4.35.1.4.3.1.4.192.0.2.10", 0x02, 0, 0, 0, 0, 0x10),
		topologyIntegerPDU("1.3.6.1.2.1.4.35.1.6.3.1.4.192.0.2.10", 3),
		topologyIntegerPDU("1.3.6.1.2.1.4.35.1.7.3.1.4.192.0.2.10", 1),
		topologyOctetPDU("1.3.6.1.2.1.4.22.1.2.3.192.0.2.10", 0x02, 0, 0, 0, 0, 0x10),
		topologyIntegerPDU("1.3.6.1.2.1.4.22.1.4.3.192.0.2.10", 3),
	}
}

func topologyIntegerPDU(name string, value int) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{Name: name, Type: gosnmp.Integer, Value: value}
}

func topologyOctetPDU(name string, value ...byte) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{Name: name, Type: gosnmp.OctetString, Value: value}
}

func topologyOctetStringPDU(name, value string) gosnmp.SnmpPDU {
	return topologyOctetPDU(name, []byte(value)...)
}

func topologySysDescrPDU(value string) gosnmp.SnmpPDU {
	return topologyOctetStringPDU("1.3.6.1.2.1.1.1.0", value)
}
