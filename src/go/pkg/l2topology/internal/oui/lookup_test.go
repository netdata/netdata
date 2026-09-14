// SPDX-License-Identifier: GPL-3.0-or-later

package oui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildVendorIndex(t *testing.T) {
	index := buildVendorIndex(strings.Join([]string{
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

func TestLookupVendorByMACInIndex(t *testing.T) {
	index := buildVendorIndex(`
08EA44	Extreme Networks Headquarters
08EA4411	Extreme Specific
`)

	tests := map[string]struct {
		mac    string
		vendor string
		prefix string
	}{
		"longest prefix": {
			mac:    "08ea.4411.2233",
			vendor: "Extreme Specific",
			prefix: "08EA4411",
		},
		"delimited hex": {
			mac:    "08-ea-44-aa-bb-cc",
			vendor: "Extreme Networks Headquarters",
			prefix: "08EA44",
		},
		"dotted decimal": {
			mac:    "8.234.68.170.187.204",
			vendor: "Extreme Networks Headquarters",
			prefix: "08EA44",
		},
		"unknown": {
			mac:    "00:11:22:33:44:55",
			vendor: "",
			prefix: "",
		},
		"invalid": {
			mac:    "not-a-mac",
			vendor: "",
			prefix: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			vendor, prefix := lookupVendorByMACInIndex(index, tc.mac)

			require.Equal(t, tc.vendor, vendor)
			require.Equal(t, tc.prefix, prefix)
		})
	}
}
