// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

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

func TestTableRowProcessor_ProcessIndexTag_RegexMappedFamily(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	tagName, tagValue, err := p.processIndexTag(ddprofiledefinition.MetricTagConfig{
		Tag: "subsequent_address_family",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			Name:                 "cbgpPeer2AddrFamilySafiIndex",
			ExtractValueCompiled: mustCompileRegex(`^(?:\d+\.)+\d+\.(\d+)$`),
		},
		Mapping: map[string]string{
			"128": "vpn",
		},
	}, "1.4.192.0.2.1.1.128")

	require.NoError(t, err)
	assert.Equal(t, "subsequent_address_family", tagName)
	assert.Equal(t, "vpn", tagValue)
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
	assert.Equal(t, "fe80:0102:0000:0000:c2d5:82fd:fe7b:22a7%0.0.14.132", tagValue)
}
