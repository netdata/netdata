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

func TestParseWalk_MultilineQuotedValueWithEscapes(t *testing.T) {
	input := strings.Join([]string{
		`.1.3.6.1.2.1.1.6.0 = STRING: "line1\\`,
		`line2\"`,
		`line3"`,
	}, "\n")

	records, err := ParseWalk(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "line1\\\\\nline2\"\nline3", records[0].Value)
}

func TestWalkDatasetLookupCachesByOID(t *testing.T) {
	ds := WalkDataset{
		Records: []WalkRecord{
			{OID: "1.3.6.1.2.1.1.5.0", Type: "STRING", Value: "test-host"},
		},
	}

	record, ok := ds.Lookup(".1.3.6.1.2.1.1.5.0")
	require.True(t, ok)
	require.Equal(t, "test-host", record.Value)
	require.Contains(t, ds.byOID, "1.3.6.1.2.1.1.5.0")
}

func TestWalkDatasetSortedOIDs_UsesNumericOIDOrdering(t *testing.T) {
	ds := WalkDataset{
		Records: []WalkRecord{
			{OID: "1.3.6.1.2.1.1.10.0"},
			{OID: "1.3.6.1.2.1.1.2.0"},
			{OID: "1.3.6.1.2.1.1.2.1"},
		},
	}

	require.Equal(t, []string{
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.2.1",
		"1.3.6.1.2.1.1.10.0",
	}, ds.SortedOIDs())
}
