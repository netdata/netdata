// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/stretchr/testify/require"
)

func TestCandidateProtectionAndSerializedUninstall(t *testing.T) {
	s, now := testStore(t)
	m := importFixture(t, s, "candidate")
	candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
	require.NoError(t, err)
	*now = now.Add(365 * 24 * time.Hour)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	require.NoError(t, other.Sweep())
	requirePrepared(t, other, m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, other.Uninstall(ctx), context.Canceled)
	_, err = other.beginFixture(t, ctx, m, InstallRelease)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, candidate.Close())
	require.NoError(t, other.Uninstall(context.Background()))
	_, err = candidate.BeginUse()
	require.ErrorContains(t, err, "closed")
}

func TestReusedPreparedPayloadMustBeIntactBeforePublication(t *testing.T) {
	for _, operation := range []string{"select", "preload"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := testStore(t)
			m := importFixture(t, s, "canceled-preparation")
			require.NoError(t, os.Remove(filepath.Join(s.path, "generations", m.Archive.SHA256, m.Layout.Node)))
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			defer candidate.Close()
			if operation == "select" {
				err = candidate.Select()
			} else {
				err = candidate.SealPreload()
			}
			require.ErrorIs(t, err, ErrDamaged)
			require.NoFileExists(t, filepath.Join(s.path, "preload.json"))
			_, err = s.AcquireSelected()
			require.ErrorIs(t, err, ErrUnavailable)
		})
	}
}

func TestPreparationReportsOneCandidateClaim(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "status")
	candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, candidate.Close()) })
	ready, resume := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
			return io.NopCloser(&resumeReader{
				ready:  ready,
				resume: resume,
				tail:   bytes.NewReader(archive),
			}), nil
		})
	}()
	joined := false
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
		if !joined {
			require.NoError(t, <-done)
		}
	})
	<-ready
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.CandidateClaims, "one installer is not two claims while staging")
	require.Positive(t, status.ReservedBytes)
	require.Positive(t, status.PreparingBytes)
	require.Zero(t, status.ProcessClaims)
	close(resume)
	result := <-done
	joined = true
	require.NoError(t, result)
	require.NoError(t, candidate.Select())
}

func TestUnsupportedContractKeepsInstalledSelection(t *testing.T) {
	s, _ := testStore(t)
	selected := installFixture(t, s, "selected")
	unsupported := selected
	unsupported.Contract = 1
	candidate, err := s.beginFixture(t, context.Background(), unsupported, InstallRelease)
	require.Nil(t, candidate)
	require.ErrorContains(t, err, "unsupported runtime execution contract 1")
	require.ErrorIs(t, err, ErrIncompatible)
	require.ErrorContains(t, err, "matching Agent/runtime pair")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, selected, status.Release.Manifest)
}

func TestPublishedDiscoverySerializesUninstallAndCancellation(t *testing.T) {
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 1, AutomaticUpdates)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.BeginPublishedInstall(
			ctx,
			ReleaseInstallOptions{
				Operation: InstallRelease,
				Updates:   ManualUpdates,
			},
			func(ctx context.Context) (VerifiedRelease, error) {
				close(entered)
				<-ctx.Done()
				return VerifiedRelease{}, ctx.Err()
			},
		)
		done <- err
	}()
	<-entered
	consumer, err := other.AcquireSelected()
	require.NoError(t, err, "network discovery must not hold inventory coordination")
	require.Equal(t, current.Manifest, consumer.Manifest())
	require.NoError(t, consumer.Close())
	blocked, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	require.ErrorIs(t, other.Uninstall(blocked), context.DeadlineExceeded)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, other.Uninstall(context.Background()))
	// Pair preparation after uninstall fails before release discovery.
	_, err = s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: PrepareRelease,
		},
		func(context.Context) (VerifiedRelease, error) {
			t.Fatal("discovered after uninstall")
			return VerifiedRelease{}, nil
		},
	)
	require.ErrorContains(t, err, "install an authenticated runtime release before paired preparation")
}

