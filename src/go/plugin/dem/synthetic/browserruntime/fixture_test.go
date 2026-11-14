// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"os"
	"strings"
	"testing"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/demruntimetest"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/stretchr/testify/require"
)

// bundleFixture is the shared payload described by a manifest for this host.
func bundleFixture(t testing.TB, label string) (Manifest, []byte) {
	t.Helper()
	b := demruntimetest.NewBundle(t, label)
	m := Manifest{
		Format:   1,
		Contract: 2,
		Version:  "test-1",
		Platform: hostPlatform(),
		Archive:  Archive(b.Bounds),
		Layout:   Layout(b.Layout),
	}
	m.Archive.URL = "https://artifacts.example.invalid/" + m.Archive.SHA256 + ".tar.zst"
	for _, binding := range b.Bindings {
		m.Bindings = append(m.Bindings, Binding(binding))
	}
	require.NoError(t, m.Validate())
	return m, b.Data
}

func archiveFixture(t testing.TB, entries []demruntimetest.Entry) (Archive, []byte) {
	t.Helper()
	a := demruntimetest.NewArchive(t, entries)
	return Archive(a.Bounds), a.Data
}

// boundPayloadFixture extracts and binds the shared payload without a store.
func boundPayloadFixture(t *testing.T) (*os.Root, Manifest) {
	t.Helper()
	m, archive := bundleFixture(t, "bound")
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	require.NoError(t, extractArchive(bytes.NewReader(archive), root, m.Archive))
	require.NoError(t, bindPrefix(root, "/prepared/runtime", m))
	return root, m
}

// releaseFixture is a cross-architecture release signed by its own key, for
// the envelope and trust tests.
func releaseFixture(t *testing.T) (Release, *openpgp.Entity) {
	t.Helper()
	m, _ := bundleFixture(t, "signed-release")
	m.Platform = "linux/arm64"
	m.Archive.URL = "https://artifacts.example.invalid/dem-runtime/1/arm64/" + m.Archive.SHA256 + ".tar.zst"
	return Release{
		Format:   1,
		Sequence: 23,
		Manifest: m,
		Sources: ReleaseSource{
			URL:    "https://artifacts.example.invalid/dem-runtime/1/sources.tar.zst",
			SHA256: strings.Repeat("a", 64),
			Bytes:  100,
		},
	}, pgptest.Key(t, 1)
}

func testTrust(t *testing.T, key *openpgp.Entity) *ReleaseTrust {
	t.Helper()
	trust, err := ParseReleaseKeyring(pgptest.Public(t, key))
	require.NoError(t, err)
	return trust
}
