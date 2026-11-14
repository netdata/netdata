// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s, err := Open(DefaultConfig(filepath.Join(t.TempDir(), "store")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, &now
}

// archiveFile writes archive bytes to a private offline input file.
func archiveFile(t testing.TB, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.zst")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
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

var fixtureSequence atomic.Uint64

// fixtureRelease publishes m with a sequence above every earlier fixture in this
// process.
func fixtureRelease(m Manifest) Release {
	return publishedRelease(m, fixtureSequence.Add(1))
}

// beginFixture starts an installation or repair candidate for m through the
// published-installation path with manual updates.
func (s *Store) beginFixture(ctx context.Context, m Manifest, operation ReleaseOperation) (*Candidate, error) {
	return s.beginRelease(ctx, fixtureRelease(m), operation)
}

func (s *Store) beginRelease(ctx context.Context, release Release, operation ReleaseOperation) (*Candidate, error) {
	options := ReleaseInstallOptions{
		Operation: operation,
		Updates:   ManualUpdates,
	}
	candidate, err := s.BeginPublishedInstall(ctx, options, func(context.Context) (VerifiedRelease, error) { return verifyFixture(release) })
	if err == nil && candidate == nil {
		err = errors.New("fixture installation was an unexpected no-op")
	}
	return candidate, err
}

// installRelease runs the administrator installation transaction: begin with
// policy, import the offline archive, select and close.
func installRelease(t *testing.T, s *Store, release Release, archive string, policy UpdatePolicy) Release {
	t.Helper()
	options := ReleaseInstallOptions{
		Operation: InstallRelease,
		Updates:   policy,
	}
	candidate, err := s.BeginPublishedInstall(context.Background(), options, func(context.Context) (VerifiedRelease, error) { return verifyFixture(release) })
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	return release
}

func installFixture(t *testing.T, s *Store, label string) Manifest {
	t.Helper()
	m, archive := bundleFixture(t, label)
	return installRelease(t, s, fixtureRelease(m), archiveFile(t, archive), ManualUpdates).Manifest
}

// publishedFixture is a release with an explicit sequence, its offline archive
// and a loader that authenticates it.
func publishedFixture(t *testing.T, sequence uint64) (Release, string, func(context.Context) (VerifiedRelease, error)) {
	t.Helper()
	m, archive := bundleFixture(t, strings.Repeat("release", int(sequence)))
	release := publishedRelease(m, sequence)
	return release, archiveFile(t, archive), func(context.Context) (VerifiedRelease, error) { return verifyFixture(release) }
}

func installPublishedFixture(t *testing.T, s *Store, sequence uint64, policy UpdatePolicy) Release {
	t.Helper()
	release, archive, _ := publishedFixture(t, sequence)
	return installRelease(t, s, release, archive, policy)
}

// importFixture and importUnselected prepare an unselected generation, for
// retention and owner-reclamation tests. Installation behavior is tested
// through the public API separately.
func importFixture(t *testing.T, s *Store, label string) Manifest {
	t.Helper()
	m, archive := bundleFixture(t, label)
	require.NoError(t, s.importUnselected(context.Background(), m, archiveFile(t, archive)))
	return m
}

func (s *Store) importUnselected(ctx context.Context, m Manifest, path string) error {
	candidate, err := s.beginFixture(ctx, m, InstallRelease)
	if err != nil {
		return err
	}
	defer candidate.Close()
	return candidate.Import(ctx, path)
}

// requirePrepared asserts that m is published and intact without acquiring
// execution ownership or changing retention timestamps.
func requirePrepared(t *testing.T, s *Store, m Manifest) {
	t.Helper()
	require.NoError(t, s.withLock(func() error {
		_, err := s.prepared(m)
		return err
	}))
}

func (s *Store) testProcessClaim(id string) (*claim, error) {
	var owner *claim
	err := s.withLock(func() error { var err error; owner, err = s.ledger.claimLocked(id, "process"); return err })
	return owner, err
}

// helperCrashExit is the status a helper process exits with to stop without
// Go cleanup, as a crashed writer would.
const helperCrashExit = 73

// helperCommand re-executes this test binary to run only test as a separate
// process. A helper test returns immediately unless env selects its role.
func helperCommand(ctx context.Context, test string, env ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+test+"$")
	cmd.Env = append(os.Environ(), env...)
	return cmd
}

// runHelper runs test in a helper process for at most ten seconds and returns
// its combined output and exit status.
func runHelper(t *testing.T, test string, env ...string) ([]byte, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := helperCommand(ctx, test, env...).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return output, exit.ExitCode()
	}
	require.NoError(t, err, "%s", output)
	return output, 0
}
