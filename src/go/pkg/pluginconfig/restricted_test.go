// SPDX-License-Identifier: GPL-3.0-or-later

package pluginconfig

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/executable"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestrictedConfigRoots(t *testing.T) {
	originalUser, originalStock := buildinfo.UserConfigDir, buildinfo.StockConfigDir
	t.Cleanup(func() { buildinfo.UserConfigDir, buildinfo.StockConfigDir = originalUser, originalStock })
	root := t.TempDir()
	buildinfo.UserConfigDir = filepath.Join(root, "installed", "etc", "netdata")
	buildinfo.StockConfigDir = filepath.Join(root, "installed", "usr", "lib", "netdata", "conf.d")
	var got directories
	require.NoError(t, got.build(InitInput{RestrictConfig: true}, envData{
		userDir:    filepath.Join(root, "caller", "user"),
		stockDir:   filepath.Join(root, "caller", "stock"),
		watchPath:  filepath.Join(root, "caller", "*.conf"),
		cygwinBase: filepath.Join(root, "remap"),
	}, "go.d", filepath.Join(root, "development", "bin")))
	assert.Equal(t, []string{buildinfo.UserConfigDir, buildinfo.StockConfigDir}, []string(got.configDir()))
	assert.Equal(
		t,
		[]string{filepath.Join(buildinfo.UserConfigDir, "go.d"), filepath.Join(buildinfo.StockConfigDir, "go.d")},
		[]string(got.collectorsDir()),
	)
	assert.Equal(
		t,
		[]string{
			filepath.Join(buildinfo.UserConfigDir, "go.d", "sd"),
			filepath.Join(buildinfo.StockConfigDir, "go.d", "sd"),
		},
		[]string(got.serviceDiscoveryDir()),
	)
	assert.Empty(t, got.collectorsWatch)
}

func TestRestrictedConfigRejectsCLI(t *testing.T) {
	for name, input := range map[string]InitInput{
		"config": {RestrictConfig: true, ConfDir: []string{"/etc/netdata"}},
		"watch":  {RestrictConfig: true, WatchPath: []string{"relative/*.conf"}},
	} {
		t.Run(name, func(t *testing.T) {
			var got directories
			require.Error(t, got.build(input, envData{}, "go.d", t.TempDir()))
		})
	}
}

func TestRestrictedConfigRejectsMissingInstalledRoots(t *testing.T) {
	originalUser, originalStock := buildinfo.UserConfigDir, buildinfo.StockConfigDir
	t.Cleanup(func() { buildinfo.UserConfigDir, buildinfo.StockConfigDir = originalUser, originalStock })
	absolute := t.TempDir()
	for name, roots := range map[string][2]string{
		"missing user": {"", absolute}, "missing stock": {absolute, ""},
		"relative user": {"relative", absolute}, "relative stock": {absolute, "relative"},
	} {
		t.Run(name, func(t *testing.T) {
			buildinfo.UserConfigDir, buildinfo.StockConfigDir = roots[0], roots[1]
			var got directories
			require.Error(t, got.build(InitInput{RestrictConfig: true}, envData{}, "go.d", absolute))
		})
	}
}

func TestRestrictedInitialization(t *testing.T) {
	originalEnv, originalDirs, originalTerm := env, dirs, isTerm
	originalUser, originalStock := buildinfo.UserConfigDir, buildinfo.StockConfigDir
	originalName := executable.Name
	t.Cleanup(func() {
		env, dirs, isTerm = originalEnv, originalDirs, originalTerm
		buildinfo.UserConfigDir, buildinfo.StockConfigDir = originalUser, originalStock
		executable.Name = originalName
		initOnce = sync.Once{}
		initErr, initIgnored = nil, nil
	})
	root := t.TempDir()
	user, stock := filepath.Join(root, "installed-user"), filepath.Join(root, "installed-stock")
	buildinfo.UserConfigDir, buildinfo.StockConfigDir = user, stock
	executable.Name = "go.d"
	// Installed symlinks remain supported; the policy selects roots, not targets.
	target := filepath.Join(root, "actual-user")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "go.d"), 0755))
	require.NoError(t, os.Symlink(target, user))
	require.NoError(t, os.WriteFile(filepath.Join(target, "go.d", "example.conf"), []byte("jobs: []\n"), 0644))
	for name, tc := range map[string]struct {
		user, stock, watch   string
		restricted, terminal bool
		wantIgnored          []string
	}{
		"all redirected": {user: "caller-marker\nuser", stock: "caller-marker-stock", watch: "caller-marker/*.conf", restricted: true,
			wantIgnored: []string{"NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR", "NETDATA_PLUGINS_GOD_WATCH_PATH"}},
		"terminal redirects": {user: "caller-marker-user", stock: "caller-marker-stock", restricted: true, terminal: true,
			wantIgnored: []string{"NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR"}},
		"matching install": {user: user + string(filepath.Separator), stock: stock, restricted: true},
		"ordinary caller":  {user: "ordinary-user", stock: "ordinary-stock", watch: "ordinary/*.conf"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NETDATA_USER_CONFIG_DIR", tc.user)
			t.Setenv("NETDATA_STOCK_CONFIG_DIR", tc.stock)
			t.Setenv("NETDATA_PLUGINS_GOD_WATCH_PATH", tc.watch)
			t.Setenv("NETDATA_CYGWIN_BASE_PATH", "")
			t.Setenv("NETDATA_HOST_PREFIX", filepath.Join(root, "host"))
			t.Setenv("NETDATA_LIB_DIR", filepath.Join(root, "varlib"))
			isTerm = tc.terminal
			initOnce = sync.Once{}
			ignored, err := Init(InitInput{RestrictConfig: tc.restricted})
			require.NoError(t, err)
			assert.Equal(t, tc.wantIgnored, ignored)
			assert.Equal(t, tc.restricted, ConfigRestricted())
			assert.Equal(t, filepath.Join(root, "host"), HostPrefix())
			assert.Equal(t, filepath.Join(root, "varlib"), VarLibDir())
			if tc.restricted {
				assert.Equal(t, []string{user, stock}, []string(ConfigDir()))
				assert.Empty(t, CollectorsConfigWatchPaths())
				found, err := CollectorsDir().Find("example.conf")
				require.NoError(t, err)
				assert.Equal(t, filepath.Join(user, "go.d", "example.conf"), found)
			} else {
				assert.Equal(t, []string{tc.user, tc.stock}, []string(ConfigDir()))
				assert.Equal(t, []string{tc.watch}, CollectorsConfigWatchPaths())
			}
			// Standalone resolution used by other commands remains unrestricted.
			if !tc.terminal {
				assert.Equal(
					t,
					[]string{"other-command", filepath.Clean(tc.user), filepath.Clean(tc.stock)},
					[]string(ResolveConfigDirs([]string{"other-command"})),
				)
			}
		})
	}
}
