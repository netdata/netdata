// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestStringValueProcessor_ProcessValue_HexFormat(t *testing.T) {
	processor := newValueProcessor()
	pdu := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.15.3.1.14.192.0.2.1",
		Type:  gosnmp.OctetString,
		Value: []byte{0x04, 0x03},
	}
	zeroPDU := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.15.3.1.14.192.0.2.2",
		Type:  gosnmp.OctetString,
		Value: []byte{0x00, 0x00},
	}

	tests := map[string]struct {
		symbol   ddprofiledefinition.SymbolConfig
		pdu      gosnmp.SnmpPDU
		expected int64
	}{
		"code": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:                  "1.3.6.1.2.1.15.3.1.14",
				Name:                 "bgpPeerLastErrorCode",
				Format:               "hex",
				ExtractValueCompiled: mustCompileRegex(`^([0-9a-f]{2})`),
			},
			pdu:      pdu,
			expected: 4,
		},
		"subcode": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:                  "1.3.6.1.2.1.15.3.1.14",
				Name:                 "bgpPeerLastErrorSubcode",
				Format:               "hex",
				ExtractValueCompiled: mustCompileRegex(`^[0-9a-f]{2}([0-9a-f]{2})`),
			},
			pdu:      pdu,
			expected: 3,
		},
		"zero code": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:                  "1.3.6.1.2.1.15.3.1.14",
				Name:                 "bgpPeerLastErrorCode",
				Format:               "hex",
				ExtractValueCompiled: mustCompileRegex(`^(00)`),
			},
			pdu:      zeroPDU,
			expected: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			value, err := processor.processValue(tc.symbol, tc.pdu)
			require.NoError(t, err)
			require.Equal(t, tc.expected, value)
		})
	}
}

func TestNumericValueProcessor_ProcessValue_Uint32Format(t *testing.T) {
	processor := newValueProcessor()

	tests := map[string]struct {
		symbol   ddprofiledefinition.SymbolConfig
		pdu      gosnmp.SnmpPDU
		expected int64
	}{
		"reinterpret signed int32 as uint32": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:    "1.3.6.1.2.1.15.3.1.9",
				Name:   "bgpPeerRemoteAs",
				Format: "uint32",
			},
			pdu: gosnmp.SnmpPDU{
				Name:  "1.3.6.1.2.1.15.3.1.9.169.254.1.1",
				Type:  gosnmp.Integer,
				Value: -94967296,
			},
			expected: 4200000000,
		},
		"preserve positive value": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:    "1.3.6.1.2.1.15.3.1.9",
				Name:   "bgpPeerRemoteAs",
				Format: "uint32",
			},
			pdu: gosnmp.SnmpPDU{
				Name:  "1.3.6.1.2.1.15.3.1.9.192.0.2.1",
				Type:  gosnmp.Integer,
				Value: 64512,
			},
			expected: 64512,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			value, err := processor.processValue(tc.symbol, tc.pdu)
			require.NoError(t, err)
			require.Equal(t, tc.expected, value)
		})
	}
}
