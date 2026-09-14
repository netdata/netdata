// SPDX-License-Identifier: GPL-3.0-or-later

//go:build snmp_topology_fixtures

package snmptopology

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func TestTopologyProductionPath_CatalogFixtureReconcilesLegacyAndModernIPRows(t *testing.T) {
	fixture := loadTopologySNMPRecHandler(t, filepath.Join("../../../../testdata/snmp/snmprec", "iosxe_c9800.snmprec"))
	dev, profileMetrics := collectCatalogFixtureTopology(t, fixture)
	requireModernIPAddressWalkEnvelope(t, fixture.walkRoots)

	type sourceRows struct {
		legacy map[string]string
		modern map[string]string
	}
	rowsByIP := make(map[string]*sourceRows)
	for _, pm := range profileMetrics {
		for _, metric := range pm.TopologyMetrics {
			if metric.TopologyKind != ddsnmp.KindIpIfIndex {
				continue
			}
			ip := metric.Tags[tagTopoIPAddr]
			if ip == "" {
				continue
			}
			rows := rowsByIP[ip]
			if rows == nil {
				rows = &sourceRows{}
				rowsByIP[ip] = rows
			}
			switch metric.Tags[tagTopoIPSource] {
			case topoIPSourceLegacy:
				rows.legacy = metric.Tags
			case topoIPSourceModern:
				rows.modern = metric.Tags
			}
		}
	}

	var ip string
	var rows *sourceRows
	for candidateIP, candidateRows := range rowsByIP {
		if candidateRows.legacy != nil && candidateRows.modern != nil &&
			candidateRows.legacy[tagTopoIfIndex] == candidateRows.modern[tagTopoIfIndex] {
			ip, rows = candidateIP, candidateRows
			break
		}
	}
	require.NotEmpty(t, ip, "fixture should contain one address represented by both IP-MIB generations")

	cache := newTestTopologyCache(dev)
	cache.ingestTopologyProfileMetrics(profileMetrics)
	cache.finalize()

	require.Equal(t, rows.legacy[tagTopoIfIndex], cache.ipIfIndex(ip))
	require.Equal(t, rows.legacy[tagTopoIPMask], cache.ipNetmask(ip))
	_, hasL3 := cache.ipL3Interface(ip)
	require.True(t, hasL3)
	require.Contains(t, cache.localDevice.ManagementAddresses, topologymodel.ManagementAddress{
		Address: ip, AddressType: "ipv4", Source: "ip_mib",
	})
	require.Equal(t, "management_address", cache.trapMatchMethodByIP[ip])
}

func requireModernIPAddressWalkEnvelope(t testing.TB, walkRoots []string) {
	t.Helper()
	const entryOID = "1.3.6.1.2.1.4.34.1"
	var modernRoots []string
	for _, root := range walkRoots {
		if root == entryOID || strings.HasPrefix(root, entryOID+".") {
			modernRoots = append(modernRoots, root)
		}
	}
	require.Len(t, modernRoots, 5, "modern IPv4 inventory must stay within the five-field wire envelope")
	for _, root := range modernRoots {
		parts := strings.Split(strings.TrimPrefix(root, entryOID+"."), ".")
		require.Len(t, parts, 3, "modern route must be a narrow column root with structural family and length suffixes: %s", root)
		require.Equal(t, "1", parts[1], "modern route must be constrained to the IPv4 InetAddressType index: %s", root)
		require.Equal(t, "4", parts[2], "modern route must require the four-octet IPv4 InetAddress length: %s", root)
	}
}

func TestTopologyProductionPath_CatalogFixtureKeepsModernInventoryWithoutPrefix(t *testing.T) {
	fixture := loadTopologySNMPRecHandler(t, filepath.Join("../../../../testdata/snmp/snmprec", "gam.snmprec"))
	dev, profileMetrics := collectCatalogFixtureTopology(t, fixture)

	var modernTags map[string]string
	for _, pm := range profileMetrics {
		for _, metric := range pm.TopologyMetrics {
			if metric.TopologyKind == ddsnmp.KindIpIfIndex &&
				metric.Tags[tagTopoIPSource] == topoIPSourceModern &&
				metric.Tags[tagTopoIPPrefix] == "0.0" {
				modernTags = metric.Tags
				break
			}
		}
		if modernTags != nil {
			break
		}
	}
	require.NotNil(t, modernTags, "fixture should expose an RFC 4293 IPv4 row without a prefix row")
	ip := modernTags[tagTopoIPAddr]
	require.NotEmpty(t, ip)

	cache := newTestTopologyCache(dev)
	cache.ingestTopologyProfileMetrics(profileMetrics)
	cache.finalize()

	require.Equal(t, modernTags[tagTopoIfIndex], cache.ipIfIndex(ip))
	require.Empty(t, cache.ipNetmask(ip))
	_, hasL3 := cache.ipL3Interface(ip)
	require.False(t, hasL3)
	require.Contains(t, cache.localDevice.ManagementAddresses, topologymodel.ManagementAddress{
		Address: ip, AddressType: "ipv4", Source: "ip_mib",
	})
	require.Equal(t, "management_address", cache.trapMatchMethodByIP[ip])
}

