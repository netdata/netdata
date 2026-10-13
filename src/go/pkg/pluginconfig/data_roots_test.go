// SPDX-License-Identifier: GPL-3.0-or-later

package pluginconfig

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataRoots(t *testing.T) {
	originalEnv, originalDirs, originalTerm := env, dirs, isTerm
	originalCache, originalStock := buildinfo.CacheDir, buildinfo.StockDataDir
	t.Cleanup(func() {
		env, dirs, isTerm = originalEnv, originalDirs, originalTerm
		buildinfo.CacheDir, buildinfo.StockDataDir = originalCache, originalStock
		initOnce = sync.Once{}
	})

	root := t.TempDir()
	for _, tc := range []struct {
		name          string
		cache         string
		stock         string
		compiledCache string
		compiledStock string
		base          string
		terminal      bool
		wantCache     string
		wantStock     string
	}{
		{
			name:          "compiled defaults",
			compiledCache: "/var/cache/netdata",
			compiledStock: "/usr/share/netdata",
			wantCache:     "/var/cache/netdata",
			wantStock:     "/usr/share/netdata",
		},
		{
			name:          "custom compiled roots",
			compiledCache: root + "/install/cache/../cache",
			compiledStock: root + "/install/share/../share",
			wantCache:     filepath.Join(root, "install", "cache"),
			wantStock:     filepath.Join(root, "install", "share"),
		},
		{
			name:          "runtime overrides compiled roots in terminal mode",
			cache:         root + "/runtime/cache/../cache",
			stock:         root + "/runtime/stock/../stock",
			compiledCache: "/compiled/cache",
			compiledStock: "/compiled/stock",
			terminal:      true,
			wantCache:     filepath.Join(root, "runtime", "cache"),
			wantStock:     filepath.Join(root, "runtime", "stock"),
		},
		{
			name:          "runtime Windows POSIX roots normalize once",
			cache:         "/var/cache/netdata/../custom",
			stock:         "/usr/share/netdata/../custom",
			base:          root,
			compiledCache: "/compiled/cache",
			compiledStock: "/compiled/stock",
			wantCache:     filepath.Join(root, "var", "cache", "custom"),
			wantStock:     filepath.Join(root, "usr", "share", "custom"),
		},
		{
			name:          "compiled Windows POSIX roots normalize",
			base:          root,
			compiledCache: "/var/cache/netdata",
			compiledStock: "/usr/share/netdata",
			wantCache:     filepath.Join(root, "var", "cache", "netdata"),
			wantStock:     filepath.Join(root, "usr", "share", "netdata"),
		},
		{
			name:          "runtime POSIX parent components stop at mapped root",
			base:          root,
			cache:         "/../../../../cache",
			stock:         "/usr/../../../../stock",
			compiledCache: "/compiled/cache",
			compiledStock: "/compiled/stock",
			wantCache:     filepath.Join(root, "cache"),
			wantStock:     filepath.Join(root, "stock"),
		},
		{
			name:          "compiled POSIX parent components stop at mapped root",
			base:          root,
			compiledCache: "/../../../../cache",
			compiledStock: "/usr/../../../../stock",
			wantCache:     filepath.Join(root, "cache"),
			wantStock:     filepath.Join(root, "stock"),
		},
		{
			name:          "independent runtime stock override",
			stock:         filepath.Join(root, "runtime", "stock"),
			compiledCache: filepath.Join(root, "compiled", "cache"),
			compiledStock: filepath.Join(root, "compiled", "stock"),
			wantCache:     filepath.Join(root, "compiled", "cache"),
			wantStock:     filepath.Join(root, "runtime", "stock"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NETDATA_CACHE_DIR", tc.cache)
			t.Setenv("NETDATA_STOCK_DATA_DIR", tc.stock)
			t.Setenv("NETDATA_CYGWIN_BASE_PATH", tc.base)
			buildinfo.CacheDir, buildinfo.StockDataDir = tc.compiledCache, tc.compiledStock
			isTerm = tc.terminal
			initOnce = sync.Once{}
			MustInit(InitInput{})
			assert.Equal(t, filepath.Clean(tc.wantCache), CacheDir())
			assert.Equal(t, filepath.Clean(tc.wantStock), StockDataDir())
			// No existence filter: provisioners can create these roots later.
			if tc.base != "" {
				require.NoDirExists(t, CacheDir())
				require.NoDirExists(t, StockDataDir())
			}
		})
	}
}

func TestDataRootsWindowsDebugPaths(t *testing.T) {
	t.Setenv("NETDATA_CACHE_DIR", "/var/cache/netdata")
	t.Setenv("NETDATA_STOCK_DATA_DIR", "/usr/share/netdata")
	t.Setenv("NETDATA_CYGWIN_BASE_PATH", "")
	e := readEnvFromOS(`C:\msys64\usr\libexec\netdata\plugins.d`)
	var d directories
	require.NoError(t, d.build(InitInput{}, e, executable.Name, executable.Directory))
	assert.Equal(t, filepath.Join(`C:\msys64`, "var", "cache", "netdata"), d.cacheDir)
	assert.Equal(t, filepath.Join(`C:\msys64`, "usr", "share", "netdata"), d.stockDataDir)
}

func TestHandleDirOnWinRootSemantics(t *testing.T) {
	base := filepath.FromSlash("C:/msys64")
	for _, tc := range []struct{ name, base, input, want string }{
		{"absolute parent components", base, "/var/../../cache", filepath.Join(base, "cache")},
		{"mapped root", base, "/../../", base},
		{"ordinary normalization", base, "/var/cache/../data", filepath.Join(base, "var", "data")},
		{"native drive", base, `D:\data\cache`, `D:\data\cache`},
		{"native UNC", base, `\\server\share\cache`, `\\server\share\cache`},
		{"relative path", base, "../cache", "../cache"},
		{"no mapping", "", "/../../cache", "/../../cache"},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, handleDirOnWin(tc.base, tc.input, "")) })
	}
}
