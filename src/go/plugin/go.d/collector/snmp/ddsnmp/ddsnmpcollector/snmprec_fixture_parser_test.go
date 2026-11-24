// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"net/netip"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSNMPRecPDU_SupportsAdditionalTypes(t *testing.T) {
	t.Run("object identifier", func(t *testing.T) {
		pdu, err := snmprecPDU("1.2.3.0", "6", "1.3.6.1.4.1.6527.1.3.17")
		require.NoError(t, err)
		assert.Equal(t, gosnmp.ObjectIdentifierValue("1.3.6.1.4.1.6527.1.3.17"), pdu.Value)
	})

	t.Run("timeticks", func(t *testing.T) {
		pdu, err := snmprecPDU("1.2.3.0", "67", "2158388928")
		require.NoError(t, err)
		assert.Equal(t, gosnmp.TimeTicksValue(2158388928), pdu.Value)
	})

	t.Run("counter64", func(t *testing.T) {
		pdu, err := snmprecPDU("1.2.3.0", "70", "151712621952719")
		require.NoError(t, err)
		assert.Equal(t, gosnmp.Counter64Value(151712621952719), pdu.Value)
	})

	t.Run("hex ipaddress", func(t *testing.T) {
		pdu, err := snmprecPDU("1.2.3.0", "64x", "C0A80001")
		require.NoError(t, err)
		assert.Equal(t, gosnmp.IPAddressValue(netip.MustParseAddr("192.168.0.1")), pdu.Value)
	})
}