func TestTopologyProductionPath_CatalogFixtureRejectsMalformedModernIPv4Indexes(t *testing.T) {
	fixture := loadTopologySNMPRecHandler(t, filepath.Join("../../../../testdata/snmp/snmprec", "iosxe_c9800.snmprec"))
	addMalformedModernIPv4FixtureRow(fixture, "1.16.0.0.0.0.0.0.0.0.0.0.255.255.203.0.113.241", 7001)
	addMalformedModernIPv4FixtureRow(fixture, "1.4.0.0.0.0.0.0.0.0.0.0.255.255.203.0.113.242", 7002)
	addMalformedModernIPv4FixtureRow(fixture, "1.4.1.2.3.4.5.6.7.8", 7003)
	addMalformedModernIPv4FixtureRow(fixture, "1.4.192.0.2.256", 7004)
	addMalformedModernIPv4FixtureRow(fixture, "1.4.123.45.67", 7005)

	dev, profileMetrics := collectCatalogFixtureTopology(t, fixture)
	cache := newTestTopologyCache(dev)
	cache.ingestTopologyProfileMetrics(profileMetrics)
	cache.finalize()

	require.Empty(t, cache.ipIfIndex("203.0.113.241"), "IPv4 type with a 16-octet declared address must be rejected")
	require.Empty(t, cache.ipIfIndex("203.0.113.242"), "IPv4 length with trailing mapped payload must be rejected")
	for _, aliasedAddress := range []string{"18.52.86.120", "25.32.34.86", "1.35.69.103"} {
		require.Empty(t, cache.ipIfIndex(aliasedAddress), "malformed index must not alias to %s", aliasedAddress)
	}
	for _, address := range cache.ipAddressesByIP {
		require.NotContains(t, []string{"7001", "7002", "7003", "7004", "7005"}, address.ifIndex)
	}
}

func addMalformedModernIPv4FixtureRow(fixture *topologySNMPRecHandler, index string, ifIndex int) {
	const entryOID = "1.3.6.1.2.1.4.34.1"
	fixture.addEntries(
		gosnmp.SnmpPDU{Name: entryOID + ".3." + index, Type: gosnmp.Integer, Value: ifIndex},
		gosnmp.SnmpPDU{Name: entryOID + ".4." + index, Type: gosnmp.Integer, Value: 1},
		gosnmp.SnmpPDU{
			Name:  entryOID + ".5." + index,
			Type:  gosnmp.ObjectIdentifier,
			Value: fmt.Sprintf("1.3.6.1.2.1.4.32.1.5.%d.1.4.203.0.113.0.24", ifIndex),
		},
		gosnmp.SnmpPDU{Name: entryOID + ".7." + index, Type: gosnmp.Integer, Value: 1},
		gosnmp.SnmpPDU{Name: entryOID + ".10." + index, Type: gosnmp.Integer, Value: 1},
	)
}

func collectCatalogFixtureTopology(
	t testing.TB,
	fixture *topologySNMPRecHandler,
) (ddsnmp.DeviceConnectionInfo, []*ddsnmp.ProfileMetrics) {
	t.Helper()
	sysObjectPDU, ok := fixture.byOID["1.3.6.1.2.1.1.2.0"]
	require.True(t, ok, "fixture should contain sysObjectID")
	sysObjectID, ok := sysObjectPDU.Value.(string)
	require.True(t, ok)

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           161,
		SNMPVersion:    gosnmp.Version2c.String(),
		SysObjectID:    sysObjectID,
		SysName:        "catalog-fixture",
		MaxOIDs:        60,
		MaxRepetitions: 25,
	}
	profiles := (&Collector{}).findTopologyProfiles(dev)
	require.NotEmpty(t, profiles, "stock catalog should resolve topology profiles")

	profileMetrics, err := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  fixture,
		Profiles:    profiles,
		Log:         newTestSNMPTopologyCollector().Logger,
		SysObjectID: dev.SysObjectID,
	}).Collect()
	require.NoError(t, err)
	require.NotEmpty(t, profileMetrics)
	return dev, profileMetrics
}

