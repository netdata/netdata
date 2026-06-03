// SPDX-License-Identifier: GPL-3.0-or-later

//go:build snmp_topology_fixtures

package parity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadWalkFile_NMS8003Fixture(t *testing.T) {
	ds, err := LoadWalkFile("../../../../testdata/snmp/enlinkd/nms8003/fixtures/NMM-R1.snmpwalk.txt")
	require.NoError(t, err)
	require.NotEmpty(t, ds.Records)

	sysName, ok := ds.Lookup(".1.0.8802.1.1.2.1.3.3.0")
	require.True(t, ok)
	require.Equal(t, "NMM-R1.informatik.hs-fulda.de", sysName.Value)

	lldpRemotes := ds.Prefix("1.0.8802.1.1.2.1.4.1.1")
	require.NotEmpty(t, lldpRemotes)

	oids := ds.SortedOIDs()
	require.NotEmpty(t, oids)
	require.Equal(t, "1.0.8802.1.1.2.1.3.1.0", oids[0])
}
