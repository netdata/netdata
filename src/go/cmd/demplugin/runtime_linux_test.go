// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
	manifest := runtime.Manifest{Format: 1, Version: "cli-fixture", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(compressed.Len()), TarBytes: int64(raw.Len()), ExpandedBytes: expanded, Entries: len(entries)},
		Layout:   runtime.Layout{Node: "node", Browser: "browser", Dependencies: "dependencies", Loader: "loader", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: prefix}, {Path: "browser", Expected: prefix}},
	}
	require.NoError(t, manifest.Validate())
	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	root := t.TempDir()
	manifestPath = filepath.Join(root, "trusted.json")
	archivePath = filepath.Join(root, "runtime.tar.gz")
	require.NoError(t, os.WriteFile(manifestPath, manifestJSON, 0600))
	require.NoError(t, os.WriteFile(archivePath, compressed.Bytes(), 0600))
	return manifestPath, archivePath
}

func TestRuntimeOfflineImportStatusAndReuse(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	base := []string{"--config-dir", root, "--manifest", manifest, "--cache", cache, "--allow-download=false"}
	report, err := runtimeCommand(t, append([]string{"runtime", "prepare"}, base...)...)
	require.ErrorContains(t, err, "downloads are disabled")
	assert.Equal(t, "unavailable", report.State)
	report, err = runtimeCommand(t, append(append([]string{"runtime", "import"}, base...), "--archive", archive)...)
	require.NoError(t, err)
	assert.Equal(t, "prepared", report.State)
	require.Nil(t, report.Qualification)
	require.NotNil(t, report.Storage)
	assert.Positive(t, report.Storage.UsedBytes)
	for _, command := range []string{"status", "prepare"} {
		report, err = runtimeCommand(t, append([]string{"runtime", command}, base...)...)
		require.NoError(t, err)
		assert.Equal(t, "prepared", report.State)
		require.Nil(t, report.Qualification)
	}
	report, err = runtimeCommand(t, append([]string{"runtime", "qualify"}, base...)...)
	require.Error(t, err)
	assert.NotEqual(t, "qualified", report.State)
	require.NotNil(t, report.Qualification)
	assert.False(t, report.Qualification.SandboxedLocalJourney)
}

func TestRuntimeOfflinePreloadUsesFinalRootAndRejectsReplacement(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	final := filepath.Join(root, "preloaded")
	args := []string{"runtime", "preload", "--config-dir", root, "--manifest", manifest, "--archive", archive, "--root", final, "--allow-download=false"}
	report, err := runtimeCommand(t, args...)
	require.NoError(t, err)
	require.NotNil(t, report.PreloadedConfig)
	cfg := report.PreloadedConfig["runtime"].(map[string]any)
	assert.Equal(t, "preloaded", cfg["mode"])
	assert.Equal(t, final, cfg["preload_path"])
	assert.NotContains(t, cfg, "node_path")
	body, err := yaml.Marshal(report.PreloadedConfig)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "dem.conf"), body, 0600))
	// The actual emitted configuration must resolve the preload without opening a
	// managed store or adding lifecycle records, even with a read-only directory.
	require.NoError(t, os.Chmod(final, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(final, 0700)) })
	loaded := dem.DefaultConfig()
	require.NoError(t, yaml.Unmarshal(body, &loaded))
	handle := dem.OpenRuntime(loaded.Runtime, "/unused/assets", "/unused/cache")
	require.NoError(t, handle.SetupError)
	require.Nil(t, handle.Store)
	require.Nil(t, handle.Host)
	use, err := handle.Source.BeginUse()
	require.NoError(t, err)
	assert.Contains(t, use.Paths().Node, filepath.Join(final, "generations", report.Generation))
	require.NoError(t, use.Finish(true))
	require.NoError(t, handle.Close())
	status, err := runtimeCommand(t, "runtime", "status", "--config-dir", root)
	require.NoError(t, err)
	assert.Equal(t, "prepared", status.State)
	assert.Equal(t, report.Generation, status.Generation)
	assert.Nil(t, status.Storage)
	assert.Nil(t, status.Qualification)
	// This archive is structurally prepared but is not an executable browser.
	status, err = runtimeCommand(t, "runtime", "qualify", "--config-dir", root)
	require.Error(t, err)
	require.NotNil(t, status.Qualification)
	assert.NotEqual(t, "qualified", status.State)
	assert.Contains(t, report.Notice, "Stop all consumers")
	_, err = runtimeCommand(t, args...)
	require.ErrorContains(t, err, "preload root must be empty")
}

func TestRuntimeImportDigestFailureNeverReportsPrepared(t *testing.T) {
	manifest, archive := adminBundleFixture(t)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(archive, []byte("corrupt"), 0600))
	report, err := runtimeCommand(t, "runtime", "import", "--config-dir", root, "--manifest", manifest, "--archive", archive, "--cache", filepath.Join(root, "cache"))
	require.Error(t, err)
	assert.Equal(t, "unavailable", report.State)
	assert.NotEmpty(t, report.SetupError)
	report, err = runtimeCommand(t, "runtime", "status", "--config-dir", root, "--manifest", manifest, "--cache", filepath.Join(root, "cache"))
	require.ErrorIs(t, err, runtime.ErrUnavailable)
	assert.Equal(t, "unavailable", report.State)
}
