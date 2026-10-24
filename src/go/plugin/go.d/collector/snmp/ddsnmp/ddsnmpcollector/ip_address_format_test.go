// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"net/netip"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvPduToIPAddress(t *testing.T) {
	t.Run("raw ipv4 octets", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte{169, 254, 247, 2}))
		require.NoError(t, err)
		assert.Equal(t, "169.254.247.2", s)
	})

	t.Run("textual octet string ipv4", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte("169.254.247.2")))
		require.NoError(t, err)
		assert.Equal(t, "169.254.247.2", s)
	})

	t.Run("decimal encoded octets that decode to textual ip", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte("49.57.50.46.49.54.56.46.50.53.53.46.49.48")))
		require.NoError(t, err)
		assert.Equal(t, "192.168.255.10", s)
	})

	t.Run("textual ipv6 octet string", func(t *testing.T) {
		input := "2001:0550:0002:002f:0000:0000:0033:0001"
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte("2001:0550:0002:002f:0000:0000:0033:0001")))
		require.NoError(t, err)
		assert.Equal(t, netip.MustParseAddr(input).String(), s)
	})

	t.Run("textual ipv6-mapped ipv4 octet string is unmapped", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte("::ffff:192.0.2.10")))
		require.NoError(t, err)
		assert.Equal(t, "192.0.2.10", s)
	})

	t.Run("raw ipv4z octets", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte{192, 0, 2, 1, 0, 0, 0, 7}))
		require.NoError(t, err)
		assert.Equal(t, "192.0.2.1%0.0.0.7", s)
	})

	t.Run("raw ipv6 octets are canonicalized like text", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte{
			32, 1, 5, 80, 0, 2, 0, 47, 0, 0, 0, 0, 0, 51, 0, 1,
		}))
		require.NoError(t, err)
		assert.Equal(t, "2001:550:2:2f::33:1", s)
	})

	t.Run("raw ipv6z octets", func(t *testing.T) {
		s, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte{
			254, 128, 1, 2, 0, 0, 0, 0, 194, 213, 130, 253, 254, 123, 34, 167,
			0, 0, 14, 132,
		}))
		require.NoError(t, err)
		assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132", s)
	})

	t.Run("invalid octet string", func(t *testing.T) {
		_, err := convPduToIPAddress(createPDU("1.2.3", gosnmp.OctetString, []byte("not-an-ip")))
		require.Error(t, err)
	})
}
