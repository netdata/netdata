// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strconv"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvPduToStringf_SNMPDateAndTime(t *testing.T) {
	tests := []struct {
		name string
		pdu  gosnmp.SnmpPDU
		want int64
	}{
		{
			name: "without timezone",
			pdu: gosnmp.SnmpPDU{
				Type:  gosnmp.OctetString,
				Value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x00},
			},
			want: time.Date(2024, time.April, 3, 10, 11, 12, 0, time.UTC).Unix(),
		},
		{
			name: "with timezone",
			pdu: gosnmp.SnmpPDU{
				Type:  gosnmp.OctetString,
				Value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x00, '+', 0x02, 0x00},
			},
			want: time.Date(2024, time.April, 3, 8, 11, 12, 0, time.UTC).Unix(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convPduToStringf(tt.pdu, "snmp_dateandtime")
			require.NoError(t, err)
			assert.Equal(t, tt.want, mustParseInt64(t, got))
		})
	}
}

func TestConvPduToDateAndTimeUnix_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{
			name:  "invalid month",
			value: []byte{0x07, 0xE8, 0x0D, 0x03, 0x0A, 0x0B, 0x0C, 0x00},
		},
		{
			name:  "invalid day",
			value: []byte{0x07, 0xE8, 0x04, 0x00, 0x0A, 0x0B, 0x0C, 0x00},
		},
		{
			name:  "invalid hour",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x18, 0x0B, 0x0C, 0x00},
		},
		{
			name:  "invalid minute",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x3C, 0x0C, 0x00},
		},
		{
			name:  "invalid second",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x3D, 0x00},
		},
		{
			name:  "leap second unsupported",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x3C, 0x00},
		},
		{
			name:  "invalid decisecond",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x0A},
		},
		{
			name:  "invalid timezone direction",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x00, 'x', 0x02, 0x00},
		},
		{
			name:  "invalid timezone hours",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x00, '+', 0x0E, 0x00},
		},
		{
			name:  "invalid timezone minutes",
			value: []byte{0x07, 0xE8, 0x04, 0x03, 0x0A, 0x0B, 0x0C, 0x00, '+', 0x02, 0x3C},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convPduToDateAndTimeUnix(gosnmp.SnmpPDU{
				Type:  gosnmp.OctetString,
				Value: tt.value,
			})
			require.Error(t, err)
		})
	}
}

func mustParseInt64(t *testing.T, s string) int64 {
	t.Helper()

	v, err := strconv.ParseInt(s, 10, 64)
	require.NoError(t, err)
	return v
}
