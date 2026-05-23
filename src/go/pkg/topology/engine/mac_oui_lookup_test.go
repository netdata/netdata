// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestBuildTopologyOUIVendorIndex_IgnoresInvalidLinesAndKeepsFirstDuplicate(t *testing.T) {
	index := buildTopologyOUIVendorIndex(strings.Join([]string{
		"# comment",
		"08EA44\tExtreme Networks Headquarters",
		"08EA44\tduplicate should be ignored",
		"286FB9\tNokia Shanghai Bell Co., Ltd.",
		"08EA4411\tExtreme Specific",
		"bad-line-without-tab",
		"12345\ttoo short",
		"1234567890123\ttoo long",
		"GGGGGG\tvalid-length non-hex prefix",
		"ABCDEF\t",
		"\tNo Prefix",
	}, "\n"))

	require.Equal(t, []int{8, 6}, index.prefixLens)
	require.Equal(t, "Extreme Networks Headquarters", index.byPrefixLen[6]["08EA44"])
	require.Equal(t, "Nokia Shanghai Bell Co., Ltd.", index.byPrefixLen[6]["286FB9"])
	require.Equal(t, "Extreme Specific", index.byPrefixLen[8]["08EA4411"])
	require.NotContains(t, index.byPrefixLen[6], "GGGGGG")
	require.Len(t, index.byPrefixLen[6], 2)
}

func TestLookupTopologyVendorByMACInIndex_PrefersLongestPrefixAndNormalizesMAC(t *testing.T) {
	index := buildTopologyOUIVendorIndex(`
08EA44	Extreme Networks Headquarters
08EA4411	Extreme Specific
`)

	vendor, prefix := lookupTopologyVendorByMACInIndex(index, "08ea.4411.2233")
	require.Equal(t, "Extreme Specific", vendor)
	require.Equal(t, "08EA4411", prefix)

	vendor, prefix = lookupTopologyVendorByMACInIndex(index, "08-ea-44-aa-bb-cc")
	require.Equal(t, "Extreme Networks Headquarters", vendor)
	require.Equal(t, "08EA44", prefix)
}

func TestInferTopologyVendorFromMatch_UsesDeterministicCandidateOrder(t *testing.T) {
	match := topology.Match{
		MacAddresses: []string{
			"28:6f:b9:00:00:22",
			"08:ea:44:11:22:33",
		},
		ChassisIDs: []string{
			"28:6f:b9:00:00:22",
			"08ea.4411.2233",
		},
	}

	vendor, prefix := inferTopologyVendorFromMatch(match)
	require.Equal(t, "Extreme Networks Headquarters", vendor)
	require.Equal(t, "08EA44", prefix)
}
