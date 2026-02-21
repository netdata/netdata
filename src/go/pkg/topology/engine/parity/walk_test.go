// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseWalk(t *testing.T) {
	input := strings.Join([]string{
		`.1.3.6.1.2.1.1.5.0 = STRING: "test-host"`,
		`.1.3.6.1.2.1.1.1.0 = STRING: "line1`,
		`line2"`,
		`invalid line should be ignored`,
		`.1.3.6.1.2.1.1.2.0 = OID: .1.3.6.1.4.1.9.1.1045`,
	}, "\n")

	records, err := ParseWalk(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, records, 3)

	require.Equal(t, "1.3.6.1.2.1.1.5.0", records[0].OID)
	require.Equal(t, "STRING", records[0].Type)
	require.Equal(t, "test-host", records[0].Value)

	require.Equal(t, "1.3.6.1.2.1.1.1.0", records[1].OID)
	require.Equal(t, "line1\nline2", records[1].Value)

	require.Equal(t, "1.3.6.1.2.1.1.2.0", records[2].OID)
	require.Equal(t, ".1.3.6.1.4.1.9.1.1045", records[2].Value)
}

func TestLoadWalkFile_NMS8003Fixture(t *testing.T) {
	ds, err := LoadWalkFile("../testdata/enlinkd/nms8003/fixtures/NMM-R1.snmpwalk.txt")
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
