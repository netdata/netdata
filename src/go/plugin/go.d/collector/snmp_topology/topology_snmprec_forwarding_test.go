// SPDX-License-Identifier: GPL-3.0-or-later

//go:build snmp_topology_fixtures

package snmptopology

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

type snmprecForwardingFixture struct {
	bridgeMetadata  map[string]ddsnmp.MetaTag
	ifNameEntries   map[string]map[string]string
	ifStatusEntries map[string]map[string]string
	ipIfEntries     map[string]map[string]string
	bridgePorts     map[string]map[string]string
	fdbEntries      map[string]map[string]string
	qBridgeFdb      map[string]map[string]string
	qBridgeVLANs    map[string]map[string]string
	stpPorts        map[string]map[string]string
	vtpVLANs        map[string]map[string]string
	arpEntries      map[string]map[string]string
	arpIPs          map[string]struct{}
	vtpVLANNames    map[string]struct{}
}

func TestTopologyCache_RealSnmprecForwardingFixtures(t *testing.T) {
	tests := []struct {
		name           string
		fixture        string
		wantARP        bool
		wantSTP        bool
		wantDot1qFDB   bool
		wantVTPVLANMap bool
	}{
		{
			name:    "ArubaCX",
			fixture: "arubaos-cx_10.10.snmprec",
			wantARP: true,
			wantSTP: true,
		},
		{
			name:         "CiscoSmallBusiness",
			fixture:      "ciscosb_sg350x-24p.snmprec",
			wantDot1qFDB: true,
			wantSTP:      true,
		},
		{
			name:           "IOSXE",
			fixture:        "iosxe_ie32008t2s-ios17-12.snmprec",
			wantARP:        true,
			wantSTP:        true,
			wantVTPVLANMap: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			data := parseSnmprecForwardingFixture(t, filepath.Join("../../../../testdata/snmp/snmprec", tt.fixture))

			require.NotEmpty(t, data.ifNameEntries, "fixture %q should expose ifName data", tt.fixture)
			require.NotEmpty(t, data.bridgePorts, "fixture %q should expose bridge port mappings", tt.fixture)
			require.True(t, len(data.fdbEntries) > 0 || len(data.qBridgeFdb) > 0, "fixture %q should expose FDB data", tt.fixture)

			coll := replaySnmprecForwardingFixture(t, tt.fixture, data)
			obs := coll.topologyCache.buildEngineObservation(coll.topologyCache.localDevice)

			require.NotEmpty(t, obs.Interfaces, "expected observed interfaces from fixture %q", tt.fixture)
			require.NotEmpty(t, obs.BridgePorts, "expected observed bridge ports from fixture %q", tt.fixture)
			require.NotEmpty(t, obs.FDBEntries, "expected observed FDB entries from fixture %q", tt.fixture)

			if tt.wantDot1qFDB {
				require.True(t, observedFDBHasVLANID(obs), "expected VLAN-aware FDB entries from fixture %q", tt.fixture)
			}
			if tt.wantARP {
				require.NotEmpty(t, obs.ARPNDEntries, "expected ARP/ND entries from fixture %q", tt.fixture)
				require.True(t, observedARPContainsIP(obs, data.arpIPs), "expected ARP IP from fixture %q", tt.fixture)
			}
			if tt.wantSTP {
				require.NotEmpty(t, obs.STPPorts, "expected STP ports from fixture %q", tt.fixture)
				require.True(t, observedSTPHasInterface(obs), "expected STP port/interface correlation from fixture %q", tt.fixture)
			}
			if tt.wantVTPVLANMap {
				require.NotEmpty(t, data.vtpVLANs, "fixture %q should expose VTP VLAN data", tt.fixture)
				require.True(t, cacheContainsAnyVLANName(coll.topologyCache.vlanIDToName, data.vtpVLANNames), "expected VTP VLAN names from fixture %q", tt.fixture)
			}
		})
	}
}

