// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableTagProcessor_ProcessTag_Uint32Format(t *testing.T) {
	processor := newTableTagProcessor()
	ta := tagAdder{tags: map[string]string{}}

	err := processor.processTag(ddprofiledefinition.MetricTagConfig{
		Tag: "remote_as",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID:    "1.3.6.1.2.1.15.3.1.9",
			Name:   "bgpPeerRemoteAs",
			Format: "uint32",
		},
	}, gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.15.3.1.9.169.254.1.1",
		Type:  gosnmp.Integer,
		Value: -94967296,
	}, ta)

	require.NoError(t, err)
	assert.Equal(t, "4200000000", ta.tags["remote_as"])
}

func TestTableTagProcessor_ProcessTag_TextDateNoValueSkipsTag(t *testing.T) {
	processor := newTableTagProcessor()
	ta := tagAdder{tags: map[string]string{}}

	err := processor.processTag(ddprofiledefinition.MetricTagConfig{
		Tag: "license_expiry",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID:    "1.3.6.1.4.1.999.1.2",
			Format: "text_date",
		},
	}, createStringPDU("1.3.6.1.4.1.999.1.2.0", "n/a"), ta)

	require.NoError(t, err)
	assert.Empty(t, ta.tags)
}

func TestMetricTagDisplayName(t *testing.T) {
	tests := map[string]struct {
		cfg  ddprofiledefinition.MetricTagConfig
		want string
	}{
		"explicit tag": {
			cfg: ddprofiledefinition.MetricTagConfig{
				Tag: "neighbor",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					Name: "peer_addr",
				},
			},
			want: "neighbor",
		},
		"symbol name fallback": {
			cfg: ddprofiledefinition.MetricTagConfig{
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					Name: "peer_addr",
				},
			},
			want: "peer_addr",
		},
		"index fallback": {
			cfg: ddprofiledefinition.MetricTagConfig{
				Index: 2,
			},
			want: "index2",
		},
		"raw index fallback": {
			cfg:  ddprofiledefinition.MetricTagConfig{},
			want: "index",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, metricTagDisplayName(tc.cfg))
		})
	}
}
