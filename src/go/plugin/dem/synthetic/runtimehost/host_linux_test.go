// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/demruntimetest"
	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/logger"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *runtime.Store {
	t.Helper()
	config := runtime.DefaultConfig(filepath.Join(t.TempDir(), "store"))
	config.PreviousRetention = 0
	store, err := runtime.Open(config)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

var installSequence atomic.Uint64

// Each installation publishes a sequence above every earlier one in this process.
func installBundle(t *testing.T, store *runtime.Store, version string) runtime.Manifest {
	t.Helper()
	release, data := publishedBundle(t, version, installSequence.Add(1))
	selectPublished(t, store, release, data, runtime.ManualUpdates)
	return release.Manifest
}

// publishedBundle describes the shared fixture payload as a published release.
// Its synthetic bytes prove storage and host ownership, not browser
// qualification.
func publishedBundle(t *testing.T, version string, sequence uint64) (runtime.Release, []byte) {
	t.Helper()
	b := demruntimetest.NewBundle(t, version)
	m := runtime.Manifest{
		Format:   1,
		Contract: 2,
		Version:  version,
		Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive(b.Bounds),
		Layout:   runtime.Layout(b.Layout),
	}
	m.Archive.URL = "https://artifacts.example.invalid/runtime.tar.zst"
	for _, binding := range b.Bindings {
		m.Bindings = append(m.Bindings, runtime.Binding(binding))
	}
	require.NoError(t, m.Validate())
	return runtime.Release{Format: 1, Sequence: sequence, Manifest: m,
		Sources: runtime.ReleaseSource{URL: "https://artifacts.example.invalid/sources.tar.zst", SHA256: strings.Repeat("a", 64), Bytes: 100}}, b.Data
}

// verifiedRelease signs release with the fixture publisher key and authenticates
// it as a production loader does.
func verifiedRelease(t *testing.T, release runtime.Release) runtime.VerifiedRelease {
	t.Helper()
	key := pgptest.Key(t, 66)
	public, err := runtime.ParseReleaseKeyring(pgptest.Public(t, key))
	require.NoError(t, err)
	envelope, err := pgptest.Release(release, key)
	require.NoError(t, err)
	verified, err := runtime.VerifyRelease(envelope, public)
	require.NoError(t, err)
	return verified
}

func selectPublished(t *testing.T, store *runtime.Store, release runtime.Release, data []byte, policy runtime.UpdatePolicy) {
	t.Helper()
	verified := verifiedRelease(t, release)
	// Non-executable bytes test acquisition/ownership; this does not qualify a browser.
	path := filepath.Join(t.TempDir(), "archive.tar.zst")
	require.NoError(t, os.WriteFile(path, data, 0600))
	candidate, err := store.BeginPublishedInstall(context.Background(), runtime.ReleaseInstallOptions{Operation: runtime.InstallRelease, Updates: policy}, func(context.Context) (runtime.VerifiedRelease, error) { return verified, nil })
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), path))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
}

func newTestHost(t *testing.T, store *runtime.Store) *Host {
	t.Helper()
	host := NewManaged(store)
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	return host
}

func TestHostSelectionIsFixedAcrossInstallationUpdates(t *testing.T) {
	store := testStore(t)
	first := installBundle(t, store, "first")
	old := newTestHost(t, store)
	require.NoError(t, old.Ready())
	second := installBundle(t, store, "second")
	newer := newTestHost(t, store)
	require.NoError(t, newer.Ready())
	require.NoError(t, store.Sweep())
	for _, test := range []struct {
		host     *Host
		manifest runtime.Manifest
	}{{old, first}, {newer, second}} {
		use, err := test.host.BeginUse()
		require.NoError(t, err)
		require.Equal(t, test.manifest.Archive.SHA256, use.Paths().Identity.SHA256)
		require.Equal(t, test.manifest.Version, use.Paths().Identity.Version)
		require.Equal(t, test.manifest.Contract, use.Paths().Identity.Contract)
		require.FileExists(t, use.Paths().Node)
		content, err := os.ReadFile(use.Paths().EntryPoint)
		require.NoError(t, err)
		require.Equal(t, test.manifest.Version, string(content))
		require.NotNil(t, use.Completion())
		require.NoError(t, use.Finish(true))
	}
	status := old.Status()
	require.Equal(t, first.Archive.SHA256, status.Running.SHA256)
	require.Equal(t, second.Archive.SHA256, status.Storage.Selected.Archive.SHA256)
	require.Equal(t, 2, status.Storage.ProcessClaims)
	require.ErrorIs(t, store.Uninstall(t.Context()), runtime.ErrInUse)
	require.NoError(t, old.Close())
	require.Error(t, old.Ready())
	require.ErrorIs(t, store.Uninstall(t.Context()), runtime.ErrInUse)
	require.NoError(t, newer.Close())
	require.NoError(t, store.Uninstall(t.Context()))
}

