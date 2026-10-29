// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/cli"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationStartup(t *testing.T) {
	mode := os.Getenv("NETDATA_TEST_CONFIGURATION_STARTUP")
	if mode == "" {
		binary, err := os.Executable()
		require.NoError(t, err)
		for _, mode := range []string{"probe error", "CLI override", "environment override", "ordinary caller"} {
			t.Run(mode, func(t *testing.T) {
				cmd := exec.Command(binary, "-test.run=^TestConfigurationStartup$")
				cmd.Env = append(os.Environ(), "NETDATA_TEST_CONFIGURATION_STARTUP="+mode)
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", output)
				assert.NotContains(t, string(output), "synthetic-caller-marker")
				if mode == "environment override" {
					for _, name := range []string{"NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR", "NETDATA_PLUGINS_GOD_WATCH_PATH"} {
						assert.Contains(t, string(output), "ignoring "+name)
					}
				}
			})
		}
		return
	}
	root := t.TempDir()
	buildinfo.UserConfigDir, buildinfo.StockConfigDir = filepath.Join(root, "user"), filepath.Join(root, "stock")
	t.Setenv("NETDATA_CYGWIN_BASE_PATH", "")
	t.Setenv("NETDATA_USER_CONFIG_DIR", buildinfo.UserConfigDir)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", buildinfo.StockConfigDir)
	t.Setenv("NETDATA_PLUGINS_GOD_WATCH_PATH", "")
	opts := &cli.Option{}
	probeErr := errors.New("synthetic probe failure")
	switch mode {
	case "probe error":
		err := initConfig(opts, func() (bool, error) { return false, probeErr })
		require.ErrorIs(t, err, probeErr)
		assert.Empty(t, pluginconfig.ConfigDir())
	case "CLI override":
		opts.ConfDir = []string{filepath.Join(root, "synthetic-caller-marker")}
		err := initConfig(opts, func() (bool, error) { return true, nil })
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "synthetic-caller-marker")
		assert.Empty(t, pluginconfig.ConfigDir())
	case "environment override":
		for _, name := range []string{"NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR", "NETDATA_PLUGINS_GOD_WATCH_PATH"} {
			t.Setenv(name, filepath.Join(root, "synthetic-caller-marker"))
		}
		require.NoError(t, initConfig(opts, func() (bool, error) { return true, nil }))
		assert.Equal(t, []string{buildinfo.UserConfigDir, buildinfo.StockConfigDir}, []string(pluginconfig.ConfigDir()))
		assert.True(t, pluginconfig.ConfigRestricted())
	case "ordinary caller":
		opts.ConfDir = []string{filepath.Join(root, "custom")}
		opts.WatchPath = []string{filepath.Join(root, "*.conf")}
		require.NoError(t, initConfig(opts, func() (bool, error) { return false, nil }))
		assert.Equal(
			t,
			[]string{opts.ConfDir[0], buildinfo.UserConfigDir, buildinfo.StockConfigDir},
			[]string(pluginconfig.ConfigDir()),
		)
		assert.Equal(t, opts.WatchPath, pluginconfig.CollectorsConfigWatchPaths())
		assert.False(t, pluginconfig.ConfigRestricted())
	default:
		t.Fatal("unknown startup test mode")
	}
}
