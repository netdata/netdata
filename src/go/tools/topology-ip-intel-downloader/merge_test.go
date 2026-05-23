// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAsnSourcesSplitsOverlapsByPriority(t *testing.T) {
	merged, err := mergeAsnSources([][]asnRange{
		{
			{
				start: mustParseAddr(t, "10.20.0.0"),
				end:   mustParseAddr(t, "10.20.255.255"),
				asn:   2,
				org:   "high",
			},
		},
		{
			{
				start: mustParseAddr(t, "10.0.0.0"),
				end:   mustParseAddr(t, "10.255.255.255"),
				asn:   1,
				org:   "low",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []asnRange{
		{
			start: mustParseAddr(t, "10.0.0.0"),
			end:   mustParseAddr(t, "10.19.255.255"),
			asn:   1,
			org:   "low",
		},
		{
			start: mustParseAddr(t, "10.20.0.0"),
			end:   mustParseAddr(t, "10.20.255.255"),
			asn:   2,
			org:   "high",
		},
		{
			start: mustParseAddr(t, "10.21.0.0"),
			end:   mustParseAddr(t, "10.255.255.255"),
			asn:   1,
			org:   "low",
		},
	}, merged)
}

func TestMergeGeoSourcesCoalescesAdjacentIntervals(t *testing.T) {
	merged, err := mergeGeoSources([][]geoRange{
		{
			{
				start:   mustParseAddr(t, "1.0.0.0"),
				end:     mustParseAddr(t, "1.0.0.127"),
				country: "US",
				state:   "California",
				city:    "Mountain View",
			},
			{
				start:   mustParseAddr(t, "1.0.0.128"),
				end:     mustParseAddr(t, "1.0.0.255"),
				country: "US",
				state:   "California",
				city:    "Mountain View",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []geoRange{
		{
			start:   mustParseAddr(t, "1.0.0.0"),
			end:     mustParseAddr(t, "1.0.0.255"),
			country: "US",
			state:   "California",
			city:    "Mountain View",
		},
	}, merged)
}

func TestMergeGeoSourcesKeepsHigherPriorityCityDetail(t *testing.T) {
	merged, err := mergeGeoSources([][]geoRange{
		{
			{
				start:       mustParseAddr(t, "8.8.8.0"),
				end:         mustParseAddr(t, "8.8.8.255"),
				country:     "US",
				state:       "California",
				city:        "Mountain View",
				latitude:    37.422,
				longitude:   -122.085,
				hasLocation: true,
			},
		},
		{
			{
				start:   mustParseAddr(t, "8.8.8.0"),
				end:     mustParseAddr(t, "8.8.8.255"),
				country: "US",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []geoRange{
		{
			start:       mustParseAddr(t, "8.8.8.0"),
			end:         mustParseAddr(t, "8.8.8.255"),
			country:     "US",
			state:       "California",
			city:        "Mountain View",
			latitude:    37.422,
			longitude:   -122.085,
			hasLocation: true,
		},
	}, merged)
}

func TestMergeAsnSourcesHandlesIPv6Independently(t *testing.T) {
	merged, err := mergeAsnSources([][]asnRange{
		{
			{
				start: mustParseAddr(t, "2001:db8::100"),
				end:   mustParseAddr(t, "2001:db8::1ff"),
				asn:   65001,
				org:   "high",
			},
		},
		{
			{
				start: mustParseAddr(t, "2001:db8::"),
				end:   mustParseAddr(t, "2001:db8::ffff"),
				asn:   65000,
				org:   "low",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []asnRange{
		{
			start: mustParseAddr(t, "2001:db8::"),
			end:   mustParseAddr(t, "2001:db8::ff"),
			asn:   65000,
			org:   "low",
		},
		{
			start: mustParseAddr(t, "2001:db8::100"),
			end:   mustParseAddr(t, "2001:db8::1ff"),
			asn:   65001,
			org:   "high",
		},
		{
			start: mustParseAddr(t, "2001:db8::200"),
			end:   mustParseAddr(t, "2001:db8::ffff"),
			asn:   65000,
			org:   "low",
		},
	}, merged)
}
