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

	data, ok := snapshotTopologyCacheForTest(cache)
	require.True(t, ok, "default managed-fabric inference should render the collected cache")
	require.NotEmpty(t, data.Actors)
}

type topologySNMPRecHandler struct {
	gosnmp.Handler
	entries []gosnmp.SnmpPDU
	byOID   map[string]gosnmp.SnmpPDU
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
		if name == root || strings.HasPrefix(name, prefix) {
			out = append(out, pdu)
		}
	}
	return out
}

func (h *topologySNMPRecHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (h *topologySNMPRecHandler) MaxOids() int                { return 60 }

func topologySNMPRecPDU(name, typeCode, raw string) (gosnmp.SnmpPDU, error) {
	pdu := gosnmp.SnmpPDU{Name: strings.TrimPrefix(name, ".")}
	switch typeCode {
	case "2":
		value, err := strconv.Atoi(raw)
		pdu.Type, pdu.Value = gosnmp.Integer, value
		return pdu, err
	case "4":
		pdu.Type, pdu.Value = gosnmp.OctetString, []byte(raw)
		return pdu, nil
	case "4x":
		value, err := hex.DecodeString(raw)
		pdu.Type, pdu.Value = gosnmp.OctetString, value
		return pdu, err
	case "6":
		pdu.Type, pdu.Value = gosnmp.ObjectIdentifier, strings.TrimPrefix(raw, ".")
		return pdu, nil
	case "64":
		pdu.Type, pdu.Value = gosnmp.IPAddress, raw
		return pdu, nil
	case "65", "66", "67", "70":
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, err
		}
		switch typeCode {
		case "65":
			pdu.Type, pdu.Value = gosnmp.Counter32, uint(value)
		case "66":
			pdu.Type, pdu.Value = gosnmp.Gauge32, uint(value)
		case "67":
			pdu.Type, pdu.Value = gosnmp.TimeTicks, uint32(value)
		case "70":
			pdu.Type, pdu.Value = gosnmp.Counter64, value
		}
		return pdu, nil
	default:
		return gosnmp.SnmpPDU{}, fmt.Errorf("unsupported snmprec type %q", typeCode)
	}
}
