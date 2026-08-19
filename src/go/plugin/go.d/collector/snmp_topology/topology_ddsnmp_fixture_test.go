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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

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
	cache.finalizeTopologyCache()

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
	peer.finalizeTopologyCache()

	registry := newTopologyRegistry()
	registry.register(cache)
	registry.register(peer)
	data, ok := snapshotTopologyRegistryForTest(registry)
	require.True(t, ok, "default managed-fabric inference should render the collected cache")
	require.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "bridge"), 1,
		"a real collected FDB row should attach the fixture device to a broadcast segment")
	require.GreaterOrEqual(t, testCountTopologyLinksByType(data.Links, "fdb"), 1,
		"the broadcast segment should resolve the real learned MAC to the synthetic managed peer")
}

type topologySNMPRecHandler struct {
	gosnmp.Handler
	entries           []gosnmp.SnmpPDU
	byOID             map[string]gosnmp.SnmpPDU
	hiddenOIDPrefixes []string
}

func loadTopologySNMPRecHandler(t *testing.T, path string) *topologySNMPRecHandler {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	handler := &topologySNMPRecHandler{
		Handler: gosnmp.NewHandler(),
		byOID:   make(map[string]gosnmp.SnmpPDU),
	}
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
		handler.entries = append(handler.entries, pdu)
		handler.byOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}
	require.NoError(t, scanner.Err())
	return handler
}

func (h *topologySNMPRecHandler) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	variables := make([]gosnmp.SnmpPDU, 0, len(oids))
	for _, oid := range oids {
		key := strings.TrimPrefix(strings.TrimSpace(oid), ".")
		if h.isHiddenOID(key) {
			variables = append(variables, gosnmp.SnmpPDU{Name: key, Type: gosnmp.NoSuchObject})
			continue
		}
		if pdu, ok := h.byOID[key]; ok {
			variables = append(variables, pdu)
			continue
		}
		variables = append(variables, gosnmp.SnmpPDU{Name: key, Type: gosnmp.NoSuchObject})
	}
	return &gosnmp.SnmpPacket{Variables: variables}, nil
}

func (h *topologySNMPRecHandler) WalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	return h.walkAll(root), nil
}

func (h *topologySNMPRecHandler) BulkWalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	return h.walkAll(root), nil
}

func (h *topologySNMPRecHandler) walkAll(root string) []gosnmp.SnmpPDU {
	root = strings.TrimPrefix(strings.TrimSpace(root), ".")
	prefix := root + "."
	var out []gosnmp.SnmpPDU
	for _, pdu := range h.entries {
		name := strings.TrimPrefix(pdu.Name, ".")
		if h.isHiddenOID(name) {
			continue
		}
		if name == root || strings.HasPrefix(name, prefix) {
			out = append(out, pdu)
		}
	}
	return out
}

func (h *topologySNMPRecHandler) hideOIDPrefix(prefix string) {
	prefix = strings.TrimPrefix(strings.TrimSpace(prefix), ".")
	if prefix != "" {
		h.hiddenOIDPrefixes = append(h.hiddenOIDPrefixes, prefix)
	}
}

func (h *topologySNMPRecHandler) isHiddenOID(oid string) bool {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	for _, prefix := range h.hiddenOIDPrefixes {
		if oid == prefix || strings.HasPrefix(oid, prefix+".") {
			return true
		}
	}
	return false
}

func (h *topologySNMPRecHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (h *topologySNMPRecHandler) MaxOids() int                { return 60 }

func TestTopologySNMPRecPDURejectsMalformedValues(t *testing.T) {
	tests := map[string]struct {
		typeCode string
		raw      string
	}{
		"integer":    {typeCode: "2", raw: "invalid"},
		"octets":     {typeCode: "4x", raw: "not-hex"},
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
