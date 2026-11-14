// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
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
		Archive:  archiveBounds(b.Archive),
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
	return archiveBounds(a), a.Data
}

func archiveBounds(a demruntimetest.Archive) Archive {
	return Archive{
		SHA256:        a.SHA256,
		Bytes:         a.Bytes,
		TarBytes:      a.TarBytes,
		ExpandedBytes: a.ExpandedBytes,
		Entries:       a.Entries,
	}
}

// archiveFile writes archive bytes to a private offline input file.
func archiveFile(t testing.TB, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.zst")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
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

// publishedRelease describes m as a published release with sequence.
func publishedRelease(m Manifest, sequence uint64) Release {
	return Release{
		Format:   1,
		Sequence: sequence,
		Manifest: m,
		Sources: ReleaseSource{
			URL:    "https://artifacts.example.invalid/sources.tar.zst",
			SHA256: strings.Repeat("b", 64),
			Bytes:  100,
		},
	}
}

// fixturePublisher signs fixture releases. Installation accepts only releases
// that VerifyRelease authenticated, so fixtures take the same path as published
// ones. Helper processes create their own publisher.
var fixturePublisher = sync.OnceValues(func() (fixtureKeys, error) {
	key, err := openpgp.NewEntity("Runtime fixture", "", "fixture@example.invalid", &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA})
	if err != nil {
		return fixtureKeys{}, err
	}
	var public bytes.Buffer
	if err := key.Serialize(&public); err != nil {
		return fixtureKeys{}, err
	}
	trust, err := ParseReleaseKeyring(public.Bytes())
	return fixtureKeys{key: key, trust: trust}, err
})

type fixtureKeys struct {
	key   *openpgp.Entity
	trust *ReleaseTrust
}

// verifyFixture signs release as its publisher and authenticates it as a
// production loader does; validation errors come from VerifyRelease.
func verifyFixture(release Release) (VerifiedRelease, error) {
	keys, err := fixturePublisher()
	if err != nil {
		return VerifiedRelease{}, err
	}
	envelope, err := pgptest.Release(release, keys.key)
	if err != nil {
		return VerifiedRelease{}, err
	}
	return VerifyRelease(envelope, keys.trust)
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
