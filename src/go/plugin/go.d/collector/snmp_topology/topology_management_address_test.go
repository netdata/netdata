// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestNormalizeManagementAddressHonorsProtocolFamilies(t *testing.T) {
	tests := map[string]struct {
		normalize func(string, string) (string, string)
		addr      string
		addrType  string
		wantAddr  string
		wantType  string
	}{
		"LLDP IPv4":                   {normalize: normalizeLLDPManagementAddress, addr: "0A14043C", addrType: "1", wantAddr: "10.20.4.60", wantType: "ipv4"},
		"LLDP IPv6":                   {normalize: normalizeLLDPManagementAddress, addr: "20010db8000000000000000000000001", addrType: "2", wantAddr: "2001:db8::1", wantType: "ipv6"},
		"LLDP IPv4 family mismatch":   {normalize: normalizeLLDPManagementAddress, addr: "20010db8000000000000000000000001", addrType: "1", wantAddr: "20010db8000000000000000000000001", wantType: "1"},
		"LLDP IPv6 family mismatch":   {normalize: normalizeLLDPManagementAddress, addr: "0A14043C", addrType: "2", wantAddr: "0A14043C", wantType: "2"},
		"LLDP explicit DNS family":    {normalize: normalizeLLDPManagementAddress, addr: "636f7265", addrType: "16", wantAddr: "636f7265", wantType: "16"},
		"CDP IPv4":                    {normalize: normalizeCDPManagementAddress, addr: "0A14043C", addrType: "1", wantAddr: "10.20.4.60", wantType: "ipv4"},
		"CDP IPv6":                    {normalize: normalizeCDPManagementAddress, addr: "20010db8000000000000000000000001", addrType: "20", wantAddr: "2001:db8::1", wantType: "ipv6"},
		"CDP DECnet bytes":            {normalize: normalizeCDPManagementAddress, addr: "0a000003", addrType: "2", wantAddr: "0a000003", wantType: "2"},
		"CDP DECnet IP-looking text":  {normalize: normalizeCDPManagementAddress, addr: "10.0.0.3", addrType: "2", wantAddr: "10.0.0.3", wantType: "2"},
		"CDP explicit unknown family": {normalize: normalizeCDPManagementAddress, addr: "0a000003", addrType: "999", wantAddr: "0a000003", wantType: "999"},
		"missing family inference":    {normalize: normalizeCDPManagementAddress, addr: "31302E32302E342E323035", wantAddr: "10.20.4.205", wantType: "ipv4"},
		"mapped IPv4 text":            {normalize: normalizeLLDPManagementAddress, addr: "::ffff:192.0.2.1", addrType: "2", wantAddr: "192.0.2.1", wantType: "ipv4"},
		"mapped IPv4 hex":             {normalize: normalizeLLDPManagementAddress, addr: "00000000000000000000ffffc0000201", addrType: "2", wantAddr: "192.0.2.1", wantType: "ipv4"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr, addrType := tc.normalize(tc.addr, tc.addrType)
			require.Equal(t, tc.wantAddr, addr)
			require.Equal(t, tc.wantType, addrType)
		})
	}
}

func TestReconstructLldpRemMgmtAddrHex_FromOctets(t *testing.T) {
	require.Equal(t, "0a14043c", reconstructLldpRemMgmtAddrHex(map[string]string{
		tagLldpRemMgmtAddrLen:             "4",
		tagLldpRemMgmtAddrOctetPref + "1": "10",
		tagLldpRemMgmtAddrOctetPref + "2": "20",
		tagLldpRemMgmtAddrOctetPref + "3": "4",
		tagLldpRemMgmtAddrOctetPref + "4": "60",
	}))
}

func TestAppendManagementAddressFiltersUnusableIPsAndKeepsNonIPFamilies(t *testing.T) {
	var addrs []topologymodel.ManagementAddress
	for _, address := range []string{
		"127.0.0.1",
		"::1",
		"169.254.1.1",
		"fe80::1",
		"0.0.0.0",
		"::",
		"224.0.0.1",
		"255.255.255.255",
		"ff02::1",
		"192.0.2.10",
		"::ffff:192.0.2.10",
		"opaque-management-address",
	} {
		addrs = appendManagementAddress(addrs, topologymodel.ManagementAddress{
			Address: address,
			Source:  "test",
		})
	}

	require.Equal(t, []topologymodel.ManagementAddress{
		{Address: "192.0.2.10", AddressType: "ipv4", Source: "test"},
		{Address: "opaque-management-address", Source: "test"},
	}, addrs)
}

func TestPickManagementIPUsesSourceScopeAndNumericPrecedence(t *testing.T) {
	tests := map[string]struct {
		addrs []topologymodel.ManagementAddress
		want  string
	}{
		"advertised before IP MIB scope": {
			addrs: []topologymodel.ManagementAddress{
				{Address: "10.0.0.1", Source: "ip_mib"},
				{Address: "198.51.100.1", Source: "lldp_remote"},
			},
			want: "198.51.100.1",
		},
		"private before other in same source": {
			addrs: []topologymodel.ManagementAddress{
				{Address: "198.51.100.1", Source: "ip_mib"},
				{Address: "10.0.0.1", Source: "ip_mib"},
			},
			want: "10.0.0.1",
		},
		"numeric before lexical": {
			addrs: []topologymodel.ManagementAddress{
				{Address: "10.20.4.205", Source: "ip_mib"},
				{Address: "10.20.4.60", Source: "ip_mib"},
			},
			want: "10.20.4.60",
		},
		"mapped IPv4 is canonical": {
			addrs: []topologymodel.ManagementAddress{
				{Address: "::ffff:192.0.2.10", Source: "ip_mib"},
			},
			want: "192.0.2.10",
		},
		"non IP and unusable values are ignored": {
			addrs: []topologymodel.ManagementAddress{
				{Address: "opaque"},
				{Address: "127.0.0.1", Source: "lldp_remote"},
				{Address: "fe80::1", Source: "cdp_cache_address"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, pickManagementIP(tc.addrs))
		})
	}
}