func TestPublishedInstallRefusesUnverifiedRelease(t *testing.T) {
	s, _ := testStore(t)
	for _, operation := range []ReleaseOperation{InstallRelease, RepairRelease} {
		candidate, err := s.BeginPublishedInstall(
			context.Background(),
			ReleaseInstallOptions{
				Operation: operation,
				Updates:   ManualUpdates,
			},
			func(context.Context) (VerifiedRelease, error) { return VerifiedRelease{}, nil },
		)
		require.Nil(t, candidate)
		require.ErrorContains(t, err, "invalid runtime release format")
	}
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Nil(t, status.Release)
}

// A caller changing the manifest a candidate hands out cannot change what the
// candidate prepares or selects: only the verified release is installed.
func TestCandidateManifestChangesDoNotReachSelection(t *testing.T) {
	s, _ := testStore(t)
	release, archive, load := publishedFixture(t, 1)
	candidate, err := s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   ManualUpdates,
		},
		load,
	)
	require.NoError(t, err)
	changed := candidate.Manifest()
	changed.Bindings[0].Offset++
	changed.Bindings[0].Expected[0] = '!'
	require.Equal(t, release.Manifest, candidate.Manifest())
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, release, *status.Release)
}

// A mirror may move the archive without changing executable identity: the
// prepared generation is reused and the selection stays healthy.
func TestExplicitInstallAcrossArchiveMirrorReusesGeneration(t *testing.T) {
	s, _ := testStore(t)
	first := installPublishedFixture(t, s, 2, ManualUpdates)
	moved := first
	moved.Sequence = 3
	moved.Manifest.Archive.URL = "https://mirror.example.invalid/runtime.tar.zst"
	var requests atomic.Int32
	previous := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected archive acquisition")
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{
		Operation: InstallRelease,
		Updates:   ManualUpdates,
	},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, moved) })
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.NoError(t, candidate.Download(context.Background()))
	require.Zero(t, requests.Load(), "the prepared generation must be reused without acquisition")
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.SelectionError)
	require.Equal(t, moved, *status.Release)
	require.Len(t, status.GenerationErrors, 0)
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, moved.Manifest, consumer.Manifest())
	require.NoError(t, consumer.Close())
}

func TestPublishedCandidateFailurePreservesSelectedPolicy(t *testing.T) {
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 2, AutomaticUpdates)
	_, archive, load := publishedFixture(t, 3)
	candidate, err := s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   ManualUpdates,
		},
		load,
	)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	// A caller aborts after failed qualification without Select.
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, current, *status.Release)
	require.True(t, status.AutomaticUpdates)
	_, _, same := publishedFixture(t, 2)
	candidate, err = s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   ManualUpdates,
		},
		same,
	)
	require.NoError(t, err)
	require.Nil(t, candidate)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.False(t, status.AutomaticUpdates)
}

func TestPublishedSelectionUncertaintyRetainsOneCompleteRecord(t *testing.T) {
	s, _ := testStore(t)
	installPublishedFixture(t, s, 1, ManualUpdates)
	next, archive, load := publishedFixture(t, 2)
	candidate, err := s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   AutomaticUpdates,
		},
		load,
	)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	original := s.syncSelection
	calls := 0
	s.syncSelection = func() error {
		calls++
		if calls > 1 {
			return errors.New("directory sync failed")
		}
		return original()
	}
	var uncertain *SelectionError
	require.ErrorAs(t, candidate.Select(), &uncertain)
	// Inspect reads the visible atomic record without claiming crash durability.
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, next.Manifest, status.Release.Manifest)
	require.Equal(t, next, *status.Release)
	require.True(t, status.AutomaticUpdates)
	s.syncSelection = original
	require.NoError(t, candidate.Close())
}

