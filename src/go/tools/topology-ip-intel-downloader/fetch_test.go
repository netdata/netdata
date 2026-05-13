// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecodePayloadAutoGzip(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	decoded, err := decodePayload(compressed.Bytes())
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), decoded)
}

func TestDecodePayloadAutoZip(t *testing.T) {
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	entry, err := zw.Create("data.txt")
	require.NoError(t, err)
	_, err = entry.Write([]byte("world"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	decoded, err := decodePayload(zipped.Bytes())
	require.NoError(t, err)
	require.Equal(t, []byte("world"), decoded)
}

func TestResolveDBIPArtifactURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/download", r.URL.Path)
		_, _ = w.Write([]byte(`
			<html>
			  <body>
			    <a href="https://download.db-ip.com/free/dbip-city-lite-2026-03.mmdb.gz">MMDB</a>
			    <a href="https://download.db-ip.com/free/dbip-city-lite-2026-03.csv.gz">CSV</a>
			  </body>
			</html>
		`))
	}))
	defer server.Close()

	dl := newDownloader(httpConfig{timeout: time.Second, userAgent: "test"})
	resolved, err := dl.resolveDBIPArtifactURL(server.URL+"/download", artifactDBIPCityLite, formatMMDB)
	require.NoError(t, err)
	require.Equal(t, "https://download.db-ip.com/free/dbip-city-lite-2026-03.mmdb.gz", resolved)
}

func TestResolveCAIDAPrefix2ASURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/routing/pfx2as-creation.log", r.URL.Path)
		_, _ = w.Write([]byte(
			"1\t1778084001\t2026/05/routeviews-rv2-20260505-1200.pfx2as.gz\n" +
				"2\t1778170446\t2026/05/routeviews-rv2-20260506-1200.pfx2as.gz\n" +
				"3\t1777997562\t2026/05/routeviews-rv2-20260504-1200.pfx2as.gz\n",
		))
	}))
	defer server.Close()

	dl := newDownloader(httpConfig{timeout: time.Second, userAgent: "test"})
	resolved, err := dl.resolveCAIDAPrefix2ASURL(server.URL + "/routing/pfx2as-creation.log?mirror=local")
	require.NoError(t, err)
	require.Equal(t, server.URL+"/routing/2026/05/routeviews-rv2-20260506-1200.pfx2as.gz", resolved)
}

func TestResolveMaxMindSourceExpandsEnvForFetchAndRedactsMetadata(t *testing.T) {
	t.Setenv("MAXMIND_LICENSE_KEY", "secret-license-key")

	dl := newDownloader(httpConfig{timeout: time.Second, userAgent: "test"})
	resolved, err := dl.resolveSource(sourceEntry{
		name:     "maxmind-asn",
		family:   sourceFamilyASN,
		provider: providerMaxMind,
		artifact: artifactMaxMindGeoLite2ASN,
		format:   formatMMDB,
	})
	require.NoError(t, err)
	require.Contains(t, resolved.fetchURL, "secret-license-key")
	require.NotContains(t, resolved.ref.URL, "secret-license-key")
	require.Contains(t, resolved.ref.URL, "redacted")
}

func TestResolveMaxMindSourceReportsMissingEnvWithoutSecretURL(t *testing.T) {
	t.Setenv("MAXMIND_LICENSE_KEY", "")

	dl := newDownloader(httpConfig{timeout: time.Second, userAgent: "test"})
	_, err := dl.resolveSource(sourceEntry{
		name:     "maxmind-asn",
		family:   sourceFamilyASN,
		provider: providerMaxMind,
		artifact: artifactMaxMindGeoLite2ASN,
		format:   formatMMDB,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MAXMIND_LICENSE_KEY")
}

func TestDecodeMaxMindASNPayloadExtractsTarredMMDB(t *testing.T) {
	payload := "fake mmdb"
	tarball := buildTarGZ(t, map[string]string{
		"GeoLite2-ASN_20260508/GeoLite2-ASN.mmdb": payload,
	})

	decoded, err := decodeMaxMindASNPayload(tarball)
	require.NoError(t, err)
	require.Equal(t, []byte(payload), decoded)
}

func TestDecodeMaxMindASNPayloadExtractsTarWithoutUSTARMagic(t *testing.T) {
	payload := "legacy tar mmdb"
	rawTar := buildTar(t, map[string]string{
		"GeoLite2-ASN_20260508/GeoLite2-ASN.mmdb": payload,
	})
	clearTarMagicAndRecomputeChecksum(t, rawTar[:512])

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(rawTar)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	decoded, err := decodeMaxMindASNPayload(compressed.Bytes())
	require.NoError(t, err)
	require.Equal(t, []byte(payload), decoded)
}

func TestDecodeMaxMindASNPayloadAcceptsGzippedMMDB(t *testing.T) {
	payload := []byte("fake mmdb")
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(payload)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	decoded, err := decodeMaxMindASNPayload(compressed.Bytes())
	require.NoError(t, err)
	require.Equal(t, payload, decoded)
}

func TestDecodeMaxMindASNPayloadRejectsCorruptTarPayload(t *testing.T) {
	tarLikeCorruptPayload := make([]byte, 512)
	copy(tarLikeCorruptPayload[257:], []byte("ustar"))

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write(tarLikeCorruptPayload)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	_, err = decodeMaxMindASNPayload(compressed.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to extract MaxMind ASN MMDB from tar payload")
}

func TestExpandEnvPlaceholdersRequiresMissingVariable(t *testing.T) {
	t.Setenv("TOPOLOGY_TEST_PRESENT", "value")
	_, err := expandEnvPlaceholders("https://example.test/${TOPOLOGY_TEST_MISSING}/${TOPOLOGY_TEST_PRESENT}")
	require.Error(t, err)
	require.Contains(t, err.Error(), "TOPOLOGY_TEST_MISSING")
	require.NotContains(t, err.Error(), "value")
}

func TestResolveSourceUsesLocalPathWithoutNetwork(t *testing.T) {
	dl := newDownloader(httpConfig{timeout: time.Second, userAgent: "test"})
	resolved, err := dl.resolveSource(sourceEntry{
		name:     "custom-city",
		family:   sourceFamilyGeo,
		provider: providerDBIP,
		artifact: artifactDBIPCityLite,
		format:   formatMMDB,
		path:     "/tmp/custom-city.mmdb",
	})
	require.NoError(t, err)
	require.Equal(t, "/tmp/custom-city.mmdb", resolved.fetchPath)
	require.Equal(t, "path", resolved.ref.Source)
	require.Equal(t, "/tmp/custom-city.mmdb", resolved.ref.Path)
}

func clearTarMagicAndRecomputeChecksum(t *testing.T, header []byte) {
	t.Helper()

	require.Len(t, header, 512)
	for i := 257; i < 265; i++ {
		header[i] = 0
	}
	for i := 148; i < 156; i++ {
		header[i] = ' '
	}
	sum := 0
	for _, b := range header {
		sum += int(b)
	}
	copy(header[148:156], []byte(fmt.Sprintf("%06o\x00 ", sum)))
}
