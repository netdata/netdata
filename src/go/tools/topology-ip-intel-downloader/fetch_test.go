// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
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
