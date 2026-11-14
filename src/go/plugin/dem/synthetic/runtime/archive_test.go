// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/internal/demruntimetest"
	"github.com/stretchr/testify/require"
)

func TestArchiveRejectsUnsafeAndMismatchedInput(t *testing.T) {
	cases := map[string][]demruntimetest.Entry{
		"escape":           {{Name: "../outside", Contents: []byte("no"), Mode: 0600}},
		"absolute":         {{Name: "/outside", Contents: []byte("no"), Mode: 0600}},
		"hardlink":         {{Name: "alias", Link: "file", Kind: tar.TypeLink}},
		"external-symlink": {{Name: "alias", Link: "../outside", Kind: tar.TypeSymlink}},
		"chained-escape":   {{Name: "d", Kind: tar.TypeDir}, {Name: "other", Kind: tar.TypeDir}, {Name: "d/alias", Link: "../other", Kind: tar.TypeSymlink}, {Name: "escape", Link: "d/alias/../../outside", Kind: tar.TypeSymlink}},
		"symlink-parent":   {{Name: "alias", Link: "inside", Kind: tar.TypeSymlink}, {Name: "alias/file", Contents: []byte("no"), Mode: 0600}},
		"duplicate":        {{Name: "file", Contents: []byte("first"), Mode: 0600}, {Name: "file", Contents: []byte("last"), Mode: 0600}},
		"device":           {{Name: "device", Kind: tar.TypeChar, Mode: 0600}},
		"setuid":           {{Name: "file", Contents: []byte("no"), Mode: 04755}},
		"reserved":         {{Name: "prepared.json", Contents: []byte("{}"), Mode: 0600}},
		"native reserved":  {{Name: "native.json", Contents: []byte("{}"), Mode: 0600}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			a, archive := archiveFixture(t, entries)
			root, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			require.Error(t, extractArchive(bytes.NewReader(archive), root, a))
		})
	}
	for _, field := range []string{"compressed", "expanded", "tar", "entries", "digest"} {
		t.Run(field, func(t *testing.T) {
			m, archive := bundleFixture(t, "test")
			root, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			switch field {
			case "expanded":
				m.Archive.ExpandedBytes--
			case "tar":
				m.Archive.TarBytes--
			case "entries":
				m.Archive.Entries--
			case "compressed":
				m.Archive.Bytes--
			case "digest":
				m.Archive.SHA256 = strings.Repeat("0", 64)
			}
			if field == "compressed" || field == "digest" {
				f, err := os.Create(filepath.Join(t.TempDir(), "archive"))
				require.NoError(t, err)
				defer f.Close()
				require.Error(t, saveArchive(bytes.NewReader(archive), f, m.Archive))
			} else {
				require.Error(t, extractArchive(bytes.NewReader(archive), root, m.Archive))
			}
		})
	}
}

func TestArchiveZstdIntegrityAndBounds(t *testing.T) {
	a, compressed := archiveFixture(t, []demruntimetest.Entry{{Name: "file", Contents: []byte("payload"), Mode: 0600}})
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	raw, err := decoder.DecodeAll(compressed, nil)
	decoder.Close()
	require.NoError(t, err)
	var legacy bytes.Buffer
	gzipWriter := gzip.NewWriter(&legacy)
	_, err = gzipWriter.Write(raw)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	damaged := append([]byte(nil), compressed...)
	damaged[len(damaged)-1] ^= 1
	writer, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	defer writer.Close()
	cases := map[string][]byte{
		"gzip":                          legacy.Bytes(),
		"checksum":                      damaged,
		"truncated checksum":            compressed[:len(compressed)-1],
		"trailing junk":                 append(append([]byte(nil), compressed...), []byte("junk")...),
		"trailing byte":                 append(append([]byte(nil), compressed...), 0),
		"truncated following frame":     append(append([]byte(nil), compressed...), 0x28, 0xb5, 0x2f),
		"concatenated decoded overflow": append(append([]byte(nil), compressed...), writer.EncodeAll([]byte("extra"), nil)...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			require.Error(t, extractArchive(bytes.NewReader(data), root, a))
		})
	}
	t.Run("window exceeds builder contract", func(t *testing.T) {
		root, err := os.OpenRoot(t.TempDir())
		require.NoError(t, err)
		defer root.Close()
		// Frame header declares a 16 MiB window before any block is decoded.
		frame := []byte{0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x70, 0x01, 0x00, 0x00}
		require.ErrorIs(t, extractArchive(bytes.NewReader(frame), root, a), zstd.ErrWindowSizeExceeded)
	})
	t.Run("skippable frames retain bounded decoded content", func(t *testing.T) {
		root, err := os.OpenRoot(t.TempDir())
		require.NoError(t, err)
		defer root.Close()
		// A valid empty skippable frame contributes no tar bytes.
		frame := []byte{0x50, 0x2a, 0x4d, 0x18, 0, 0, 0, 0}
		require.NoError(t, extractArchive(io.MultiReader(bytes.NewReader(frame), bytes.NewReader(compressed)), root, a))
	})
}
