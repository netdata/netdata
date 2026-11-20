// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestStringValueProcessor_ProcessValue_HexFormat(t *testing.T) {
	processor := newValueProcessor()
	pdu := gosnmp.VarBind{
		Name:  "1.3.6.1.2.1.15.3.1.14.192.0.2.1",
		Value: gosnmp.OctetStringValue([]byte{0x04, 0x03}),
	}
	zeroPDU := gosnmp.VarBind{
		Name:  "1.3.6.1.2.1.15.3.1.14.192.0.2.2",
		Value: gosnmp.OctetStringValue([]byte{0x00, 0x00}),
	}

	tests := map[string]struct {
		symbol   ddprofiledefinition.SymbolConfig
		pdu      gosnmp.VarBind
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
		pdu      gosnmp.VarBind
		expected int64
	}{
		"reinterpret signed int32 as uint32": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:    "1.3.6.1.2.1.15.3.1.9",
				Name:   "bgpPeerRemoteAs",
				Format: "uint32",
			},
			pdu: gosnmp.VarBind{
				Name:  "1.3.6.1.2.1.15.3.1.9.169.254.1.1",
				Value: gosnmp.IntegerValue(-94967296),
			},
			expected: 4200000000,
		},
		"preserve positive value": {
			symbol: ddprofiledefinition.SymbolConfig{
				OID:    "1.3.6.1.2.1.15.3.1.9",
				Name:   "bgpPeerRemoteAs",
				Format: "uint32",
			},
			pdu: gosnmp.VarBind{
				Name:  "1.3.6.1.2.1.15.3.1.9.192.0.2.1",
				Value: gosnmp.IntegerValue(64512),
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

func TestValueProcessor_ProcessValue_TextDateFormatOnNumericPDU(t *testing.T) {
	processor := newValueProcessor()
	symbol := ddprofiledefinition.SymbolConfig{
		OID:    "1.3.6.1.4.1.99999.1.1.0",
		Name:   "licenseExpiry",
		Format: "text_date",
	}

	value, err := processor.processValue(symbol, gosnmp.VarBind{
		Name:  "1.3.6.1.4.1.99999.1.1.0",
		Value: gosnmp.Gauge32Value(1798675200),
	})
	require.NoError(t, err)
	require.EqualValues(t, 1798675200, value)

	_, err = processor.processValue(symbol, gosnmp.VarBind{
		Name:  "1.3.6.1.4.1.99999.1.1.0",
		Value: gosnmp.Gauge32Value(4294967295),
	})
	require.ErrorIs(t, err, errNoTextDateValue)
}
