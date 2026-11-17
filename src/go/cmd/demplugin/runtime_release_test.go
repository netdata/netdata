// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/stretchr/testify/require"
)

func adminSigningKey(t *testing.T) *openpgp.Entity {
	return pgptest.Key(t, 29)
}

func writeAdminRelease(t *testing.T, path string, manifest browserruntime.Manifest) {
	t.Helper()
	writeAdminPairedRelease(t, path, manifest, fixtureAgent(manifest.Version))
}

func fixtureAgent(version string) browserruntime.AgentRelease {
	return browserruntime.AgentRelease{Version: version, Channel: "stable", Source: strings.Repeat("a", 40)}
}

// These tests run serially; each simulated installed helper restores its build
// metadata before the next test or parent scope continues.
func setRuntimeAgent(t *testing.T, agent browserruntime.AgentRelease) {
	t.Helper()
	version, channel, source := buildinfo.ReleaseVersion, buildinfo.ReleaseChannel, buildinfo.ReleaseRef
	t.Cleanup(func() {
		buildinfo.ReleaseVersion, buildinfo.ReleaseChannel, buildinfo.ReleaseRef = version, channel, source
	})
	buildinfo.ReleaseVersion, buildinfo.ReleaseChannel, buildinfo.ReleaseRef = agent.Version, agent.Channel, agent.Source
}

func writeAdminPairedRelease(t *testing.T, path string, manifest browserruntime.Manifest, agent browserruntime.AgentRelease) {
	t.Helper()
	if manifest.Archive.URL == "" {
		manifest.Archive.URL = "https://example.invalid/runtime.tar.zst"
	}
	release := browserruntime.Release{
		Format:   2,
		Agent:    &agent,
		Manifest: manifest,
		Sources: browserruntime.ReleaseSource{
			URL:    "https://example.invalid/sources.tar.zst",
			SHA256: manifest.Archive.SHA256,
			Bytes:  1,
		},
	}
	data, err := pgptest.Release(release, adminSigningKey(t))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func writeAdminTrust(t *testing.T, root, cache string) {
	t.Helper()
	keyPath := filepath.Join(root, "release-public.gpg")
	require.NoError(t, os.WriteFile(keyPath, pgptest.Public(t, adminSigningKey(t)), 0600))
	writeRuntimeTrustConfig(t, root, cache, keyPath)
}

func writeRuntimeTrustConfig(t *testing.T, root, cache, keyPath string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "dem.conf"),
		[]byte(fmt.Sprintf("runtime:\n  cache_path: %q\n  offline_keyring: %q\n", cache, keyPath)), 0600))
}

func adminTrust(t *testing.T) *browserruntime.ReleaseTrust {
	t.Helper()
	trust, err := browserruntime.ParseReleaseKeyring(pgptest.Public(t, adminSigningKey(t)))
	require.NoError(t, err)
	return trust
}
