// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/browserruntimetest"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminBundleFixture writes the shared fixture payload and its signed release.
// It proves archive plumbing and final-path binding, not executable/browser
// readiness: qualify must reject it.
func adminBundleFixture(t *testing.T) (manifestPath, archivePath string) {
	t.Helper()
	setRuntimeAgent(t, fixtureAgent("cli-fixture"))
	return adminNamedBundleFixture(t, "cli-fixture")
}

func adminNamedBundleFixture(t *testing.T, label string) (manifestPath, archivePath string) {
	t.Helper()
	agent := fixtureAgent(label)
	b := browserruntimetest.NewBundle(t, label, browserruntimetest.Entry{
		Name: "release-info", Mode: 0644,
		Contents: []byte("version=" + agent.Version + "\nchannel=" + agent.Channel + "\nsource=" + agent.Source + "\n"),
	})
	manifest := browserruntime.Manifest{
		Format:   1,
		Contract: 2,
		Version:  label,
		Platform: "linux/" + runtime.GOARCH,
		Archive:  browserruntime.Archive(b.Bounds),
		Layout:   browserruntime.Layout(b.Layout),
	}
	for _, binding := range b.Bindings {
		manifest.Bindings = append(manifest.Bindings, browserruntime.Binding(binding))
	}
	require.NoError(t, manifest.Validate())
	root := t.TempDir()
	manifestPath = filepath.Join(root, "trusted.json")
	archivePath = filepath.Join(root, "runtime.tar.zst")
	writeAdminRelease(t, manifestPath, manifest)
	require.NoError(t, os.WriteFile(archivePath, b.Data, 0600))
	return manifestPath, archivePath
}

func TestRuntimeInstallDoesNotExecuteBrowser(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	cache := filepath.Join(root, "cache")
	report, err := runtimeCommand(t, "runtime", "install", "--config-dir", root,
		"--release", manifest, "--cache", cache, "--archive", archive)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Equal(t, "selected", report.CandidateState)
	assert.Nil(t, report.Qualification)
	require.NotNil(t, report.Storage)
	require.NotNil(t, report.Storage.Release)
	report, err = runtimeCommand(t, "runtime", "qualify", "--config-dir", root, "--cache", cache)
	require.Error(t, err)
	assert.Equal(t, "installed", report.State)
	require.NotNil(t, report.Qualification)
	assert.False(t, report.Qualification.SandboxedLocalJourney)
	report, err = runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Nil(t, report.Qualification)
}

func TestRuntimePreloadDoesNotExecuteBrowserAndRequiresFreshRoot(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	final := filepath.Join(root, "preloaded")
	args := []string{
		"runtime",
		"preload",
		"--config-dir",
		root,
		"--release",
		manifest,
		"--archive",
		archive,
		"--root",
		final,
	}
	report, err := runtimeCommand(t, args...)
	require.NoError(t, err)
	assert.NotNil(t, report.PreloadedConfig)
	assert.Nil(t, report.Qualification)
	assert.FileExists(t, filepath.Join(final, "preload.json"))
	config, err := json.Marshal(map[string]any{
		"runtime": map[string]any{"mode": "preloaded", "preload_path": final},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "dem.conf"), config, 0600))
	report, err = runtimeCommand(t, "runtime", "qualify", "--config-dir", root)
	require.Error(t, err)
	assert.Equal(t, "installed", report.State)
	require.NotNil(t, report.Qualification)
	assert.False(t, report.Qualification.SandboxedLocalJourney)
	_, err = runtimeCommand(t, args...)
	require.ErrorContains(t, err, "preload root must be empty")
}

func TestRuntimeInstallDigestFailureNeverReportsPrepared(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	require.NoError(t, os.WriteFile(archive, []byte("corrupt"), 0600))
	report, err := runtimeCommand(
		t,
		"runtime",
		"install",
		"--config-dir",
		root,
		"--release",
		manifest,
		"--archive",
		archive,
		"--cache",
		filepath.Join(root, "cache"),
	)
	require.Error(t, err)
	assert.Equal(t, "unavailable", report.State)
	assert.NotEmpty(t, report.SetupError)
	require.NotNil(t, report.Storage)
	assert.Nil(t, report.Storage.Release)
}

