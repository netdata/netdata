// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Select the authenticated, non-executable fixture through store APIs solely to
// exercise installation diagnostics. These tests do not establish qualification.
func selectedAdminFixture(t *testing.T) (root, cache, manifest, archive string, selected runtime.Manifest) {
	t.Helper()
	manifest, archive = adminBundleFixture(t)
	root = t.TempDir()
	writeAdminTrust(t, root, "")
	cfg := dem.DefaultRuntimeConfig()
	cfg.OfflineKeyring = filepath.Join(root, "release-public.gpg")
	release, err := cfg.ReleaseLoader(manifest, false)(context.Background())
	require.NoError(t, err)
	m := release.Manifest
	cache = filepath.Join(root, "cache")
	store, err := runtime.Open(runtime.DefaultConfig(cache))
	require.NoError(t, err)
	candidate, err := store.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	require.NoError(t, store.Close())
	return root, cache, manifest, archive, m
}

func TestRuntimeStatusReportsDamagedInstallation(t *testing.T) {
	for _, damage := range []string{"selection", "selected generation", "old generation"} {
		t.Run(damage, func(t *testing.T) {
			root, cache, _, _, selected := selectedAdminFixture(t)
			path := filepath.Join(cache, "selection.json")
			if damage == "selected generation" {
				path = filepath.Join(cache, "generations", selected.Archive.SHA256, "prepared.json")
			} else if damage == "old generation" {
				path = filepath.Join(cache, "generations", strings.Repeat("a", 64), "prepared.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
			}
			require.NoError(t, os.WriteFile(path, []byte("{broken"), 0600))
			report, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
			require.ErrorIs(t, err, runtime.ErrDamaged)
			require.NotNil(t, report.Storage)
			assert.Positive(t, report.Storage.UsedBytes)
			assert.NotEmpty(t, report.StorageError)
			assert.Empty(t, report.Storage.InventoryError)
			if damage == "old generation" {
				assert.Equal(t, "installed", report.State)
				assert.Equal(t, selected.Archive.SHA256, report.Generation)
				assert.NotEmpty(t, report.Storage.GenerationErrors)
			} else {
				assert.Equal(t, "damaged", report.State)
				assert.NotEmpty(t, report.Storage.SelectionError)
			}
		})
	}
}

func TestRuntimeFailedRepairReportsSelectedStateSeparately(t *testing.T) {
	for _, target := range []string{"same release", "different release"} {
		t.Run(target, func(t *testing.T) {
			root, cache, manifest, archive, selected := selectedAdminFixture(t)
			requested := selected
			if target == "different release" {
				requested.Archive.SHA256 = strings.Repeat("b", 64)
				writeAdminRelease(t, manifest, requested)
			}
			report, err := runtimeCommand(t, "runtime", "repair", "--config-dir", root, "--cache", cache, "--release", manifest, "--archive", archive)
			require.Error(t, err)
			require.NotNil(t, report.Candidate)
			assert.Equal(t, requested.Archive.SHA256, report.Candidate.SHA256)
			require.NotNil(t, report.Storage)
			assert.Contains(t, report.Notice, "A cleared selection is not restored")
			if target == "same release" {
				assert.Equal(t, "prepared", report.CandidateState)
				assert.Equal(t, "unavailable", report.State)
				assert.Nil(t, report.Runtime)
				assert.Empty(t, report.Generation)
				assert.Nil(t, report.Storage.Selected)
				require.NotNil(t, report.Qualification)
				assert.False(t, report.Qualification.SandboxedLocalJourney)
				assert.False(t, report.Qualification.SandboxedLocalLighthouse)
			} else {
				assert.Equal(t, "requested", report.CandidateState)
				assert.Equal(t, "installed", report.State)
				assert.Equal(t, selected.Archive.SHA256, report.Generation)
				require.NotNil(t, report.Runtime)
				assert.Equal(t, selected.Archive.SHA256, report.Runtime.SHA256)
				assert.Nil(t, report.Qualification)
			}
		})
	}
}

func TestRuntimeRepairValidatesTrustBeforeChangingDamagedStore(t *testing.T) {
	for _, fault := range []string{"malformed", "untrusted signer", "missing trust"} {
		t.Run(fault, func(t *testing.T) {
			root, cache, releasePath, archive, selected := selectedAdminFixture(t)
			selectionPath := filepath.Join(cache, "selection.json")
			damaged := []byte("{broken")
			require.NoError(t, os.WriteFile(selectionPath, damaged, 0600))
			switch fault {
			case "malformed":
				require.NoError(t, os.WriteFile(releasePath, []byte("{}"), 0600))
			case "missing trust":
				writeRuntimeConfig(t, root, cache)
			case "untrusted signer":
				release, err := runtime.ReadRelease(releasePath, adminTrust(t))
				require.NoError(t, err)
				wrong := pgptest.Key(t, 30)
				data, err := pgptest.Release(release, wrong)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(releasePath, data, 0600))
			}
			report, err := runtimeCommand(t, "runtime", "repair", "--config-dir", root, "--cache", cache, "--release", releasePath, "--archive", archive)
			require.Error(t, err)
			assert.Nil(t, report.Candidate)
			assert.Nil(t, report.Qualification)
			remaining, err := os.ReadFile(selectionPath)
			require.NoError(t, err)
			assert.Equal(t, damaged, remaining)
			assert.FileExists(t, filepath.Join(cache, "generations", selected.Archive.SHA256, "prepared.json"))
		})
	}
}

