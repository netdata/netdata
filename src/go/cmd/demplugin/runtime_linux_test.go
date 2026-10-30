// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This authenticated fixture proves archive plumbing and final-path binding, not
// executable/browser readiness. qualify must reject it.
func adminBundleFixture(t *testing.T) (manifestPath, archivePath string) {
	t.Helper()
	prefix := append([]byte("/unprepared/"), make([]byte, 1012)...)
	entries := []struct {
		name string
		body []byte
	}{
		{"node", prefix}, {"browser", prefix}, {"loader", []byte("loader")},
		{"dependencies/package.json", []byte("{}")}, {"fonts/test.ttf", []byte("font")},
		{"runner/run.mjs", []byte("// non-executable qualification fixture")},
		{"runtime.json", []byte(`{"contract":1,"version":"cli-fixture","paths":{"dependencies":"dependencies","browser":"chrome"}}`)},
	}
	var raw, compressed bytes.Buffer
	tw := tar.NewWriter(&raw)
	var expanded int64
	for _, entry := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0755, Size: int64(len(entry.body)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(entry.body)
		require.NoError(t, err)
		expanded += int64(len(entry.body))
	}
	require.NoError(t, tw.Close())
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	digest := sha256.Sum256(compressed.Bytes())
	manifest := runtime.Manifest{Format: 1, Contract: 1, Version: "cli-fixture", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(compressed.Len()), TarBytes: int64(raw.Len()), ExpandedBytes: expanded, Entries: len(entries)},
		Layout:   runtime.Layout{EntryPoint: "runner/run.mjs", Node: "node", Browser: "browser", Dependencies: "dependencies", Loader: "loader", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: prefix}, {Path: "browser", Expected: prefix}},
	}
	require.NoError(t, manifest.Validate())
	root := t.TempDir()
	manifestPath = filepath.Join(root, "trusted.json")
	archivePath = filepath.Join(root, "runtime.tar.gz")
	writeAdminRelease(t, manifestPath, manifest)
	require.NoError(t, os.WriteFile(archivePath, compressed.Bytes(), 0600))
	return manifestPath, archivePath
}

func TestRuntimeInstallRejectsUnqualifiedArchive(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	cache := filepath.Join(root, "cache")
	report, err := runtimeCommand(t, "runtime", "install", "--config-dir", root, "--release", manifest, "--cache", cache, "--archive", archive)
	require.Error(t, err)
	assert.Equal(t, "unavailable", report.State)
	assert.Equal(t, "prepared", report.CandidateState)
	require.NotNil(t, report.Qualification)
	assert.False(t, report.Qualification.SandboxedLocalJourney)
	require.NotNil(t, report.Storage)
	assert.Nil(t, report.Storage.Selected)
	report, err = runtimeCommand(t, "runtime", "status", "--config-dir", root, "--cache", cache)
	require.ErrorIs(t, err, runtime.ErrUnavailable)
	assert.Equal(t, "unavailable", report.State)
}

func TestRuntimePreloadRequiresQualifiedArchiveAndFreshRoot(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	final := filepath.Join(root, "preloaded")
	args := []string{"runtime", "preload", "--config-dir", root, "--release", manifest, "--archive", archive, "--root", final}
	report, err := runtimeCommand(t, args...)
	require.Error(t, err)
	assert.Nil(t, report.PreloadedConfig)
	assert.NoFileExists(t, filepath.Join(final, "preload.json"))
	_, err = runtimeCommand(t, args...)
	require.ErrorContains(t, err, "preload root must be empty")
}

func TestRuntimeInstallDigestFailureNeverReportsPrepared(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	writeAdminTrust(t, root, "")
	require.NoError(t, os.WriteFile(archive, []byte("corrupt"), 0600))
	report, err := runtimeCommand(t, "runtime", "install", "--config-dir", root, "--release", manifest, "--archive", archive, "--cache", filepath.Join(root, "cache"))
	require.Error(t, err)
	assert.Equal(t, "unavailable", report.State)
	assert.NotEmpty(t, report.SetupError)
	require.NotNil(t, report.Storage)
	assert.Nil(t, report.Storage.Selected)
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
	require.Error(t, err) // Structurally valid fixture cannot qualify as an executable runtime.
	require.Equal(t, "unavailable", report.State)
	require.Equal(t, "prepared", report.CandidateState)
	require.Equal(t, service.Runtime.CachePath, report.CachePath)
	require.DirExists(t, filepath.Join(serviceCache, "generations", report.Candidate.SHA256))
	require.NoDirExists(t, wrongCache)
	overrideCache := filepath.Join(t.TempDir(), "override-cache")
	report, err = runtimeCommand(t, "runtime", "install", "--release", manifest, "--archive", archive, "--cache", overrideCache)
	require.Error(t, err)
	require.Equal(t, overrideCache, report.CachePath)
	require.DirExists(t, filepath.Join(overrideCache, "generations", report.Candidate.SHA256))
	require.NoDirExists(t, wrongCache)
}

// This opt-in test uses authentic release descriptors and archives through the
// production CLI acquisition, qualification and publication path.
func TestRealRuntimeInstallQualifyAndUninstall(t *testing.T) {
	manifest, archive, helper := os.Getenv("DEM_RUNTIME_TEST_RELEASE"), os.Getenv("DEM_RUNTIME_TEST_ARCHIVE"), os.Getenv("DEM_RUNTIME_TEST_HELPER")
	publicKey := os.Getenv("DEM_RUNTIME_TEST_PUBLIC_KEY")
	if manifest == "" || archive == "" || helper == "" || publicKey == "" {
		t.Skip("requires DEM_RUNTIME_TEST_RELEASE, DEM_RUNTIME_TEST_ARCHIVE, DEM_RUNTIME_TEST_PUBLIC_KEY and DEM_RUNTIME_TEST_HELPER")
	}
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
	require.NotNil(t, report.Qualification)
	require.True(t, report.Qualification.ExecutionContract)
	require.True(t, report.Qualification.SandboxedLocalJourney)
	require.True(t, report.Qualification.SandboxedLocalLighthouse)
	require.True(t, report.Qualification.ProcessTreeDrained)
	require.Equal(t, report.Runtime, report.Qualification.Runtime)
	for _, command := range []string{"status", "qualify"} {
		result, err := runtimeCommand(t, append([]string{"runtime", command}, common...)...)
		require.NoError(t, err)
		require.Equal(t, report.Runtime, result.Runtime)
	}
	store, err := runtime.Open(runtime.DefaultConfig(cache))
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
	require.ErrorIs(t, err, runtime.ErrUnavailable)
}
