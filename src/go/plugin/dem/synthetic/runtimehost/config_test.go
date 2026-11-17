// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
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

func trustedFixture() browserruntime.Manifest {
	return browserruntime.Manifest{
		Format:   1,
		Contract: 2,
		Version:  "test",
		Platform: "linux/" + runtime.GOARCH,
		Archive: browserruntime.Archive{
			SHA256:        strings.Repeat("a", 64),
			Bytes:         100,
			TarBytes:      10240,
			ExpandedBytes: 200,
			Entries:       2,
		},
		Layout: browserruntime.Layout{
			EntryPoint:   "runtime.mjs",
			Node:         "node",
			Dependencies: "dependencies",
			Browser:      "browser",
			Loader:       "loader",
			Fonts:        "fonts",
		},
		Bindings: []browserruntime.Binding{
			{Path: "node", Expected: []byte("loader\x00")},
			{Path: "browser", Expected: []byte("loader\x00")},
		},
	}
}

func fixtureAgent(version string) browserruntime.AgentRelease {
	return browserruntime.AgentRelease{Version: version, Channel: "devel", Source: strings.Repeat("a", 40)}
}

func useFixtureAgent(t *testing.T, version string) browserruntime.AgentRelease {
	t.Helper()
	previous := AgentRelease()
	agent := fixtureAgent(version)
	buildinfo.ReleaseVersion, buildinfo.ReleaseChannel, buildinfo.ReleaseRef = agent.Version, agent.Channel, agent.Source
	t.Cleanup(func() {
		buildinfo.ReleaseVersion, buildinfo.ReleaseChannel, buildinfo.ReleaseRef = previous.Version, previous.Channel, previous.Source
	})
	return agent
}

func signedSetupFixture(t *testing.T) (Config, browserruntime.Release, []byte) {
	t.Helper()
	manifest := trustedFixture()
	agent := useFixtureAgent(t, manifest.Version)
	manifest.Archive.URL = "https://example.invalid/runtime.tar.zst"
	release := browserruntime.Release{
		Format:   2,
		Agent:    &agent,
		Manifest: manifest,
		Sources: browserruntime.ReleaseSource{
			URL:    "https://example.invalid/sources.tar.zst",
			SHA256: strings.Repeat("b", 64),
			Bytes:  1,
		},
	}
	key := pgptest.Key(t, 19)
	raw, err := pgptest.Release(release, key)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "public.gpg")
	require.NoError(t, os.WriteFile(path, pgptest.Public(t, key), 0600))
	cfg := DefaultConfig()
	cfg.OfflineKeyring = path
	return cfg, release, raw
}

func TestDefaultRuntimeURLNamesExactAgentAndArchitecture(t *testing.T) {
	for _, channel := range []string{"stable", "nightly"} {
		for _, arch := range []string{"amd64", "arm64"} {
			agent := fixtureAgent("v2.12.0")
			agent.Channel = channel
			if channel == "nightly" {
				agent.Version = "v2.12.0-9-nightly"
			}
			assert.Equal(t, "https://artifacts.netdata.cloud/dem-runtime/"+channel+"/"+agent.Version+"/"+arch+"/release.json",
				defaultReleaseURL(agent, arch))
		}
	}
	assert.Empty(t, defaultReleaseURL(browserruntime.AgentRelease{}, "amd64"), "developer builds must not guess a published release")
	assert.Empty(t, defaultReleaseURL(fixtureAgent("v2.12.0"), "386"))
}

