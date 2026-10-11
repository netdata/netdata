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
	for _, body := range []string{"mode: auto", "max_bytes: 0", "previous_retention_days: -1", "idle_retention_days: 36501", "node_path: /usr/bin/node"} {
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

func TestExternalMissingPathsAreSetupUnavailable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {mode: external, node_path: relative}"), 0600))
	cfg, err := LoadConfig(multipath.New(dir))
	require.NoError(t, err)
	handle := OpenRuntime(cfg.Runtime, "/installed/dem", "")
	require.ErrorContains(t, handle.SetupError, "must be absolute")
	require.Nil(t, handle.Host)
	_, err = handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
}
