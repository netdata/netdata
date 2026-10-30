// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareNativePackageBothArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			m, data := bundleFixture(t, "package")
			m.Platform = "linux/" + arch
			archive := filepath.Join(t.TempDir(), "archive.tar.zst")
			require.NoError(t, os.WriteFile(archive, data, 0600))
			output := filepath.Join(t.TempDir(), "payload")
			require.NoError(t, PrepareNativePackage(context.Background(), m, archive, output))
			root, err := os.OpenRoot(output)
			require.NoError(t, err)
			defer root.Close()
			var record NativePackage
			raw, err := root.ReadFile("native.json")
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &record))
			assert.Equal(t, NativePackage{
				Format:   1,
				Path:     DefaultNativeConfig().PayloadPath,
				Manifest: m,
			}, record)
			for _, binding := range m.Bindings {
				data, err := root.ReadFile(binding.Path)
				require.NoError(t, err)
				expected := make([]byte, len(binding.Expected))
				copy(expected, filepath.Join(record.Path, m.Layout.Loader))
				assert.Equal(t, expected, data[binding.Offset:binding.Offset+int64(len(binding.Expected))])
			}
			fonts, launcher, err := boundConfiguration(record.Path, m.Layout)
			require.NoError(t, err)
			for name, expected := range map[string][]byte{"fonts.conf": fonts, "chrome": launcher} {
				actual, err := root.ReadFile(name)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
				assert.NotContains(t, string(actual), output)
			}
			link, err := root.Readlink("dependencies/node_modules/.bin/tool")
			require.NoError(t, err)
			assert.Equal(t, "../package/tool.js", link)
			require.NoError(t, fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
				require.NoError(t, err)
				info, err := entry.Info()
				require.NoError(t, err)
				if info.Mode()&os.ModeSymlink != 0 {
					return nil
				}
				expected := os.FileMode(0644)
				if info.IsDir() || info.Mode().Perm()&0111 != 0 {
					expected = 0755
				}
				assert.Equal(t, expected, info.Mode().Perm(), name)
				return nil
			}))
			for _, name := range []string{"prepared.json", "generations", "claims", "admission.json", "selection.json"} {
				assert.NoFileExists(t, filepath.Join(output, name))
			}
			require.ErrorContains(t, PrepareNativePackage(context.Background(), m, archive, output), "must be empty")
			preserved, err := root.ReadFile("native.json")
			require.NoError(t, err)
			assert.Equal(t, raw, preserved)
		})
	}
}

func TestPrepareNativePackageFailuresNeverComplete(t *testing.T) {
	for _, fault := range []string{"digest", "short", "long", "binding", "bounds", "native metadata"} {
		t.Run(fault, func(t *testing.T) {
			m, data := bundleFixture(t, "failure")
			switch fault {
			case "digest":
				m.Archive.SHA256 = strings.Repeat("b", 64)
			case "short":
				data = data[:len(data)-1]
			case "long":
				data = append(data, 0)
			case "binding":
				m.Bindings[0].Expected[0] ^= 1
			case "bounds":
				m.Archive.Entries++
			case "native metadata":
				m.Archive, data = archiveFixture(t, []fixtureEntry{{name: "native.json", contents: []byte(`{"format":1}`), mode: 0644}})
			}
			archive := filepath.Join(t.TempDir(), "archive")
			require.NoError(t, os.WriteFile(archive, data, 0600))
			output := filepath.Join(t.TempDir(), "payload")
			require.Error(t, PrepareNativePackage(context.Background(), m, archive, output))
			assert.NoFileExists(t, filepath.Join(output, "native.json"))
			if fault == "digest" || fault == "short" || fault == "long" {
				entries, err := os.ReadDir(output)
				require.NoError(t, err)
				assert.Empty(t, entries, "compressed bytes must be authenticated before extraction")
			}
		})
	}
}

func TestPrepareNativePackageStagingIsolation(t *testing.T) {
	m, data := bundleFixture(t, "isolation")
	archive := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(archive, data, 0600))
	for _, output := range []string{"relative", DefaultNativeConfig().PayloadPath, DefaultNativeConfig().ControlPath} {
		require.Error(t, PrepareNativePackage(context.Background(), m, archive, output))
	}
	existing := t.TempDir()
	sentinel := filepath.Join(existing, "keep")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	require.ErrorContains(t, PrepareNativePackage(context.Background(), m, archive, existing), "must be empty")
	unchanged, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, []byte("preserve"), unchanged)
	empty := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(empty, link))
	require.ErrorContains(t, PrepareNativePackage(context.Background(), m, archive, link), "plain directory")
	entries, err := os.ReadDir(empty)
	require.NoError(t, err)
	assert.Empty(t, entries)
	require.NoError(t, PrepareNativePackage(context.Background(), m, archive, empty))
}

func TestPrepareNativePackageRejectsNativeLayoutThroughSymlink(t *testing.T) {
	m, _ := bundleFixture(t, "layout")
	// Managed extraction permits internal links, but native admission requires
	// each declared executable/data layout path to have plain directory parents.
	m.Layout.Loader = "alias/loader.so"
	program := append(make([]byte, 32), m.Bindings[0].Expected...)
	entries := []fixtureEntry{
		{name: m.Layout.Node, contents: program, mode: 0755},
		{name: m.Layout.Browser, contents: program, mode: 0755},
		{name: "lib/loader.so", contents: []byte("loader"), mode: 0755},
		{name: "alias", link: "lib", kind: tar.TypeSymlink},
		{name: m.Layout.EntryPoint, contents: []byte("export {}"), mode: 0644},
		{name: "dependencies/package.json", contents: []byte("{}"), mode: 0644},
		{name: "fonts/font.ttf", contents: []byte("font"), mode: 0644},
	}
	var data []byte
	m.Archive, data = archiveFixture(t, entries)
	archive := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(archive, data, 0600))
	output := filepath.Join(t.TempDir(), "payload")
	require.ErrorContains(t, PrepareNativePackage(context.Background(), m, archive, output), "non-directory parent")
	assert.NoFileExists(t, filepath.Join(output, "native.json"))
}
