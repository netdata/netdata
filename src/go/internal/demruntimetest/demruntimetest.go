// SPDX-License-Identifier: GPL-3.0-or-later

// Package demruntimetest builds synthetic DEM browser-runtime archives without
// runtime dependencies, so the runtime store, its host and the commands that
// administer it test against one payload.
package demruntimetest

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// Entry is one archive member. Kind defaults to a regular file.
type Entry struct {
	Name     string
	Link     string
	Contents []byte
	Kind     byte
	Mode     int64
}

// Archive is a zstd-compressed tar and the bounds a runtime manifest signs.
type Archive struct {
	Data []byte
	Bounds
}

// Bounds, Layout and Binding mirror the runtime manifest types field for field,
// so consumers convert them directly. Bounds leaves the URL to the consumer.
type Bounds struct {
	URL           string
	SHA256        string
	Bytes         int64
	TarBytes      int64
	ExpandedBytes int64
	Entries       int
}

// NewArchive writes entries in order, including unsafe ones: callers test how
// extraction rejects them.
func NewArchive(t testing.TB, entries []Entry) Archive {
	t.Helper()
	var raw, compressed bytes.Buffer
	writer := tar.NewWriter(&raw)
	var expanded int64
	for _, entry := range entries {
		kind := entry.Kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.Contents))
		require.NoError(t, writer.WriteHeader(&tar.Header{
			Name:     entry.Name,
			Typeflag: kind,
			Mode:     entry.Mode,
			Size:     size,
			Linkname: entry.Link,
		}))
		if kind == tar.TypeReg {
			_, err := writer.Write(entry.Contents)
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
	return Archive{
		Data: compressed.Bytes(),
		Bounds: Bounds{
			SHA256:        hex.EncodeToString(sum[:]),
			Bytes:         int64(compressed.Len()),
			TarBytes:      int64(raw.Len()),
			ExpandedBytes: expanded,
			Entries:       len(entries),
		},
	}
}

type Layout struct {
	EntryPoint   string
	Node         string
	Dependencies string
	Browser      string
	Loader       string
	Fonts        string
}

type Binding struct {
	Path     string
	Offset   int64
	Expected []byte
}

// Bundle is a bindable runtime payload. Its executables are synthetic bytes
// that are never launched: it proves storage, binding and ownership, not
// browser readiness, so qualification rejects it.
type Bundle struct {
	Archive
	Layout   Layout
	Bindings []Binding
}

// slotOffset places the interpreter slot after a header, so binding must honor
// the authenticated offset.
const slotOffset = 32

// NewBundle builds the standard payload: nested executable and loader paths, a
// dependency tree with an internal symlink, an entrypoint and fonts. The
// entrypoint contains label, so each label has its own digest and a test can
// tell which generation a path belongs to.
func NewBundle(t testing.TB, label string) Bundle {
	t.Helper()
	slot := func() []byte { return append([]byte("/not-prepared/"), make([]byte, 1010)...) }
	program := append([]byte(strings.Repeat("H", slotOffset)), slot()...)
	archive := NewArchive(t, []Entry{
		{Name: "node/bin/node", Contents: program, Mode: 0755},
		{Name: "browser/browser", Contents: program, Mode: 0755},
		{Name: "lib/loader.so", Contents: []byte("loader"), Mode: 0755},
		{Name: "dependencies/package.json", Contents: []byte("{}"), Mode: 0644},
		{Name: "dependencies/node_modules/.bin/tool", Link: "../package/tool.js", Kind: tar.TypeSymlink},
		{Name: "dependencies/node_modules/package/tool.js", Contents: []byte("tool"), Mode: 0644},
		{Name: "runner/run.mjs", Contents: []byte(label), Mode: 0644},
		{Name: "fonts/font.ttf", Contents: []byte("font"), Mode: 0644},
	})
	return Bundle{
		Archive: archive,
		Layout: Layout{
			EntryPoint:   "runner/run.mjs",
			Node:         "node/bin/node",
			Dependencies: "dependencies",
			Browser:      "browser/browser",
			Loader:       "lib/loader.so",
			Fonts:        "fonts",
		},
		Bindings: []Binding{
			{Path: "node/bin/node", Offset: slotOffset, Expected: slot()},
			{Path: "browser/browser", Offset: slotOffset, Expected: slot()},
		},
	}
}
