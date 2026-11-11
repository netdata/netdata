// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/stretchr/testify/require"
)

// Run each installation in a fresh process: service initialization is deliberately
// once per process, and must not be reset or replaced with manufactured state.
func runtimeConfigProcess(t *testing.T, scenario string) bool {
	t.Helper()
	if os.Getenv("NETDATA_TEST_RUNTIME_CONFIG") == scenario {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), "NETDATA_TEST_RUNTIME_CONFIG="+scenario)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return false
}

func runtimeConfigRoots(t *testing.T) (user, stock, buildUser, buildStock, cli string) {
	t.Helper()
	root := t.TempDir()
	user, stock = filepath.Join(root, "user"), filepath.Join(root, "stock")
	buildUser, buildStock, cli = filepath.Join(root, "build-user"), filepath.Join(root, "build-stock"), filepath.Join(root, "cli")
	for _, dir := range []string{user, stock, buildUser, buildStock, cli} {
		require.NoError(t, os.MkdirAll(dir, 0700))
	}
	// These exported values are installation inputs, not pluginconfig's cached state.
	buildinfo.UserConfigDir, buildinfo.StockConfigDir = buildUser, buildStock
	t.Setenv("NETDATA_USER_CONFIG_DIR", user)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", stock)
	t.Setenv("NETDATA_CYGWIN_BASE_PATH", "")
	return
}

func writeRuntimeConfig(t *testing.T, dir, cache string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime:\n  cache_path: "+cache+"\n"), 0600))
}

func TestRuntimeConfigurationMatchesService(t *testing.T) {
	for _, scenario := range []string{
		"environment-replaces-build-user", "environment-replaces-build-stock",
		"cli-inherits-user", "cli-inherits-stock", "cli-takes-priority", "build-defaults",
		"relative-user", "relative-stock", "common-user", "common-stock",
	} {
		t.Run(scenario, func(t *testing.T) {
			if !runtimeConfigProcess(t, scenario) {
				return
			}
			user, stock, buildUser, buildStock, cli := runtimeConfigRoots(t)
			selected := stock
			var confDirs []string
			switch scenario {
			case "environment-replaces-build-user":
				writeRuntimeConfig(t, buildUser, "/wrong-cache")
			case "environment-replaces-build-stock":
				writeRuntimeConfig(t, buildStock, "/wrong-cache")
			case "cli-inherits-user":
				selected, confDirs = user, []string{cli}
			case "cli-inherits-stock":
				confDirs = []string{cli}
			case "cli-takes-priority":
				selected, confDirs = cli, []string{cli}
				writeRuntimeConfig(t, user, "/wrong-cache")
			case "build-defaults":
				t.Setenv("NETDATA_USER_CONFIG_DIR", "")
				t.Setenv("NETDATA_STOCK_CONFIG_DIR", "")
				selected = buildUser
			case "common-user", "common-stock":
				t.Setenv("NETDATA_USER_CONFIG_DIR", "")
				t.Setenv("NETDATA_STOCK_CONFIG_DIR", "")
				buildinfo.UserConfigDir, buildinfo.StockConfigDir = "", ""
				root := t.TempDir()
				// Map common absolute installation paths into a private fixture.
				t.Setenv("NETDATA_CYGWIN_BASE_PATH", root)
				executable.Directory = filepath.Join(root, "missing/plugins")
				user, stock = filepath.Join(root, "etc/netdata"), filepath.Join(root, "usr/lib/netdata/conf.d")
				for _, dir := range []string{user, stock} {
					require.NoError(t, os.MkdirAll(dir, 0700))
				}
				selected = user
				if scenario == "common-stock" {
					selected = stock
				}
			case "relative-user", "relative-stock":
				t.Setenv("NETDATA_USER_CONFIG_DIR", "")
				t.Setenv("NETDATA_STOCK_CONFIG_DIR", "")
				buildinfo.UserConfigDir, buildinfo.StockConfigDir = "", ""
				root := t.TempDir()
				executable.Directory = filepath.Join(root, "usr/libexec/netdata/plugins.d")
				user, stock = filepath.Join(root, "etc/netdata"), filepath.Join(root, "usr/lib/netdata/conf.d")
				for _, dir := range []string{executable.Directory, user, stock} {
					require.NoError(t, os.MkdirAll(dir, 0700))
				}
				selected = user
				if scenario == "relative-stock" {
					selected = stock
				}
			}
			varlib := filepath.Join(t.TempDir(), "varlib")
			t.Setenv("NETDATA_LIB_DIR", varlib)
			if scenario != "environment-replaces-build-stock" {
				writeRuntimeConfig(t, selected, filepath.Join(selected, "cache"))
			}
			args := []string{"runtime", "status"}
			if len(confDirs) > 0 {
				args = append(args, "--config-dir", confDirs[0])
			}
			report, err := runtimeCommand(t, args...)
			require.Error(t, err) // Cache selection is observable before opening an absent installation.
			pluginconfig.MustInit(pluginconfig.InitInput{ConfDir: confDirs})
			service, err := dem.LoadConfig(pluginconfig.ConfigDir())
			require.NoError(t, err)
			storeConfig, err := service.Runtime.StoreConfig(varlib)
			require.NoError(t, err)
			require.Equal(t, storeConfig.Path, report.CachePath)
			require.False(t, report.DownloadAllowed)
			override := filepath.Join(selected, "override-cache")
			report, err = runtimeCommand(t, append(args, "--cache", override)...)
			require.Error(t, err)
			require.Equal(t, override, report.CachePath)
		})
	}
}
