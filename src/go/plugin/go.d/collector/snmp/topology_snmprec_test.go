// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

type snmprecTopology struct {
	lldpLocalTags map[string]string
	lldpLocPorts  map[string]map[string]string
	lldpRemotes   map[string]map[string]string
	cdpRemotes    map[string]map[string]string
	ifNames       map[string]string
	lldpSysNames  map[string]struct{}
	cdpDeviceIDs  map[string]struct{}
}

func TestTopologyCache_LLDP_RealSnmprec(t *testing.T) {
	data := parseSnmprecTopology(t, "testdata/snmprec/arista_eos.snmprec")
	require.NotEmpty(t, data.lldpRemotes)

	coll := &Collector{
		Config:        Config{Hostname: "192.0.2.10"},
		topologyCache: newTopologyCache(),
		sysInfo:       &snmputils.SysInfo{SysObjectID: "1.3.6.1.4.1.9.1.1", Name: "arista"},
	}
	coll.resetTopologyCache()

	if len(data.lldpLocalTags) > 0 {
		coll.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{{Tags: data.lldpLocalTags}})
	}
	for _, tags := range data.lldpLocPorts {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpLocPortEntry, Tags: tags})
	}
	for _, tags := range data.lldpRemotes {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricLldpRemEntry, Tags: tags})
	}
	coll.finalizeTopologyCache()

	coll.topologyCache.mu.RLock()
	snapshot, ok := coll.topologyCache.snapshot()
	coll.topologyCache.mu.RUnlock()

	require.True(t, ok)
	require.Greater(t, len(snapshot.Devices), 1)
	require.Greater(t, len(snapshot.Links), 0)
	require.NotEmpty(t, snapshot.Devices[0].ChassisID)

	found := false
	for _, link := range snapshot.Links {
		if link.Protocol != "lldp" {
			continue
		}
		if _, ok := data.lldpSysNames[link.Dst.SysName]; ok {
			found = true
			break
		}
	}
	require.True(t, found, "expected LLDP link with sysName from snmprec data")
}

func TestTopologyCache_CDP_RealSnmprec(t *testing.T) {
	data := parseSnmprecTopology(t, "testdata/snmprec/ios_2960x.snmprec")
	require.NotEmpty(t, data.cdpRemotes)
	require.NotEmpty(t, data.cdpDeviceIDs)

	coll := &Collector{
		Config:        Config{Hostname: "192.0.2.20"},
		topologyCache: newTopologyCache(),
		sysInfo:       &snmputils.SysInfo{SysObjectID: "1.3.6.1.4.1.9.1.1208", Name: "ios"},
	}
	coll.resetTopologyCache()

	for _, tags := range data.cdpRemotes {
		coll.updateTopologyCacheEntry(ddsnmp.Metric{Name: metricCdpCacheEntry, Tags: tags})
	}
	coll.finalizeTopologyCache()

	coll.topologyCache.mu.RLock()
	snapshot, ok := coll.topologyCache.snapshot()
	coll.topologyCache.mu.RUnlock()

	require.True(t, ok)
	require.Greater(t, len(snapshot.Devices), 1)
	require.Greater(t, len(snapshot.Links), 0)

	found := false
	for _, link := range snapshot.Links {
		if link.Protocol != "cdp" {
			continue
		}
		if _, ok := data.cdpDeviceIDs[link.Dst.SysName]; ok {
			found = true
			break
		}
	}
	require.True(t, found, "expected CDP link with device ID from snmprec data")
}

func parseSnmprecTopology(t *testing.T, path string) snmprecTopology {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	data := snmprecTopology{
		lldpLocalTags: make(map[string]string),
		lldpLocPorts:  make(map[string]map[string]string),
		lldpRemotes:   make(map[string]map[string]string),
		cdpRemotes:    make(map[string]map[string]string),
		ifNames:       make(map[string]string),
		lldpSysNames:  make(map[string]struct{}),
		cdpDeviceIDs:  make(map[string]struct{}),
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