func replaySnmprecForwardingFixture(t *testing.T, fixture string, data snmprecForwardingFixture) *Collector {
	t.Helper()

	coll := newTestCollector(ddsnmp.DeviceConnectionInfo{
		Hostname:    "192.0.2.10",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
		SysName:     fixture,
	})

	if len(data.bridgeMetadata) > 0 {
		coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{DeviceMetadata: data.bridgeMetadata}})
	}
	for _, tags := range data.ifNameEntries {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricTopologyIfNameEntry, Tags: tags})
	}
	for _, tags := range data.ifStatusEntries {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricTopologyIfStatusEntry, Tags: tags})
	}
	for _, tags := range data.ipIfEntries {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricTopologyIPIfEntry, Tags: tags})
	}
	for _, tags := range data.bridgePorts {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricBridgePortMapEntry, Tags: tags})
	}
	for _, tags := range data.qBridgeVLANs {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricDot1qVlanEntry, Tags: tags})
	}
	for _, tags := range data.vtpVLANs {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricVtpVlanEntry, Tags: tags})
	}
	for _, tags := range data.fdbEntries {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricFdbEntry, Tags: tags})
	}
	for _, tags := range data.qBridgeFdb {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricDot1qFdbEntry, Tags: tags})
	}
	for _, tags := range data.stpPorts {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricStpPortEntry, Tags: tags})
	}
	for _, tags := range data.arpEntries {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricArpEntry, Tags: tags})
	}

	return coll
}

