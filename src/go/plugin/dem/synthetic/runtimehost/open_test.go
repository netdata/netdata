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
	cfg.Mode = ModeManaged // independent of the build default
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
	require.Nil(t, handle.store)
	_, err := handle.Source.BeginUse()
	require.Error(t, err)
	require.NoError(t, handle.Close())
	_, err = os.Stat(cfg.PreloadPath)
	require.True(t, os.IsNotExist(err))
}

func TestOpenKeepsTheConfiguredModeWhenUnusable(t *testing.T) {
	for name, tc := range map[string]struct {
		mode     Mode
		host     bool
		reported Mode
	}{
		"managed":   {mode: ModeManaged, host: true, reported: ModeManaged},
		"native":    {mode: ModeNative, host: true, reported: ModeNative},
		"invalid":   {mode: "auto", host: true, reported: ModeManaged},
		"preloaded": {mode: ModePreloaded},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Mode = tc.mode
			switch tc.mode {
			case ModeManaged:
				cfg.CachePath = filepath.Join(t.TempDir(), "cache")
			case ModePreloaded:
				cfg.PreloadPath = filepath.Join(t.TempDir(), "absent")
			}
			handle := Open(cfg, "")
			t.Cleanup(func() { require.NoError(t, handle.Close()) })
			require.NotNil(t, handle.Source)
			if !tc.host {
				require.Nil(t, handle.Host)
				require.Error(t, handle.SetupError)
			} else {
				require.NotNil(t, handle.Host)
				assert.Equal(t, tc.reported, handle.Host.Status().Mode)
				assert.Equal(t, handle.Host.Ready(), handle.SetupError)
			}
			if tc.mode == "auto" {
				require.ErrorContains(t, handle.SetupError, "mode must be managed, native or preloaded")
			}
			if handle.SetupError != nil {
				_, err := handle.Source.BeginUse()
				require.Error(t, err)
			}
		})
	}
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
