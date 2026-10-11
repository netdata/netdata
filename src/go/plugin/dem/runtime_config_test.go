// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigDefaultsAndOverrides(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	assert.Equal(t, "managed", cfg.Mode)
	assert.True(t, cfg.AllowDownload)
	store, err := cfg.StoreConfig("/var/lib/netdata")
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/netdata/dem/runtime", store.Path)
	assert.Equal(t, int64(4<<30), store.MaxBytes)
	assert.Equal(t, 7*24*time.Hour, store.PreviousRetention)
	assert.Equal(t, 30*24*time.Hour, store.IdleRetention)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {allow_download: false, max_bytes: 2147483648, previous_retention_days: 0}"), 0600))
	loaded, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	assert.False(t, loaded.Runtime.AllowDownload)
	assert.Equal(t, "managed", loaded.Runtime.Mode)
	assert.Equal(t, 30, loaded.Runtime.IdleRetentionDays)
	assert.Equal(t, 0, loaded.Runtime.PreviousRetentionDays)
}

func TestRuntimeConfigRejectsAmbiguousAndInvalidPolicy(t *testing.T) {
	for _, body := range []string{"mode: auto", "max_bytes: 0", "previous_retention_days: -1", "idle_retention_days: 36501"} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {"+body+"}"), 0600))
			_, err := LoadConfig(multipath.New(dir))
			require.ErrorContains(t, err, "runtime.")
		})
	}
}

func TestRuntimePathsHaveNoImplicitWorkingDirectory(t *testing.T) {
	cfg := DefaultRuntimeConfig()
	for _, path := range []string{"", "relative"} {
		_, err := cfg.StoreConfig(path)
		require.Error(t, err)
	}
	cfg.CachePath = "relative"
	_, err := cfg.StoreConfig("/var/lib/netdata")
	require.Error(t, err)
	cfg.CachePath = filepath.Join(t.TempDir(), "cache")
	store, err := cfg.StoreConfig("")
	require.NoError(t, err)
	assert.Equal(t, cfg.CachePath, store.Path)
}

func TestPreloadedMissingPathIsSetupUnavailable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {mode: preloaded, preload_path: relative}"), 0600))
	cfg, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	handle := OpenRuntime(cfg.Runtime, "/installed/dem", "")
	require.ErrorContains(t, handle.SetupError, "must be absolute")
	require.Nil(t, handle.Host)
	_, err = handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
}

func TestRuntimeConfigRejectsRemovedComponentsAndUnknownSettings(t *testing.T) {
	for _, body := range []string{"mode: external", "node_path: /usr/bin/node", "browser_path: /usr/bin/chromium", "dependencies_path: /opt/deps", "allow_downlaod: false", "mode: managed, preload_path: /bundle", "mode: preloaded, preload_path: /bundle, cache_path: /cache"} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("enabled: true\nruntime: {"+body+"}\n"), 0600))
			_, err := LoadConfig(multipath.New(dir))
			require.ErrorContains(t, err, "runtime")
		})
	}
}

func TestRuntimeConfigErrorsReferToOriginalInput(t *testing.T) {
	for name, tc := range map[string]struct {
		body, line, detail string
	}{
		"unknown":   {"allow_downlaod: false", "line 4:", "field allow_downlaod not found"},
		"duplicate": {"allow_download: true\n  allow_download: false", "line 5:", "field allow_download already set"},
		"type":      {"allow_download: [false]", "line 4:", "cannot unmarshal !!seq into bool"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			body := "enabled: true\n# original configuration\nruntime:\n  " + tc.body + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(body), 0600))
			_, err := LoadConfig(multipath.New(dir))
			require.ErrorContains(t, err, tc.line)
			require.ErrorContains(t, err, tc.detail)
			require.NotContains(t, err.Error(), "dem.plain")
		})
	}
}

func TestRuntimeConfigMergeValidation(t *testing.T) {
	for name, body := range map[string]string{
		"merged unknown":   "runtime:\n  <<: {node_path: /tmp/node}\n",
		"merged duplicate": "runtime:\n  <<: {allow_download: false, allow_download: true}\n",
		"alias unknown":    "base: &base {allow_downlaod: false}\nruntime: {<<: *base}\n",
		"alias duplicate":  "base: &base {mode: managed, mode: preloaded}\nruntime: {<<: *base}\n",
		"top duplicate":    "enabled: true\nenabled: false\nruntime: {allow_download: false}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(body), 0600))
			_, err := LoadConfig(multipath.New(dir))
			require.Error(t, err)
		})
	}
}

func TestRuntimeConfigLegitimateMergesPreserveDefaults(t *testing.T) {
	for name, body := range map[string]string{
		"override":  "runtime: {<<: {allow_download: true}, allow_download: false}\n",
		"sequence":  "runtime: {<<: [{allow_download: false}, {allow_download: true}]}\n",
		"alias":     "base: &base {allow_download: false}\nruntime: {<<: *base}\n",
		"top merge": "base: &base {runtime: {allow_download: false}}\n<<: *base\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(body), 0600))
			cfg, err := LoadConfig(multipath.New(dir))
			require.NoError(t, err)
			expected := DefaultRuntimeConfig()
			expected.AllowDownload = false
			require.Equal(t, expected, cfg.Runtime)
		})
	}
}

func TestRuntimeConfigMergeErrorsUseOriginalSourceLines(t *testing.T) {
	for name, tc := range map[string]struct{ source, line, detail string }{
		"unknown":   {"defaults: &defaults\n  node_path: /tmp/node\n\nruntime: {<<: *defaults}\n", "line 2:", "node_path"},
		"duplicate": {"defaults: &defaults\n  allow_download: true\n  allow_download: false\n\nruntime: {<<: *defaults}\n", "line 3:", "allow_download"},
		"type":      {"defaults: &defaults\n  allow_download: [false]\n\nruntime: {<<: *defaults}\n", "line 2:", "bool"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(tc.source), 0600))
			_, err := LoadConfig(multipath.New(dir))
			require.ErrorContains(t, err, tc.line)
			require.ErrorContains(t, err, tc.detail)
			require.NotContains(t, err.Error(), "dem.plain")
		})
	}
}
