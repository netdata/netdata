// SPDX-License-Identifier: GPL-3.0-or-later

//go:build topology_fixtures

package snmptopology

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestTopologyIntegrationWithSnmpsim(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_ENDPOINT"))
	communitiesRaw := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_COMMUNITIES"))
	if endpoint == "" || communitiesRaw == "" {
		t.Skip("missing NETDATA_SNMPSIM_ENDPOINT or NETDATA_SNMPSIM_COMMUNITIES")
	}

	host, port := parseSnmpEndpoint(t, endpoint)
	communities := splitCSV(communitiesRaw)

	for _, community := range communities {
		expectation := integrationExpectationForCommunity(t, community)
		snapshot := collectTopologySnapshotFromDevice(t, integrationV2DeviceInfo(host, port, community))
		assertExpectedTopologySnapshot(t, community, expectation, snapshot)
	}
}

type topologyIntegrationExpectation struct {
	protocol    string
	fixture     string
	fixtureData snmprecTopology
}

func integrationExpectationForCommunity(t *testing.T, community string) topologyIntegrationExpectation {
	t.Helper()

	var expectation topologyIntegrationExpectation
	switch strings.ToLower(strings.TrimSpace(community)) {
	case "lldp1":
		expectation.protocol = "lldp"
		expectation.fixture = "arubaos-cx_10.10.snmprec"
	case "lldp2":
		expectation.protocol = "lldp"
		expectation.fixture = "aos6.snmprec"
	case "cdp1":
		expectation.protocol = "cdp"
		expectation.fixture = "ciscosb_sg350x-24p.snmprec"
	default:
		t.Fatalf("unexpected integration community %q", community)
	}

	expectation.fixtureData = parseSnmprecTopology(t, filepath.Join("../../../../testdata/snmp/snmprec", expectation.fixture))
	return expectation
}

func TestTopologyIntegrationWithSnmpsimV3(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_ENDPOINT"))
	contextsRaw := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_CONTEXTS"))
	user := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_USER"))
	level := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_SECURITY_LEVEL"))
	authProto := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_AUTH_PROTO"))
	authKey := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_AUTH_KEY"))
	privProto := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_PRIV_PROTO"))
	privKey := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_V3_PRIV_KEY"))

	if endpoint == "" || contextsRaw == "" || user == "" || level == "" || authProto == "" || authKey == "" || privProto == "" || privKey == "" {
		t.Skip("missing NETDATA_SNMPSIM_V3_* variables")
	}

	host, port := parseSnmpEndpoint(t, endpoint)
	contexts := splitCSV(contextsRaw)

	for _, contextName := range contexts {
		expectation := integrationExpectationForCommunity(t, contextName)
		snapshot := collectTopologySnapshotFromDevice(t, integrationV3DeviceInfo(host, port, contextName, user, level, authProto, authKey, privProto, privKey))
		assertExpectedTopologySnapshot(t, contextName, expectation, snapshot)
	}
}