func TestRuntimeInstallUsesServiceConfiguration(t *testing.T) {
	if !runtimeConfigProcess(t, "offline-install") {
		return
	}
	manifest, archive := adminBundleFixture(t)
	_, stock, buildUser, _, _ := runtimeConfigRoots(t)
	serviceCache, wrongCache := filepath.Join(t.TempDir(), "service-cache"), filepath.Join(t.TempDir(), "wrong-cache")
	writeAdminTrust(t, stock, serviceCache)
	writeRuntimeConfig(t, buildUser, wrongCache)
	pluginconfig.MustInit(pluginconfig.InitInput{})
	service, err := dem.LoadConfig(pluginconfig.ConfigDir())
	require.NoError(t, err)
	require.Equal(t, serviceCache, service.Runtime.CachePath)
	report, err := runtimeCommand(t, "runtime", "install", "--release", manifest, "--archive", archive)
	require.NoError(t, err)
	require.Equal(t, "installed", report.State)
	require.Equal(t, "selected", report.CandidateState)
	require.Equal(t, service.Runtime.CachePath, report.CachePath)
	require.DirExists(t, filepath.Join(serviceCache, "generations", report.Candidate.SHA256))
	require.NoDirExists(t, wrongCache)
	overrideCache := filepath.Join(t.TempDir(), "override-cache")
	report, err = runtimeCommand(
		t,
		"runtime",
		"install",
		"--release",
		manifest,
		"--archive",
		archive,
		"--cache",
		overrideCache,
	)
	require.NoError(t, err)
	require.Equal(t, overrideCache, report.CachePath)
	require.DirExists(t, filepath.Join(overrideCache, "generations", report.Candidate.SHA256))
	require.NoDirExists(t, wrongCache)
}

// This opt-in test uses authentic release descriptors and archives through the
// production CLI acquisition, qualification and publication path.
func TestRealRuntimeInstallQualifyAndUninstall(t *testing.T) {
	manifest := os.Getenv("DEM_RUNTIME_TEST_RELEASE")
	archive := os.Getenv("DEM_RUNTIME_TEST_ARCHIVE")
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	publicKey := os.Getenv("DEM_RUNTIME_TEST_PUBLIC_KEY")
	if manifest == "" || archive == "" || helper == "" || publicKey == "" {
		t.Skip(
			"requires DEM_RUNTIME_TEST_RELEASE, DEM_RUNTIME_TEST_ARCHIVE, DEM_RUNTIME_TEST_PUBLIC_KEY and DEM_RUNTIME_TEST_HELPER",
		)
	}
	require.NoError(t, runtimehost.AgentRelease().Validate(), "real runtime tests require compiled matching ReleaseVersion, ReleaseChannel and ReleaseRef linker metadata")
	require.NotZero(t, os.Geteuid())
	restore := ndexec.SetRunnerPathsForTests(helper, "")
	defer restore()
	root := t.TempDir()
	writeRuntimeTrustConfig(t, root, "", publicKey)
	cache := filepath.Join(root, "installed")
	common := []string{"--config-dir", root, "--cache", cache}
	install := append(append([]string{"runtime", "install"}, common...), "--release", manifest, "--archive", archive)
	report, err := runtimeCommand(t, install...)
	require.NoError(t, err)
	require.Equal(t, "installed", report.State)
	require.NotNil(t, report.Runtime)
	require.Nil(t, report.Qualification)
	// Installing remains exact-pair checked. Execution uses the same contract and
	// integrity checks as an ordinary developer build with no release stamps.
	setRuntimeAgent(t, browserruntime.AgentRelease{})
	for _, command := range []string{"status", "qualify"} {
		result, err := runtimeCommand(t, append([]string{"runtime", command}, common...)...)
		require.NoError(t, err)
		require.Equal(t, report.Runtime, result.Runtime)
		if command == "qualify" {
			require.Equal(t, "qualified", result.State)
			require.NotNil(t, result.Qualification)
			require.True(t, result.Qualification.ExecutionContract)
			require.True(t, result.Qualification.SandboxedLocalJourney)
			require.True(t, result.Qualification.SandboxedLocalLighthouse)
			require.True(t, result.Qualification.ProcessTreeDrained)
			require.Equal(t, report.Runtime, result.Qualification.Runtime)
		} else {
			require.Nil(t, result.Qualification)
		}
	}
	store, err := browserruntime.Open(browserruntime.DefaultConfig(cache))
	require.NoError(t, err)
	consumer, err := store.AcquireSelected()
	require.NoError(t, err)
	_, err = runtimeCommand(t, append([]string{"runtime", "uninstall"}, common...)...)
	require.Error(t, err)
	require.NoError(t, consumer.Close())
	require.NoError(t, store.Close())
	result, err := runtimeCommand(t, append([]string{"runtime", "uninstall"}, common...)...)
	require.NoError(t, err)
	require.Equal(t, "uninstalled", result.State)
	_, err = runtimeCommand(t, append([]string{"runtime", "status"}, common...)...)
	require.ErrorIs(t, err, browserruntime.ErrUnavailable)
}

