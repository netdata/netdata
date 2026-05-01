// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIPToASNCombinedTSVAsn(t *testing.T) {
	payload := []byte(
		"1.0.0.0\t1.0.0.255\t13335\tUS\tCloudflare\n" +
			"2001:db8::\t2001:db8::ffff\t64512\tDE\tExample ASN\n",
	)
	asnRanges, err := parseIPToASNCombinedTSVAsn(payload)
	require.NoError(t, err)
	require.Len(t, asnRanges, 2)
	require.EqualValues(t, 13335, asnRanges[0].asn)
	require.Equal(t, "Cloudflare", asnRanges[0].org)
	require.EqualValues(t, 64512, asnRanges[1].asn)
}

func TestParseIPToASNCombinedTSVGeo(t *testing.T) {
	payload := []byte(
		"1.0.0.0\t1.0.0.255\t13335\tUS\tCloudflare\n" +
			"2001:db8::\t2001:db8::ffff\t64512\tDE\tExample ASN\n",
	)
	geoRanges, err := parseIPToASNCombinedTSVGeo(payload)
	require.NoError(t, err)
	require.Len(t, geoRanges, 2)
	require.Equal(t, "US", geoRanges[0].country)
	require.Equal(t, "DE", geoRanges[1].country)
}

func TestParseDBIPAsnCSV(t *testing.T) {
	payload := []byte("\"1.0.0.0\",\"1.0.0.255\",\"13335\",\"Cloudflare\"\n")
	ranges, err := parseDBIPAsnCSV(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	require.EqualValues(t, 13335, ranges[0].asn)
	require.Equal(t, "Cloudflare", ranges[0].org)
}

func TestParseDBIPCountryCSV(t *testing.T) {
	payload := []byte("\"1.0.0.0\",\"1.0.0.255\",\"US\"\n")
	ranges, err := parseDBIPCountryCSV(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	require.Equal(t, "US", ranges[0].country)
}

func TestParseDBIPCityCSV(t *testing.T) {
	payload := []byte("\"1.0.0.0\",\"1.0.0.255\",\"NA\",\"US\",\"California\",\"Mountain View\",\"37.3861\",\"-122.0839\"\n")
	ranges, err := parseDBIPCityCSV(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	require.Equal(t, "US", ranges[0].country)
	require.Equal(t, "California", ranges[0].state)
	require.Equal(t, "Mountain View", ranges[0].city)
	require.True(t, ranges[0].hasLocation)
	require.InDelta(t, 37.3861, ranges[0].latitude, 0.0001)
	require.InDelta(t, -122.0839, ranges[0].longitude, 0.0001)
}

func TestParseIPDecimalIPv4(t *testing.T) {
	addr, err := parseIP("16777216") // 1.0.0.0
	require.NoError(t, err)
	require.Equal(t, "1.0.0.0", addr.String())
}

func TestParseRangeRejectsFamilyMix(t *testing.T) {
	_, _, err := parseRangeEndpoints("1.0.0.0", "2001:db8::1")
	require.Error(t, err)
}
