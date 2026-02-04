// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

type snmprecTopology struct {
	lldpLocalTags   map[string]string
	lldpLocPorts    map[string]map[string]string
	lldpLocManAddrs map[string]map[string]string
	lldpRemotes     map[string]map[string]string
	lldpRemManAddrs map[string]map[string]string
	cdpRemotes      map[string]map[string]string
	ifNames         map[string]string
	lldpSysNames    map[string]struct{}
	lldpMgmtAddrs   map[string]struct{}
	cdpDeviceIDs    map[string]struct{}
	cdpSysNames     map[string]struct{}
	cdpMgmtAddrs    map[string]struct{}
}

func TestTopologyCache_RealSnmprecFixtures(t *testing.T) {
	files, err := filepath.Glob("testdata/snmprec/*.snmprec")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data := parseSnmprecTopology(t, path)
			if len(data.lldpRemotes) == 0 && len(data.cdpRemotes) == 0 {
				t.Skip("no LLDP/CDP data detected")
			}

			coll := &Collector{
				Config:        Config{Hostname: "192.0.2.10"},
				topologyCache: newTopologyCache(),
				sysInfo:       &snmputils.SysInfo{SysObjectID: "1.3.6.1.4.1.9.1.1", Name: filepath.Base(path)},
			}
			coll.resetTopologyCache()

			if len(data.lldpLocalTags) > 0 {
				coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{Tags: data.lldpLocalTags}})
			}
			for _, tags := range data.lldpLocPorts {
				coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpLocPortEntry, Tags: tags})
			}
			for _, tags := range data.lldpLocManAddrs {
				coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpLocManAddrEntry, Tags: tags})
			}
			for _, tags := range data.lldpRemotes {
				coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpRemEntry, Tags: tags})
			}
			for _, tags := range data.lldpRemManAddrs {
				coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpRemManAddrEntry, Tags: tags})
			}
			for _, tags := range data.cdpRemotes {
				coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricCdpCacheEntry, Tags: tags})
			}
			coll.finalizeTopologyCache()

			coll.topologyCache.mu.RLock()
			snapshot, ok := coll.topologyCache.snapshot()
			coll.topologyCache.mu.RUnlock()

			require.True(t, ok)
			require.GreaterOrEqual(t, len(snapshot.Devices), 1)
			if hasLinkableLLDP(data) || hasLinkableCDP(data) {
				require.Greater(t, len(snapshot.Links), 0)
			}
			require.NotEmpty(t, snapshot.Devices[0].ChassisID)

			if len(data.lldpRemotes) > 0 || len(data.lldpRemManAddrs) > 0 {
				if hasLinkableLLDP(data) {
					require.True(t, hasProtocolLink(snapshot, "lldp"), "expected LLDP links")
				}
				if len(data.lldpSysNames) > 0 && hasLinkableLLDP(data) {
					require.True(t, containsSysName(snapshot, data.lldpSysNames), "expected LLDP sysName from snmprec data")
				}
			}
			if len(data.cdpRemotes) > 0 {
				if hasLinkableCDP(data) {
					require.True(t, hasProtocolLink(snapshot, "cdp"), "expected CDP links")
				}
				if len(data.cdpDeviceIDs) > 0 {
					require.True(t, containsIdentifier(snapshot, data.cdpDeviceIDs), "expected CDP device ID from snmprec data")
				}
				if len(data.cdpSysNames) > 0 && hasLinkableCDP(data) {
					require.True(t, containsSysName(snapshot, data.cdpSysNames), "expected CDP sysName from snmprec data")
				}
			}
			if len(data.lldpMgmtAddrs) > 0 {
				require.True(t, containsMgmtAddr(snapshot, data.lldpMgmtAddrs), "expected LLDP management address from snmprec data")
			}
			if len(data.cdpMgmtAddrs) > 0 {
				require.True(t, containsMgmtAddr(snapshot, data.cdpMgmtAddrs), "expected CDP management address from snmprec data")
			}
		})
	}
}