func TestPublishedRepairAllowsExplicitRecoveryAndRequiresDrain(t *testing.T) {
	s, _ := testStore(t)
	installPublishedFixture(t, s, 3, AutomaticUpdates)
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	older, archive, load := publishedFixture(t, 1)
	options := ReleaseInstallOptions{
		Operation: RepairRelease,
		Updates:   ManualUpdates,
	}
	_, err = s.BeginPublishedInstall(context.Background(), options, load)
	require.ErrorIs(t, err, ErrInUse)
	require.NoError(t, consumer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(s.path, "selection.json"), []byte("broken"), 0600))
	candidate, err := s.BeginPublishedInstall(context.Background(), options, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, older, *status.Release)
	require.False(t, status.AutomaticUpdates)
}

func TestPublishedOnlineTrustFailurePreservesInstalledSelection(t *testing.T) {
	store, _ := testStore(t)
	current := installPublishedFixture(t, store, 2, AutomaticUpdates)
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	defer consumer.Close()
	next, _, _ := publishedFixture(t, 3)
	key := pgptest.Key(t, 1)
	signed, err := pgptest.Release(next, key)
	require.NoError(t, err)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	status := http.StatusServiceUnavailable
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, ReleaseKeyringURL, r.URL.String())
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(pgptest.Public(t, key))),
			Header:     make(http.Header),
		}, nil
	})
	load := func(ctx context.Context) (VerifiedRelease, error) {
		trust, err := FetchReleaseKeyring(ctx)
		if err != nil {
			return VerifiedRelease{}, err
		}
		return VerifyRelease(signed, trust)
	}
	candidate, err := store.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   ManualUpdates,
		},
		load,
	)
	require.ErrorContains(t, err, "503")
	require.Nil(t, candidate)
	inspection, err := store.Inspect()
	require.NoError(t, err)
	require.Equal(t, current, *inspection.Release)
	require.Equal(t, current.Manifest, consumer.Manifest())
	require.Equal(t, 1, calls)
	// The next attempt fetches trust afresh and can proceed under the same owner.
	status = http.StatusOK
	candidate, err = store.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: InstallRelease,
			Updates:   ManualUpdates,
		},
		load,
	)
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.NoError(t, candidate.Close())
	require.Equal(t, 2, calls)
}

func TestSelectedPayloadErrorPreservesInconclusiveFailure(t *testing.T) {
	cause := errors.New("payload validation interrupted")
	err := selectedPayloadError(cause)
	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, ErrDamaged)
}

func TestPreparedPayloadOpenPreservesOperationalFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires the unprivileged service identity")
	}
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 1, ManualUpdates)
	path := filepath.Join(s.path, "generations", current.Manifest.Archive.SHA256)
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0700)) })
	err := s.validatePreparedPayload(current.Manifest)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NotErrorIs(t, err, ErrDamaged)
}

func TestPublishedInstallCancellationDuringDiscoveryDoesNotEnroll(t *testing.T) {
	s, _ := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	options := ReleaseInstallOptions{Operation: InstallRelease, Updates: AutomaticUpdates}
	_, err := s.BeginPublishedInstall(ctx, options, func(ctx context.Context) (VerifiedRelease, error) {
		cancel()
		return VerifiedRelease{}, ctx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Nil(t, status.Release)
	require.False(t, status.AutomaticUpdates)
	// Cancellation released installer exclusion for an ordinary retry.
	_, _, load := publishedFixture(t, 1)
	candidate, err := s.BeginPublishedInstall(context.Background(), options, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Close())
}

func TestIndependentUpdateOperationIsRejectedBeforeDiscovery(t *testing.T) {
	s, _ := testStore(t)
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: "update"},
		func(context.Context) (VerifiedRelease, error) {
			t.Fatal("retired independent update attempted discovery")
			return VerifiedRelease{}, nil
		})
	require.ErrorIs(t, err, errInvalidOperation)
	require.Nil(t, candidate)
}
