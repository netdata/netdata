// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, ModeManaged, cfg.Mode)
	store, err := cfg.StoreConfig("/var/lib/netdata")
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/netdata/dem/runtime", store.Path)
	assert.Equal(t, int64(4<<30), store.MaxBytes)
	assert.Equal(t, 7*24*time.Hour, store.PreviousRetention)
}

func TestNativeRuntimeConfigurationUsesFixedPackagePaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeNative
	require.NoError(t, cfg.Validate())
	cfg.CachePath = "/alternate/proof"
	require.ErrorContains(t, cfg.Validate(), "cache_path is only valid in managed mode")
	cfg.CachePath = ""
	cfg.PreloadPath = "/alternate/payload"
	require.ErrorContains(t, cfg.Validate(), "preload_path requires mode: preloaded")
}

func TestRuntimePathsHaveNoImplicitWorkingDirectory(t *testing.T) {
	cfg := DefaultConfig()
	for _, path := range []string{"", "relative"} {
		_, err := cfg.StoreConfig(path)
		require.Error(t, err)
	}
	cfg.CachePath = "relative"
	_, err := cfg.StoreConfig("/var/lib/netdata")
	require.Error(t, err)
	cfg.CachePath = filepath.Join(t.TempDir(), "cache")
	store, err := cfg.StoreConfig("")
	require.NoError(t, err)
	assert.Equal(t, cfg.CachePath, store.Path)
}

func trustedFixture() runtime.Manifest {
	return runtime.Manifest{Format: 1, Contract: 2, Version: "test", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: strings.Repeat("a", 64), Bytes: 100, TarBytes: 10240, ExpandedBytes: 200, Entries: 2},
		Layout:   runtime.Layout{EntryPoint: "runtime.mjs", Node: "node", Dependencies: "dependencies", Browser: "browser", Loader: "loader", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: []byte("loader\x00")}, {Path: "browser", Expected: []byte("loader\x00")}}}
}
func signedSetupFixture(t *testing.T) (Config, runtime.Release, []byte) {
	t.Helper()
	manifest := trustedFixture()
	manifest.Archive.URL = "https://example.invalid/runtime.tar.zst"
	release := runtime.Release{Format: 1, Sequence: 1, Manifest: manifest,
		Sources: runtime.ReleaseSource{URL: "https://example.invalid/sources.tar.zst", SHA256: strings.Repeat("b", 64), Bytes: 1}}
	key := pgptest.Key(t, 19)
	raw, err := pgptest.Release(release, key)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "public.gpg")
	require.NoError(t, os.WriteFile(path, pgptest.Public(t, key), 0600))
	cfg := DefaultConfig()
	cfg.OfflineKeyring = path
	return cfg, release, raw
}

func TestInstalledReleaseTrust(t *testing.T) {
	cfg, release, raw := signedSetupFixture(t)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	loaded, err := cfg.ReleaseLoader(path, false)(context.Background())
	require.NoError(t, err)
	assert.Equal(t, release, loaded.Release())
	// Envelope decoding (trailing data, unknown fields, size) is the runtime package's contract.
	_, err = cfg.ReleaseLoader("relative.json", false)(context.Background())
	require.ErrorContains(t, err, "absolute")
	_, err = cfg.ReleaseLoader(t.TempDir(), false)(context.Background())
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	untrusted := cfg
	untrusted.OfflineKeyring = ""
	_, err = untrusted.ReleaseLoader(path, false)(context.Background())
	require.ErrorContains(t, err, "no runtime release verification key")
	other := pgptest.Key(t, 20)
	wrong, err := pgptest.Release(release, other)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, wrong, 0600))
	_, err = cfg.ReleaseLoader(path, false)(context.Background())
	require.ErrorContains(t, err, "not signed by a trusted key")
}

func TestInstalledReleaseRejectsOtherArchitecture(t *testing.T) {
	cfg, release, _ := signedSetupFixture(t)
	release.Manifest.Platform = "linux/arm64"
	if goruntime.GOARCH == "arm64" {
		release.Manifest.Platform = "linux/amd64"
	}
	raw, err := pgptest.Release(release, pgptest.Key(t, 19))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, err = cfg.ReleaseLoader(path, false)(context.Background())
	require.ErrorContains(t, err, "does not match this architecture")
}