func parseSnmprecTopology(t *testing.T, path string) snmprecTopology {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	data := snmprecTopology{
		lldpLocalTags:   make(map[string]string),
		lldpLocPorts:    make(map[string]map[string]string),
		lldpLocManAddrs: make(map[string]map[string]string),
		lldpRemotes:     make(map[string]map[string]string),
		lldpRemManAddrs: make(map[string]map[string]string),
		cdpRemotes:      make(map[string]map[string]string),
		ifNames:         make(map[string]string),
		lldpSysNames:    make(map[string]struct{}),
		lldpMgmtAddrs:   make(map[string]struct{}),
		cdpDeviceIDs:    make(map[string]struct{}),
		cdpSysNames:     make(map[string]struct{}),
		cdpMgmtAddrs:    make(map[string]struct{}),
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
		case "1.0.8802.1.1.2.1.3.1.0":
			data.lldpLocalTags[tagLldpLocChassisIDSubtype] = val
			continue
		case "1.0.8802.1.1.2.1.3.2.0":
			data.lldpLocalTags[tagLldpLocChassisID] = val
			continue
		case "1.0.8802.1.1.2.1.3.3.0":
			data.lldpLocalTags[tagLldpLocSysName] = val
			continue
		case "1.0.8802.1.1.2.1.3.4.0":
			data.lldpLocalTags[tagLldpLocSysDesc] = val
			continue
		case "1.0.8802.1.1.2.1.3.5.0":
			data.lldpLocalTags[tagLldpLocSysCapSupported] = val
			continue
		case "1.0.8802.1.1.2.1.3.6.0":
			data.lldpLocalTags[tagLldpLocSysCapEnabled] = val
			continue
		}

		if portNum, ok := parseOIDIndex(oid, "1.0.8802.1.1.2.1.3.7.1.2"); ok {
			entry := ensureTagMap(data.lldpLocPorts, portNum)
			entry[tagLldpLocPortNum] = portNum
			entry[tagLldpLocPortIDSubtype] = val
			continue
		}
		if portNum, ok := parseOIDIndex(oid, "1.0.8802.1.1.2.1.3.7.1.3"); ok {
			entry := ensureTagMap(data.lldpLocPorts, portNum)
			entry[tagLldpLocPortNum] = portNum
			entry[tagLldpLocPortID] = val
			continue
		}
		if portNum, ok := parseOIDIndex(oid, "1.0.8802.1.1.2.1.3.7.1.4"); ok {
			entry := ensureTagMap(data.lldpLocPorts, portNum)
			entry[tagLldpLocPortNum] = portNum
			entry[tagLldpLocPortDesc] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.1"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddrSubtype] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.2"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddr] = val
			if val != "" {
				data.lldpMgmtAddrs[val] = struct{}{}
			}
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.3"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddrLen] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.4"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddrIfSubtype] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.5"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddrIfID] = val
			continue
		}
		if suffix, ok := parseOIDSuffix(oid, "1.0.8802.1.1.2.1.3.8.1.6"); ok {
			entry := ensureTagMap(data.lldpLocManAddrs, suffix)
			entry[tagLldpLocMgmtAddrOID] = val
			continue
		}

		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.4", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemChassisIDSubtype] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.5", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemChassisID] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.6", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemPortIDSubtype] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.7", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemPortID] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.8", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemPortDesc] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.9", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemSysName] = val
			if val != "" {
				data.lldpSysNames[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.10", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemSysDesc] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.11", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemSysCapSupported] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.0.8802.1.1.2.1.4.1.1.12", 3); ok {
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemotes, localPort+":"+remIndex)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemSysCapEnabled] = val
			continue
		}

		if indexes, ok := parseOIDLeadingIndexes(oid, "1.0.8802.1.1.2.1.4.2.1.1", 3); ok {
			suffix := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.4.2.1.1.")
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemManAddrs, localPort+":"+remIndex+":"+suffix)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemMgmtAddrSubtype] = val
			continue
		}
		if indexes, ok := parseOIDLeadingIndexes(oid, "1.0.8802.1.1.2.1.4.2.1.2", 3); ok {
			suffix := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.4.2.1.2.")
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemManAddrs, localPort+":"+remIndex+":"+suffix)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemMgmtAddr] = val
			if val != "" {
				data.lldpMgmtAddrs[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDLeadingIndexes(oid, "1.0.8802.1.1.2.1.4.2.1.3", 3); ok {
			suffix := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.4.2.1.3.")
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemManAddrs, localPort+":"+remIndex+":"+suffix)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemMgmtAddrIfSubtype] = val
			continue
		}
		if indexes, ok := parseOIDLeadingIndexes(oid, "1.0.8802.1.1.2.1.4.2.1.4", 3); ok {
			suffix := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.4.2.1.4.")
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemManAddrs, localPort+":"+remIndex+":"+suffix)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemMgmtAddrIfID] = val
			continue
		}
		if indexes, ok := parseOIDLeadingIndexes(oid, "1.0.8802.1.1.2.1.4.2.1.5", 3); ok {
			suffix := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.4.2.1.5.")
			localPort := indexes[1]
			remIndex := indexes[2]
			entry := ensureTagMap(data.lldpRemManAddrs, localPort+":"+remIndex+":"+suffix)
			entry[tagLldpLocPortNum] = localPort
			entry[tagLldpRemIndex] = remIndex
			entry[tagLldpRemMgmtAddrOID] = val
			continue
		}

		if ifIndex, ok := parseOIDIndex(oid, "1.3.6.1.2.1.31.1.1.1.1"); ok {
			data.ifNames[ifIndex] = val
			continue
		}

		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.6", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpDeviceID] = val
			if val != "" {
				data.cdpDeviceIDs[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.3", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpAddressType] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.7", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpDevicePort] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.8", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpPlatform] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.5", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpVersion] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.9", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpCaps] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.4", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpAddress] = val
			if val != "" {
				data.cdpMgmtAddrs[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.10", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpVTPDomain] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.11", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpNativeVLAN] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.12", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpDuplex] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.15", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpPower] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.16", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpMTU] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.17", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpSysName] = val
			if val != "" {
				data.cdpSysNames[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.18", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpSysObjectID] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.19", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpPrimaryMgmtAddrType] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.20", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpPrimaryMgmtAddr] = val
			if val != "" {
				data.cdpMgmtAddrs[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.21", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpSecondaryMgmtAddrType] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.22", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpSecondaryMgmtAddr] = val
			if val != "" {
				data.cdpMgmtAddrs[val] = struct{}{}
			}
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.23", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpPhysicalLocation] = val
			continue
		}
		if indexes, ok := parseOIDIndexes(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.24", 2); ok {
			ifIndex := indexes[0]
			deviceIndex := indexes[1]
			entry := ensureTagMap(data.cdpRemotes, ifIndex+":"+deviceIndex)
			entry[tagCdpIfIndex] = ifIndex
			entry[tagCdpDeviceIndex] = deviceIndex
			entry[tagCdpLastChange] = val
			continue
		}
	}

	require.NoError(t, scanner.Err())

	for key, entry := range data.cdpRemotes {
		parts := strings.Split(key, ":")
		if len(parts) == 2 {
			if name := data.ifNames[parts[0]]; name != "" {
				entry[tagCdpIfName] = name
			}
		}
	}

	return data
}

