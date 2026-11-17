// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Select the authenticated, non-executable fixture through store APIs solely to
// exercise installation diagnostics. These tests do not establish qualification.
func selectedAdminFixture(t *testing.T) (root, cache, manifest, archive string, selected browserruntime.Manifest) {
	t.Helper()
	manifest, archive = adminBundleFixture(t)
	root = t.TempDir()
	writeAdminTrust(t, root, "")
	cfg := runtimehost.DefaultConfig()
	cfg.OfflineKeyring = filepath.Join(root, "release-public.gpg")
	cache = filepath.Join(root, "cache")
	cfg.CachePath = cache
	storeConfig, err := cfg.StoreConfig("")
	require.NoError(t, err)
	store, err := browserruntime.Open(storeConfig)
	require.NoError(t, err)
	options := browserruntime.ReleaseInstallOptions{
		Operation: browserruntime.InstallRelease,
		Updates:   browserruntime.ManualUpdates,
	}
	candidate, err := store.BeginPublishedInstall(context.Background(), options, cfg.ReleaseLoader(manifest, false))
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	m := candidate.Manifest()
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
			require.ErrorIs(t, err, browserruntime.ErrDamaged)
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
			if target == "same release" {
				// Repair must validate the payload even when the signed identity is unchanged.
				require.NoError(t, os.WriteFile(archive, []byte("corrupt"), 0600))
			}
			if target == "different release" {
				requested.Archive.SHA256 = strings.Repeat("b", 64)
				writeAdminRelease(t, manifest, requested)
			}
			report, err := runtimeCommand(
				t,
				"runtime",
				"repair",
				"--config-dir",
				root,
				"--cache",
				cache,
				"--release",
				manifest,
				"--archive",
				archive,
			)
			require.Error(t, err)
			require.NotNil(t, report.Candidate)
			assert.Equal(t, requested.Archive.SHA256, report.Candidate.SHA256)
			require.NotNil(t, report.Storage)
			assert.Contains(t, report.Notice, "A cleared selection is not restored")
			if target == "same release" {
				assert.Equal(t, "requested", report.CandidateState)
				assert.Equal(t, "unavailable", report.State)
				assert.Nil(t, report.Runtime)
				assert.Empty(t, report.Generation)
				assert.Nil(t, report.Storage.Release)
				assert.Nil(t, report.Qualification)
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
				release, err := browserruntime.ReadRelease(releasePath, adminTrust(t))
				require.NoError(t, err)
				wrong := pgptest.Key(t, 30)
				data, err := pgptest.Release(release.Release(), wrong)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(releasePath, data, 0600))
			}
			report, err := runtimeCommand(
				t,
				"runtime",
				"repair",
				"--config-dir",
				root,
				"--cache",
				cache,
				"--release",
				releasePath,
				"--archive",
				archive,
			)
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
func realAdminInputs(t *testing.T) (manifest, archive, publicKey string) {
	t.Helper()
	manifest = os.Getenv("DEM_RUNTIME_TEST_RELEASE")
	archive = os.Getenv("DEM_RUNTIME_TEST_ARCHIVE")
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	publicKey = os.Getenv("DEM_RUNTIME_TEST_PUBLIC_KEY")
	if manifest == "" || archive == "" || helper == "" || publicKey == "" {
		t.Skip(
			"requires DEM_RUNTIME_TEST_RELEASE, DEM_RUNTIME_TEST_ARCHIVE, DEM_RUNTIME_TEST_PUBLIC_KEY and DEM_RUNTIME_TEST_HELPER",
		)
	}
	require.NoError(t, runtimehost.AgentRelease().Validate(), "real runtime tests require compiled matching ReleaseVersion, ReleaseChannel and ReleaseRef linker metadata")
	require.NotZero(t, os.Geteuid())
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	return manifest, archive, publicKey
}