func parseSnmprecForwardingFixture(t *testing.T, path string) snmprecForwardingFixture {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	data := snmprecForwardingFixture{
		bridgeMetadata:  make(map[string]ddsnmp.MetaTag),
		ifNameEntries:   make(map[string]map[string]string),
		ifStatusEntries: make(map[string]map[string]string),
		ipIfEntries:     make(map[string]map[string]string),
		bridgePorts:     make(map[string]map[string]string),
		fdbEntries:      make(map[string]map[string]string),
		qBridgeFdb:      make(map[string]map[string]string),
		qBridgeVLANs:    make(map[string]map[string]string),
		stpPorts:        make(map[string]map[string]string),
		vtpVLANs:        make(map[string]map[string]string),
		arpEntries:      make(map[string]map[string]string),
		arpIPs:          make(map[string]struct{}),
		vtpVLANNames:    make(map[string]struct{}),
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		oid := parts[0]
		typ := parts[1]
		val := parts[2]
		if strings.HasSuffix(typ, "x") {
			val = strings.ToLower(val)
		}

		switch oid {
		case "1.3.6.1.2.1.17.1.1.0":
			data.bridgeMetadata[tagBridgeBaseAddress] = ddsnmp.MetaTag{Value: val}
			continue
		}

		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.31.1.1.1.1"); ok {
			entry := ensureTagMap(data.ifNameEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfName] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.31.1.1.1.18"); ok {
			entry := ensureTagMap(data.ifNameEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfAlias] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.31.1.1.1.15"); ok {
			entry := ensureTagMap(data.ifNameEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfHigh] = val
			continue
		}

		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.2"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfDescr] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.3"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfType] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.5"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfSpeed] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.6"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfPhys] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.7"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfAdmin] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.8"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfOper] = val
			continue
		}
		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.2.2.1.9"); ok {
			entry := ensureTagMap(data.ifStatusEntries, ifIndex)
			entry[tagTopoIfIndex] = ifIndex
			entry[tagTopoIfLast] = val
			continue
		}

		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.20.1.1"); ok {
			entry := ensureTagMap(data.ipIfEntries, suffix)
			entry[tagTopoIPAddr] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.20.1.2"); ok {
			entry := ensureTagMap(data.ipIfEntries, suffix)
			entry[tagTopoIfIndex] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.20.1.3"); ok {
			entry := ensureTagMap(data.ipIfEntries, suffix)
			entry[tagTopoIPMask] = val
			continue
		}

		if basePort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.1.4.1.2"); ok {
			entry := ensureTagMap(data.bridgePorts, basePort)
			entry[tagBridgeBasePort] = basePort
			entry[tagBridgeIfIndex] = val
			continue
		}

		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.4.3.1.1"); ok {
			entry := ensureTagMap(data.fdbEntries, suffix)
			entry[tagFdbMac] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.4.3.1.2"); ok {
			entry := ensureTagMap(data.fdbEntries, suffix)
			if entry[tagFdbMac] == "" {
				entry[tagFdbMac] = macFromOIDIndexSuffix(strings.Split(suffix, "."))
			}
			entry[tagFdbBridgePort] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.4.3.1.3"); ok {
			entry := ensureTagMap(data.fdbEntries, suffix)
			if entry[tagFdbMac] == "" {
				entry[tagFdbMac] = macFromOIDIndexSuffix(strings.Split(suffix, "."))
			}
			entry[tagFdbStatus] = val
			continue
		}

		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.7.1.2.2.1.1"); ok {
			entry := ensureTagMap(data.qBridgeFdb, suffix)
			parts := strings.Split(suffix, ".")
			if len(parts) >= 7 {
				entry[tagDot1qFdbID] = parts[0]
			}
			entry[tagDot1qFdbMac] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.7.1.2.2.1.2"); ok {
			entry := ensureTagMap(data.qBridgeFdb, suffix)
			parts := strings.Split(suffix, ".")
			if len(parts) >= 7 {
				entry[tagDot1qFdbID] = parts[0]
				if entry[tagDot1qFdbMac] == "" {
					entry[tagDot1qFdbMac] = macFromOIDIndexSuffix(parts[1:])
				}
			}
			entry[tagDot1qFdbPort] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.7.1.2.2.1.3"); ok {
			entry := ensureTagMap(data.qBridgeFdb, suffix)
			parts := strings.Split(suffix, ".")
			if len(parts) >= 7 {
				entry[tagDot1qFdbID] = parts[0]
				if entry[tagDot1qFdbMac] == "" {
					entry[tagDot1qFdbMac] = macFromOIDIndexSuffix(parts[1:])
				}
			}
			entry[tagDot1qFdbStatus] = val
			continue
		}

		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.17.7.1.4.2.1.3"); ok {
			entry := ensureTagMap(data.qBridgeVLANs, suffix)
			parts := strings.Split(suffix, ".")
			switch len(parts) {
			case 1:
				entry[tagDot1qVlanID] = parts[0]
				entry[tagDot1qVlanID1] = parts[0]
			default:
				entry[tagDot1qVlanID1] = parts[0]
				entry[tagDot1qVlanID] = parts[1]
			}
			entry[tagDot1qVlanFdbID] = val
			continue
		}

		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.2"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortPriority] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.3"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortState] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.4"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortEnable] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.5"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortPathCost] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.6"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortDesignatedRoot] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.7"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortDesignatedCost] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.8"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortDesignatedBridge] = val
			continue
		}
		if stpPort, ok := parseOIDIndex(oid, "1.3.6.1.2.1.17.2.15.1.9"); ok {
			entry := ensureTagMap(data.stpPorts, stpPort)
			entry[tagStpPort] = stpPort
			entry[tagStpPortDesignatedPort] = val
			continue
		}

		if vlanID, ok := parseOIDIndex(oid, "1.3.6.1.4.1.9.9.46.1.3.1.1.2"); ok {
			entry := ensureTagMap(data.vtpVLANs, vlanID)
			entry[tagVtpVlanIndex] = vlanID
			entry[tagVtpVlanState] = val
			continue
		}
		if vlanID, ok := parseOIDIndex(oid, "1.3.6.1.4.1.9.9.46.1.3.1.1.3"); ok {
			entry := ensureTagMap(data.vtpVLANs, vlanID)
			entry[tagVtpVlanIndex] = vlanID
			entry[tagVtpVlanType] = val
			continue
		}
		if vlanID, ok := parseOIDIndex(oid, "1.3.6.1.4.1.9.9.46.1.3.1.1.4"); ok {
			entry := ensureTagMap(data.vtpVLANs, vlanID)
			entry[tagVtpVlanIndex] = vlanID
			entry[tagVtpVlanName] = val
			if val != "" {
				data.vtpVLANNames[val] = struct{}{}
			}
			continue
		}

		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.35.1.1"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			entry[tagArpIfIndex] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.35.1.2"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			entry[tagArpAddrType] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.35.1.3"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			entry[tagArpIP] = val
			if val != "" {
				data.arpIPs[val] = struct{}{}
			}
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.35.1.4"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			if entry[tagArpIfIndex] == "" || entry[tagArpAddrType] == "" || entry[tagArpIP] == "" {
				ifIndex, addrType, ip := arpModernIndexFields(strings.Split(suffix, "."))
				if entry[tagArpIfIndex] == "" {
					entry[tagArpIfIndex] = ifIndex
				}
				if entry[tagArpAddrType] == "" {
					entry[tagArpAddrType] = addrType
				}
				if entry[tagArpIP] == "" {
					entry[tagArpIP] = ip
				}
				if ip != "" {
					data.arpIPs[ip] = struct{}{}
				}
			}
			entry[tagArpMac] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.35.1.6"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			entry[tagArpState] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.22.1.2"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			if entry[tagArpIfIndex] == "" || entry[tagArpIP] == "" {
				ifIndex, ip := arpLegacyIndexFields(strings.Split(suffix, "."))
				if entry[tagArpIfIndex] == "" {
					entry[tagArpIfIndex] = ifIndex
				}
				if entry[tagArpIP] == "" {
					entry[tagArpIP] = ip
				}
				if ip != "" {
					data.arpIPs[ip] = struct{}{}
				}
			}
			entry[tagArpMac] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.3.6.1.2.1.4.22.1.4"); ok {
			entry := ensureTagMap(data.arpEntries, suffix)
			entry[tagArpType] = val
			continue
		}
	}

	require.NoError(t, scanner.Err())
	return data
}