func ensureTagMap(target map[string]map[string]string, key string) map[string]string {
	entry := target[key]
	if entry == nil {
		entry = make(map[string]string)
		target[key] = entry
	}
	return entry
}

func parseOIDIndex(oid, prefix string) (string, bool) {
	if !strings.HasPrefix(oid, prefix+".") {
		return "", false
	}
	suffix := strings.TrimPrefix(oid, prefix+".")
	if suffix == "" {
		return "", false
	}
	parts := strings.Split(suffix, ".")
	return parts[len(parts)-1], true
}

func parseOIDIndexes(oid, prefix string, count int) ([]string, bool) {
	if !strings.HasPrefix(oid, prefix+".") {
		return nil, false
	}
	suffix := strings.TrimPrefix(oid, prefix+".")
	parts := strings.Split(suffix, ".")
	if len(parts) < count {
		return nil, false
	}
	return parts[len(parts)-count:], true
}

func parseOIDLeadingIndexes(oid, prefix string, count int) ([]string, bool) {
	if !strings.HasPrefix(oid, prefix+".") {
		return nil, false
	}
	suffix := strings.TrimPrefix(oid, prefix+".")
	parts := strings.Split(suffix, ".")
	if len(parts) < count {
		return nil, false
	}
	return parts[:count], true
}

