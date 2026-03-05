// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodePayloadAutoGzip(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err := zw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	decoded, err := decodePayload(compressed.Bytes(), compressionAuto)
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

	decoded, err := decodePayload(zipped.Bytes(), compressionAuto)
	require.NoError(t, err)
	require.Equal(t, []byte("world"), decoded)
}