func TestReleaseLoaderRejectsInvalidConfiguredKeys(t *testing.T) {
	for _, body := range []string{"not a PEM key", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n"} {
		cfg, _, raw := signedSetupFixture(t)
		path := filepath.Join(t.TempDir(), "release.json")
		require.NoError(t, os.WriteFile(path, raw, 0600))
		require.NoError(t, os.WriteFile(cfg.OfflineKeyring, []byte(body), 0600))
		_, err := cfg.ReleaseLoader(path, false)(context.Background())
		require.ErrorContains(t, err, "runtime verification key")
	}
}

type releaseSetupTransport func(*http.Request) (*http.Response, error)

func (f releaseSetupTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReleaseLoaderUsesAcquisitionConsentForTrust(t *testing.T) {
	cfg, release, raw := signedSetupFixture(t)
	cfg.ReleaseURL = "https://releases.example.invalid/latest"
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var requests []string
	fail := false
	http.DefaultTransport = releaseSetupTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		body := raw
		if r.URL.String() == runtime.ReleaseKeyringURL {
			body = pgptest.Public(t, pgptest.Key(t, 19))
		} else {
			require.Equal(t, cfg.ReleaseURL, r.URL.String())
		}
		status := http.StatusOK
		if fail {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})
	// Local descriptor plus download consent must use fresh canonical online trust,
	// ignoring the offline keyring even when that administrator file is malformed.
	require.NoError(t, os.WriteFile(cfg.OfflineKeyring, []byte("invalid"), 0600))
	got, err := cfg.ReleaseLoader(path, true)(context.Background())
	require.NoError(t, err)
	require.Equal(t, release, got.Release())
	require.Equal(t, []string{runtime.ReleaseKeyringURL}, requests)
	requests = nil
	got, err = cfg.ReleaseLoader("", true)(context.Background())
	require.NoError(t, err)
	require.Equal(t, release, got.Release())
	require.Equal(t, []string{runtime.ReleaseKeyringURL, cfg.ReleaseURL}, requests)
	requests = nil
	fail = true
	_, err = cfg.ReleaseLoader(path, true)(context.Background())
	require.ErrorContains(t, err, "503")
	require.Equal(t, []string{runtime.ReleaseKeyringURL}, requests, "failed canonical refresh must not fall back to the offline ring")
	requests = nil
	require.NoError(t, os.WriteFile(cfg.OfflineKeyring, pgptest.Public(t, pgptest.Key(t, 19)), 0600))
	got, err = cfg.ReleaseLoader(path, false)(context.Background())
	require.NoError(t, err)
	require.Equal(t, release, got.Release())
	require.Empty(t, requests, "explicit offline import must never acquire trust online")
	_, err = cfg.ReleaseLoader("", false)(context.Background())
	require.ErrorContains(t, err, "network consent")
	require.Empty(t, requests)
	cfg.OfflineKeyring = ""
	_, err = cfg.ReleaseLoader(path, false)(context.Background())
	require.ErrorContains(t, err, "offline_keyring")
	require.Empty(t, requests)
	cfg.CachePath = filepath.Join(t.TempDir(), "cache")
	handle := Open(cfg, "")
	require.Error(t, handle.SetupError)
	require.NoError(t, handle.Close())
	require.Empty(t, requests, "synchronous runtime startup must never retrieve public keys or descriptors")
}

func TestReleaseLoaderRequiresDiscoveryURLBeforeNetworkAccess(t *testing.T) {
	cfg := DefaultConfig()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = releaseSetupTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody, Header: make(http.Header)}, nil
	})
	_, err := cfg.ReleaseLoader("", true)(context.Background())
	require.ErrorContains(t, err, "runtime.release_url")
	require.Zero(t, requests, "invalid local setup must not acquire remote trust")
}