func TestMissingStartupSelectionRequiresNewProcess(t *testing.T) {
	store := testStore(t)
	empty := newTestHost(t, store)
	require.ErrorIs(t, empty.Ready(), runtime.ErrUnavailable)
	require.ErrorContains(t, empty.Ready(), "install a browser runtime and restart")
	require.Equal(t, StateMissing, empty.Status().State)
	manifest := installBundle(t, store, "installed-after-start")
	empty.sweep()
	require.ErrorIs(t, empty.Ready(), runtime.ErrUnavailable)
	require.Nil(t, empty.Status().Running)
	_, err := empty.BeginUse()
	require.ErrorIs(t, err, runtime.ErrUnavailable)
	next := newTestHost(t, store)
	require.NoError(t, next.Ready())
	require.Equal(t, manifest.Version, next.Status().Running.Version)
}

func TestSelectedInstallationSurvivesMaintenanceWithoutBrowserJobs(t *testing.T) {
	store := testStore(t)
	manifest := installBundle(t, store, "retained")
	host := newTestHost(t, store)
	host.sweepInterval = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go host.Run(ctx)
	require.Eventually(t, func() bool {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.started
	}, time.Second, time.Millisecond)
	require.NoError(t, host.Close())
	require.NoError(t, store.Sweep())
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, manifest.Archive.SHA256, consumer.Manifest().Archive.SHA256)
	require.NoError(t, consumer.Close())
}

func TestLostSelectedGenerationRequiresRepairRatherThanInstall(t *testing.T) {
	store := testStore(t)
	installBundle(t, store, "installed")
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	use, err := consumer.BeginUse()
	require.NoError(t, err)
	// The generated launcher sits at the generation root.
	generation := filepath.Dir(use.Paths.Browser)
	require.NoError(t, use.Finish(true))
	require.NoError(t, consumer.Close())
	require.NoError(t, os.Rename(generation, filepath.Join(t.TempDir(), "lost")))
	host := newTestHost(t, store)
	require.ErrorIs(t, host.Ready(), runtime.ErrDamaged)
	require.NotErrorIs(t, host.Ready(), runtime.ErrUnavailable)
	require.Equal(t, StateUnavailable, host.Status().State)
	require.Contains(t, host.Status().SetupError, "runtime repair")
}

func TestUncertainExecutionOutlivesHost(t *testing.T) {
	store := testStore(t)
	installBundle(t, store, "uncertain")
	host := newTestHost(t, store)
	use, err := host.BeginUse()
	require.NoError(t, err)
	require.NoError(t, use.Finish(false))
	require.NoError(t, host.Close())
	status, err := store.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ProcessClaims)
	require.Equal(t, 1, status.UncertainPins)
	require.ErrorIs(t, store.Uninstall(t.Context()), runtime.ErrInUse)
}

func TestExecutionFinishErrorIsMaintenanceNotReadiness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	store, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	installBundle(t, store, "maintenance")
	host := newTestHost(t, store)
	var logs healthLog
	host.log = logger.NewWithWriter(&logs)
	use, err := host.BeginUse()
	require.NoError(t, err)
	trash := filepath.Join(path, "trash")
	require.NoError(t, os.Rename(trash, trash+".saved"))
	require.NoError(t, os.WriteFile(trash, []byte("blocked"), 0600))
	finishErr := use.Finish(true)
	require.NoError(t, os.Rename(trash, trash+".failed"))
	require.NoError(t, os.Rename(trash+".saved", trash))
	require.Error(t, finishErr)
	require.NotEmpty(t, host.Status().MaintenanceError)
	require.Contains(t, logs.String(), "execution completion failed")
	require.NoError(t, host.Ready())
	other, err := host.BeginUse()
	require.NoError(t, err)
	require.NoError(t, other.Finish(true))
	require.NotEmpty(t, host.Status().MaintenanceError)
	host.sweep()
	require.Empty(t, host.Status().MaintenanceError)
	require.Contains(t, logs.String(), "execution completion recovered")
	status, err := store.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ExecutionPins)
}

func TestExecutionAdmissionFailureCanRetrySameSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	store, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	manifest := installBundle(t, store, "retry")
	host := newTestHost(t, store)
	uses := filepath.Join(path, "uses")
	require.NoError(t, os.Rename(uses, uses+".saved"))
	require.NoError(t, os.WriteFile(uses, []byte("blocked"), 0600))
	_, useErr := host.BeginUse()
	require.NoError(t, os.Rename(uses, uses+".failed"))
	require.NoError(t, os.Rename(uses+".saved", uses))
	require.Error(t, useErr)
	require.NoError(t, host.Ready())
	use, err := host.BeginUse()
	require.NoError(t, err)
	require.Equal(t, manifest.Archive.SHA256, use.Paths().Identity.SHA256)
	require.NoError(t, use.Finish(true))
}