func TestRealRuntimeRepairAndExplicitQualification(t *testing.T) {
	manifest, archive, publicKey := realAdminInputs(t)
	root := t.TempDir()
	writeRuntimeTrustConfig(t, root, "", publicKey)
	cache := filepath.Join(root, "cache")
	store, err := browserruntime.Open(browserruntime.DefaultConfig(cache))
	require.NoError(t, err)
	require.NoError(t, store.Close())
	require.NoError(t, os.WriteFile(filepath.Join(cache, "selection.json"), []byte("{broken"), 0600))
	report, err := runtimeCommand(
		t,
		"runtime",
		"repair",
		"--config-dir",
		root,
		"--cache",
		cache,
		"--release",
		manifest,
		"--archive",
		archive,
	)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Equal(t, "selected", report.CandidateState)
	require.NotNil(t, report.Runtime)
	assert.Equal(t, report.Candidate, report.Runtime)
	assert.Nil(t, report.Qualification)
	installed := report.Runtime
	report, err = runtimeCommand(t, "runtime", "qualify", "--config-dir", root, "--cache", cache)
	require.NoError(t, err)
	require.NotNil(t, report.Qualification)
	assert.Equal(t, "qualified", report.State)
	assert.Equal(t, installed, report.Runtime)
	assert.True(t, report.Qualification.ExecutionContract)
	assert.True(t, report.Qualification.SandboxedLocalJourney)
	assert.True(t, report.Qualification.SandboxedLocalLighthouse)
	assert.True(t, report.Qualification.ProcessTreeDrained)
	assert.Equal(t, report.Runtime, report.Qualification.Runtime)
}

func TestRealRuntimePreloadReportsPreparedCandidate(t *testing.T) {
	manifest, archive, publicKey := realAdminInputs(t)
	root := t.TempDir()
	writeRuntimeTrustConfig(t, root, "", publicKey)
	final := filepath.Join(root, "preload")
	report, err := runtimeCommand(t, "runtime", "preload", "--config-dir", root,
		"--root", final, "--release", manifest, "--archive", archive)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Equal(t, "prepared", report.CandidateState)
	assert.FileExists(t, filepath.Join(final, "preload.json"))
	data, err := os.ReadFile(filepath.Join(final, "selection.json"))
	require.NoError(t, err)
	var selection struct {
		Manifest *browserruntime.Manifest `json:"manifest"`
	}
	require.NoError(t, json.Unmarshal(data, &selection))
	assert.Nil(t, selection.Manifest)
	assert.Nil(t, report.Qualification)
	config, err := json.Marshal(map[string]any{
		"runtime": map[string]any{"mode": "preloaded", "preload_path": final},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "dem.conf"), config, 0600))
	report, err = runtimeCommand(t, "runtime", "qualify", "--config-dir", root)
	require.NoError(t, err)
	assert.Equal(t, "qualified", report.State)
	require.NotNil(t, report.Qualification)
	assert.True(t, report.Qualification.SandboxedLocalJourney)
	assert.True(t, report.Qualification.SandboxedLocalLighthouse)
	assert.True(t, report.Qualification.ProcessTreeDrained)
}

func TestRuntimeStatusReportsIncompleteOwnershipInventory(t *testing.T) {
	root, cache, _, _, _ := selectedAdminFixture(t)
	store, err := browserruntime.Open(browserruntime.DefaultConfig(cache))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	defer func() { require.NoError(t, consumer.Close()) }()
	claims, err := os.ReadDir(filepath.Join(cache, "claims"))
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(cache, "claims", claims[0].Name(), "owner.json"), []byte("{broken"), 0600),
	)
	report, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.Error(t, err)
	assert.Equal(t, "unknown", report.State)
	require.NotNil(t, report.Storage)
	assert.NotEmpty(t, report.Storage.InventoryError)
	assert.NotEmpty(t, report.StorageError)
}

// This simulates the internally consistent records left by a previous Agent;
// execution eligibility must not turn readable installation metadata into damage.
func TestRuntimeStatusReportsIncompatibleSelection(t *testing.T) {
	root, cache, _, _, selected := selectedAdminFixture(t)
	for _, path := range []string{
		filepath.Join(cache, "selection.json"),
		filepath.Join(cache, "generations", selected.Archive.SHA256, "prepared.json"),
	} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var record map[string]any
		require.NoError(t, json.Unmarshal(data, &record))
		owner := record
		if release, ok := record["release"].(map[string]any); ok {
			owner = release
		}
		owner["manifest"].(map[string]any)["contract"] = 1
		data, err = json.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0600))
	}
	report, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.ErrorIs(t, err, browserruntime.ErrIncompatible)
	require.NotErrorIs(t, err, browserruntime.ErrDamaged)
	assert.Equal(t, "incompatible", report.State)
	assert.Equal(t, selected.Archive.SHA256, report.Generation)
	require.NotNil(t, report.Runtime)
	assert.Equal(t, 1, report.Runtime.Contract)
	require.NotNil(t, report.Storage)
	assert.Empty(t, report.Storage.SelectionError)
	assert.Empty(t, report.StorageError)
	assert.Contains(t, report.Notice, "execution contract")
	assert.Nil(t, report.Qualification)
}
