// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableRowProcessor_ProcessIndexTag_DropRightIPAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "neighbor",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "cbgpPeer2RemoteAddrIndex",
			Format: "ip_address",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{
				Start:     2,
				DropRight: 2,
			},
		},
	}, "1.4.192.0.2.1.1.128")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "192.0.2.1", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_TailMACAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "fdb_mac",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "dot1qTpFdbAddress",
			Format: "mac_address",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 1},
		},
	}, "7.0.80.86.171.205.239")

	require.NoError(t, err)
	assert.Equal(t, "fdb_mac", tagName)
	assert.Equal(t, "00:50:56:ab:cd:ef", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_LengthPrefixedMACAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	_, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "fdb_mac",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "dot1qTpFdbAddress",
			Format: "mac_address",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 1},
		},
	}, "7.6.0.80.86.171.205.239")

	require.NoError(t, err)
	assert.Equal(t, "00:50:56:ab:cd:ef", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_InvalidMACAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	_, _, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "fdb_mac",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "dot1qTpFdbAddress",
			Format: "mac_address",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 1},
		},
	}, "7.0.80.86.171.205.999")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert transformed index")
}

func TestFormatMACAddress_ColumnAndIndexParity(t *testing.T) {
	columnValue, err := convPhysAddressToString(gosnmp.SnmpPDU{
		Type:  gosnmp.OctetString,
		Value: []byte{0x00, 0x50, 0x56, 0xab, 0xcd, 0xef},
	})
	require.NoError(t, err)

	indexValue, err := formatIndexTagValue("0.80.86.171.205.239", "mac_address")
	require.NoError(t, err)

	assert.Equal(t, columnValue, indexValue)
	assert.Equal(t, "00:50:56:ab:cd:ef", indexValue)
}

func TestTableRowProcessor_ProcessIndexTag_Hex(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "lldp_loc_mgmt_addr",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "lldpLocManAddr",
			Format: "hex",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 2},
		},
	}, "1.4.10.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, "lldp_loc_mgmt_addr", tagName)
	assert.Equal(t, "0a000001", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_HexPreservesNonIPLength(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	_, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "lldp_loc_mgmt_addr",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "lldpLocManAddr",
			Format: "hex",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 2},
		},
	}, "6.6.0.80.86.171.205.239")

	require.NoError(t, err)
	assert.Equal(t, "005056abcdef", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_InvalidHex(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	_, _, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "lldp_loc_mgmt_addr",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "lldpLocManAddr",
			Format: "hex",
		},
		IndexTransform: []ddprofiledefinition.MetricIndexTransform{
			{Start: 2},
		},
	}, "1.4.10.0.0.999")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert transformed index")
}

func TestTableRowProcessor_ProcessIndexTag_RegexMappedFamily(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "subsequent_address_family",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:                 "cbgpPeer2AddrFamilySafiIndex",
			ExtractValueCompiled: mustCompileRegex(`^(?:\d+\.)+\d+\.(\d+)$`),
		},
		Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
			"128": "vpn",
		}),
	}, "1.4.192.0.2.1.1.128")

	require.NoError(t, err)
	assert.Equal(t, "subsequent_address_family", tagName)
	assert.Equal(t, "vpn", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_PositionUsesSymbolNameFallback(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Index: 2,
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name: "neighbor",
		},
		Mapping: ddprofiledefinition.NewExactMapping(map[string]string{
			"42": "mapped",
		}),
	}, "7.42.9")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "mapped", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_IPv4zAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "neighbor",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "bgpPeerRemoteAddrIndex",
			Format: "ip_address",
		},
	}, "192.0.2.1.0.0.0.7")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "192.0.2.1%0.0.0.7", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_IPv6zAddress(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "neighbor",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "bgpPeerRemoteAddrIndex",
			Format: "ip_address",
		},
	}, "254.128.1.2.0.0.0.0.194.213.130.253.254.123.34.167.0.0.14.132")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_TextIPv6AddressIsCanonicalized(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "neighbor",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "bgpPeerRemoteAddrIndex",
			Format: "ip_address",
		},
	}, "2001:0550:0002:002f:0000:0000:0033:0001")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "2001:550:2:2f::33:1", tagValue)
}

func TestTableRowProcessor_ProcessIndexTag_RawIPv6AddressIsCanonicalized(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "neighbor",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:   "bgpPeerRemoteAddrIndex",
			Format: "ip_address",
		},
	}, "32.1.5.80.0.2.0.47.0.0.0.0.0.51.0.1")

	require.NoError(t, err)
	assert.Equal(t, "neighbor", tagName)
	assert.Equal(t, "2001:550:2:2f::33:1", tagValue)
}
