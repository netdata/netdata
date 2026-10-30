// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"testing"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	dir      string
	manifest runtime.Manifest
	private  ed25519.PrivateKey
	args     []string
}

func newFixture(t *testing.T, arch string) fixture {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) { require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600)) }
	write("archive", []byte("local runtime archive fixture"))
	write("sources", []byte("local source archive fixture"))
	hash, size, err := hashFile(filepath.Join(dir, "archive"))
	require.NoError(t, err)
	m := runtime.Manifest{
		Format:   1,
		Contract: 1,
		Version:  "release-1",
		Platform: "linux/" + arch,
		Archive: runtime.Archive{
			SHA256:        hash,
			Bytes:         size,
			TarBytes:      4096,
			ExpandedBytes: 100,
			Entries:       8,
		},
		Layout: runtime.Layout{
			EntryPoint:   "runner/run.mjs",
			Node:         "bin/node",
			Dependencies: "dependencies",
			Browser:      "bin/chromium",
			Loader:       "lib/loader",
			Fonts:        "fonts",
		},
		Bindings: []runtime.Binding{{Path: "bin/node", Offset: 42, Expected: []byte("loader\x00")}, {Path: "bin/chromium", Offset: 84, Expected: []byte("loader\x00")}},
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)
	write("manifest.json", data)
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	write("private.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}))
	der, err = x509.MarshalPKIXPublicKey(private.Public())
	require.NoError(t, err)
	write("public.pem", pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}))
	return fixture{
		dir:      dir,
		manifest: m,
		private:  private,
		args:     []string{"sign", "--key", filepath.Join(dir, "private.pem"), "--manifest", filepath.Join(dir, "manifest.json"), "--archive", filepath.Join(dir, "archive"), "--sources", filepath.Join(dir, "sources"), "--archive-url", "https://example.invalid/runtime.tar.gz", "--sources-url", "https://example.invalid/sources.tar.gz", "--sequence", "17", "--output", filepath.Join(dir, "release.json")},
	}
}

func (f fixture) verifyArgs() []string {
	return []string{"verify", "--key", filepath.Join(f.dir, "public.pem"), "--release", filepath.Join(f.dir, "release.json"), "--archive", filepath.Join(f.dir, "archive"), "--sources", filepath.Join(f.dir, "sources")}
}

func TestSignVerifyBothArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			f := newFixture(t, arch)
			require.NoError(t, run(f.args, io.Discard))
			require.NoError(t, run(f.verifyArgs(), io.Discard))
			data, err := os.ReadFile(filepath.Join(f.dir, "release.json"))
			require.NoError(t, err)
			release, err := runtime.VerifyRelease(data, []ed25519.PublicKey{f.private.Public().(ed25519.PublicKey)})
			require.NoError(t, err)
			expected := f.manifest
			expected.Archive.URL = "https://example.invalid/runtime.tar.gz"
			assert.Equal(t, expected, release.Manifest)
			assert.Equal(t, uint64(17), release.Sequence)
			hash, size, err := hashFile(filepath.Join(f.dir, "sources"))
			require.NoError(t, err)
			assert.Equal(t, hash, release.Sources.SHA256)
			assert.Equal(t, size, release.Sources.Bytes)
			assert.ErrorContains(t, run(f.args, io.Discard), "existing outputs are never replaced")
			after, err := os.ReadFile(filepath.Join(f.dir, "release.json"))
			require.NoError(t, err)
			assert.Equal(t, data, after)
		})
	}
}

func TestSignRejectsInvalidInputs(t *testing.T) {
	for _, kind := range []string{"digest", "size", "unknown", "trailing", "oversize", "url", "sequence", "key", "empty-sources"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, "amd64")
			switch kind {
			case "digest":
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, "archive"), bytes.Repeat([]byte("x"), int(f.manifest.Archive.Bytes)), 0600))
			case "size":
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, "archive"), []byte("short"), 0600))
			case "unknown", "trailing", "oversize":
				p := filepath.Join(f.dir, "manifest.json")
				data, err := os.ReadFile(p)
				require.NoError(t, err)
				if kind == "unknown" {
					data = append([]byte(`{"unexpected":1,`), data[1:]...)
				}
				if kind == "trailing" {
					data = append(data, []byte(` {}`)...)
				}
				if kind == "oversize" {
					data = bytes.Repeat([]byte(" "), runtime.ReleaseEnvelopeLimit+1)
				}
				require.NoError(t, os.WriteFile(p, data, 0600))
			case "url":
				f.args = append(f.args, "--archive-url", "http://example.invalid/archive")
			case "sequence":
				f.args = append(f.args, "--sequence", "0")
			case "key":
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, "private.pem"), []byte("PRIVATE CONTENT MUST NOT APPEAR IN ERROR"), 0600))
			case "empty-sources":
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, "sources"), nil, 0600))
			}
			err := run(f.args, io.Discard)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "PRIVATE CONTENT")
			_, err = os.Stat(filepath.Join(f.dir, "release.json"))
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestVerifyRequiresAuthenticationAndArtifactIntegrity(t *testing.T) {
	for _, kind := range []string{"wrong-key", "tampered", "archive", "sources"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, "arm64")
			require.NoError(t, run(f.args, io.Discard))
			switch kind {
			case "wrong-key":
				key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, ed25519.SeedSize))
				der, err := x509.MarshalPKIXPublicKey(key.Public())
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, "public.pem"), pem.EncodeToMemory(&pem.Block{
					Type:  "PUBLIC KEY",
					Bytes: der,
				}), 0600))
			case "tampered":
				p := filepath.Join(f.dir, "release.json")
				data, err := os.ReadFile(p)
				require.NoError(t, err)
				var envelope map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &envelope))
				envelope["signature"] = json.RawMessage(`"AAAA"`)
				data, err = json.Marshal(envelope)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(p, data, 0600))
			default:
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, kind), []byte("changed"), 0600))
			}
			require.Error(t, run(f.verifyArgs(), io.Discard))
		})
	}
}

func TestCommandErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"sign"}, {"verify"}, {"sign", "--sequence", "-1"}, {"verify", "extra"}, {"sign", "--unknown"}} {
		assert.Error(t, run(args, io.Discard))
	}
}

func TestStageNativeUsesBuilderManifestWithoutSigningKey(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			f := newFixture(t, arch)
			slot := make([]byte, 128)
			copy(slot, "/unprepared/loader")
			for i := range f.manifest.Bindings {
				f.manifest.Bindings[i].Offset = 0
				f.manifest.Bindings[i].Expected = slot
			}
			entries := []struct {
				name string
				data []byte
				mode int64
			}{
				{f.manifest.Layout.Node, slot, 0755}, {f.manifest.Layout.Browser, slot, 0755},
				{f.manifest.Layout.Loader, []byte("loader"), 0755}, {f.manifest.Layout.EntryPoint, []byte("export {}"), 0644},
				{f.manifest.Layout.Dependencies + "/package.json", []byte("{}"), 0644}, {f.manifest.Layout.Fonts + "/font.ttf", []byte("font"), 0644},
			}
			var tarData, compressed bytes.Buffer
			tw := tar.NewWriter(&tarData)
			var expanded int64
			for _, entry := range entries {
				require.NoError(t, tw.WriteHeader(&tar.Header{
					Name:     entry.name,
					Mode:     entry.mode,
					Size:     int64(len(entry.data)),
					Typeflag: tar.TypeReg,
				}))
				_, err := tw.Write(entry.data)
				require.NoError(t, err)
				expanded += int64(len(entry.data))
			}
			require.NoError(t, tw.Close())
			gz := gzip.NewWriter(&compressed)
			_, err := gz.Write(tarData.Bytes())
			require.NoError(t, err)
			require.NoError(t, gz.Close())
			archive := filepath.Join(f.dir, "archive")
			require.NoError(t, os.WriteFile(archive, compressed.Bytes(), 0600))
			hash, size, err := hashFile(archive)
			require.NoError(t, err)
			f.manifest.Archive = runtime.Archive{
				SHA256:        hash,
				Bytes:         size,
				TarBytes:      int64(tarData.Len()),
				ExpandedBytes: expanded,
				Entries:       len(entries),
			}
			raw, err := json.Marshal(f.manifest)
			require.NoError(t, err)
			manifestPath := filepath.Join(f.dir, "manifest.json")
			require.NoError(t, os.WriteFile(manifestPath, raw, 0600))
			output := filepath.Join(f.dir, "payload")
			args := []string{"stage-native", "--manifest", manifestPath, "--archive", archive, "--output", output}
			require.NoError(t, run(args, io.Discard))
			raw, err = os.ReadFile(filepath.Join(output, "native.json"))
			require.NoError(t, err)
			var record runtime.NativePackage
			require.NoError(t, json.Unmarshal(raw, &record))
			assert.Equal(t, f.manifest, record.Manifest)
			assert.Equal(t, runtime.DefaultNativeConfig().PayloadPath, record.Path)
			require.ErrorContains(t, run(args, io.Discard), "must be empty")
		})
	}
}

func TestStageNativeRejectsInvalidInputs(t *testing.T) {
	for _, args := range [][]string{{"stage-native"}, {"stage-native", "extra"}, {"stage-native", "--key", "key.pem"}} {
		require.Error(t, run(args, io.Discard))
	}
	f := newFixture(t, "amd64")
	output := filepath.Join(f.dir, "payload")
	// This signed-tool fixture has a valid manifest and matching bytes but no tar
	// structure: staging must reject it through the shared production extractor.
	err := run([]string{"stage-native", "--manifest", filepath.Join(f.dir, "manifest.json"), "--archive", filepath.Join(f.dir, "archive"), "--output", output}, io.Discard)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(output, "native.json"))
}