func observedFDBHasVLANID(obs topologyengine.L2Observation) bool {
	for _, entry := range obs.FDBEntries {
		if strings.TrimSpace(entry.VLANID) != "" {
			return true
		}
	}
	return false
}

func observedARPContainsIP(obs topologyengine.L2Observation, ips map[string]struct{}) bool {
	for _, entry := range obs.ARPNDEntries {
		if _, ok := ips[strings.TrimSpace(entry.IP)]; ok {
			return true
		}
	}
	return false
}

func observedSTPHasInterface(obs topologyengine.L2Observation) bool {
	for _, entry := range obs.STPPorts {
		if entry.IfIndex > 0 || strings.TrimSpace(entry.IfName) != "" {
			return true
		}
	}
	return false
}

func cacheContainsAnyVLANName(names map[string]string, expected map[string]struct{}) bool {
	for _, name := range names {
		if _, ok := expected[strings.TrimSpace(name)]; ok {
			return true
		}
	}
	return false
}

func arpModernIndexFields(parts []string) (ifIndex, addrType, ip string) {
	if len(parts) < 4 {
		return "", "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.Join(parts[3:], ".")
}

func arpLegacyIndexFields(parts []string) (ifIndex, ip string) {
	if len(parts) < 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.Join(parts[1:], ".")
}

func macFromOIDIndexSuffix(parts []string) string {
	if len(parts) < 6 {
		return ""
	}

	parts = parts[len(parts)-6:]
	octets := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 255 {
			return ""
		}
		octets = append(octets, fmt.Sprintf("%02x", value))
	}
	return strings.Join(octets, ":")
}