func TestTopologyProductionPath_StockIOSXEFixture(t *testing.T) {
	fixture := loadTopologySNMPRecHandler(t, filepath.Join("../../../../testdata/snmp/snmprec", "iosxe_ie32008t2s-ios17-12.snmprec"))
	// Model the reported LLDP-less device behavior at the SNMP boundary while
	// retaining the fixture's real BRIDGE-MIB, interface, and VTP rows.
	fixture.hideOIDPrefix("1.0.8802.1.1.2")
	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           161,
		SNMPVersion:    gosnmp.Version2c.String(),
		SysObjectID:    "1.3.6.1.4.1.9.1.2683",
		SysName:        "iosxe-fixture",
		MaxOIDs:        60,
		MaxRepetitions: 25,
	}

	profiles := (&Collector{}).findTopologyProfiles(dev)
	require.NotEmpty(t, profiles, "stock catalog should resolve Cisco topology profiles")

	profileMetrics, err := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  fixture,
		Profiles:    profiles,
		Log:         newTestSNMPTopologyCollector().Logger,
		SysObjectID: dev.SysObjectID,
	}).Collect()
	require.NoError(t, err)
	require.NotEmpty(t, profileMetrics)

	var foundFDB bool
	var foundVLAN200 bool
	for _, pm := range profileMetrics {
		for _, metric := range pm.TopologyMetrics {
			switch metric.TopologyKind {
			case ddsnmp.KindFdbEntry, ddsnmp.KindQbridgeFdbEntry:
				foundFDB = true
				require.NotEmpty(t, topologyutil.FirstNonEmptyString(metric.Tags[tagFdbMac], metric.Tags[tagDot1qFdbMac]), metric.Tags)
			case ddsnmp.KindVtpVlan:
				if metric.Tags[tagVtpVlanIndex] == "200" && metric.Tags[tagVtpVlanName] == "PLC" {
					foundVLAN200 = true
				}
			}
		}
	}
	require.True(t, foundFDB, "real ddsnmp collection should emit fixture FDB rows")
	require.True(t, foundVLAN200, "real ddsnmp collection should preserve the second VTP index component")

	cache := newTestTopologyCache(dev)
	cache.updateTopologyProfileTags(profileMetrics)
	cache.ingestTopologyProfileMetrics(profileMetrics)
	cache.finalize()

	observation := cache.buildEngineObservation(cache.localDevice)
	require.NotEmpty(t, observation.FDBEntries, "cache should retain collected FDB rows")
	peerMAC := ""
	for _, entry := range observation.FDBEntries {
		mac := topologyutil.NormalizeMAC(entry.MAC)
		if mac != "" && mac != topologyutil.NormalizeMAC(observation.ChassisID) {
			peerMAC = mac
			break
		}
	}
	require.NotEmpty(t, peerMAC, "fixture should expose a learned MAC distinct from the local bridge identity")

	peer := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.11",
		SysObjectID: "1.3.6.1.4.1.8072.3.2.10",
		SysName:     "fixture-fdb-peer",
	})
	peer.localDevice.ChassisID = peerMAC
	peer.localDevice.ChassisIDType = "macAddress"
	peer.localDevice.Capabilities = []string{"bridge"}
	peer.localDevice.Labels = map[string]string{"type": "switch"}
	peer.finalize()

	registry := newTopologyRegistry()
	publishTestTopologyBuilder(registry, cache)
	publishTestTopologyBuilder(registry, peer)
	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok, "default managed-fabric inference should render the collected cache")
	require.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "bridge"), 1,
		"a real collected FDB row should attach the fixture device to a broadcast segment")
	require.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "fdb"), 1,
		"the broadcast segment should resolve the real learned MAC to the synthetic managed peer")
}