func parseOIDSuffix(oid, prefix string) (string, bool) {
	if !strings.HasPrefix(oid, prefix+".") {
		return "", false
	}
	return strings.TrimPrefix(oid, prefix+"."), true
}

func hasProtocolLink(snapshot topologyData, protocol string) bool {
	for _, link := range snapshot.Links {
		if link.Protocol == protocol {
			return true
		}
	}
	return false
}

func containsSysName(snapshot topologyData, names map[string]struct{}) bool {
	for _, link := range snapshot.Links {
		if link.Dst.SysName == "" {
			continue
		}
		if _, ok := names[link.Dst.SysName]; ok {
			return true
		}
	}
	for _, dev := range snapshot.Devices {
		if dev.SysName == "" {
			continue
		}
		if _, ok := names[dev.SysName]; ok {
			return true
		}
	}
	return false
}

func containsMgmtAddr(snapshot topologyData, addrs map[string]struct{}) bool {
	for _, dev := range snapshot.Devices {
		if dev.ManagementIP != "" {
			if _, ok := addrs[dev.ManagementIP]; ok {
				return true
			}
		}
		for _, addr := range dev.ManagementAddresses {
			if addr.Address == "" {
				continue
			}
			if _, ok := addrs[addr.Address]; ok {
				return true
			}
		}
	}
	for _, link := range snapshot.Links {
		if link.Dst.ManagementIP != "" {
			if _, ok := addrs[link.Dst.ManagementIP]; ok {
				return true
			}
		}
		for _, addr := range link.Dst.ManagementAddresses {
			if addr.Address == "" {
				continue
			}
			if _, ok := addrs[addr.Address]; ok {
				return true
			}
		}
	}
	return false
}

func hasLinkableLLDP(data snmprecTopology) bool {
	for _, tags := range data.lldpRemotes {
		if tags[tagLldpRemChassisID] != "" || tags[tagLldpRemMgmtAddr] != "" {
			return true
		}
	}
	for _, tags := range data.lldpRemManAddrs {
		if tags[tagLldpRemMgmtAddr] != "" {
			return true
		}
	}
	return false
}

func hasLinkableCDP(data snmprecTopology) bool {
	for _, tags := range data.cdpRemotes {
		if tags[tagCdpDeviceID] != "" || tags[tagCdpAddress] != "" || tags[tagCdpPrimaryMgmtAddr] != "" || tags[tagCdpSecondaryMgmtAddr] != "" {
			return true
		}
	}
	return false
}

func containsIdentifier(snapshot topologyData, ids map[string]struct{}) bool {
	for _, link := range snapshot.Links {
		if link.Dst.SysName != "" {
			if _, ok := ids[link.Dst.SysName]; ok {
				return true
			}
		}
		if link.Dst.ChassisID != "" {
			if _, ok := ids[link.Dst.ChassisID]; ok {
				return true
			}
		}
		if link.Dst.ManagementIP != "" {
			if _, ok := ids[link.Dst.ManagementIP]; ok {
				return true
			}
		}
	}
	for _, dev := range snapshot.Devices {
		if dev.SysName != "" {
			if _, ok := ids[dev.SysName]; ok {
				return true
			}
		}
		if dev.ChassisID != "" {
			if _, ok := ids[dev.ChassisID]; ok {
				return true
			}
		}
		if dev.ManagementIP != "" {
			if _, ok := ids[dev.ManagementIP]; ok {
				return true
			}
		}
	}
	return false
}