func collectTopologySnapshotFromDevice(t *testing.T, dev ddsnmp.DeviceConnectionInfo) topologyData {
	t.Helper()

	deviceKey := "integration:" + dev.SNMPVersion + ":" + dev.SysName
	ddsnmp.DeviceRegistry.Register(deviceKey, dev)
	defer ddsnmp.DeviceRegistry.Unregister(deviceKey)

	coll := New()
	coll.Config = Config{UpdateEvery: 1}
	require.NoError(t, coll.Init(context.Background()))
	defer coll.Cleanup(context.Background())

	require.NoError(t, coll.Check(context.Background()))
	_ = coll.Collect(context.Background())

	var snapshot topologyData
	cacheKey := dev.Hostname + ":" + strconv.Itoa(dev.Port)
	require.Eventuallyf(t, func() bool {
		cache := coll.deviceCaches[cacheKey]
		if cache == nil {
			return false
		}
		cache.mu.RLock()
		defer cache.mu.RUnlock()

		var ok bool
		snapshot, ok = cache.snapshot()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "topology snapshot did not become available for %q", dev.SysName)

	return snapshot
}

func integrationV2DeviceInfo(host string, port int, community string) ddsnmp.DeviceConnectionInfo {
	return ddsnmp.DeviceConnectionInfo{
		Hostname:       host,
		Port:           port,
		SNMPVersion:    gosnmp.Version2c.String(),
		Community:      community,
		MaxRepetitions: 25,
		MaxOIDs:        60,
		Timeout:        5,
		Retries:        1,
		SysName:        community,
		SysObjectID:    integrationSysObjectID(community),
	}
}

func integrationV3DeviceInfo(host string, port int, contextName, user, level, authProto, authKey, privProto, privKey string) ddsnmp.DeviceConnectionInfo {
	return ddsnmp.DeviceConnectionInfo{
		Hostname:        host,
		Port:            port,
		SNMPVersion:     gosnmp.Version3.String(),
		V3User:          user,
		V3SecurityLevel: level,
		V3AuthProto:     authProto,
		V3AuthKey:       authKey,
		V3PrivProto:     privProto,
		V3PrivKey:       privKey,
		V3ContextName:   contextName,
		MaxRepetitions:  25,
		MaxOIDs:         60,
		Timeout:         5,
		Retries:         1,
		SysName:         contextName,
		SysObjectID:     integrationSysObjectID(contextName),
	}
}

func assertExpectedTopologySnapshot(t *testing.T, subject string, expectation topologyIntegrationExpectation, snapshot topologyData) {
	t.Helper()

	require.Greaterf(t, len(snapshot.Links), 0, "expected topology links for %q", subject)
	require.Truef(t, hasProtocolLink(snapshot, expectation.protocol), "expected %s links for %q", expectation.protocol, subject)

	switch expectation.protocol {
	case "lldp":
		require.Truef(t, hasLinkableLLDP(expectation.fixtureData), "fixture %q does not contain linkable LLDP data", expectation.fixture)
		if len(expectation.fixtureData.lldpSysNames) > 0 {
			require.Truef(t, containsSysName(snapshot, expectation.fixtureData.lldpSysNames), "expected LLDP sysName from fixture %q", expectation.fixture)
		}
		if len(expectation.fixtureData.lldpMgmtAddrs) > 0 {
			require.Truef(t, containsMgmtAddr(snapshot, expectation.fixtureData.lldpMgmtAddrs), "expected LLDP management address from fixture %q", expectation.fixture)
		}
	case "cdp":
		require.Truef(t, hasLinkableCDP(expectation.fixtureData), "fixture %q does not contain linkable CDP data", expectation.fixture)
		if len(expectation.fixtureData.cdpDeviceIDs) > 0 {
			require.Truef(t, containsIdentifier(snapshot, expectation.fixtureData.cdpDeviceIDs), "expected CDP device identifier from fixture %q", expectation.fixture)
		}
		if len(expectation.fixtureData.cdpSysNames) > 0 {
			require.Truef(t, containsSysName(snapshot, expectation.fixtureData.cdpSysNames), "expected CDP sysName from fixture %q", expectation.fixture)
		}
		if len(expectation.fixtureData.cdpMgmtAddrs) > 0 {
			require.Truef(t, containsMgmtAddr(snapshot, expectation.fixtureData.cdpMgmtAddrs), "expected CDP management address from fixture %q", expectation.fixture)
		}
	}
}

func integrationSysObjectID(community string) string {
	switch strings.ToLower(community) {
	case "lldp1":
		return "1.3.6.1.4.1.47196.4.1.1.1.254" // arubaos-cx_10.10.snmprec
	case "lldp2":
		return "1.3.6.1.4.1.6486.800.1.1.2.1.13.1.2" // aos6.snmprec
	case "cdp1":
		return "1.3.6.1.4.1.9.6.1.94.24.5" // ciscosb_sg350x-24p.snmprec
	default:
		return ""
	}
}

func parseSnmpEndpoint(t *testing.T, endpoint string) (string, int) {
	if host, portStr, err := net.SplitHostPort(endpoint); err == nil {
		port, err := parsePort(portStr)
		require.NoError(t, err)
		return host, port
	}
	return endpoint, 161
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