func TestTopologyProductionPath_LegacyARPPhysicalOnly(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{
			Name:  "1.3.6.1.2.1.4.22.1.2.17.192.0.2.18",
			Type:  gosnmp.OctetString,
			Value: []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef},
		},
		{
			Name:  "1.3.6.1.2.1.4.22.1.4.17.192.0.2.18",
			Type:  gosnmp.Integer,
			Value: 3,
		},
	}
	handler := &topologySNMPRecHandler{
		Handler: gosnmp.NewHandler(),
		entries: pdus,
		byOID:   make(map[string]gosnmp.SnmpPDU, len(pdus)),
	}
	for _, pdu := range pdus {
		handler.byOID[pdu.Name] = pdu
	}

	dev := ddsnmp.DeviceConnectionInfo{
		Hostname:       "192.0.2.10",
		Port:           161,
		SNMPVersion:    gosnmp.Version2c.String(),
		SysObjectID:    "1.3.6.1.4.1.9.1.403",
		SysName:        "legacy-arp-fixture",
		MaxOIDs:        60,
		MaxRepetitions: 25,
	}
	profiles := (&Collector{}).findTopologyProfiles(dev)
	require.NotEmpty(t, profiles)

	profileMetrics, err := ddsnmpcollector.New(ddsnmpcollector.Config{
		SnmpClient:  handler,
		Profiles:    profiles,
		Log:         newTestSNMPTopologyCollector().Logger,
		SysObjectID: dev.SysObjectID,
	}).Collect()
	require.NoError(t, err)

	cache := newTestTopologyCache(dev)
	cache.ingestTopologyProfileMetrics(profileMetrics)
	cache.finalize()

	observation := cache.buildEngineObservation(cache.localDevice)
	require.Len(t, observation.ARPNDEntries, 1)
	require.Equal(t, 17, observation.ARPNDEntries[0].IfIndex)
	require.Equal(t, "192.0.2.18", observation.ARPNDEntries[0].IP)
	require.Equal(t, "00:50:56:ab:cd:ef", observation.ARPNDEntries[0].MAC)
	require.Equal(t, "dynamic", observation.ARPNDEntries[0].State)
}

func loadTopologySNMPRecHandler(t *testing.T, path string) *topologySNMPRecHandler {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var entries []gosnmp.SnmpPDU
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		require.Lenf(t, parts, 3, "invalid snmprec line %d", lineNumber)
		pdu, err := topologySNMPRecPDU(parts[0], parts[1], parts[2])
		require.NoErrorf(t, err, "invalid snmprec line %d", lineNumber)
		entries = append(entries, pdu)
	}
	require.NoError(t, scanner.Err())
	return newTopologySNMPHandler(entries)
}

func TestTopologySNMPRecPDURejectsMalformedValues(t *testing.T) {
	tests := map[string]struct {
		typeCode string
		raw      string
	}{
		"integer":    {typeCode: "2", raw: "invalid"},
		"octets":     {typeCode: "4x", raw: "not-hex"},
		"ip-address": {typeCode: "64x", raw: "not-hex"},
		"counter32":  {typeCode: "65", raw: "4294967296"},
		"gauge32":    {typeCode: "66", raw: "4294967296"},
		"time-ticks": {typeCode: "67", raw: "4294967296"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pdu, err := topologySNMPRecPDU("1.2.3", tc.typeCode, tc.raw)
			require.Error(t, err)
			require.Equal(t, gosnmp.SnmpPDU{}, pdu)
		})
	}
}

func topologySNMPRecPDU(name, typeCode, raw string) (gosnmp.SnmpPDU, error) {
	name = strings.TrimPrefix(name, ".")
	switch typeCode {
	case "2":
		value, err := strconv.Atoi(raw)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.Integer,
			Value: value,
		}, nil
	case "4":
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.OctetString,
			Value: []byte(raw),
		}, nil
	case "4x":
		value, err := hex.DecodeString(raw)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.OctetString,
			Value: value,
		}, nil
	case "6":
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.ObjectIdentifier,
			Value: strings.TrimPrefix(raw, "."),
		}, nil
	case "64":
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.IPAddress,
			Value: raw,
		}, nil
	case "64x":
		value, err := hex.DecodeString(raw)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		if len(value) != 4 && len(value) != 16 {
			return gosnmp.SnmpPDU{}, fmt.Errorf("unsupported 64x length %d", len(value))
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.IPAddress,
			Value: value,
		}, nil
	case "65":
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.Counter32,
			Value: uint(value),
		}, nil
	case "66":
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.Gauge32,
			Value: uint(value),
		}, nil
	case "67":
		value, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.TimeTicks,
			Value: uint32(value),
		}, nil
	case "70":
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		return gosnmp.SnmpPDU{
			Name:  name,
			Type:  gosnmp.Counter64,
			Value: value,
		}, nil
	default:
		return gosnmp.SnmpPDU{}, fmt.Errorf("unsupported snmprec type %q", typeCode)
	}
}
