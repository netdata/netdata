// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"strings"
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

func TestParseCAIDAPrefix2AS(t *testing.T) {
	payload := []byte(
		"1.0.0.0\t24\t13335_38803\n" +
			"2001:db8::\t48\t64512\n",
	)
	ranges, err := parseCAIDAPrefix2AS(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	require.Equal(t, "1.0.0.0", ranges[0].start.String())
	require.Equal(t, "1.0.0.255", ranges[0].end.String())
	require.EqualValues(t, 13335, ranges[0].asn)
	require.Equal(t, "2001:db8::", ranges[1].start.String())
	require.EqualValues(t, 64512, ranges[1].asn)
}

func TestParseMaxMindCountryCSVZip(t *testing.T) {
	payload := buildZip(t, map[string]string{
		"GeoLite2-Country-CSV_20260501/GeoLite2-Country-Blocks-IPv4.csv": strings.Join([]string{
			"network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,is_anycast",
			"1.0.0.0/24,6252001,,,,0,0",
		}, "\n"),
		"GeoLite2-Country-CSV_20260501/GeoLite2-Country-Blocks-IPv6.csv": strings.Join([]string{
			"network,geoname_id,registered_country_geoname_id,represented_country_geoname_id,is_anonymous_proxy,is_satellite_provider,is_anycast",
			"2001:db8::/48,2921044,,,,0,0",
		}, "\n"),
		"GeoLite2-Country-CSV_20260501/GeoLite2-Country-Locations-en.csv": strings.Join([]string{
			"geoname_id,locale_code,continent_code,continent_name,country_iso_code,country_name,is_in_european_union",
			"6252001,en,NA,North America,US,United States,0",
			"2921044,en,EU,Europe,DE,Germany,1",
		}, "\n"),
	})

	ranges, err := parseMaxMindCountryCSVZip(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	require.Equal(t, "US", ranges[0].country)
	require.Equal(t, "1.0.0.0", ranges[0].start.String())
	require.Equal(t, "DE", ranges[1].country)
	require.Equal(t, "2001:db8::", ranges[1].start.String())
}

func TestParseIP2LocationCountryZip(t *testing.T) {
	payload := buildZip(t, map[string]string{
		"IP2LOCATION-LITE-DB1.CSV": "\"16777216\",\"16777471\",\"AU\",\"Australia\"\n",
	})
	ranges, err := parseIP2LocationCountryZip(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	require.Equal(t, "1.0.0.0", ranges[0].start.String())
	require.Equal(t, "1.0.0.255", ranges[0].end.String())
	require.Equal(t, "AU", ranges[0].country)
}

func TestParseIPDenyCountryTarGZ(t *testing.T) {
	payload := buildTarGZ(t, map[string]string{
		"./us.zone": "1.0.0.0/24\n",
		"./GR.ZONE": "1.0.1.0/24\n",
	})
	ranges, err := parseIPDenyCountryTarGZ(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 2)
	byCountry := make(map[string]geoRange, len(ranges))
	for _, rec := range ranges {
		byCountry[rec.country] = rec
	}
	require.Equal(t, "1.0.0.255", byCountry["US"].end.String())
	require.Equal(t, "1.0.1.255", byCountry["GR"].end.String())
}

func TestParseIPIPCountryZip(t *testing.T) {
	payload := buildZip(t, map[string]string{
		"country.txt": "1.0.0.0/24\tANYCAST\n1.0.1.0/24\tCN\n",
	})
	ranges, err := parseIPIPCountryZip(payload)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	require.Equal(t, "CN", ranges[0].country)
	require.Equal(t, "1.0.1.0", ranges[0].start.String())
	require.Equal(t, "1.0.1.255", ranges[0].end.String())
}

func TestEstimatedRangeCapacity(t *testing.T) {
	require.Equal(t, 0, estimatedRangeCapacity(0, 64, 1<<20))
	require.Equal(t, 0, estimatedRangeCapacity(128, 0, 1<<20))
	require.Equal(t, 2, estimatedRangeCapacity(128, 64, 1<<20))
	require.Equal(t, 4, estimatedRangeCapacity(1024, 64, 4))
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

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func buildTarGZ(t *testing.T, files map[string]string) []byte {
	t.Helper()

	rawTar := buildTar(t, files)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(rawTar)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func buildTar(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		})
		require.NoError(t, err)
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}
