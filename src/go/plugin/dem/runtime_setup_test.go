// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trustedFixture() runtime.Manifest {
	return runtime.Manifest{Format: 1, Contract: 1, Version: "test", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: strings.Repeat("a", 64), Bytes: 100, TarBytes: 10240, ExpandedBytes: 200, Entries: 2},
		Layout:   runtime.Layout{EntryPoint: "runtime.mjs", Node: "node", Dependencies: "dependencies", Browser: "browser", Loader: "loader", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: []byte("loader\x00")}, {Path: "browser", Expected: []byte("loader\x00")}}}
}
func signedSetupFixture(t *testing.T) (RuntimeConfig, runtime.Release, []byte) {
	t.Helper()
	manifest := trustedFixture()
	manifest.Archive.URL = "https://example.invalid/runtime.tar.gz"
	release := runtime.Release{Format: 1, Sequence: 1, Manifest: manifest,
		Sources: runtime.ReleaseSource{URL: "https://example.invalid/sources.tar.gz", SHA256: strings.Repeat("b", 64), Bytes: 1}}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{19}, ed25519.SeedSize))
	raw, err := runtime.SignRelease(release, key)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(key.Public())
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "public.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0600))
	cfg := DefaultRuntimeConfig()
	cfg.VerificationKeys = []string{path}
	return cfg, release, raw
}

func TestInstalledReleaseTrust(t *testing.T) {
	cfg, release, raw := signedSetupFixture(t)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	loaded, err := cfg.ReleaseLoader(path)(context.Background())
	require.NoError(t, err)
	assert.Equal(t, release, loaded)
	for name, body := range map[string][]byte{
		"trailing JSON":    append(append([]byte{}, raw...), []byte(" {}")...),
		"unknown field":    []byte(strings.Replace(string(raw), `"format":1`, `"format":1,"download_override":"https://untrusted.invalid"`, 1)),
		"invalid envelope": []byte(`{"format":1}`),
		"oversized":        bytes.Repeat([]byte(" "), runtime.ReleaseEnvelopeLimit+1),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, body, 0600))
			_, err := cfg.ReleaseLoader(path)(context.Background())
			require.Error(t, err)
		})
	}
	_, err = cfg.ReleaseLoader("relative.json")(context.Background())
	require.ErrorContains(t, err, "absolute")
	_, err = cfg.ReleaseLoader(t.TempDir())(context.Background())
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	untrusted := cfg
	untrusted.VerificationKeys = nil
	_, err = untrusted.ReleaseLoader(path)(context.Background())
	require.ErrorContains(t, err, "no runtime release verification key")
	other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{20}, ed25519.SeedSize))
	wrong, err := runtime.SignRelease(release, other)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, wrong, 0600))
	_, err = cfg.ReleaseLoader(path)(context.Background())
	require.ErrorContains(t, err, "not signed by a trusted key")
}

func TestInstalledReleaseRejectsOtherArchitecture(t *testing.T) {
	cfg, release, _ := signedSetupFixture(t)
	release.Manifest.Platform = "linux/arm64"
	if goruntime.GOARCH == "arm64" {
		release.Manifest.Platform = "linux/amd64"
	}
	raw, err := runtime.SignRelease(release, ed25519.NewKeyFromSeed(bytes.Repeat([]byte{19}, ed25519.SeedSize)))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, err = cfg.ReleaseLoader(path)(context.Background())
	require.ErrorContains(t, err, "does not match this architecture")
}

func TestMissingInstallationLeavesRUMSetupAvailable(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cache := filepath.Join(t.TempDir(), "cache")
	cfg.CachePath = cache
	handle := OpenRuntime(cfg, "")
	require.Error(t, handle.SetupError)
	require.NotNil(t, handle.Source)
	require.NotNil(t, handle.Host)
	assert.Contains(t, []string{"unavailable", "missing"}, handle.Host.Status().State)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	require.NoError(t, handle.Close())
}
func TestPreloadedRuntimeRequiresProtectedInstallation(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	cfg.Mode = "preloaded"
	cfg.PreloadPath = filepath.Join(t.TempDir(), "not-created")
	handle := OpenRuntime(cfg, "")
	require.Error(t, handle.SetupError)
	require.Nil(t, handle.Host)
	require.Nil(t, handle.Store)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	_, err = os.Stat(cfg.PreloadPath)
	require.True(t, os.IsNotExist(err))
}

func TestReleaseLoaderRejectsInvalidConfiguredKeys(t *testing.T) {
	for _, body := range []string{"not a PEM key", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n"} {
		cfg, _, raw := signedSetupFixture(t)
		path := filepath.Join(t.TempDir(), "release.json")
		require.NoError(t, os.WriteFile(path, raw, 0600))
		require.NoError(t, os.WriteFile(cfg.VerificationKeys[0], []byte(body), 0600))
		_, err := cfg.ReleaseLoader(path)(context.Background())
		require.ErrorContains(t, err, "runtime verification key")
	}
}