func TestReleaseLoaderRejectsAuthenticatedWrongAgentPair(t *testing.T) {
	for _, field := range []string{"version", "channel", "source"} {
		t.Run(field, func(t *testing.T) {
			cfg, release, _ := signedSetupFixture(t)
			switch field {
			case "version":
				release.Agent.Version, release.Manifest.Version = "another-Agent", "another-Agent"
			case "channel":
				release.Agent.Channel = "stable"
			case "source":
				release.Agent.Source = strings.Repeat("b", 40)
			}
			raw, err := pgptest.Release(release, pgptest.Key(t, 19))
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "release.json")
			require.NoError(t, os.WriteFile(path, raw, 0600))
			_, err = cfg.ReleaseLoader(path, false)(context.Background())
			require.ErrorIs(t, err, browserruntime.ErrIncompatible)
		})
	}
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
	if runtime.GOARCH == "arm64" {
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
	const releaseURL = "https://releases.example.invalid/latest"
	tests := map[string]struct {
		localDescriptor bool
		allowDownload   bool
		keyring         string // offline keyring: "valid", "invalid" or "none"
		unavailable     bool   // the network answers 503
		wantRequests    []string
		wantErr         string
	}{
		// Download consent uses fresh canonical online trust and ignores the
		// offline keyring, even when that administrator file is malformed.
		"local descriptor with download consent": {
			localDescriptor: true,
			allowDownload:   true,
			keyring:         "invalid",
			wantRequests:    []string{browserruntime.ReleaseKeyringURL},
		},
		"latest descriptor with download consent": {
			allowDownload: true,
			keyring:       "invalid",
			wantRequests:  []string{browserruntime.ReleaseKeyringURL, releaseURL},
		},
		"failed canonical refresh does not fall back to the offline keyring": {
			localDescriptor: true,
			allowDownload:   true,
			keyring:         "valid",
			unavailable:     true,
			wantRequests:    []string{browserruntime.ReleaseKeyringURL},
			wantErr:         "503",
		},
		"offline import never acquires trust online": {
			localDescriptor: true,
			keyring:         "valid",
		},
		"latest descriptor without download consent": {
			keyring: "valid",
			wantErr: "network consent",
		},
		"offline import without a keyring": {
			localDescriptor: true,
			keyring:         "none",
			wantErr:         "offline_keyring",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, release, raw := signedSetupFixture(t)
			cfg.ReleaseURL = releaseURL
			switch tc.keyring {
			case "invalid":
				require.NoError(t, os.WriteFile(cfg.OfflineKeyring, []byte("invalid"), 0600))
			case "none":
				cfg.OfflineKeyring = ""
			}
			path := ""
			if tc.localDescriptor {
				path = filepath.Join(t.TempDir(), "release.json")
				require.NoError(t, os.WriteFile(path, raw, 0600))
			}
			requests := serveReleaseSetup(t, cfg, raw, tc.unavailable)
			got, err := cfg.ReleaseLoader(path, tc.allowDownload)(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, release, got.Release())
			}
			assert.Equal(t, tc.wantRequests, *requests)
		})
	}
}

func TestRuntimeStartupNeverRetrievesTrust(t *testing.T) {
	cfg, _, raw := signedSetupFixture(t)
	cfg.ReleaseURL = "https://releases.example.invalid/latest"
	cfg.CachePath = filepath.Join(t.TempDir(), "cache")
	requests := serveReleaseSetup(t, cfg, raw, false)
	handle := Open(cfg, "")
	require.Error(t, handle.SetupError)
	require.NoError(t, handle.Close())
	assert.Empty(t, *requests, "synchronous runtime startup must never retrieve public keys or descriptors")
}

// serveReleaseSetup answers the canonical keyring and cfg.ReleaseURL in place of
// the network and records the requested URLs.
func serveReleaseSetup(t *testing.T, cfg Config, descriptor []byte, unavailable bool) *[]string {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var requests []string
	http.DefaultTransport = releaseSetupTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		body := descriptor
		if r.URL.String() == browserruntime.ReleaseKeyringURL {
			body = pgptest.Public(t, pgptest.Key(t, 19))
		} else {
			require.Equal(t, cfg.ReleaseURL, r.URL.String())
		}
		status := http.StatusOK
		if unavailable {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	return &requests
}

func TestReleaseLoaderRequiresDiscoveryURLBeforeNetworkAccess(t *testing.T) {
	cfg := DefaultConfig()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	requests := 0
	http.DefaultTransport = releaseSetupTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       http.NoBody,
			Header:     make(http.Header),
		}, nil
	})
	_, err := cfg.ReleaseLoader("", true)(context.Background())
	require.ErrorContains(t, err, "runtime.release_url")
	require.Zero(t, requests, "invalid local setup must not acquire remote trust")
}