func TestRuntimeStatusReportsUnknownWhenSafetyStructureIsMissing(t *testing.T) {
	root, cache, _, _, _ := selectedAdminFixture(t)
	require.NoError(t, os.Rename(filepath.Join(cache, "claims"), filepath.Join(root, "lost-claims")))
	report, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.Error(t, err)
	assert.Equal(t, "unknown", report.State)
	assert.Nil(t, report.Storage)
	assert.NotEmpty(t, report.SetupError)
	assert.NoDirExists(t, filepath.Join(cache, "claims"))
}

// Only authentic release inputs can establish both workload qualifications.
func TestRealRuntimeRepairQualifiesBothWorkloads(t *testing.T) {
	manifest, archive, helper := os.Getenv("DEM_RUNTIME_TEST_RELEASE"), os.Getenv("DEM_RUNTIME_TEST_ARCHIVE"), os.Getenv("DEM_RUNTIME_TEST_HELPER")
	publicKey := os.Getenv("DEM_RUNTIME_TEST_PUBLIC_KEY")
	if manifest == "" || archive == "" || helper == "" || publicKey == "" {
		t.Skip("requires DEM_RUNTIME_TEST_RELEASE, DEM_RUNTIME_TEST_ARCHIVE, DEM_RUNTIME_TEST_PUBLIC_KEY and DEM_RUNTIME_TEST_HELPER")
	}
	require.NotZero(t, os.Geteuid())
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	root := t.TempDir()
	writeRuntimeTrustConfig(t, root, "", publicKey)
	cache := filepath.Join(root, "cache")
	store, err := runtime.Open(runtime.DefaultConfig(cache))
	require.NoError(t, err)
	require.NoError(t, store.Close())
	require.NoError(t, os.WriteFile(filepath.Join(cache, "selection.json"), []byte("{broken"), 0600))
	report, err := runtimeCommand(t, "runtime", "repair", "--config-dir", root, "--cache", cache, "--release", manifest, "--archive", archive)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Equal(t, "selected", report.CandidateState)
	require.NotNil(t, report.Runtime)
	assert.Equal(t, report.Candidate, report.Runtime)
	require.NotNil(t, report.Qualification)
	assert.True(t, report.Qualification.ExecutionContract)
	assert.True(t, report.Qualification.SandboxedLocalJourney)
	assert.True(t, report.Qualification.SandboxedLocalLighthouse)
	assert.True(t, report.Qualification.ProcessTreeDrained)
	assert.Equal(t, report.Runtime, report.Qualification.Runtime)
}

func TestRuntimeStatusReportsIncompleteOwnershipInventory(t *testing.T) {
	root, cache, _, _, _ := selectedAdminFixture(t)
	store, err := runtime.Open(runtime.DefaultConfig(cache))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	defer func() { require.NoError(t, consumer.Close()) }()
	claims, err := os.ReadDir(filepath.Join(cache, "claims"))
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.NoError(t, os.WriteFile(filepath.Join(cache, "claims", claims[0].Name(), "owner.json"), []byte("{broken"), 0600))
	report, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.Error(t, err)
	assert.Equal(t, "unknown", report.State)
	require.NotNil(t, report.Storage)
	assert.NotEmpty(t, report.Storage.InventoryError)
	assert.NotEmpty(t, report.StorageError)
}
