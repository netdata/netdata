// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingInstallationLeavesRUMSetupAvailable(t *testing.T) {
	cfg := DefaultConfig()
	cache := filepath.Join(t.TempDir(), "cache")
	cfg.CachePath = cache
	handle := Open(cfg, "")
	require.Error(t, handle.SetupError)
	require.NotNil(t, handle.Source)
	require.NotNil(t, handle.Host)
	assert.Contains(t, []string{"unavailable", "missing"}, handle.Host.Status().State)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	require.NoError(t, handle.Close())
}
func TestPreloadedRuntimeRequiresProtectedInstallation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModePreloaded
	cfg.PreloadPath = filepath.Join(t.TempDir(), "not-created")
	handle := Open(cfg, "")
	require.Error(t, handle.SetupError)
	require.Nil(t, handle.Host)
	require.Nil(t, handle.Store)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	_, err = os.Stat(cfg.PreloadPath)
	require.True(t, os.IsNotExist(err))
}

func TestPreloadedMissingPathIsSetupUnavailable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModePreloaded
	cfg.PreloadPath = "relative"
	require.NoError(t, cfg.Validate())
	handle := Open(cfg, "")
	require.ErrorContains(t, handle.SetupError, "must be absolute")
	require.Nil(t, handle.Host)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
}
