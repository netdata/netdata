// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/netdata/netdata/go/plugins/internal/demruntimetest"
	"github.com/stretchr/testify/require"
)

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

func TestBoundPayloadRequiresExecuteBitOfRunningClass(t *testing.T) {
	tests := map[string]struct {
		execute os.FileMode
		base    os.FileMode
		mode    os.FileMode
		valid   bool
	}{
		"owner class accepts owner execute": {
			execute: ownerExecute,
			base:    0500,
			mode:    0500,
			valid:   true,
		},
		"owner class rejects group and other execute only": {
			execute: ownerExecute,
			base:    0500,
			mode:    0455,
		},
		"non-owner class accepts other execute": {
			execute: nonOwnerExecute,
			base:    0755,
			mode:    0705,
			valid:   true,
		},
		"non-owner class accepts group execute": {
			execute: nonOwnerExecute,
			base:    0755,
			mode:    0750,
			valid:   true,
		},
		"non-owner class rejects owner execute only": {
			execute: nonOwnerExecute,
			base:    0755,
			mode:    0744,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root, m := boundPayloadFixture(t)
			executables := []string{m.Layout.Node, m.Layout.Browser, m.Layout.Loader, "chrome"}
			for _, executable := range executables {
				require.NoError(t, root.Chmod(executable, test.base))
			}
			require.NoError(t, verifyBoundPayload(root, m, test.execute))
			for _, executable := range executables {
				require.NoError(t, root.Chmod(executable, test.mode))
				err := verifyBoundPayload(root, m, test.execute)
				if test.valid {
					require.NoError(t, err, executable)
				} else {
					var layout payloadLayoutError
					require.ErrorAs(t, err, &layout, executable)
				}
				require.NoError(t, root.Chmod(executable, test.base))
			}
		})
	}
}

func TestBoundPayloadStructuralDamageIsDamage(t *testing.T) {
	tests := map[string]func(t *testing.T, root *os.Root, m Manifest){
		"missing launcher": func(t *testing.T, root *os.Root, _ Manifest) {
			require.NoError(t, root.Remove("chrome"))
		},
		"missing font configuration": func(t *testing.T, root *os.Root, _ Manifest) {
			require.NoError(t, root.Remove("fonts.conf"))
		},
		"entrypoint is a directory": func(t *testing.T, root *os.Root, m Manifest) {
			require.NoError(t, root.Remove(m.Layout.EntryPoint))
			require.NoError(t, root.Mkdir(m.Layout.EntryPoint, 0700))
		},
		"fonts path is a file": func(t *testing.T, root *os.Root, m Manifest) {
			require.NoError(t, root.RemoveAll(m.Layout.Fonts))
			require.NoError(t, writeNewFile(root, m.Layout.Fonts, nil, 0400))
		},
		"linked executable parent": func(t *testing.T, root *os.Root, m Manifest) {
			parent := filepath.Dir(m.Layout.Loader)
			require.NoError(t, root.Rename(parent, parent+"-target"))
			require.NoError(t, root.Symlink(parent+"-target", parent))
		},
	}
	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			root, m := boundPayloadFixture(t)
			damage(t, root, m)
			err := verifyBoundPayload(root, m, ownerExecute)
			require.Error(t, err)
			require.ErrorIs(t, selectedPayloadError(err), ErrDamaged)
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
