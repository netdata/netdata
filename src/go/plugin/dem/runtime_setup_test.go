// SPDX-License-Identifier: GPL-3.0-or-later

package dem

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

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
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
	manifest.Archive.URL = "https://example.invalid/runtime.tar.zst"
	release := runtime.Release{Format: 1, Sequence: 1, Manifest: manifest,
		Sources: runtime.ReleaseSource{URL: "https://example.invalid/sources.tar.zst", SHA256: strings.Repeat("b", 64), Bytes: 1}}
	key := pgptest.Key(t, 19)
	raw, err := pgptest.Release(release, key)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "public.gpg")
	require.NoError(t, os.WriteFile(path, pgptest.Public(t, key), 0600))
	cfg := DefaultRuntimeConfig()
	cfg.OfflineKeyring = path
	return cfg, release, raw
}

func TestInstalledReleaseTrust(t *testing.T) {
	cfg, release, raw := signedSetupFixture(t)
	path := filepath.Join(t.TempDir(), "release.json")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	loaded, err := cfg.ReleaseLoader(path, false)(context.Background())
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
			_, err := cfg.ReleaseLoader(path, false)(context.Background())
			require.Error(t, err)
		})
	}
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
	require.Equal(t, release, got)
	require.Equal(t, []string{runtime.ReleaseKeyringURL}, requests)
	requests = nil
	got, err = cfg.ReleaseLoader("", true)(context.Background())
	require.NoError(t, err)
	require.Equal(t, release, got)
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
	require.Equal(t, release, got)
	require.Empty(t, requests, "explicit offline import must never acquire trust online")
	_, err = cfg.ReleaseLoader("", false)(context.Background())
	require.ErrorContains(t, err, "network consent")
	require.Empty(t, requests)
	cfg.OfflineKeyring = ""
	_, err = cfg.ReleaseLoader(path, false)(context.Background())
	require.ErrorContains(t, err, "offline_keyring")
	require.Empty(t, requests)
	cfg.CachePath = filepath.Join(t.TempDir(), "cache")
	handle := OpenRuntime(cfg, "")
	require.Error(t, handle.SetupError)
	require.NoError(t, handle.Close())
	require.Empty(t, requests, "synchronous runtime startup must never retrieve public keys or descriptors")
}
