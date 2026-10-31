// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

func adminSigningKey(t *testing.T) *openpgp.Entity {
	return pgptest.Key(t, 29)
}

func writeAdminRelease(t *testing.T, path string, manifest runtime.Manifest) {
	t.Helper()
	if manifest.Archive.URL == "" {
		manifest.Archive.URL = "https://example.invalid/runtime.tar.zst"
	}
	release := runtime.Release{Format: 1, Sequence: 1, Manifest: manifest,
		Sources: runtime.ReleaseSource{URL: "https://example.invalid/sources.tar.zst", SHA256: manifest.Archive.SHA256, Bytes: 1}}
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

func adminTrust(t *testing.T) *runtime.ReleaseTrust {
	t.Helper()
	trust, err := runtime.ParseReleaseKeyring(pgptest.Public(t, adminSigningKey(t)))
	require.NoError(t, err)
	return trust
}
