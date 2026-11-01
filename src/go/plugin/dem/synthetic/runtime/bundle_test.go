// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

type fixtureEntry struct {
	name, link string
	contents   []byte
	kind       byte
	mode       int64
}

func bundleFixture(t testing.TB, label string) (Manifest, []byte) {
	t.Helper()
	expected := append([]byte("/not-prepared/"), make([]byte, 1010)...)
	program := append([]byte(strings.Repeat("H", 32)), expected...)
	entries := []fixtureEntry{
		{name: "node/bin/node", contents: program, mode: 0755},
		{name: "browser/browser", contents: program, mode: 0755},
		{name: "lib/loader.so", contents: []byte("loader"), mode: 0755},
		{name: "dependencies/package.json", contents: []byte(label), mode: 0644},
		{name: "dependencies/node_modules/.bin/tool", link: "../package/tool.js", kind: tar.TypeSymlink},
		{name: "dependencies/node_modules/package/tool.js", contents: []byte("tool"), mode: 0644},
		{name: "runner/run.mjs", contents: []byte("export {}"), mode: 0644},
		{name: "fonts/font.ttf", contents: []byte("font"), mode: 0644},
	}
	a, archive := archiveFixture(t, entries)
	m := Manifest{Format: 1, Contract: 1, Version: "test-1", Platform: "linux/" + goruntime.GOARCH, Archive: a,
		Layout:   Layout{EntryPoint: "runner/run.mjs", Node: "node/bin/node", Dependencies: "dependencies", Browser: "browser/browser", Loader: "lib/loader.so", Fonts: "fonts"},
		Bindings: []Binding{{Path: "node/bin/node", Offset: 32, Expected: expected}, {Path: "browser/browser", Offset: 32, Expected: expected}},
	}
	require.NoError(t, m.Validate())
	return m, archive
}

func archiveFixture(t testing.TB, entries []fixtureEntry) (Archive, []byte) {
	t.Helper()
	var raw, compressed bytes.Buffer
	writer := tar.NewWriter(&raw)
	var expanded int64
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.contents))
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: entry.name, Typeflag: kind, Mode: entry.mode, Size: size, Linkname: entry.link}))
		if kind == tar.TypeReg {
			_, err := writer.Write(entry.contents)
			require.NoError(t, err)
			expanded += size
		}
	}
	require.NoError(t, writer.Close())
	encoder, err := zstd.NewWriter(&compressed, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	_, err = encoder.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	sum := sha256.Sum256(compressed.Bytes())
	return Archive{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(compressed.Len()), TarBytes: int64(raw.Len()), ExpandedBytes: expanded, Entries: len(entries)}, compressed.Bytes()
}

func TestBundleExtractionAndBinding(t *testing.T) {
	m, archive := bundleFixture(t, "first")
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, extractArchive(bytes.NewReader(archive), root, m.Archive))
	prefix := "/a path/it's $(printf BAD) & <private>/generation"
	require.NoError(t, bindPrefix(root, prefix, m))
	data, err := root.ReadFile(m.Layout.Node)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("H", 32), string(data[:32]))
	require.Equal(t, prefix+"/lib/loader.so", string(bytes.TrimRight(data[32:], "\x00")))
	fontData, err := root.ReadFile("fonts.conf")
	require.NoError(t, err)
	var fonts struct {
		Dir string `xml:"dir"`
	}
	require.NoError(t, xml.Unmarshal(fontData, &fonts))
	require.Equal(t, prefix+"/fonts", fonts.Dir)
	quoted, err := exec.Command("/bin/sh", "-c", "printf %s "+shellQuote(prefix)).Output()
	require.NoError(t, err)
	require.Equal(t, prefix, string(quoted))
	launcher, err := root.ReadFile("chrome")
	require.NoError(t, err)
	require.Contains(t, string(launcher), "exec "+shellQuote(prefix+"/browser/browser"))
	// Authenticated original bytes prohibit rebinding an existing generation.
	require.Error(t, bindPrefix(root, "/another/prefix", m))
}

func TestArchiveRejectsUnsafeAndMismatchedInput(t *testing.T) {
	cases := map[string][]fixtureEntry{
		"escape":           {{name: "../outside", contents: []byte("no"), mode: 0600}},
		"absolute":         {{name: "/outside", contents: []byte("no"), mode: 0600}},
		"hardlink":         {{name: "alias", link: "file", kind: tar.TypeLink}},
		"external-symlink": {{name: "alias", link: "../outside", kind: tar.TypeSymlink}},
		"chained-escape":   {{name: "d", kind: tar.TypeDir}, {name: "other", kind: tar.TypeDir}, {name: "d/alias", link: "../other", kind: tar.TypeSymlink}, {name: "escape", link: "d/alias/../../outside", kind: tar.TypeSymlink}},
		"symlink-parent":   {{name: "alias", link: "inside", kind: tar.TypeSymlink}, {name: "alias/file", contents: []byte("no"), mode: 0600}},
		"duplicate":        {{name: "file", contents: []byte("first"), mode: 0600}, {name: "file", contents: []byte("last"), mode: 0600}},
		"device":           {{name: "device", kind: tar.TypeChar, mode: 0600}},
		"setuid":           {{name: "file", contents: []byte("no"), mode: 04755}},
		"reserved":         {{name: "prepared.json", contents: []byte("{}"), mode: 0600}},
		"native reserved":  {{name: "native.json", contents: []byte("{}"), mode: 0600}},
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

func TestBindingRejectsOutOfBoundsAndWrongBytes(t *testing.T) {
	for _, kind := range []string{"long-prefix", "offset", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			m, archive := bundleFixture(t, "test")
			root, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, extractArchive(bytes.NewReader(archive), root, m.Archive))
			prefix := "/prepared/runtime"
			switch kind {
			case "long-prefix":
				prefix = "/" + strings.Repeat("a", 2048)
			case "offset":
				m.Bindings[0].Offset = 999999
			case "bytes":
				m.Bindings[0].Expected[0] = '!'
			}
			require.Error(t, bindPrefix(root, prefix, m))
		})
	}
}

func TestBindingRejectsPayloadBeyondAbsolutePathLimit(t *testing.T) {
	m, _ := bundleFixture(t, "long-prefix")
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	defer root.Close()
	for i := range m.Bindings {
		m.Bindings[i].Expected = make([]byte, 4096)
		require.NoError(t, plainParents(root, m.Bindings[i].Path))
		require.NoError(t, writeNewFile(root, m.Bindings[i].Path, append(make([]byte, 32), m.Bindings[i].Expected...), 0500))
	}
	require.NoError(t, root.Mkdir("lib", 0700))
	require.NoError(t, writeNewFile(root, m.Layout.Loader, []byte("loader"), 0500))
	require.NoError(t, root.Mkdir("fonts", 0700))
	require.NoError(t, root.Mkdir("dependencies", 0700))
	require.NoError(t, writeNewFile(root, "dependencies/a-longer-required-file.js", []byte("module.exports={}"), 0400))
	length := 4095 - len("/"+m.Layout.Loader)
	prefix := strings.Repeat("/"+strings.Repeat("p", 99), length/100) + strings.Repeat("p", length%100)
	require.Len(t, prefix+"/"+m.Layout.Loader, 4095)
	require.ErrorContains(t, bindPrefix(root, prefix, m), "absolute path limit")
}

func TestArchiveZstdIntegrityAndBounds(t *testing.T) {
	a, compressed := archiveFixture(t, []fixtureEntry{{name: "file", contents: []byte("payload"), mode: 0600}})
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
