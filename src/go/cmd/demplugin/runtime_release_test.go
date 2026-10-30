// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

func adminSigningKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{29}, ed25519.SeedSize))
}

func writeAdminRelease(t *testing.T, path string, manifest runtime.Manifest) {
	t.Helper()
	if manifest.Archive.URL == "" {
		manifest.Archive.URL = "https://example.invalid/runtime.tar.gz"
	}
	release := runtime.Release{Format: 1, Sequence: 1, Manifest: manifest,
		Sources: runtime.ReleaseSource{URL: "https://example.invalid/sources.tar.gz", SHA256: manifest.Archive.SHA256, Bytes: 1}}
	data, err := runtime.SignRelease(release, adminSigningKey())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func writeAdminTrust(t *testing.T, root, cache string) {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(adminSigningKey().Public())
	require.NoError(t, err)
	keyPath := filepath.Join(root, "release-public.pem")
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0600))
	writeRuntimeTrustConfig(t, root, cache, keyPath)
}

func writeRuntimeTrustConfig(t *testing.T, root, cache, keyPath string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "dem.conf"),
		[]byte(fmt.Sprintf("runtime:\n  cache_path: %q\n  verification_keys: [%q]\n", cache, keyPath)), 0600))
}