func TestRuntimePairedUpdateDoesNotExecuteBrowser(t *testing.T) {
	root, cache, _, _, previous := selectedAdminFixture(t)
	release, archive := adminNamedBundleFixture(t, "updated-fixture")
	// The installation owner uses the target helper to prepare, then activates
	// after replacing Agent files. Neither command executes the browser.
	setRuntimeAgent(t, fixtureAgent("updated-fixture"))
	report, err := runtimeCommand(t, "runtime", "prepare", "--config-dir", root,
		"--release", release, "--cache", cache, "--archive", archive)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.State)
	assert.Equal(t, "prepared", report.CandidateState)
	assert.Equal(t, previous.Archive.SHA256, report.Runtime.SHA256)
	assert.Nil(t, report.Qualification)
	report, err = runtimeCommand(t, "runtime", "activate", "--config-dir", root, "--cache", cache)
	require.NoError(t, err)
	assert.Nil(t, report.Qualification)
	require.NotNil(t, report.Runtime)
	assert.Equal(t, "updated-fixture", report.Runtime.Version)
	assert.NotEqual(t, previous.Archive.SHA256, report.Runtime.SHA256)
	assert.FileExists(t, filepath.Join(cache, "generations", previous.Archive.SHA256, "prepared.json"))
}

func TestRuntimeInstallRejectsDifferentAgentPair(t *testing.T) {
	for name, change := range map[string]func(*browserruntime.AgentRelease){
		"version": func(agent *browserruntime.AgentRelease) { agent.Version = "next-Agent-release" },
		"channel": func(agent *browserruntime.AgentRelease) { agent.Channel = "nightly" },
		"source":  func(agent *browserruntime.AgentRelease) { agent.Source = strings.Repeat("b", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			root, cache, release, archive, selected := selectedAdminFixture(t)
			agent := fixtureAgent(selected.Version)
			change(&agent)
			if name == "version" {
				// Acquiring a later runtime does not advance the installed helper.
				release, archive = adminNamedBundleFixture(t, agent.Version)
			} else {
				writeAdminPairedRelease(t, release, selected, agent)
			}
			selection := filepath.Join(cache, "selection.json")
			before, err := os.ReadFile(selection)
			require.NoError(t, err)
			report, err := runtimeCommand(t, "runtime", "install", "--config-dir", root,
				"--cache", cache, "--release", release, "--archive", archive)
			require.ErrorIs(t, err, browserruntime.ErrIncompatible)
			require.Nil(t, report.Candidate)
			require.Nil(t, report.Qualification)
			after, err := os.ReadFile(selection)
			require.NoError(t, err)
			require.Equal(t, before, after)
			status, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
			require.NoError(t, err)
			require.NotNil(t, status.Runtime)
			require.Equal(t, selected.Identity(), *status.Runtime)
		})
	}
}

func TestRuntimeInstallRequiresCompiledAgentIdentity(t *testing.T) {
	release, archive := adminBundleFixture(t)
	setRuntimeAgent(t, browserruntime.AgentRelease{})
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	cache := filepath.Join(root, "cache")
	report, err := runtimeCommand(t, "runtime", "install", "--config-dir", root,
		"--cache", cache, "--release", release, "--archive", archive)
	require.ErrorContains(t, err, "expected Agent release")
	require.Nil(t, report.Candidate)
	require.Nil(t, report.Qualification)
	status, err := runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.ErrorIs(t, err, browserruntime.ErrUnavailable)
	require.Nil(t, status.Runtime)
}
